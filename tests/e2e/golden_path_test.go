// Package e2e validates the directory-watcher main flow end-to-end by spawning
// the actual binary and observing its hook side effects against a log file.
//
// Scope (trilha 02 of the testing roadmap):
//   - C/U/D event filtering (watchEvent: CDU)
//   - 7 template variables interpolation
//   - 5-stage lifecycle hook order (global + target layers)
//   - YAML config parsing through the real entry point
//
// Windows limitation: onFinishWatch hooks are not verified because the
// production code only notifies SIGINT/SIGTERM, which cannot be delivered to a
// separate process group on Windows. See signal_windows.go for detail.
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	watcherBin string
	hookrecBin string
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	binDir, err := os.MkdirTemp("", "dw-e2e-bin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdir temp:", err)
		return 1
	}
	defer os.RemoveAll(binDir)

	watcherBin = filepath.Join(binDir, binName("directory-watcher"))
	hookrecBin = filepath.Join(binDir, binName("hookrec"))

	if out, err := exec.Command("go", "build", "-o", watcherBin, "../..").CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "build watcher:", err, "\n"+string(out))
		return 1
	}
	if out, err := exec.Command("go", "build", "-o", hookrecBin, "./hookrec").CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "build hookrec:", err, "\n"+string(out))
		return 1
	}

	return m.Run()
}

func binName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func TestGoldenPath(t *testing.T) {
	work := t.TempDir()

	// The watcher splits hook commands on spaces (no shell). Any path with a
	// whitespace character would fragment the command and break the test. Fail
	// early with a clear message rather than producing cryptic exec errors.
	for _, p := range []string{work, watcherBin, hookrecBin} {
		if strings.ContainsAny(p, " \t") {
			t.Fatalf("test path contains whitespace, incompatible with command runner: %q", p)
		}
	}

	watchDir := filepath.Join(work, "watch")
	logFile := filepath.Join(work, "log.txt")
	configFile := filepath.Join(work, "config.yml")

	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatalf("mkdir watch: %v", err)
	}
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatalf("init log: %v", err)
	}
	if err := os.WriteFile(configFile, []byte(buildConfig(hookrecBin, logFile, watchDir)), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, watcherBin, "-c", configFile)
	cmd.Dir = work
	setupProcGroup(cmd)
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	cmd.Stdout = stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start watcher: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	const lineWaitTimeout = 10 * time.Second
	const debounceSettle = 400 * time.Millisecond

	// Startup: 2 lines (global + target onStartWatch)
	waitForLines(t, logFile, 2, lineWaitTimeout, stderr)

	target := filepath.Join(watchDir, "foo.txt")

	// CREATE: open with O_RDONLY so inotify does not emit IN_CLOSE_WRITE
	// (which fsnotify maps to Write and would make the debounce flush as U).
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	_ = f.Close()
	waitForLines(t, logFile, 5, lineWaitTimeout, stderr)
	time.Sleep(debounceSettle)

	// UPDATE
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("update target: %v", err)
	}
	waitForLines(t, logFile, 8, lineWaitTimeout, stderr)
	time.Sleep(debounceSettle)

	// DELETE
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}
	waitForLines(t, logFile, 11, lineWaitTimeout, stderr)
	time.Sleep(debounceSettle)

	expected := expectedSequence(target)

	if canVerifyFinishHooks {
		if err := gracefulStop(cmd); err != nil {
			t.Fatalf("graceful stop: %v", err)
		}
		expected = append(expected, "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
		waitForLines(t, logFile, len(expected), lineWaitTimeout, stderr)
	} else {
		_ = gracefulStop(cmd)
	}

	waitForExit(t, cmd, 10*time.Second)

	actual := readLines(t, logFile)
	if !equalLines(expected, actual) {
		t.Fatalf("log mismatch\nexpected (%d lines):\n%s\nactual (%d lines):\n%s\nstderr:\n%s",
			len(expected), strings.Join(expected, "\n"),
			len(actual), strings.Join(actual, "\n"),
			stderr.String())
	}
}

func buildConfig(hookrec, logFile, watchDir string) string {
	changeTmpl := "{{.Path}}|{{.AbsPath}}|{{.FileName}}|{{.ExtName}}|{{.DirPath}}|{{.DirAbsPath}}|{{.Event}}"
	cmdLine := func(label, suffix string) string {
		line := label
		if suffix != "" {
			line = label + ":" + suffix
		}
		return fmt.Sprintf("'%s --file=%s --line=%s'", hookrec, logFile, line)
	}

	return fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
      - %s
    onBeforeChange:
      - %s
    onAfterChange:
      - %s
    onFinishWatch:
      - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      waitMillisecond: 100
      watchEvent: CDU
`,
		cmdLine("GLOBAL:onStartWatch", ""),
		cmdLine("GLOBAL:onBeforeChange", changeTmpl),
		cmdLine("GLOBAL:onAfterChange", changeTmpl),
		cmdLine("GLOBAL:onFinishWatch", ""),
		watchDir,
		cmdLine("TARGET:onStartWatch", ""),
		cmdLine("TARGET:onChange", changeTmpl),
		cmdLine("TARGET:onFinishWatch", ""),
	)
}

func expectedSequence(target string) []string {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		panic(err)
	}
	fileName := filepath.Base(absTarget)
	ext := filepath.Ext(absTarget)
	dir := filepath.Dir(absTarget)

	interp := func(op string) string {
		return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", absTarget, absTarget, fileName, ext, dir, dir, op)
	}

	seq := []string{
		"GLOBAL:onStartWatch",
		"TARGET:onStartWatch",
	}
	for _, op := range []string{"C", "U", "D"} {
		seq = append(seq,
			"GLOBAL:onBeforeChange:"+interp(op),
			"TARGET:onChange:"+interp(op),
			"GLOBAL:onAfterChange:"+interp(op),
		)
	}
	return seq
}

func waitForLines(t *testing.T, file string, want int, timeout time.Duration, stderr fmt.Stringer) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		lines := readLinesLenient(file)
		if len(lines) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %d lines, got %d\nlog:\n%s\nstderr:\n%s",
				want, len(lines), strings.Join(lines, "\n"), stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func readLinesLenient(file string) []string {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	s := strings.TrimRight(string(data), "\r\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func readLines(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := strings.TrimRight(string(data), "\r\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func waitForExit(t *testing.T, cmd *exec.Cmd, timeout time.Duration) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("watcher did not exit within %s", timeout)
	}
}
