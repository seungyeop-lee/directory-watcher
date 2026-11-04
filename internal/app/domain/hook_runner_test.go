package domain

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type hookCommand func(*process.WatcherContext, *Event) error

func (f hookCommand) Run(ctx *process.WatcherContext, _ Path, event *Event) error {
	return f(ctx, event)
}
func emptyHook(*process.WatcherContext, *Event) error { return nil }

func testLogger() helper.Logger {
	return helper.NewBasicLogger(helper.LogLevelString("ERROR").GetLogLevel())
}

func TestReplacementWaitsForCanceledGeneration(t *testing.T) {
	helper.GlobalLogger = helper.NewBasicLogger(helper.LogLevelString("ERROR").GetLogLevel())
	started, canceled, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := hookCommand(func(ctx *process.WatcherContext, _ *Event) error {
		if calls.Add(1) != 1 {
			return nil
		}
		original := ctx.Context()
		close(started)
		<-original.Done()
		close(canceled)
		<-release
		if ctx.Context() != original {
			t.Error("canceled generation received a replacement context")
		}
		close(completed)
		return original.Err()
	})
	hooks := RunPolicy{Concurrency: 1, Interrupt: InterruptOldest}.BuildHookFuncBuilder("test", HookInfo{hookCommand(emptyHook), command, hookCommand(emptyHook), hookCommand(emptyHook), hookCommand(emptyHook)}, helper.GlobalLogger)(".")
	// 첫 명령이 onBeforeChange를 차지하고 있으므로, 대체 명령은 그 실행에 합류해야 한다.
	hooks.RunChangeHook(Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.RunChangeHook(Event{}); close(replaced) }()
	<-canceled
	select {
	case <-replaced:
		t.Error("replacement returned before canceled generation completed")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement did not finish")
	}
	<-completed
	hooks.RunFinishHook()
}

func TestShutdownDuringReplacementPreventsNewGeneration(t *testing.T) {
	helper.GlobalLogger = helper.NewBasicLogger(helper.LogLevelString("ERROR").GetLogLevel())
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := hookCommand(func(ctx *process.WatcherContext, _ *Event) error {
		if calls.Add(1) > 1 {
			return nil
		}
		close(started)
		<-ctx.Context().Done()
		close(canceled)
		<-release
		return ctx.Context().Err()
	})
	empty := hookCommand(emptyHook)
	hooks := RunPolicy{Concurrency: 1, Interrupt: InterruptOldest}.BuildHookFuncBuilder("test", HookInfo{OnStartWatch: empty, OnBeforeChange: command, OnChange: empty, OnAfterChange: empty, OnFinishWatch: empty}, helper.GlobalLogger)(".")
	hooks.RunChangeHook(Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.RunChangeHook(Event{}); close(replaced) }()
	<-canceled
	hooks.Cancel()
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement remained blocked after stop")
	}
	hooks.RunFinishHook()
	if got := calls.Load(); got != 1 {
		t.Fatalf("new generation started after shutdown: %d", got)
	}
}

// runProbe는 hook 명령이다. 실행마다 "start a"처럼 시작을 알리고, 그 경로를 release할 때까지 끝나지 않는다.
// 끝나면 "done a"를, 취소되면 "cancel a"를 알린다. onStartWatch처럼 이벤트가 없는 실행의 경로는 watch다.
type runProbe struct {
	mu       sync.Mutex
	active   int
	peak     int
	releases map[string]chan struct{}
	events   chan string
}

func newRunProbe() *runProbe {
	return &runProbe{releases: map[string]chan struct{}{}, events: make(chan string, 64)}
}

func (p *runProbe) releaseOf(path string) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.releases[path]
	if !ok {
		ch = make(chan struct{})
		p.releases[path] = ch
	}
	return ch
}

// release는 경로 path의 실행을 끝낸다. 그 뒤에 시작하는 path의 실행은 바로 끝난다.
func (p *runProbe) release(path string) { close(p.releaseOf(path)) }

func (p *runProbe) Run(ctx *process.WatcherContext, _ Path, event *Event) error {
	path := "watch"
	if event != nil {
		path = event.Path.String()
	}
	p.mu.Lock()
	p.active++
	p.peak = max(p.peak, p.active)
	p.mu.Unlock()
	p.events <- "start " + path
	result, err := "done ", error(nil)
	select {
	case <-p.releaseOf(path):
	case <-ctx.Context().Done():
		result, err = "cancel ", ctx.Context().Err()
	}
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	p.events <- result + path
	return err
}

// record는 실행마다 "before a"처럼 name과 경로를 probe에 알리는 명령이다.
func (p *runProbe) record(name string) Cmd {
	return hookCommand(func(_ *process.WatcherContext, event *Event) error {
		path := "watch"
		if event != nil {
			path = event.Path.String()
		}
		p.events <- name + " " + path
		return nil
	})
}

// expect는 probe가 want를 차례로 알리는지 확인한다.
func (p *runProbe) expect(t *testing.T, want ...string) {
	t.Helper()
	for _, w := range want {
		select {
		case got := <-p.events:
			if got != w {
				t.Fatalf("event = %q, want %q", got, w)
			}
		case <-time.After(time.Second):
			t.Fatalf("no event, want %q", w)
		}
	}
}

// expectAnyOrder는 probe가 want를 순서와 관계없이 알리는지 확인한다.
func (p *runProbe) expectAnyOrder(t *testing.T, want ...string) {
	t.Helper()
	var got []string
	for range want {
		select {
		case e := <-p.events:
			got = append(got, e)
		case <-time.After(time.Second):
			t.Fatalf("events = %q, want %q in any order", got, want)
		}
	}
	slices.Sort(got)
	if sorted := slices.Sorted(slices.Values(want)); !slices.Equal(got, sorted) {
		t.Fatalf("events = %q, want %q in any order", got, sorted)
	}
}

// expectNothing은 probe가 잠시 아무것도 알리지 않는지, 곧 실행이 시작하거나 끝나지 않고 기다리는지 확인한다.
func (p *runProbe) expectNothing(t *testing.T) {
	t.Helper()
	select {
	case got := <-p.events:
		t.Fatalf("unexpected event %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

// probeHooks는 onChange가 probe이고 다른 명령은 아무것도 하지 않는 hook을 policy로 만든다.
func probeHooks(t *testing.T, policy RunPolicy, p *runProbe) *HookFunc {
	empty := hookCommand(emptyHook)
	hooks := policy.BuildHookFuncBuilder("test", HookInfo{OnStartWatch: empty, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: empty}, testLogger())(".")
	t.Cleanup(hooks.RunFinishHook)
	return hooks
}

func change(path string) Event { return Event{Path: Path(path)} }

// concurrency: 3에서 실행은 RunChangeHook에 넘긴 순서대로 시작하고, 동시에 세 개까지만 돈다.
func TestConcurrencyStartsRunsInOrderUpToLimit(t *testing.T) {
	p := newRunProbe()
	hooks := probeHooks(t, RunPolicy{Concurrency: 3}, p)
	go func() {
		for _, path := range []string{"a", "b", "c", "d", "e"} {
			hooks.RunChangeHook(change(path))
		}
	}()
	p.expectAnyOrder(t, "start a", "start b", "start c")
	p.expectNothing(t)
	p.release("b")
	p.expect(t, "done b", "start d")
	p.expectNothing(t)
	p.release("a")
	p.expect(t, "done a", "start e")
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.peak != 3 {
		t.Fatalf("peak = %d, want 3", p.peak)
	}
}

// 각 interrupt 값이 concurrency 1과 2에서 USAGE의 표대로 실행을 취소하거나 자리가 날 때까지 기다린다.
func TestInterruptCancelsRunsByPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   RunPolicy
		scenario func(t *testing.T, hooks *HookFunc, p *runProbe)
	}{
		{"none/concurrency=1", RunPolicy{Concurrency: 1, Interrupt: InterruptNone}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			go hooks.RunChangeHook(change("a"))
			p.expectNothing(t)
			p.release("a")
			p.expect(t, "done a", "start a", "done a")
		}},
		{"none/concurrency=2", RunPolicy{Concurrency: 2, Interrupt: InterruptNone}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			go hooks.RunChangeHook(change("b"))
			p.expectNothing(t)
			p.release("a")
			p.expectAnyOrder(t, "done a", "done a", "start b")
		}},
		{"samePath/concurrency=1", RunPolicy{Concurrency: 1, Interrupt: InterruptSamePath}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			hooks.RunChangeHook(change("a"))
			p.expect(t, "cancel a", "start a")
			go hooks.RunChangeHook(change("b"))
			p.expectNothing(t)
			p.release("a")
			p.expect(t, "done a", "start b")
		}},
		{"samePath/concurrency=2", RunPolicy{Concurrency: 2, Interrupt: InterruptSamePath}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			// 자리가 남아 있어도 같은 경로의 실행은 취소한다.
			hooks.RunChangeHook(change("a"))
			p.expect(t, "cancel a", "start a")
			hooks.RunChangeHook(change("b"))
			p.expect(t, "start b")
			hooks.RunChangeHook(change("b"))
			p.expect(t, "cancel b", "start b")
			go hooks.RunChangeHook(change("c"))
			p.expectNothing(t)
			p.release("b")
			p.expect(t, "done b", "start c")
		}},
		{"oldest/concurrency=1", RunPolicy{Concurrency: 1, Interrupt: InterruptOldest}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			hooks.RunChangeHook(change("b"))
			p.expect(t, "cancel a", "start b")
		}},
		{"oldest/concurrency=2", RunPolicy{Concurrency: 2, Interrupt: InterruptOldest}, func(t *testing.T, hooks *HookFunc, p *runProbe) {
			hooks.RunChangeHook(change("a"))
			p.expect(t, "start a")
			hooks.RunChangeHook(change("b"))
			p.expect(t, "start b")
			hooks.RunChangeHook(change("c"))
			p.expect(t, "cancel a", "start c")
			hooks.RunChangeHook(change("d"))
			p.expect(t, "cancel b", "start d")
			p.expectNothing(t)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newRunProbe()
			tt.scenario(t, probeHooks(t, tt.policy, p), p)
		})
	}
}

// 실행이 동시에 돌아도 전역 onBeforeChange와 onAfterChange는 실행마다 그 실행의 onChange 앞뒤로 돈다.
func TestGlobalChangeHooksRunForEachRun(t *testing.T) {
	p := newRunProbe()
	hooks := RunPolicy{Concurrency: 2}.BuildHookFuncBuilder("test", HookInfo{OnStartWatch: hookCommand(emptyHook), OnBeforeChange: p.record("before"), OnChange: p, OnAfterChange: p.record("after"), OnFinishWatch: hookCommand(emptyHook)}, testLogger())(".")
	hooks.RunChangeHook(change("a"))
	hooks.RunChangeHook(change("b"))
	p.expectAnyOrder(t, "before a", "start a", "before b", "start b")
	p.release("a")
	p.release("b")
	p.expectAnyOrder(t, "done a", "after a", "done b", "after b")
	hooks.RunFinishHook()
}

// 감시를 멈추면 돌고 있는 모든 실행을 취소하고, 자리를 기다리던 실행은 시작하지 않으며, 모두 끝난 뒤 onFinishWatch를 실행한다.
func TestFinishCancelsAllRunsBeforeFinishHook(t *testing.T) {
	p := newRunProbe()
	empty := hookCommand(emptyHook)
	hooks := RunPolicy{Concurrency: 2}.BuildHookFuncBuilder("test", HookInfo{OnStartWatch: empty, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: p.record("finish")}, testLogger())(".")
	hooks.RunChangeHook(change("a"))
	hooks.RunChangeHook(change("b"))
	p.expectAnyOrder(t, "start a", "start b")
	waiting := make(chan struct{})
	go func() { hooks.RunChangeHook(change("c")); close(waiting) }()
	p.expectNothing(t)
	hooks.RunFinishHook()
	p.expectAnyOrder(t, "cancel a", "cancel b")
	p.expect(t, "finish watch")
	<-waiting
	p.expectNothing(t)
}

// concurrency: 2에서도 실행은 onStartWatch가 끝날 때까지 기다리고, interrupt: oldest일 때만 첫 실행이 onStartWatch를
// 취소한다.
func TestRunsWaitForStartHookUnlessOldest(t *testing.T) {
	for name, interrupt := range map[string]Interrupt{"none": InterruptNone, "samePath": InterruptSamePath, "oldest": InterruptOldest} {
		t.Run(name, func(t *testing.T) {
			p := newRunProbe()
			empty := hookCommand(emptyHook)
			hooks := RunPolicy{Concurrency: 2, Interrupt: interrupt}.BuildHookFuncBuilder("test", HookInfo{OnStartWatch: p, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: empty}, testLogger())(".")
			t.Cleanup(hooks.RunFinishHook)
			hooks.RunStartHook()
			p.expect(t, "start watch")
			go hooks.RunChangeHook(change("a"))
			if interrupt == InterruptOldest {
				p.expect(t, "cancel watch", "start a")
				return
			}
			p.expectNothing(t)
			p.release("watch")
			p.expect(t, "done watch", "start a")
		})
	}
}
