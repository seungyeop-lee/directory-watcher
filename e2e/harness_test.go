package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var watcherBin, hookrecBin string

func TestMain(m *testing.M) { os.Exit(runTests(m)) }

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "dw-e2e-bin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	watcherBin = filepath.Join(dir, binName("directory-watcher"))
	hookrecBin = filepath.Join(dir, binName("hookrec"))
	for _, build := range []struct{ output, source string }{{watcherBin, ".."}, {hookrecBin, "./hookrec"}} {
		if out, err := exec.Command("go", "build", "-o", build.output, build.source).CombinedOutput(); err != nil {
			fmt.Fprintln(os.Stderr, "build:", err, string(out))
			return 1
		}
	}
	return m.Run()
}

func binName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

type lockedLog struct {
	sync.Mutex
	data strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) { l.Lock(); defer l.Unlock(); return l.data.Write(p) }
func (l *lockedLog) String() string              { l.Lock(); defer l.Unlock(); return l.data.String() }

type scenario struct {
	t                    *testing.T
	work, watch, records string
	cmd                  *exec.Cmd
	log                  lockedLog
	done                 chan struct{}
	exit                 error
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	work, err := os.MkdirTemp("", "dw-e2e-*")
	if err != nil {
		t.Fatal(err)
	}
	s := &scenario{t: t, work: work, watch: filepath.Join(work, "watch"), records: filepath.Join(work, "records.log")}
	t.Cleanup(s.cleanup)
	for _, p := range []string{work, watcherBin, hookrecBin} {
		if strings.ContainsAny(p, " \t\r\n") {
			t.Fatalf("hook command paths cannot contain whitespace: %q", p)
		}
	}
	if err := os.Mkdir(s.watch, 0755); err != nil {
		t.Fatal(err)
	}
	s.write(s.records, nil)
	t.Logf("run directory: %s", work)
	return s
}

func (s *scenario) write(path string, data []byte) {
	s.t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *scenario) start(config string) {
	s.t.Helper()
	path := filepath.Join(s.work, "config.yml")
	s.write(path, []byte(config))
	s.cmd = exec.Command(watcherBin, "-c", path, "--log-level=DEBUG")
	s.cmd.Dir = s.work
	setupProcGroup(s.cmd)
	s.cmd.Stdout, s.cmd.Stderr = &s.log, &s.log
	s.cmd.WaitDelay = time.Second
	if err := s.cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	s.done = make(chan struct{})
	go func() { s.exit = s.cmd.Wait(); close(s.done) }()
	s.await("real filesystem readiness", 10*time.Second, func() bool {
		select {
		case <-s.done:
			s.t.Fatalf("watcher exited before ready: %v\n%s", s.exit, s.log.String())
		default:
		}
		probe := filepath.Join(s.watch, "readiness.probe")
		s.write(probe, []byte(fmt.Sprint(time.Now().UnixNano())))
		return strings.Contains(s.log.String(), "event:") && strings.Contains(s.log.String(), "readiness.probe")
	})
}

func (s *scenario) await(label string, timeout time.Duration, condition func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timeout: %s\nrecords:\n%s\nwatcher:\n%s", label, strings.Join(readLinesLenient(s.records), "\n"), s.log.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *scenario) waitLines(count int) {
	s.t.Helper()
	s.await(fmt.Sprintf("%d hook records", count), 10*time.Second, func() bool { return len(readLinesLenient(s.records)) >= count })
}

func (s *scenario) stop() {
	s.t.Helper()
	if err := gracefulStop(s.cmd); err != nil {
		s.t.Fatal(err)
	}
	s.await("watcher exit", 10*time.Second, func() bool {
		select {
		case <-s.done:
			return true
		default:
			return false
		}
	})
	if canVerifyFinishHooks && s.exit != nil {
		s.t.Fatalf("watcher exit: %v\n%s", s.exit, s.log.String())
	}
}

func (s *scenario) cleanup() {
	if s.cmd != nil && s.cmd.Process != nil {
		select {
		case <-s.done:
		default:
			_ = gracefulStop(s.cmd)
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
				_ = forceStop(s.cmd)
				<-s.done
			}
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = os.WriteFile(filepath.Join(s.work, "watcher.pid"), []byte(fmt.Sprint(s.cmd.Process.Pid)), 0644)
	}
	if !cleanupRecordedProcesses(s.records) {
		s.t.Error("process cleanup failed; inspect cleanup.log")
	}
	if s.t.Failed() || os.Getenv("DW_E2E_KEEP") == "1" {
		_ = os.WriteFile(filepath.Join(s.work, "watcher.log"), []byte(s.log.String()), 0644)
		s.t.Logf("diagnostics retained: %s", s.work)
	} else {
		_ = os.RemoveAll(s.work)
	}
}

func readLinesLenient(path string) []string {
	data, _ := os.ReadFile(path)
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
