package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type notificationLog struct {
	sync.Mutex
	messages []string
}

func (l *notificationLog) Debug(string) {}
func (l *notificationLog) Info(string)  {}
func (l *notificationLog) Error(s string) {
	l.Lock()
	defer l.Unlock()
	l.messages = append(l.messages, s)
}
func (l *notificationLog) String() string {
	l.Lock()
	defer l.Unlock()
	return strings.Join(l.messages, "\n")
}

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
	r := NewRunner(set, logger)
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	events := []chan fsnotify.Event{make(chan fsnotify.Event, 8), make(chan fsnotify.Event, 8)}
	sources := []chan error{make(chan error, 1), make(chan error, 1)}
	for n, target := range r.watchTargets {
		target.watchEvents = events[n]
		target.watchErrors = sources[n]
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
	r := NewRunner(set, &notificationLog{})
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
			r := NewRunner(set, logger)
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
