package service

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func TestReplacementWaitsForCanceledGeneration(t *testing.T) {
	started, canceled, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
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
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptOldest}, "test", ".", hookInfo{emptyCommand(), command, emptyCommand(), emptyCommand(), emptyCommand()}, newMockLogger(), process.CommandMode{})
	// 첫 명령이 onBeforeChange를 차지하고 있으므로, 대체 명령은 그 실행에 합류해야 한다.
	hooks.runChangeHook(domain.Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.runChangeHook(domain.Event{}); close(replaced) }()
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
	hooks.runFinishHook()
}

func TestShutdownDuringReplacementPreventsNewGeneration(t *testing.T) {
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
		if calls.Add(1) > 1 {
			return nil
		}
		close(started)
		<-ctx.Context().Done()
		close(canceled)
		<-release
		return ctx.Context().Err()
	})
	empty := emptyCommand()
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptOldest}, "test", ".", hookInfo{OnStartWatch: empty, OnBeforeChange: command, OnChange: empty, OnAfterChange: empty, OnFinishWatch: empty}, newMockLogger(), process.CommandMode{})
	hooks.runChangeHook(domain.Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.runChangeHook(domain.Event{}); close(replaced) }()
	<-canceled
	hooks.cancel()
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement remained blocked after stop")
	}
	hooks.runFinishHook()
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

func (p *runProbe) Run(ctx *process.WatcherContext, _ domain.Path, event *domain.Event) error {
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
func (p *runProbe) record(name string) domain.Cmd {
	return runnerCommand(func(_ *process.WatcherContext, event *domain.Event) error {
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
func probeHooks(t *testing.T, policy domain.RunPolicy, p *runProbe) hookFunc {
	empty := emptyCommand()
	hooks := newHookFunc(policy, "test", ".", hookInfo{OnStartWatch: empty, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: empty}, newMockLogger(), process.CommandMode{})
	t.Cleanup(hooks.runFinishHook)
	return hooks
}

func pathEvent(path string) domain.Event { return domain.Event{Path: domain.Path(path)} }

// concurrency: 3에서 실행은 runChangeHook에 넘긴 순서대로 시작하고, 동시에 세 개까지만 돈다.
func TestConcurrencyStartsRunsInOrderUpToLimit(t *testing.T) {
	p := newRunProbe()
	hooks := probeHooks(t, domain.RunPolicy{Concurrency: 3}, p)
	go func() {
		for _, path := range []string{"a", "b", "c", "d", "e"} {
			hooks.runChangeHook(pathEvent(path))
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

// 각 interrupt 값이 concurrency 1과 2에서 docs/guide/running.md의 interrupt 표대로 실행을 취소하거나 자리가 날 때까지 기다린다.
func TestInterruptCancelsRunsByPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   domain.RunPolicy
		scenario func(t *testing.T, hooks hookFunc, p *runProbe)
	}{
		{"none/concurrency=1", domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptNone}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			go hooks.runChangeHook(pathEvent("a"))
			p.expectNothing(t)
			p.release("a")
			p.expect(t, "done a", "start a", "done a")
		}},
		{"none/concurrency=2", domain.RunPolicy{Concurrency: 2, Interrupt: domain.InterruptNone}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			go hooks.runChangeHook(pathEvent("b"))
			p.expectNothing(t)
			p.release("a")
			p.expectAnyOrder(t, "done a", "done a", "start b")
		}},
		{"samePath/concurrency=1", domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptSamePath}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "cancel a", "start a")
			go hooks.runChangeHook(pathEvent("b"))
			p.expectNothing(t)
			p.release("a")
			p.expect(t, "done a", "start b")
		}},
		{"samePath/concurrency=2", domain.RunPolicy{Concurrency: 2, Interrupt: domain.InterruptSamePath}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			// 자리가 남아 있어도 같은 경로의 실행은 취소한다.
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "cancel a", "start a")
			hooks.runChangeHook(pathEvent("b"))
			p.expect(t, "start b")
			hooks.runChangeHook(pathEvent("b"))
			p.expect(t, "cancel b", "start b")
			go hooks.runChangeHook(pathEvent("c"))
			p.expectNothing(t)
			p.release("b")
			p.expect(t, "done b", "start c")
		}},
		{"oldest/concurrency=1", domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptOldest}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			hooks.runChangeHook(pathEvent("b"))
			p.expect(t, "cancel a", "start b")
		}},
		{"oldest/concurrency=2", domain.RunPolicy{Concurrency: 2, Interrupt: domain.InterruptOldest}, func(t *testing.T, hooks hookFunc, p *runProbe) {
			hooks.runChangeHook(pathEvent("a"))
			p.expect(t, "start a")
			hooks.runChangeHook(pathEvent("b"))
			p.expect(t, "start b")
			hooks.runChangeHook(pathEvent("c"))
			p.expect(t, "cancel a", "start c")
			hooks.runChangeHook(pathEvent("d"))
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
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 2}, "test", ".", hookInfo{OnStartWatch: emptyCommand(), OnBeforeChange: p.record("before"), OnChange: p, OnAfterChange: p.record("after"), OnFinishWatch: emptyCommand()}, newMockLogger(), process.CommandMode{})
	hooks.runChangeHook(pathEvent("a"))
	hooks.runChangeHook(pathEvent("b"))
	p.expectAnyOrder(t, "before a", "start a", "before b", "start b")
	p.release("a")
	p.release("b")
	p.expectAnyOrder(t, "done a", "after a", "done b", "after b")
	hooks.runFinishHook()
}

// 감시를 멈추면 돌고 있는 모든 실행을 취소하고, 자리를 기다리던 실행은 시작하지 않으며, 모두 끝난 뒤 onFinishWatch를 실행한다.
func TestFinishCancelsAllRunsBeforeFinishHook(t *testing.T) {
	p := newRunProbe()
	empty := emptyCommand()
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 2}, "test", ".", hookInfo{OnStartWatch: empty, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: p.record("finish")}, newMockLogger(), process.CommandMode{})
	hooks.runChangeHook(pathEvent("a"))
	hooks.runChangeHook(pathEvent("b"))
	p.expectAnyOrder(t, "start a", "start b")
	waiting := make(chan struct{})
	go func() { hooks.runChangeHook(pathEvent("c")); close(waiting) }()
	p.expectNothing(t)
	hooks.runFinishHook()
	p.expectAnyOrder(t, "cancel a", "cancel b")
	p.expect(t, "finish watch")
	<-waiting
	p.expectNothing(t)
}

// concurrency: 2에서도 실행은 onStartWatch가 끝날 때까지 기다리고, interrupt: oldest일 때만 첫 실행이 onStartWatch를
// 취소한다.
func TestRunsWaitForStartHookUnlessOldest(t *testing.T) {
	for name, interrupt := range map[string]domain.Interrupt{"none": domain.InterruptNone, "samePath": domain.InterruptSamePath, "oldest": domain.InterruptOldest} {
		t.Run(name, func(t *testing.T) {
			p := newRunProbe()
			empty := emptyCommand()
			hooks := newHookFunc(domain.RunPolicy{Concurrency: 2, Interrupt: interrupt}, "test", ".", hookInfo{OnStartWatch: p, OnBeforeChange: empty, OnChange: p, OnAfterChange: empty, OnFinishWatch: empty}, newMockLogger(), process.CommandMode{})
			t.Cleanup(hooks.runFinishHook)
			hooks.runStartHook()
			p.expect(t, "start watch")
			go hooks.runChangeHook(pathEvent("a"))
			if interrupt == domain.InterruptOldest {
				p.expect(t, "cancel watch", "start a")
				return
			}
			p.expectNothing(t)
			p.release("watch")
			p.expect(t, "done watch", "start a")
		})
	}
}

// 0이 아닌 종료 코드로 끝나는 실제 프로세스가 필요한 테스트가 이 테스트 바이너리를 다시 실행한다.
func TestHookExitProcess(t *testing.T) {
	if os.Getenv("DW_HOOK_EXIT_HELPER") != "1" {
		return
	}
	os.Exit(3)
}

func TestTargetHookFailureIsLoggedAndSkipsRemainingCommands(t *testing.T) {
	t.Setenv("DW_HOOK_EXIT_HELPER", "1")
	exitCommand := testBinaryCommand(t, "-test.run=^TestHookExitProcess$")
	missingDir := domain.Path(filepath.Join(t.TempDir(), "not_exist"))
	cases := map[string]struct {
		failing domain.Cmd
		want    string
	}{
		"start failure": {command.NewManager(command.StructuredCmd{Cmd: command.SingleCmd{Shell: command.DefaultShell(), Cmd: "pwd"}, Dir: missingDir}), "pwd"},
		"non-zero exit": {command.NewManager(command.SingleCmd{Shell: command.DefaultShell(), Cmd: exitCommand}), exitCommand},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger := &notificationLog{}
			var afterRuns atomic.Int32
			after := runnerCommand(func(*process.WatcherContext, *domain.Event) error { afterRuns.Add(1); return nil })
			info := hookInfo{OnStartWatch: emptyCommand(), OnBeforeChange: emptyCommand(), OnChange: tc.failing, OnAfterChange: after, OnFinishWatch: emptyCommand()}
			hooks := newHookFunc(domain.RunPolicy{Concurrency: 1}, "[target-alias]", ".", info, logger, process.CommandMode{})
			hooks.runChangeHook(domain.Event{})
			// 자리가 하나이면 다음 실행은 앞 실행이 끝난 뒤 시작하므로, 두 번째 runChangeHook이 돌아오면 첫 실행은
			// 끝났다. runFinishHook은 돌고 있는 실행을 취소하므로, 첫 실행이 끝나기 전에 부르면 실패가 기록되지 않는다.
			hooks.runChangeHook(domain.Event{})
			hooks.runFinishHook()
			if afterRuns.Load() != 0 {
				t.Fatal("command after the failed one still ran")
			}
			if got := logger.String(); !strings.Contains(got, "[target-alias]") || !strings.Contains(got, tc.want) {
				t.Fatalf("failure log lacks alias or command %q: %q", tc.want, got)
			}
		})
	}
}

func TestTargetFinishHookFailureIsLogged(t *testing.T) {
	t.Setenv("DW_HOOK_EXIT_HELPER", "1")
	exitCommand := testBinaryCommand(t, "-test.run=^TestHookExitProcess$")
	logger := &notificationLog{}
	info := hookInfo{OnStartWatch: emptyCommand(), OnBeforeChange: emptyCommand(), OnChange: emptyCommand(), OnAfterChange: emptyCommand(), OnFinishWatch: command.NewManager(command.SingleCmd{Shell: command.DefaultShell(), Cmd: exitCommand})}
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 1}, "[target-alias]", ".", info, logger, process.CommandMode{})
	hooks.runFinishHook()
	if got := logger.String(); !strings.Contains(got, "[target-alias]") || !strings.Contains(got, exitCommand) {
		t.Fatalf("finish hook failure log lacks alias or command: %q", got)
	}
}

func TestInterruptedRunIsNotLoggedAsFailure(t *testing.T) {
	logger := &notificationLog{}
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	interrupted := runnerCommand(func(ctx *process.WatcherContext, _ *domain.Event) error {
		started <- struct{}{}
		if calls.Add(1) > 1 {
			return nil
		}
		<-ctx.Context().Done()
		// 취소된 프로세스처럼 컨텍스트 오류가 아닌 종료 오류를 돌려준다.
		return errors.New("signal: terminated")
	})
	info := hookInfo{OnStartWatch: emptyCommand(), OnBeforeChange: emptyCommand(), OnChange: interrupted, OnAfterChange: emptyCommand(), OnFinishWatch: emptyCommand()}
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptOldest}, "[target-alias]", ".", info, logger, process.CommandMode{})
	hooks.runChangeHook(domain.Event{})
	<-started
	hooks.runChangeHook(domain.Event{})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("replacement run did not start")
	}
	hooks.runFinishHook()
	if got := logger.String(); got != "" {
		t.Fatalf("interrupted run was logged as failure: %q", got)
	}
}

func TestTargetHookFailureNamesMissingWorkingDirectory(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-dir")
	noop := command.SingleCmd{Shell: command.DefaultShell(), Cmd: testBinaryCommand(t, "-test.run=^$")}
	cases := map[string]struct {
		finish     domain.Cmd
		targetPath domain.Path
	}{
		"missing target path":    {command.NewManager(noop), domain.Path(missingDir)},
		"missing structured dir": {command.NewManager(command.StructuredCmd{Cmd: noop, Dir: domain.Path(missingDir)}), "."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger := &notificationLog{}
			info := hookInfo{OnStartWatch: emptyCommand(), OnBeforeChange: emptyCommand(), OnChange: emptyCommand(), OnAfterChange: emptyCommand(), OnFinishWatch: tc.finish}
			newHookFunc(domain.RunPolicy{Concurrency: 1}, "[target-alias]", tc.targetPath, info, logger, process.CommandMode{}).runFinishHook()
			if got := logger.String(); !strings.Contains(got, "[target-alias]") || !strings.Contains(got, "working directory "+missingDir) {
				t.Fatalf("failure log lacks alias or working directory: %q", got)
			}
		})
	}
}

func TestTargetHooksRunWithTheirHookNamesAndCommandMode(t *testing.T) {
	calls := &hookCalls{}
	info := hookInfo{OnStartWatch: calls.command("start"), OnBeforeChange: calls.command("before"), OnChange: calls.command("change"), OnAfterChange: calls.command("after"), OnFinishWatch: calls.command("finish")}
	hooks := newHookFunc(domain.RunPolicy{Concurrency: 1}, "test", ".", info, newMockLogger(), process.CommandMode{DryRun: true})
	hooks.runStartHook()
	hooks.runChangeHook(domain.Event{})
	// 자리가 하나이면 두 번째 runChangeHook은 첫 실행이 끝난 뒤에 돌아온다. 두 번째 실행은 runFinishHook이 취소할 수 있다.
	hooks.runChangeHook(domain.Event{})
	hooks.runFinishHook()
	got := calls.list()
	want := []string{
		"start onStartWatch {Silent:false DryRun:true}",
		"before onBeforeChange {Silent:false DryRun:true}",
		"change onChange {Silent:false DryRun:true}",
		"after onAfterChange {Silent:false DryRun:true}",
	}
	finish := "finish onFinishWatch {Silent:false DryRun:true}"
	if len(got) < len(want)+1 || !slices.Equal(got[:len(want)], want) || got[len(got)-1] != finish {
		t.Fatalf("hook calls = %q, want %q first and %q last", got, want, finish)
	}
}
