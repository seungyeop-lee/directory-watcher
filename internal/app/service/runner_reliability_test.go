package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func TestStopDiscardsPendingEvents(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	r.dispatcher.hooks.runChangeHook = func(domain.Event) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	}
	go r.Run()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
	<-entered
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "queued"), Op: fsnotify.Create}
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

func TestWatcherLossStopsDispatchWhileHookRuns(t *testing.T) {
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, watchErrors := controlledSource(r)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	r.dispatcher.hooks.runChangeHook = func(domain.Event) { close(entered); <-release }
	go func() { r.Run(); close(done) }()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
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

func TestRunnerPreservesOnlyFirstFatalError(t *testing.T) {
	helper.GlobalLogger = newMockLogger()
	r := NewRunner(testCommandSet(t.TempDir(), t.TempDir()), newMockLogger())
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

func TestBoundedDispatchReportsLoss(t *testing.T) {
	const boundedQueueSize = 16
	option := watchTargetOption(domain.WatchOption{FileEvent: domain.WatchEvent{Create: true}})
	option.Queue.Size = boundedQueueSize
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(t.TempDir()), Option: option}, newMockLogger())
	assertWatchDirectoriesRemovable(t, r)
	events, _ := controlledSource(r)
	entered, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	r.dispatcher.hooks.runStartHook = func() {}
	r.dispatcher.hooks.runFinishHook = func() {}
	// 실제 runChangeHook은 hook을 시작하면 돌아오고, 자리가 없으면 자리가 날 때까지 막힌다. 그래서 첫 실행은 돌고 있는
	// 실행처럼 돌아오고, 다음 실행은 자리를 기다리며 대기열에 남는다.
	var calls atomic.Int32
	r.dispatcher.hooks.runChangeHook = func(domain.Event) {
		if calls.Add(1) == 1 {
			close(entered)
			return
		}
		<-release
	}
	baseline := runtime.NumGoroutine()
	go func() { result <- r.Run() }()
	events <- fsnotify.Event{Name: filepath.Join(r.path.String(), "first"), Op: fsnotify.Create}
	<-entered
	started := time.Now()
	peak := runtime.NumGoroutine()
	for n := range boundedQueueSize + 1 {
		events <- fsnotify.Event{Name: filepath.Join(r.path.String(), fmt.Sprint(n)), Op: fsnotify.Create}
		if current := runtime.NumGoroutine(); current > peak {
			peak = current
		}
	}
	select {
	case err := <-result:
		var fatal *FatalError
		if !errors.As(err, &fatal) || !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 16)") {
			t.Fatalf("bad loss: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queue overflow blocked")
	}
	if growth := peak - baseline; growth > 4 {
		t.Errorf("unbounded goroutines: %d", growth)
	}
	t.Logf("pending capacity=%d active=1 goroutine baseline=%d sampled-peak=%d loss-latency=%s", boundedQueueSize, baseline, peak, time.Since(started))
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

// 기다린 뒤 last로 넘기면 마지막 유효 이벤트 하나만, 기다리지 않고 each로 넘기면 모든 이벤트가 생긴 순서대로 hook에
// 가는지, 그리고 run.concurrency가 1이면 run.interrupt가 none이든 oldest든 hook이 겹쳐 실행되지 않는지 확인한다.
func TestControlledModesPreservePolicy(t *testing.T) {
	modes := []struct {
		name    string
		wait    domain.Millisecond
		deliver domain.Deliver
	}{
		{"waitMillisecond=100/deliver=last", 100, domain.DeliverLast},
		{"waitMillisecond=0/deliver=each", 0, domain.DeliverEach},
	}
	for _, mode := range modes {
		for name, interrupt := range map[string]domain.Interrupt{"none": domain.InterruptNone, "oldest": domain.InterruptOldest} {
			t.Run(fmt.Sprintf("%s/interrupt=%s", mode.name, name), func(t *testing.T) {
				oldest := interrupt == domain.InterruptOldest
				helper.GlobalLogger = newMockLogger()
				each := mode.deliver == domain.DeliverEach
				var mu sync.Mutex
				var started, completed []string
				active, peak := 0, 0
				changed := make(chan struct{}, 64)
				began := make(chan struct{}, 64)
				command := runnerCommand(func(ctx *process.WatcherContext, event *domain.Event) error {
					mu.Lock()
					active++
					if active > peak {
						peak = active
					}
					started = append(started, filepath.Base(event.Path.String()))
					mu.Unlock()
					began <- struct{}{}
					defer func() { mu.Lock(); active--; mu.Unlock() }()
					select {
					case <-ctx.Context().Done():
						return ctx.Context().Err()
					case <-time.After(5 * time.Millisecond):
						mu.Lock()
						completed = append(completed, filepath.Base(event.Path.String()))
						mu.Unlock()
						changed <- struct{}{}
						return nil
					}
				})
				empty := emptyCommand()
				option := hookBatchOption(mode.wait, 1000, mode.deliver)
				option.Watch.Exclude = domain.ExcludePatterns{"**/*.swp"}
				option.Run = domain.RunPolicy{Concurrency: 1, Interrupt: interrupt}
				root := t.TempDir()
				r := NewWatchTargetRunner(
					domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnBeforeChange: empty, OnAfterChange: empty}},
					domain.WatchTargetsCommandSet{Path: domain.Path(root), LifeCycle: domain.WatchTargetsLifeCycle{OnStartWatch: empty, OnChange: command, OnFinishWatch: empty}, Option: option},
					newMockLogger())
				events, _ := controlledSource(r)
				go r.Run()
				// 파일을 모두 만든 뒤 이벤트를 몰아 넣어, last의 이벤트가 한 번 기다리는 동안 모두 모이게 한다.
				var names []string
				for n := range 24 {
					names = append(names, filepath.Join(root, fmt.Sprintf("%02d.go", n)))
					writeFile(t, names[n])
				}
				for _, name := range names {
					events <- fsnotify.Event{Name: name, Op: fsnotify.Create}
					if each && oldest {
						<-began
					}
					events <- fsnotify.Event{Name: filepath.Join(root, "excluded.swp"), Op: fsnotify.Create}
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
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
				mu.Lock()
				defer mu.Unlock()
				if peak != 1 || active != 0 {
					t.Fatalf("overlapping/leaked execution: peak=%d active=%d", peak, active)
				}
				if !each {
					if len(started) != 1 || started[0] != "23.go" {
						t.Fatalf("last: %v", started)
					}
				} else {
					if len(started) != 24 {
						t.Fatalf("each dropped work: %v", started)
					}
					for n, path := range started {
						if path != fmt.Sprintf("%02d.go", n) {
							t.Fatalf("reordered: %v", started)
						}
					}
					if !oldest && len(completed) != 24 {
						t.Fatalf("sequential work canceled: %v", completed)
					}
				}
				t.Logf("started=%d completed=%d peak=%d excluded=24", len(started), len(completed), peak)
			})
		}
	}
}
