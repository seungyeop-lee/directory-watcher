package service

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

func TestStopDiscardsPendingEvents(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: domain.WatchTargetsOption{NoWait: true, WatchEvent: domain.WatchEvent{Create: true}}}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r.runStartHook = func() {}
	r.runFinishHook = func() {}
	r.runChangeHook = func(domain.Event) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	go r.Run()
	events <- fsnotify.Event{Name: "first", Op: fsnotify.Create}
	<-entered
	events <- fsnotify.Event{Name: "queued", Op: fsnotify.Create}
	var wg sync.WaitGroup
	wg.Add(1)
	stopped := make(chan struct{})
	go func() { r.Stop(&wg); close(stopped) }()
	time.Sleep(40 * time.Millisecond)
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop blocked behind pending event")
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("shutdown started pending hook: calls=%d", got)
	}
}

func TestDebounceHandlerTerminatesWhenInputCloses(t *testing.T) {
	r := &watchTargetRunner{runChangeHook: func(domain.Event) {}, waitMillisecond: 10, logger: newMockLogger()}
	input, done := make(chan domain.Event), make(chan struct{})
	go func() { r.selectEventHandler()(input); close(done) }()
	close(input)
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("closed event input leaked debounce handler")
	}
}

func TestWatcherLossStopsDispatchWhileHookRuns(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: domain.WatchTargetsOption{NoWait: true, WatchEvent: domain.WatchEvent{Create: true}}}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, watchErrors := controlledSource(r)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r.runStartHook = func() {}
	r.runFinishHook = func() {}
	r.runChangeHook = func(domain.Event) { close(entered); <-release }
	go func() { r.Run(); close(done) }()
	events <- fsnotify.Event{Name: "first", Op: fsnotify.Create}
	<-entered
	watchErrors <- fsnotify.ErrEventOverflow
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("watcher loss was ignored while hook ran")
	}
	close(release)
	var wg sync.WaitGroup
	wg.Add(1)
	r.Stop(&wg)
}

type runnerCommand func(*helper.WatcherContext, *domain.Event) error

func (f runnerCommand) Run(ctx *helper.WatcherContext, _ domain.Path, event *domain.Event) error {
	return f(ctx, event)
}
func emptyCommand() domain.Cmd {
	return runnerCommand(func(*helper.WatcherContext, *domain.Event) error { return nil })
}

func testCommandSet(paths ...string) domain.CommandSet {
	empty := emptyCommand()
	set := domain.CommandSet{Global: domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnStartWatch: empty, OnBeforeChange: empty, OnAfterChange: empty, OnFinishWatch: empty}}}
	for _, path := range paths {
		set.WatchTargets = append(set.WatchTargets, domain.WatchTargetsCommandSet{Path: domain.Path(path), LifeCycle: domain.WatchTargetsLifeCycle{OnStartWatch: empty, OnChange: empty, OnFinishWatch: empty}, Option: domain.WatchTargetsOption{NoWait: true, WatchEvent: domain.WatchEvent{Create: true}}})
	}
	return set
}

func TestRunnerPreservesOnlyFirstFatalError(t *testing.T) {
	helper.GlobalLogger = newMockLogger()
	r := NewRunner(testCommandSet(t.TempDir(), t.TempDir()), newMockLogger())
	assertWatchDirectoriesRemovable(t, r.watchTargets...)
	_, firstErrors := controlledSource(r.watchTargets[0])
	secondErrors := make(chan error, 1)
	r.watchTargets[1].watchErrors = secondErrors
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

func TestBoundedDispatchReportsLoss(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: domain.WatchTargetsOption{NoWait: true, WatchEvent: domain.WatchEvent{Create: true}}}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	r.runStartHook = func() {}
	r.runFinishHook = func() {}
	r.runChangeHook = func(domain.Event) { close(entered); <-release }
	baseline := runtime.NumGoroutine()
	go func() { result <- r.Run() }()
	events <- fsnotify.Event{Name: "first", Op: fsnotify.Create}
	<-entered
	started := time.Now()
	peak := runtime.NumGoroutine()
	for n := range dispatchCapacity + 1 {
		events <- fsnotify.Event{Name: fmt.Sprint(n), Op: fsnotify.Create}
		if current := runtime.NumGoroutine(); current > peak {
			peak = current
		}
	}
	select {
	case err := <-result:
		var fatal *FatalError
		if !errors.As(err, &fatal) || !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full") {
			t.Fatalf("bad loss: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue overflow blocked")
	}
	if growth := peak - baseline; growth > 4 {
		t.Errorf("unbounded goroutines: %d", growth)
	}
	t.Logf("pending capacity=%d active=1 goroutine baseline=%d sampled-peak=%d noWait-timers=0 loss-latency=%s", dispatchCapacity, baseline, peak, time.Since(started))
	close(release)
	var wg sync.WaitGroup
	wg.Add(1)
	r.Stop(&wg)
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := runtime.NumGoroutine(); got > baseline {
		t.Errorf("goroutines left: baseline=%d after=%d", baseline, got)
	}
	t.Logf("goroutines after stop=%d baseline=%d", runtime.NumGoroutine(), baseline)
}

func TestControlledModesPreservePolicy(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		for _, interruptible := range []bool{false, true} {
			t.Run(fmt.Sprintf("noWait=%t/interruptible=%t", noWait, interruptible), func(t *testing.T) {
				helper.GlobalLogger = newMockLogger()
				var mu sync.Mutex
				var started, completed []string
				active, peak := 0, 0
				changed := make(chan struct{}, 64)
				began := make(chan struct{}, 64)
				command := runnerCommand(func(ctx *helper.WatcherContext, event *domain.Event) error {
					mu.Lock()
					active++
					if active > peak {
						peak = active
					}
					started = append(started, event.Path.String())
					mu.Unlock()
					began <- struct{}{}
					defer func() { mu.Lock(); active--; mu.Unlock() }()
					select {
					case <-ctx.Context().Done():
						return ctx.Context().Err()
					case <-time.After(5 * time.Millisecond):
						mu.Lock()
						completed = append(completed, event.Path.String())
						mu.Unlock()
						changed <- struct{}{}
						return nil
					}
				})
				empty := emptyCommand()
				hooks := domain.Interruptible(interruptible).BuildHookFuncBuilder("test", domain.HookInfo{OnStartWatch: empty, OnBeforeChange: empty, OnChange: command, OnAfterChange: empty, OnFinishWatch: empty})(".")
				r := &watchTargetRunner{noWait: noWait, waitMillisecond: 20, excludeSuffix: domain.PathSuffixes{".swp"}, stop: make(chan struct{}), runChangeHook: hooks.RunChangeHook}
				input, done := make(chan domain.Event, 64), make(chan struct{})
				go func() { r.selectEventHandler()(input); close(done) }()
				for n := range 24 {
					input <- domain.Event{Path: domain.Path(fmt.Sprintf("%02d.go", n))}
					if noWait && interruptible {
						<-began
					}
					input <- domain.Event{Path: "excluded.swp"}
				}
				deadline := time.After(3 * time.Second)
				for {
					mu.Lock()
					last := len(completed) > 0 && completed[len(completed)-1] == "23.go"
					mu.Unlock()
					if last {
						break
					}
					select {
					case <-changed:
					case <-deadline:
						t.Fatal("last valid event did not complete")
					}
				}
				close(r.stop)
				hooks.Cancel()
				<-done
				hooks.RunFinishHook()
				mu.Lock()
				defer mu.Unlock()
				if peak != 1 || active != 0 {
					t.Fatalf("overlapping/leaked execution: peak=%d active=%d", peak, active)
				}
				if !noWait {
					if len(started) != 1 || started[0] != "23.go" {
						t.Fatalf("debounce: %v", started)
					}
				} else {
					if len(started) != 24 {
						t.Fatalf("noWait dropped work: %v", started)
					}
					for n, path := range started {
						if path != fmt.Sprintf("%02d.go", n) {
							t.Fatalf("reordered: %v", started)
						}
					}
					if !interruptible && len(completed) != 24 {
						t.Fatalf("sequential work canceled: %v", completed)
					}
				}
				t.Logf("started=%d completed=%d peak=%d excluded=24", len(started), len(completed), peak)
			})
		}
	}
}

func controlledSource(r *watchTargetRunner) (chan fsnotify.Event, chan error) {
	events, watchErrors := make(chan fsnotify.Event), make(chan error)
	r.watchEvents, r.watchErrors = events, watchErrors
	return events, watchErrors
}

var resourceDeletionDiagnostic func(*testing.T, string)

func assertWatchDirectoriesRemovable(t *testing.T, targets ...*watchTargetRunner) {
	t.Helper()
	t.Cleanup(func() {
		for _, target := range targets {
			before := time.Now()
			err := os.RemoveAll(target.path.String())
			t.Logf("resource removal path=%q at=%s duration=%s err=%v", target.path, time.Now().UTC().Format(time.RFC3339Nano), time.Since(before), err)
			if err != nil {
				if resourceDeletionDiagnostic != nil {
					resourceDeletionDiagnostic(t, target.path.String())
				}
				t.Errorf("watch directory remains in use after Stop: %v", err)
			}
		}
	})
}

func TestZeroDebounceKeepsDefaultWait(t *testing.T) {
	called := make(chan struct{}, 1)
	r := &watchTargetRunner{runChangeHook: func(domain.Event) { called <- struct{}{} }}
	input, done := make(chan domain.Event), make(chan struct{})
	go func() { r.selectEventHandler()(input); close(done) }()
	defer func() { close(input); <-done }()
	input <- domain.Event{Path: "valid.go"}
	select {
	case <-called:
		t.Fatal("zero wait lost the existing 100ms debounce default")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("default debounce did not execute")
	}
}
