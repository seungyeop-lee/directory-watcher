package cmd_test

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	command "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

// 훅 출력 테스트가 출력을 내는 실제 프로세스로 쓰려고 이 테스트 바이너리를 다시 실행한다.
// 마지막 두 인자가 동작과 줄 수다.
//   - print N: 표준 출력과 표준 오류에 번갈아 N줄을 쓰고, 마지막 줄은 줄바꿈 없이 끝낸다.
//   - cancel N: N줄과 ready 줄을 쓰고, 종료 신호를 받으면 N줄을 더 쓴 뒤 끝난다.
//   - fail N: 아무것도 쓰지 않고 종료 코드 3으로 끝난다.
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
	return command.SingleCmd(fmt.Sprintf("%s -test.run=^TestHookOutputProcess$ -- %s %d", os.Args[0], mode, count))
}

// capturedContext는 훅 출력이 가는 표준 출력을 파일로 바꾼 뒤 만든 실행 context와, 그 파일의 내용을 읽는 함수를 돌려준다.
func capturedContext(t *testing.T) (*helper.WatcherContext, func() string) {
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
	return base.NewRun(), func() string {
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
