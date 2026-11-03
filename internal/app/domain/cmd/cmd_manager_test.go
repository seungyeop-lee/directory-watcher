package cmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	command "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

// 훅 출력 테스트가 출력을 내는 실제 프로세스로 쓰려고 이 테스트 바이너리를 다시 실행한다.
// 마지막 두 인자가 동작과 줄 수다.
//   - print N: 표준 출력과 표준 오류에 번갈아 N줄을 쓰고, 마지막 줄은 줄바꿈 없이 끝낸다.
//   - cancel N: N줄과 ready 줄을 쓰고, 종료 신호를 받으면 N줄을 더 쓴 뒤 끝난다.
//   - fail N: 아무것도 쓰지 않고 종료 코드 3으로 끝난다.
//   - leave N: 출력을 물려받아 60초 동안 살아 있는 자식 프로세스를 띄워 그 pid를 쓰고, N줄과 done 줄을 쓴 뒤 끝난다.
//   - linger N: 60초를 기다린 뒤 끝난다. leave가 남기는 자식 프로세스다.
func TestHookOutputProcess(t *testing.T) {
	if os.Getenv("DW_HOOK_OUTPUT_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-2]
	count, err := strconv.Atoi(os.Args[len(os.Args)-1])
	if err != nil {
		os.Exit(2)
	}
	switch mode {
	case "print":
		writeLines("line", count)
	case "cancel":
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		writeLines("before", count)
		fmt.Println()
		fmt.Println("ready")
		<-signals
		writeLines("canceled", count)
	case "fail":
		os.Exit(3)
	case "leave":
		child := exec.Command(os.Args[0], "-test.run=^TestHookOutputProcess$", "--", "linger", "0")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Printf("child=%d\n", child.Process.Pid)
		writeLines("line", count)
		fmt.Println()
		fmt.Println("done")
	case "linger":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func writeLines(marker string, count int) {
	for n := range count {
		out := os.Stdout
		if n%2 == 1 {
			out = os.Stderr
		}
		end := "\n"
		if n == count-1 {
			end = ""
		}
		_, _ = fmt.Fprintf(out, "%s-%d%s", marker, n, end)
	}
}

func outputCommand(t *testing.T, mode string, count int) command.SingleCmd {
	t.Setenv("DW_HOOK_OUTPUT_HELPER", "1")
	// -race로 빌드한 바이너리는 끝날 때 1초를 기다리므로, 이 바이너리를 여러 번 실행하는 테스트가 느려지지 않게 끈다.
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	return command.SingleCmd{Shell: command.DefaultShell(), Cmd: fmt.Sprintf(`"%s" "-test.run=^TestHookOutputProcess$" -- %s %d`, os.Args[0], mode, count)}
}

// capturedContext는 훅 출력이 가는 표준 출력을 파일로 바꾼 뒤 만든 실행 context와, 그 파일의 내용을 읽는 함수를 돌려준다.
func capturedContext(t *testing.T) (*helper.WatcherContext, func() string) {
	t.Helper()
	base, output := capturedBase(t)
	return base.NewRun(), output
}

// capturedBase는 capturedContext와 같지만, NewRun이나 NewTimeoutRun으로 실행 context를 직접 만들 수 있게 바탕 context를 돌려준다.
func capturedBase(t *testing.T) (*helper.WatcherContext, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = file
	base := helper.NewWatcherContext("[test]")
	os.Stdout = original
	t.Cleanup(func() {
		base.Cancel()
		_ = file.Close()
	})
	return base, func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

func TestHookOutputIsCompleteWhenRunReturns(t *testing.T) {
	const lines = 2000
	cmd := command.NewManager(outputCommand(t, "print", lines))
	for attempt := range 20 {
		ctx, output := capturedContext(t)
		if err := cmd.Run(ctx, ".", nil); err != nil {
			t.Fatal(err)
		}
		if got := strings.Count(output(), "line-"); got != lines {
			t.Fatalf("attempt %d: printed %d of %d hook output lines", attempt, got, lines)
		}
	}
}

func TestCanceledHookPrintsAllOutputUntilPipeCloses(t *testing.T) {
	const lines = 500
	ctx, output := capturedContext(t)
	cmd := command.NewManager(outputCommand(t, "cancel", lines))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = cmd.Run(ctx, ".", nil)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(output(), "ready") {
		if time.Now().After(deadline) {
			t.Fatalf("hook did not become ready: %q", output())
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx.Cancel()
	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("canceled hook did not finish")
	}
	got := output()
	for _, marker := range []string{"before-", "canceled-"} {
		if count := strings.Count(got, marker); count != lines {
			t.Errorf("printed %d of %d %q lines", count, lines, marker)
		}
	}
}

func TestHookEndsSoonAfterCommandWhenLeftoverProcessHoldsOutput(t *testing.T) {
	const lines = 2000
	ctx, output := capturedContext(t)
	t.Cleanup(func() { killLeftoverChild(t, output()) })
	cmd := command.NewManager(outputCommand(t, "leave", lines))
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if err := cmd.Run(ctx, ".", nil); err != nil {
			t.Error(err)
		}
	}()
	// 명령은 done 줄을 쓰자마자 끝나므로, done 줄이 출력된 시각을 명령이 끝난 시각으로 본다.
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(output(), "done") {
		if time.Now().After(deadline) {
			t.Fatalf("hook did not print its last line: %q", output())
		}
		time.Sleep(10 * time.Millisecond)
	}
	commandEnded := time.Now()
	select {
	case <-finished:
	case <-time.After(25 * time.Second):
		t.Fatal("hook did not finish")
	}
	waited := time.Since(commandEnded)
	t.Logf("hook finished %v after the command ended", waited)
	if waited > 2*time.Second {
		t.Errorf("hook finished %v after the command ended, want about 1s", waited)
	}
	if got := strings.Count(output(), "line-"); got != lines {
		t.Errorf("printed %d of %d hook output lines", got, lines)
	}
}

var childPID = regexp.MustCompile(`child=(\d+)`)

func killLeftoverChild(t *testing.T, output string) {
	match := childPID.FindStringSubmatch(output)
	if match == nil {
		return
	}
	pid, _ := strconv.Atoi(match[1])
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
	}
}

func TestFailedMultiLineHookLeavesNoOutputReaders(t *testing.T) {
	ctx, _ := capturedContext(t)
	cmd := command.NewManager(command.MultiLineCmd{Cmds: []command.SingleCmd{
		outputCommand(t, "fail", 0),
		outputCommand(t, "print", 1),
	}})
	goroutines, files := runtime.NumGoroutine(), openFileCount()
	if err := cmd.Run(ctx, ".", nil); err == nil {
		t.Fatal("failing first command returned no error")
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > goroutines || openFileCount() > files {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines %d -> %d, open files %d -> %d after the hook failed", goroutines, runtime.NumGoroutine(), files, openFileCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// openFileCount는 이 프로세스가 연 파일 수를 센다. 셀 수 없는 OS(Windows)에서는 0을 돌려줘 goroutine 수만 비교하게 한다.
func openFileCount() int {
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return 0
	}
	return len(entries)
}

func TestCommandIsNotStartedWhenWorkingDirectoryIsMissing(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-dir")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		builder command.ExecCmdBuilder
		runDir  string
		wantDir string
	}{
		"missing run directory":   {outputCommand(t, "print", 1), missingDir, missingDir},
		"missing structured dir":  {command.StructuredCmd{Cmd: outputCommand(t, "print", 1), Dir: domain.Path(missingDir)}, ".", missingDir},
		"run directory is a file": {outputCommand(t, "print", 1), file, file},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, output := capturedContext(t)
			err := command.NewManager(tc.builder).Run(ctx, domain.Path(tc.runDir), nil)
			if err == nil || !strings.Contains(err.Error(), "working directory "+tc.wantDir) {
				t.Fatalf("error does not name the working directory %q: %v", tc.wantDir, err)
			}
			if got := output(); got != "" {
				t.Fatalf("command ran in a missing working directory: %q", got)
			}
		})
	}
}
