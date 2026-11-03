package harness

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

// Run builds these import paths instead of paths relative to the test
// package directory, so any test package in the module can call it.
const (
	watcherPackage = "github.com/seungyeop-lee/directory-watcher/v2"
	hookrecPackage = watcherPackage + "/e2e/hookrec"
)

var watcherBin, hookrecBin string

// WatcherBin returns the directory-watcher binary built by Run.
func WatcherBin() string { return watcherBin }

// HookrecBin returns the hookrec binary built by Run.
func HookrecBin() string { return hookrecBin }

// Run is the body of a TestMain: it prepares the console and builds the
// watcher and hookrec binaries for the whole test binary run, runs m, and
// removes the binaries. A removal failure makes the returned code 1.
func Run(m *testing.M) (code int) {
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
	for _, build := range []struct{ output, source string }{{watcherBin, watcherPackage}, {hookrecBin, hookrecPackage}} {
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

// Scenario owns one run directory, its watcher or helper process, and the
// process observations made for its records. Built binaries and the console
// belong to the whole test binary run instead.
type Scenario struct {
	t                    *testing.T
	Work, Watch, Records string
	cmd                  *exec.Cmd
	log                  lockedLog
	done                 chan struct{}
	exit                 error
	procs                *processObserver
}

func NewScenario(t *testing.T) *Scenario {
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
	s := &Scenario{t: t, Work: work, Watch: filepath.Join(work, "watch"), Records: filepath.Join(work, "records.log"), procs: newProcessObserver()}
	t.Cleanup(s.cleanup)
	for _, p := range []string{work, watcherBin, hookrecBin} {
		if strings.ContainsAny(p, " \t\r\n") {
			t.Fatalf("hook command paths cannot contain whitespace: %q", p)
		}
	}
	if err := os.Mkdir(s.Watch, 0755); err != nil {
		t.Fatal(err)
	}
	s.Write(s.Records, nil)
	t.Logf("run directory: %s", work)
	return s
}

func (s *Scenario) Write(path string, data []byte) {
	s.t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *Scenario) Start(config string) {
	s.t.Helper()
	s.Launch(watcherBin, config, "--log-level=DEBUG")
	s.Await("real filesystem readiness", 10*time.Second, func() bool {
		if s.Exited() {
			s.t.Fatalf("watcher exited before ready: %v\n%s", s.exit, s.log.String())
		}
		probe := filepath.Join(s.Watch, "readiness.probe")
		s.Write(probe, []byte(fmt.Sprint(time.Now().UnixNano())))
		return strings.Contains(s.log.String(), "event:") && strings.Contains(s.log.String(), "readiness.probe")
	})
}

func (s *Scenario) Launch(binary, config string, arguments ...string) {
	s.t.Helper()
	path := filepath.Join(s.Work, "config.yml")
	s.Write(path, []byte(config))
	cmd := exec.Command(binary, append([]string{"-c", path}, arguments...)...)
	cmd.Dir = s.Work
	s.StartProcess(cmd)
}

// StartProcess runs cmd as this scenario's process in its own process group,
// so stop, cleanup and diagnostics apply to it.
func (s *Scenario) StartProcess(cmd *exec.Cmd) {
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

// Log returns the combined stdout and stderr of the scenario's process so far.
func (s *Scenario) Log() string { return s.log.String() }

// Done is closed when the scenario's process has exited.
func (s *Scenario) Done() <-chan struct{} { return s.done }

// ExitErr returns the process's Wait result; read it only after Done is closed.
func (s *Scenario) ExitErr() error { return s.exit }

// Alive reports whether a PID recorded by this scenario is still running.
func (s *Scenario) Alive(pid int) bool { return s.procs.alive(pid) }

func (s *Scenario) Exited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *Scenario) Await(label string, timeout time.Duration, condition func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timeout: %s\nrecords:\n%s\nwatcher:\n%s", label, strings.Join(s.Lines(), "\n"), s.log.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (s *Scenario) Stop() {
	s.t.Helper()
	if err := gracefulStop(s.cmd); err != nil {
		s.t.Fatal(err)
	}
	s.Await("watcher exit", 10*time.Second, s.Exited)
	if s.exit != nil {
		s.t.Fatalf("watcher exit: %v\n%s", s.exit, s.log.String())
	}
}

func (s *Scenario) cleanup() {
	if err := s.procs.writeDiagnostics(s.Records); err != nil {
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
		_ = os.WriteFile(filepath.Join(s.Work, "watcher.pid"), []byte(fmt.Sprint(s.cmd.Process.Pid)), 0644)
	}
	if !s.rescueRecordedProcesses() {
		s.t.Error("process cleanup failed; inspect cleanup.log")
	}
	s.procs.close()
	if s.t.Failed() || os.Getenv("DW_E2E_KEEP") == "1" {
		_ = os.WriteFile(filepath.Join(s.Work, "watcher.log"), []byte(s.log.String()), 0644)
		s.t.Logf("diagnostics retained: %s", s.Work)
	} else {
		if err := os.RemoveAll(s.Work); err != nil {
			s.t.Errorf("remove run directory: %v", err)
		}
	}
}
