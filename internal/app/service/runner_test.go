package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func TestFatalLossCancelsEveryTargetBeforeNotificationCompletes(t *testing.T) {
	set := testCommandSet(t.TempDir(), t.TempDir())
	started, cancelled := make(chan struct{}, 2), make(chan struct{}, 2)
	var changes, notifications atomic.Int32
	for n := range set.WatchTargets {
		set.WatchTargets[n].LifeCycle.OnChange = runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
			changes.Add(1)
			started <- struct{}{}
			<-ctx.Context().Done()
			cancelled <- struct{}{}
			return ctx.Context().Err()
		})
	}
	notifying, release := make(chan struct{}), make(chan struct{})
	set.Global.LifeCycle.OnError = runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
		notifications.Add(1)
		if ctx.Context().Err() != nil {
			t.Error("notification inherited cancelled normal context")
		}
		close(notifying)
		<-release
		return nil
	})
	logger := &notificationLog{}
	r := NewRunner(set, logger, process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	events := []chan fsnotify.Event{make(chan fsnotify.Event, 8), make(chan fsnotify.Event, 8)}
	sources := []chan error{make(chan error, 1), make(chan error, 1)}
	for n, target := range r.watchTargets {
		target.tree.events = events[n]
		target.tree.errs = sources[n]
	}
	go r.Run()
	for n, stream := range events {
		stream <- fsnotify.Event{Name: filepath.Join(r.watchTargets[n].path.String(), "first"), Op: fsnotify.Create}
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("slow hooks did not start")
		}
	}
	for n, stream := range events {
		stream <- fsnotify.Event{Name: filepath.Join(r.watchTargets[n].path.String(), "queued"), Op: fsnotify.Create}
		sources[n] <- fsnotify.ErrEventOverflow
	}
	first := <-r.Errors()
	if !errors.Is(first, fsnotify.ErrEventOverflow) {
		t.Fatalf("lost initial cause: %v", first)
	}
	select {
	case <-notifying:
	case <-time.After(time.Second):
		t.Fatal("notification not started")
	}
	for range 2 {
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("normal hook not cancelled before slow notification")
		}
	}
	stopped := make(chan struct{})
	go func() { r.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop did not wait for notification")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop blocked")
	}
	if notifications.Load() != 1 || changes.Load() != 2 {
		t.Fatalf("notifications=%d changes=%d", notifications.Load(), changes.Load())
	}
	if !strings.Contains(logger.String(), "possible event loss") || !strings.Contains(logger.String(), "overflow") || !strings.Contains(logger.String(), "watch target") {
		t.Fatalf("missing default diagnostics: %s", logger.String())
	}
	select {
	case duplicate := <-r.Errors():
		t.Fatalf("duplicate error: %v", duplicate)
	default:
	}
}

func TestNotificationRecorderProcess(t *testing.T) {
	if os.Getenv("DW_NOTIFICATION_TEST_HELPER") != "1" {
		return
	}
	data := map[string]string{}
	for _, key := range []string{"DW_ERROR_TARGET", "DW_ERROR_KIND", "DW_ERROR_CAUSE", "DW_ERROR_MESSAGE", "DW_ERROR_POSSIBLE_LOSS"} {
		data[key] = os.Getenv(key)
	}
	bytes, _ := json.Marshal(data)
	if err := os.WriteFile(os.Getenv("DW_NOTIFICATION_RECORD"), bytes, 0600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestFatalLossNotificationReceivesOriginalContext(t *testing.T) {
	record := t.TempDir() + "/notification.json"
	t.Setenv("DW_NOTIFICATION_TEST_HELPER", "1")
	t.Setenv("DW_NOTIFICATION_RECORD", record)
	t.Setenv("DW_ERROR_KIND", "stale-parent-value")
	set := testCommandSet(t.TempDir())
	set.Global.LifeCycle.OnError = command.NewManager(command.SingleCmd{Shell: command.DefaultShell(), Cmd: fmt.Sprintf(`"%s" "-test.run=^TestNotificationRecorderProcess$"`, os.Args[0])})
	r := NewRunner(set, &notificationLog{}, process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	_, source := controlledSource(r.watchTargets[0])
	go r.Run()
	source <- fsnotify.ErrEventOverflow
	first := <-r.Errors()
	r.Stop()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["DW_ERROR_TARGET"] != set.WatchTargets[0].Path.String() || fields["DW_ERROR_KIND"] != "event_loss" || fields["DW_ERROR_CAUSE"] != fsnotify.ErrEventOverflow.Error() || fields["DW_ERROR_MESSAGE"] != first.Error() || fields["DW_ERROR_POSSIBLE_LOSS"] != "true" {
		t.Fatalf("bad error context: %s", data)
	}
}

func TestNotificationFailureAndTimeoutPreserveFirstError(t *testing.T) {
	for _, mode := range []string{"absent", "failure", "timeout", "non-loss"} {
		t.Run(mode, func(t *testing.T) {
			set := testCommandSet(t.TempDir())
			logger := &notificationLog{}
			var calls atomic.Int32
			if mode != "absent" {
				set.Global.LifeCycle.OnError = runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
					calls.Add(1)
					if mode == "timeout" {
						<-ctx.Context().Done()
						return ctx.Context().Err()
					}
					return errors.New("notification failed deliberately")
				})
			}
			r := NewRunner(set, logger, process.CommandMode{})
			assertWatchDirectoriesRemovable(t, r.watchTargets...)
			_, source := controlledSource(r.watchTargets[0])
			go r.Run()
			cause := fsnotify.ErrEventOverflow
			if mode == "non-loss" {
				cause = errors.New("not a loss error")
			}
			before := time.Now()
			source <- cause
			first := <-r.Errors()
			r.Stop()
			if !errors.Is(first, cause) {
				t.Fatalf("original error changed: %v", first)
			}
			want := int32(1)
			if mode == "absent" || mode == "non-loss" {
				want = 0
			}
			if calls.Load() != want {
				t.Fatalf("notification calls=%d want=%d", calls.Load(), want)
			}
			if mode == "timeout" && time.Since(before) > 7*time.Second {
				t.Fatal("notification timeout not bounded")
			}
			if (mode == "failure" || mode == "timeout") && !strings.Contains(logger.String(), "onError notification failed") {
				t.Fatalf("missing failure diagnostic: %s", logger.String())
			}
		})
	}
}

func TestRunnerPreservesOnlyFirstFatalError(t *testing.T) {
	helper.GlobalLogger = newMockLogger()
	r := NewRunner(testCommandSet(t.TempDir(), t.TempDir()), newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	_, firstErrors := controlledSource(r.watchTargets[0])
	secondErrors := make(chan error, 1)
	r.watchTargets[1].tree.errs = secondErrors
	go r.Run()
	firstErrors <- fsnotify.ErrEventOverflow
	first := <-r.Errors()
	var fatal *FatalError
	if !errors.As(first, &fatal) || fatal.Path != r.watchTargets[0].path || !errors.Is(first, fsnotify.ErrEventOverflow) {
		t.Fatalf("lost target/cause: %v", first)
	}
	secondErrors <- errors.New("later error")
	r.Stop()
	select {
	case later := <-r.Errors():
		t.Fatalf("second error escaped: %v", later)
	default:
	}
}

func TestTargetThatFailedToStartSkipsFinishHook(t *testing.T) {
	set := testCommandSet(filepath.Join(t.TempDir(), "missing-dir"), t.TempDir())
	var missingFinished, startedFinished, globalFinished atomic.Int32
	counter := func(calls *atomic.Int32) domain.Cmd {
		return runnerCommand(func(*process.WatcherContext, *domain.Event) error { calls.Add(1); return nil })
	}
	set.WatchTargets[0].LifeCycle.OnFinishWatch = counter(&missingFinished)
	set.WatchTargets[1].LifeCycle.OnFinishWatch = counter(&startedFinished)
	set.Global.LifeCycle.OnFinishWatch = counter(&globalFinished)
	r := NewRunner(set, newMockLogger(), process.CommandMode{})
	assertWatchDirectoriesRemovable(t, r.watchTargets[1])
	go r.Run()
	if err := <-r.Errors(); err == nil {
		t.Fatal("missing target path did not stop the watcher")
	}
	r.Stop()
	if missingFinished.Load() != 0 {
		t.Fatal("onFinishWatch ran for the target that failed to start watching")
	}
	if startedFinished.Load() != 1 || globalFinished.Load() != 1 {
		t.Fatalf("finish hooks of started target=%d global=%d, want 1 each", startedFinished.Load(), globalFinished.Load())
	}
}

// queue.size를 넘는 실행은 deliver 값마다 감시를 멈추고 onError를 한 번 실행한다.
func TestQueueSizeOverflowStopsWatchingAndNotifies(t *testing.T) {
	for name, deliver := range map[string]domain.Deliver{"last": domain.DeliverLast, "lastPerPath": domain.DeliverLastPerPath, "each": domain.DeliverEach} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			set := testCommandSet(root)
			set.WatchTargets[0].Option = hookBatchOption(0, 1000, deliver)
			set.WatchTargets[0].Option.Queue.Size = 4
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			set.WatchTargets[0].LifeCycle.OnChange = runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
				once.Do(func() { close(entered) })
				select {
				case <-release:
				case <-ctx.Context().Done():
				}
				return nil
			})
			notified := make(chan error, 2)
			set.Global.LifeCycle.OnError = runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
				notified <- errors.New(hookEnvironment(ctx)["DW_ERROR_CAUSE"])
				return nil
			})
			r := NewRunner(set, &notificationLog{}, process.CommandMode{})
			assertWatchDirectoriesRemovable(t, r.watchTargets...)
			defer close(release)
			events, _ := controlledSource(r.watchTargets[0])
			go r.Run()
			send := func(name string) {
				path := filepath.Join(root, name)
				writeFile(t, path)
				events <- fsnotify.Event{Name: path, Op: fsnotify.Create}
			}
			send("first")
			<-entered
			for n := range 5 {
				send(fmt.Sprint(n))
			}
			select {
			case err := <-r.Errors():
				if !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 4)") {
					t.Fatalf("fatal error = %v, want dispatch queue full (capacity 4)", err)
				}
			case <-time.After(time.Second):
				t.Fatal("queue overflow did not stop watching")
			}
			r.Stop()
			if len(notified) != 1 {
				t.Fatalf("onError ran %d times, want 1", len(notified))
			}
			if cause := <-notified; !strings.Contains(cause.Error(), "dispatch queue full (capacity 4)") {
				t.Fatalf("DW_ERROR_CAUSE = %q", cause)
			}
		})
	}
}

func TestRunnerPassesCommandModeToGlobalAndTargetHooks(t *testing.T) {
	calls := &hookCalls{}
	set := testCommandSet(t.TempDir())
	set.Global.LifeCycle.OnStartWatch = calls.command("global")
	targetStarted := make(chan struct{})
	record := calls.command("target")
	set.WatchTargets[0].LifeCycle.OnStartWatch = runnerCommand(func(ctx *process.WatcherContext, event *domain.Event) error {
		defer close(targetStarted)
		return record.Run(ctx, "", event)
	})
	r := NewRunner(set, newMockLogger(), process.CommandMode{DryRun: true})
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	go r.Run()
	select {
	case <-targetStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("target onStartWatch did not run")
	}
	r.Stop()
	want := []string{"global onStartWatch {Silent:false DryRun:true}", "target onStartWatch {Silent:false DryRun:true}"}
	if got := calls.list(); !slices.Equal(got, want) {
		t.Fatalf("hook calls = %q, want %q", got, want)
	}
}
