//go:build windows

package process

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// breakSignalExit는 TestBreakSignalProcess가 CTRL+BREAK를 받고 끝날 때의 종료 코드다.
const breakSignalExit = 3

// TestBreakSignalProcess는 hook 대신 실행되는 Go 프로그램이다. CTRL+BREAK를 받으면 breakSignalExit로 끝난다.
// DW_BREAK_SIGNAL_RECORD가 있으면 그 파일에 시작과 신호 수신을 pid와 함께 한 줄씩 덧붙이고, 시작에는 프로세스 생성 시각도
// 덧붙인다. DW_BREAK_SIGNAL_IGNORE=1이면 신호를 받아도 끝나지 않는다.
// DW_BREAK_SIGNAL_CHILD=1이면 minRunBeforeBreak가 지난 뒤 자기 사본을 자식으로 시작하고 나서 ready를 쓴다.
// DW_BREAK_SIGNAL_LATE_CHILD=1이면 ready를 쓴 뒤 표준 입력에서 1바이트를 읽고 나서 자기 사본을 자식으로 하나 더 시작한다.
// DW_BREAK_SIGNAL_EXIT_ON_EOF=1이면 시작을 기록한 뒤 표준 입력을 끝까지 읽고 0으로 끝난다.
func TestBreakSignalProcess(t *testing.T) {
	if os.Getenv("DW_BREAK_SIGNAL_HELPER") != "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	record := func(event string, fields ...any) {
		if file := os.Getenv("DW_BREAK_SIGNAL_RECORD"); file != "" {
			f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				os.Exit(2)
			}
			fmt.Fprintln(f, append([]any{event, os.Getpid()}, fields...)...)
			f.Close()
		}
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		os.Exit(2)
	}
	record("start", created.Nanoseconds())
	if os.Getenv("DW_BREAK_SIGNAL_EXIT_ON_EOF") == "1" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if os.Getenv("DW_BREAK_SIGNAL_CHILD") == "1" {
		time.Sleep(minRunBeforeBreak)
		startBreakSignalChild()
	}
	os.Stdout.WriteString("ready\n")
	if os.Getenv("DW_BREAK_SIGNAL_LATE_CHILD") == "1" {
		if _, err := os.Stdin.Read(make([]byte, 1)); err != nil {
			os.Exit(2)
		}
		startBreakSignalChild()
	}
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-signals:
			record("signal")
			if os.Getenv("DW_BREAK_SIGNAL_IGNORE") != "1" {
				os.Exit(breakSignalExit)
			}
		case <-timeout:
			os.Exit(0)
		}
	}
}

// startBreakSignalChild는 TestBreakSignalProcess의 사본을 자식을 만들지 않는 자식으로 시작한다.
func startBreakSignalChild() {
	child := exec.Command(os.Args[0], "-test.run=^TestBreakSignalProcess$")
	child.Env = append(os.Environ(), "DW_BREAK_SIGNAL_CHILD=", "DW_BREAK_SIGNAL_LATE_CHILD=")
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
}

// breakSignalHook은 RunCommand로 실행할 TestBreakSignalProcess 명령과, 그 명령에 CTRL+BREAK를 보낸 시각들을 돌려준다.
func breakSignalHook(t *testing.T, ctx *WatcherContext) (*exec.Cmd, func() []time.Time) {
	t.Helper()
	cmd := exec.CommandContext(ctx.Context(), os.Args[0], "-test.run=^TestBreakSignalProcess$")
	return cmd, recordBreaks(t, ctx, cmd)
}

// breakSignalShellHook은 TestBreakSignalProcess를 programs번 차례로 실행하는 cmd.exe 명령 목록(a & b)과,
// 그 명령에 CTRL+BREAK를 보낸 시각들을 돌려준다.
func breakSignalShellHook(t *testing.T, ctx *WatcherContext, programs int) (*exec.Cmd, func() []time.Time) {
	t.Helper()
	program := `"` + os.Args[0] + `" "-test.run=^TestBreakSignalProcess$"`
	script := strings.Repeat(program+" & ", programs-1) + program
	cmd := exec.CommandContext(ctx.Context(), "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + script + `"`}
	return cmd, recordBreaks(t, ctx, cmd)
}

// recordBreaks는 watcher처럼 cmd에 정상 종료를 설정하고, cmd에 CTRL+BREAK를 보낸 시각들을 돌려주는 함수를 돌려준다.
func recordBreaks(t *testing.T, ctx *WatcherContext, cmd *exec.Cmd) func() []time.Time {
	t.Helper()
	cmd.Env = append(os.Environ(), "DW_BREAK_SIGNAL_HELPER=1")
	SetupForOs(cmd)
	ctx.ApplyGracefulShutdown(cmd)
	var mu sync.Mutex
	var sent []time.Time
	send := cmd.Cancel
	cmd.Cancel = func() error {
		mu.Lock()
		sent = append(sent, time.Now())
		mu.Unlock()
		return send()
	}
	return func() []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(sent)
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
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	// os/exec는 프로세스를 만든 뒤에 Stdin을 읽기 시작하므로, 첫 Read에서 취소하면 시작 직후에 취소된다.
	cmd.Stdin = readFunc(func([]byte) (int, error) {
		ctx.Cancel()
		return 0, io.EOF
	})
	before := time.Now()
	_ = ctx.RunCommand(cmd)
	ended := time.Now()

	if sent := sentAt(); len(sent) != 1 || sent[0].Sub(before) < minRunBeforeBreak {
		t.Fatalf("CTRL+BREAK sent at %v after start %v, want once at least %s after start", sent, before, minRunBeforeBreak)
	}
	assertEndedBySignal(t, cmd, ended.Sub(before), breakSignalExit, statusControlCExit)
}

// 취소된 hook이 첫 CTRL+BREAK 전에 끝나도, CTRL+BREAK를 보낼 때까지 hook의 PID는 다른 프로세스에 재사용되지 않아야 한다.
// CTRL+BREAK는 PID로 프로세스 그룹을 고르므로, PID가 재사용되면 그 PID를 받은 다른 프로세스 그룹이 신호를 받는다.
func TestRunCommandKeepsPIDOfExitedHookUntilBreak(t *testing.T) {
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("DW_BREAK_SIGNAL_RECORD", record)
	t.Setenv("DW_BREAK_SIGNAL_EXIT_ON_EOF", "1")
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	// 첫 Read에서 취소하고 나서 EOF를 돌려주므로, hook은 취소된 뒤에 끝난다.
	cmd.Stdin = readFunc(func([]byte) (int, error) {
		ctx.Cancel()
		return 0, io.EOF
	})
	out, in, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cmd.Stdout = in
	var pidErr error
	var seen []time.Time
	send := cmd.Cancel
	cmd.Cancel = func() error {
		// hook의 stdout이 닫히면 hook이 끝난 것이다.
		in.Close()
		_, _ = io.Copy(io.Discard, out)
		// os/exec는 hook이 끝난 직후에 자기 handle을 닫으므로, 그 뒤에도 PID가 hook을 가리키는지 잠시 지켜본다.
		for end := time.Now().Add(500 * time.Millisecond); pidErr == nil && time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			var created time.Time
			if created, pidErr = processCreation(uint32(cmd.Process.Pid)); pidErr == nil && !slices.Contains(seen, created) {
				seen = append(seen, created)
			}
		}
		return send()
	}
	_ = ctx.RunCommand(cmd)

	if len(sentAt()) == 0 {
		t.Fatal("CTRL+BREAK was not sent to the canceled hook")
	}
	if pidErr != nil {
		t.Fatalf("PID of the exited hook was released before CTRL+BREAK: %v", pidErr)
	}
	if want := recordedCreations(t, record); !slices.Equal(seen, want) {
		t.Fatalf("PID of the exited hook pointed to processes created at %v before CTRL+BREAK, want only the hook created at %v", seen, want)
	}
}

// processCreation은 pid를 쓰는 프로세스의 생성 시각을 돌려준다. pid를 쓰는 프로세스가 없으면 오류를 돌려준다.
func processCreation(pid uint32) (time.Time, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(process)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}

// 최소 시간이 지난 뒤에 취소된 hook은 지금처럼 바로 CTRL+BREAK를 받아야 한다.
func TestRunCommandSendsBreakAtOnceAfterMinimumRunTime(t *testing.T) {
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
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

	if sent := sentAt(); len(sent) != 1 || sent[0].Sub(canceled) >= minRunBeforeBreak {
		t.Fatalf("CTRL+BREAK sent at %v after cancel %v, want once at once", sent, canceled)
	}
	assertEndedBySignal(t, cmd, time.Since(canceled), breakSignalExit)
}

// 취소된 명령 목록에서 셸이 첫 CTRL+BREAK 뒤에 시작한 프로그램도 CTRL+BREAK를 받고 끝나야 한다.
func TestRunCommandSendsBreakAgainToProgramStartedAfterBreak(t *testing.T) {
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("DW_BREAK_SIGNAL_RECORD", record)
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalShellHook(t, ctx, 2)
	done := runUntilReady(t, ctx, cmd)
	canceled := time.Now()
	ctx.Cancel()
	select {
	case <-done:
	case <-time.After(breakGrace / 2):
		t.Fatal("canceled command list did not end on CTRL+BREAK")
	}

	events := recordedEvents(t, record)
	if !slices.Equal(events, []string{"start", "signal", "start", "signal"}) {
		t.Fatalf("recorded %v, want both programs to start and get CTRL+BREAK", events)
	}
	if sent := sentAt(); len(sent) != 2 {
		t.Fatalf("CTRL+BREAK sent %d times, want once per program", len(sent))
	}
	assertEndedBySignal(t, cmd, time.Since(canceled), breakSignalExit)
}

// 자손을 막 시작한 hook이 취소되면, 그 자손도 시작 후 minRunBeforeBreak가 지난 뒤에 CTRL+BREAK를 한 번 받고 끝나야 한다.
func TestRunCommandDelaysBreakRightAfterDescendantStart(t *testing.T) {
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("DW_BREAK_SIGNAL_RECORD", record)
	t.Setenv("DW_BREAK_SIGNAL_CHILD", "1")
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	done := runUntilReady(t, ctx, cmd)
	canceled := time.Now()
	ctx.Cancel()
	select {
	case <-done:
	case <-time.After(breakGrace / 2):
		t.Fatal("canceled hook and the program it just started did not end on CTRL+BREAK")
	}

	events := recordedEvents(t, record)
	if !slices.Equal(events, []string{"start", "start", "signal", "signal"}) {
		t.Fatalf("recorded %v, want both programs to start and get CTRL+BREAK", events)
	}
	if sent := sentAt(); len(sent) != 1 {
		t.Fatalf("CTRL+BREAK sent %d times, want once", len(sent))
	}
	assertEndedBySignal(t, cmd, time.Since(canceled), breakSignalExit)
}

// 취소된 hook이 첫 CTRL+BREAK를 기다리는 동안 시작한 자손도, 시작 후 minRunBeforeBreak가 지난 뒤에 CTRL+BREAK를 받고
// 끝나야 한다.
func TestRunCommandDelaysBreakForDescendantStartedWhileWaiting(t *testing.T) {
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("DW_BREAK_SIGNAL_RECORD", record)
	t.Setenv("DW_BREAK_SIGNAL_CHILD", "1")
	t.Setenv("DW_BREAK_SIGNAL_LATE_CHILD", "1")
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalHook(t, ctx)
	stdin, startChild, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer startChild.Close()
	cmd.Stdin = stdin
	done := runUntilReady(t, ctx, cmd)
	canceled := time.Now()
	ctx.Cancel()
	// hook이 잠들었다가 자식을 시작하게 하면 VM에서는 hook이 늦게 깨어나 첫 신호 뒤에야 자식을 시작하는 일이 잦으므로,
	// 자식을 시작할 때는 테스트가 정한다.
	time.Sleep(minRunBeforeBreak / 4)
	if _, err := startChild.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(breakGrace / 2):
		t.Fatal("canceled hook and the programs it started did not end on CTRL+BREAK")
	}

	// VM에서는 프로그램이 늦게 시작해 다른 프로그램이 신호를 받은 뒤에 시작을 기록하기도 하므로 기록 순서는 보지 않는다.
	events := recordedEvents(t, record)
	slices.Sort(events)
	if !slices.Equal(events, []string{"signal", "signal", "signal", "start", "start", "start"}) {
		t.Fatalf("recorded %v, want all three programs to start and get CTRL+BREAK once", events)
	}
	// VM이 잠시 멈추면 자식이 첫 신호 뒤에 시작하기도 하므로, 신호마다 그 전에 만들어진 프로그램만 확인한다.
	created := recordedCreations(t, record)
	for _, sent := range sentAt() {
		for _, c := range created {
			if c.Before(sent) && sent.Sub(c) < minRunBeforeBreak {
				t.Fatalf("CTRL+BREAK sent %s after a program started, want at least %s", sent.Sub(c), minRunBeforeBreak)
			}
		}
	}
	assertEndedBySignal(t, cmd, time.Since(canceled), breakSignalExit)
}

// CTRL+BREAK로 끝나지 않는 명령은 breakGrace가 지나면 셸과 자손이 함께 끝나야 한다.
func TestRunCommandEndsProcessTreeAfterGrace(t *testing.T) {
	grace := breakGrace
	breakGrace = time.Second
	t.Cleanup(func() { breakGrace = grace })
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("DW_BREAK_SIGNAL_RECORD", record)
	t.Setenv("DW_BREAK_SIGNAL_IGNORE", "1")
	ctx := NewWatcherContext("", CommandMode{}).NewRun()
	cmd, sentAt := breakSignalShellHook(t, ctx, 1)
	done := runUntilReady(t, ctx, cmd)
	// 끝난 프로그램의 pid는 바로 다른 프로세스가 쓸 수 있으므로, 실행 중에 연 handle로 종료를 확인한다.
	program, err := windows.OpenProcess(windows.SYNCHRONIZE, false, recordedPID(t, record))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(program)
	canceled := time.Now()
	ctx.Cancel()
	select {
	case <-done:
	case <-time.After(breakGrace + jobKillWait):
		t.Fatal("command ignoring CTRL+BREAK was not ended after the grace period")
	}
	ran := time.Since(canceled)

	if ran < breakGrace {
		t.Fatalf("command ended %s after cancel, want it to get %s to stop", ran, breakGrace)
	}
	if sent := sentAt(); len(sent) != 1 {
		t.Fatalf("CTRL+BREAK sent %d times, want once to the program that already got it", len(sent))
	}
	// job은 프로세스가 남지 않았다고 알린 뒤 1ms쯤 지나서야 프로세스 handle이 신호 상태가 되기도 하므로 잠깐 기다린다.
	if state, err := windows.WaitForSingleObject(program, 1000); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("program started by the shell is still running: state=%d err=%v", state, err)
	}
}

// runUntilReady는 RunCommand를 실행하고, cmd가 처음 시작한 프로그램이 ready를 쓰면 RunCommand가 끝날 때 닫히는 채널을 돌려준다.
func runUntilReady(t *testing.T, ctx *WatcherContext, cmd *exec.Cmd) <-chan struct{} {
	t.Helper()
	out, in, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
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
	return done
}

// recordedEvents는 TestBreakSignalProcess가 기록한 사건을 pid 없이 기록 순서대로 돌려준다.
func recordedEvents(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		event, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		events = append(events, event)
	}
	return events
}

// recordedCreations는 TestBreakSignalProcess가 기록한 프로세스 생성 시각들을 돌려준다.
func recordedCreations(t *testing.T, record string) []time.Time {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var created []time.Time
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event string
		var pid uint32
		var nanoseconds int64
		if n, _ := fmt.Sscan(line, &event, &pid, &nanoseconds); n == 3 && event == "start" {
			created = append(created, time.Unix(0, nanoseconds))
		}
	}
	return created
}

// recordedPID는 TestBreakSignalProcess가 처음 기록한 pid를 돌려준다.
func recordedPID(t *testing.T, record string) uint32 {
	t.Helper()
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var event string
	var pid uint32
	if _, err := fmt.Sscan(string(data), &event, &pid); err != nil {
		t.Fatalf("read pid from %q: %v", data, err)
	}
	return pid
}

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }
