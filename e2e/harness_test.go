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

func runTests(m *testing.M) (code int) {
	if err := prepareConsole(); err != nil {
		fmt.Fprintln(os.Stderr, "console:", err)
		return 1
	}
	dir, err := os.MkdirTemp("", "dw-e2e-bin-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "remove test binaries:", err)
			code = 1
		}
	}()
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

// scenario owns one run directory, its watcher or helper process, and the
// process observations made for its records. Built binaries and the console
// belong to the whole test binary run instead.
type scenario struct {
	t                    *testing.T
	work, watch, records string
	cmd                  *exec.Cmd
	log                  lockedLog
	done                 chan struct{}
	exit                 error
	procs                *processObserver
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	base := os.Getenv("DW_E2E_DIAGNOSTICS")
	if base != "" {
		if err := os.MkdirAll(base, 0755); err != nil {
			t.Fatal(err)
		}
	}
	work, err := os.MkdirTemp(base, "dw-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	s := &scenario{t: t, work: work, watch: filepath.Join(work, "watch"), records: filepath.Join(work, "records.log"), procs: newProcessObserver()}
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
	s.launch(watcherBin, config, "--log-level=DEBUG")
	s.await("real filesystem readiness", 10*time.Second, func() bool {
		if s.exited() {
			s.t.Fatalf("watcher exited before ready: %v\n%s", s.exit, s.log.String())
		}
		probe := filepath.Join(s.watch, "readiness.probe")
		s.write(probe, []byte(fmt.Sprint(time.Now().UnixNano())))
		return strings.Contains(s.log.String(), "event:") && strings.Contains(s.log.String(), "readiness.probe")
	})
}

func (s *scenario) launch(binary, config string, arguments ...string) {
	s.t.Helper()
	path := filepath.Join(s.work, "config.yml")
	s.write(path, []byte(config))
	cmd := exec.Command(binary, append([]string{"-c", path}, arguments...)...)
	cmd.Dir = s.work
	s.startProcess(cmd)
}

// startProcess runs cmd as this scenario's process in its own process group,
// so stop, cleanup and diagnostics apply to it.
func (s *scenario) startProcess(cmd *exec.Cmd) {
	s.t.Helper()
	s.cmd = cmd
	setupProcGroup(s.cmd)
	s.cmd.Stdout, s.cmd.Stderr = &s.log, &s.log
	s.cmd.WaitDelay = time.Second
	if err := s.cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	s.done = make(chan struct{})
	go func() { s.exit = s.cmd.Wait(); close(s.done) }()
}

func (s *scenario) exited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *scenario) await(label string, timeout time.Duration, condition func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timeout: %s\nrecords:\n%s\nwatcher:\n%s", label, strings.Join(s.lines(), "\n"), s.log.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *scenario) stop() {
	s.t.Helper()
	if err := gracefulStop(s.cmd); err != nil {
		s.t.Fatal(err)
	}
	s.await("watcher exit", 10*time.Second, s.exited)
	if s.exit != nil {
		s.t.Fatalf("watcher exit: %v\n%s", s.exit, s.log.String())
	}
}

func (s *scenario) cleanup() {
	if err := s.procs.writeDiagnostics(s.records); err != nil {
		s.t.Errorf("process diagnostics: %v", err)
	}
	if s.cmd != nil && s.cmd.Process != nil {
		select {
		case <-s.done:
		default:
			_ = gracefulStop(s.cmd)
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
				if err := forceStop(s.cmd); err != nil {
					s.t.Errorf("force-stop watcher: %v", err)
				}
				select {
				case <-s.done:
				case <-time.After(3 * time.Second):
					s.t.Error("watcher remains after forced cleanup")
				}
			}
		}
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = os.WriteFile(filepath.Join(s.work, "watcher.pid"), []byte(fmt.Sprint(s.cmd.Process.Pid)), 0644)
	}
	if !s.rescueRecordedProcesses() {
		s.t.Error("process cleanup failed; inspect cleanup.log")
	}
	s.procs.close()
	if s.t.Failed() || os.Getenv("DW_E2E_KEEP") == "1" {
		_ = os.WriteFile(filepath.Join(s.work, "watcher.log"), []byte(s.log.String()), 0644)
		s.t.Logf("diagnostics retained: %s", s.work)
	} else {
		if err := os.RemoveAll(s.work); err != nil {
			s.t.Errorf("remove run directory: %v", err)
		}
	}
}
