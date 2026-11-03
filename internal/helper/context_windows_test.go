//go:build windows

package helper

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"sync"
	"testing"
	"time"
)

// breakSignalExit는 TestBreakSignalProcess가 CTRL+BREAK를 받고 끝날 때의 종료 코드다.
const breakSignalExit = 3

// TestBreakSignalProcess는 hook 대신 실행되는 Go 프로그램이다. CTRL+BREAK를 받으면 breakSignalExit로 끝난다.
func TestBreakSignalProcess(t *testing.T) {
	if os.Getenv("DW_BREAK_SIGNAL_HELPER") != "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	os.Stdout.WriteString("ready\n")
	select {
	case <-signals:
		os.Exit(breakSignalExit)
	case <-time.After(30 * time.Second):
		os.Exit(0)
	}
}

// breakSignalHook은 RunCommand로 실행할 TestBreakSignalProcess 명령과, 그 명령에 CTRL+BREAK를 보낸 시각을 돌려준다.
func breakSignalHook(t *testing.T, ctx *WatcherContext) (*exec.Cmd, func() time.Time) {
	t.Helper()
	cmd := exec.CommandContext(ctx.Context(), os.Args[0], "-test.run=^TestBreakSignalProcess$")
	cmd.Env = append(os.Environ(), "DW_BREAK_SIGNAL_HELPER=1")
	if err := SetupForOs(cmd); err != nil {
		t.Fatal(err)
	}
	ctx.ApplyGracefulShutdown(cmd)
	var mu sync.Mutex
	var sent time.Time
	send := cmd.Cancel
	cmd.Cancel = func() error {
		mu.Lock()
		sent = time.Now()
		mu.Unlock()
		return send()
	}
	return cmd, func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return sent
	}
}

// statusControlCExit는 Go 프로그램이 signal.Notify를 부르기 전에 CTRL+BREAK를 받아 끝날 때의 종료 코드다.
const statusControlCExit = 0xC000013A

// assertEndedBySignal은 hook이 WaitDelay에 따른 강제 종료나 충돌이 아니라 CTRL+BREAK로 끝났는지 확인한다.
func assertEndedBySignal(t *testing.T, cmd *exec.Cmd, ran time.Duration, codes ...uint32) {
	t.Helper()
	if ran >= 10*time.Second {
		t.Fatalf("hook ran %s after cancel, want it to end on CTRL+BREAK", ran)
	}
	code := uint32(cmd.ProcessState.ExitCode())
	if !slices.Contains(codes, code) {
		t.Fatalf("hook exit code = 0x%X, want one of %#x from CTRL+BREAK", code, codes)
	}
}

// 시작 직후에 취소된 hook은 시작 후 minRunBeforeBreak가 지난 뒤에 CTRL+BREAK를 받아야 한다.
func TestRunCommandDelaysBreakRightAfterStart(t *testing.T) {
	ctx := NewWatcherContext("").NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	// os/exec는 프로세스를 만든 뒤에 Stdin을 읽기 시작하므로, 첫 Read에서 취소하면 시작 직후에 취소된다.
	cmd.Stdin = readFunc(func([]byte) (int, error) {
		ctx.Cancel()
		return 0, io.EOF
	})
	before := time.Now()
	_ = ctx.RunCommand(cmd)
	ended := time.Now()

	if sent := sentAt(); sent.Sub(before) < minRunBeforeBreak {
		t.Fatalf("CTRL+BREAK sent %s after start, want at least %s", sent.Sub(before), minRunBeforeBreak)
	}
	assertEndedBySignal(t, cmd, ended.Sub(before), breakSignalExit, statusControlCExit)
}

// 최소 시간이 지난 뒤에 취소된 hook은 지금처럼 바로 CTRL+BREAK를 받아야 한다.
func TestRunCommandSendsBreakAtOnceAfterMinimumRunTime(t *testing.T) {
	ctx := NewWatcherContext("").NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	out, in, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cmd.Stdout = in
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = ctx.RunCommand(cmd)
	}()
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("hook did not get ready: line=%q err=%v", line, err)
	}
	in.Close()
	time.Sleep(2 * minRunBeforeBreak)
	canceled := time.Now()
	ctx.Cancel()
	<-done

	if delay := sentAt().Sub(canceled); delay >= minRunBeforeBreak {
		t.Fatalf("CTRL+BREAK sent %s after cancel, want it sent at once", delay)
	}
	assertEndedBySignal(t, cmd, time.Since(canceled), breakSignalExit)
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }
