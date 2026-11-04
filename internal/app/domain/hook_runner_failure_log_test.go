package domain_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type errorLog struct {
	mu       sync.Mutex
	messages []string
}

func (l *errorLog) Debug(string) {}
func (l *errorLog) Info(string)  {}
func (l *errorLog) Error(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, message)
}
func (l *errorLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.messages, "\n")
}

type fakeCmd func(*process.WatcherContext) error

func (f fakeCmd) Run(ctx *process.WatcherContext, _ domain.Path, _ *domain.Event) error {
	return f(ctx)
}

func succeed(*process.WatcherContext) error { return nil }

// testBinaryCommand는 이 테스트 바이너리를 flags로 다시 실행하는 hook 명령이다. 인자를 큰따옴표로 감싸
// 기본 셸(sh, cmd)이 경로의 공백이나 -test.run의 ^, $를 해석하지 않게 한다.
func testBinaryCommand(flags string) string {
	return fmt.Sprintf(`"%s" "%s"`, os.Args[0], flags)
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
	exitCommand := testBinaryCommand("-test.run=^TestHookExitProcess$")
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
			logger := &errorLog{}
			var afterRuns atomic.Int32
			after := fakeCmd(func(*process.WatcherContext) error { afterRuns.Add(1); return nil })
			info := domain.HookInfo{OnStartWatch: fakeCmd(succeed), OnBeforeChange: fakeCmd(succeed), OnChange: tc.failing, OnAfterChange: after, OnFinishWatch: fakeCmd(succeed)}
			hooks := domain.RunPolicy{Concurrency: 1}.BuildHookFuncBuilder("[target-alias]", info, logger)(".")
			hooks.RunChangeHook(domain.Event{})
			// 자리가 하나이면 다음 실행은 앞 실행이 끝난 뒤 시작하므로, 두 번째 RunChangeHook이 돌아오면 첫 실행은
			// 끝났다. RunFinishHook은 돌고 있는 실행을 취소하므로, 첫 실행이 끝나기 전에 부르면 실패가 기록되지 않는다.
			hooks.RunChangeHook(domain.Event{})
			hooks.RunFinishHook()
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
	exitCommand := testBinaryCommand("-test.run=^TestHookExitProcess$")
	logger := &errorLog{}
	info := domain.HookInfo{OnStartWatch: fakeCmd(succeed), OnBeforeChange: fakeCmd(succeed), OnChange: fakeCmd(succeed), OnAfterChange: fakeCmd(succeed), OnFinishWatch: command.NewManager(command.SingleCmd{Shell: command.DefaultShell(), Cmd: exitCommand})}
	hooks := domain.RunPolicy{Concurrency: 1}.BuildHookFuncBuilder("[target-alias]", info, logger)(".")
	hooks.RunFinishHook()
	if got := logger.String(); !strings.Contains(got, "[target-alias]") || !strings.Contains(got, exitCommand) {
		t.Fatalf("finish hook failure log lacks alias or command: %q", got)
	}
}

func TestInterruptedRunIsNotLoggedAsFailure(t *testing.T) {
	logger := &errorLog{}
	started := make(chan struct{}, 2)
	var calls atomic.Int32
	interrupted := fakeCmd(func(ctx *process.WatcherContext) error {
		started <- struct{}{}
		if calls.Add(1) > 1 {
			return nil
		}
		<-ctx.Context().Done()
		// 취소된 프로세스처럼 컨텍스트 오류가 아닌 종료 오류를 돌려준다.
		return errors.New("signal: terminated")
	})
	info := domain.HookInfo{OnStartWatch: fakeCmd(succeed), OnBeforeChange: fakeCmd(succeed), OnChange: interrupted, OnAfterChange: fakeCmd(succeed), OnFinishWatch: fakeCmd(succeed)}
	hooks := domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptOldest}.BuildHookFuncBuilder("[target-alias]", info, logger)(".")
	hooks.RunChangeHook(domain.Event{})
	<-started
	hooks.RunChangeHook(domain.Event{})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("replacement run did not start")
	}
	hooks.RunFinishHook()
	if got := logger.String(); got != "" {
		t.Fatalf("interrupted run was logged as failure: %q", got)
	}
}

func TestTargetHookFailureNamesMissingWorkingDirectory(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-dir")
	noop := command.SingleCmd{Shell: command.DefaultShell(), Cmd: testBinaryCommand("-test.run=^$")}
	cases := map[string]struct {
		finish     domain.Cmd
		targetPath domain.Path
	}{
		"missing target path":    {command.NewManager(noop), domain.Path(missingDir)},
		"missing structured dir": {command.NewManager(command.StructuredCmd{Cmd: noop, Dir: domain.Path(missingDir)}), "."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logger := &errorLog{}
			info := domain.HookInfo{OnStartWatch: fakeCmd(succeed), OnBeforeChange: fakeCmd(succeed), OnChange: fakeCmd(succeed), OnAfterChange: fakeCmd(succeed), OnFinishWatch: tc.finish}
			domain.RunPolicy{Concurrency: 1}.BuildHookFuncBuilder("[target-alias]", info, logger)(tc.targetPath).RunFinishHook()
			if got := logger.String(); !strings.Contains(got, "[target-alias]") || !strings.Contains(got, "working directory "+missingDir) {
				t.Fatalf("failure log lacks alias or working directory: %q", got)
			}
		})
	}
}
