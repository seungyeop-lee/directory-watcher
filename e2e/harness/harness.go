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

// Run은 테스트 패키지 디렉터리 기준 상대 경로 대신 이 import 경로로 빌드하므로,
// 모듈 안의 어느 테스트 패키지에서든 Run을 호출할 수 있다.
const (
	watcherPackage = "github.com/seungyeop-lee/directory-watcher/v3"
	hookrecPackage = watcherPackage + "/e2e/hookrec"
	// injectTag는 감시 대상 폴더의 표지 파일로 watcher 오류 채널에 오류를 보내는 주입 지점을 넣는 build tag다.
	// 주입 지점은 internal/app/service/runner_watch_target_tree_inject.go에 있다.
	injectTag = "dw_e2e_inject"
)

var watcherBin, injectWatcherBin, hookrecBin string

// WatcherBin은 Run이 빌드한 directory-watcher 바이너리를 돌려준다.
func WatcherBin() string { return watcherBin }

// InjectWatcherBin은 Run이 주입 build tag를 붙여 빌드한 directory-watcher 바이너리를 돌려준다.
func InjectWatcherBin() string { return injectWatcherBin }

// HookrecBin은 Run이 빌드한 hookrec 바이너리를 돌려준다.
func HookrecBin() string { return hookrecBin }

// Run은 TestMain의 본문이다: 콘솔을 준비하고, 테스트 바이너리 실행 전체에서 쓸
// watcher, 주입 tag를 붙인 watcher, hookrec 바이너리를 빌드한 뒤, m을 실행하고 바이너리를 지운다. 바이너리를
// 지우지 못하면 돌려주는 코드가 1이 된다.
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
	injectWatcherBin = filepath.Join(dir, binName("directory-watcher-inject"))
	hookrecBin = filepath.Join(dir, binName("hookrec"))
	for _, build := range []struct{ output, source, tags string }{
		{watcherBin, watcherPackage, ""},
		{injectWatcherBin, watcherPackage, injectTag},
		{hookrecBin, hookrecPackage, ""},
	} {
		if out, err := exec.Command("go", "build", "-tags="+build.tags, "-o", build.output, build.source).CombinedOutput(); err != nil {
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

// Scenario는 실행 디렉터리 하나와, 그 시나리오의 watcher 또는 도우미 프로세스, 그리고
// 시나리오 기록에 대해 한 프로세스 관찰을 소유한다. 빌드한 바이너리와 콘솔은
// 시나리오가 아니라 테스트 바이너리 실행 전체에 속한다.
type Scenario struct {
	t                    *testing.T
	Work, Watch, Records string
	cmd                  *exec.Cmd
	log                  lockedLog
	done                 chan struct{}
	exit                 error
	procs                *processObserver
	started              time.Time
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
	s := &Scenario{t: t, Work: work, Watch: filepath.Join(work, "watch"), Records: filepath.Join(work, "records.log"), procs: newProcessObserver(), started: time.Now()}
	t.Cleanup(s.cleanup)
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

// StartProcess는 cmd를 이 시나리오의 프로세스로서 별도 프로세스 그룹에서 실행해,
// 정지와 정리, 진단이 그 프로세스에 적용되게 한다.
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

// Log는 시나리오 프로세스가 지금까지 낸 stdout과 stderr를 합쳐 돌려준다.
func (s *Scenario) Log() string { return s.log.String() }

// Done은 시나리오 프로세스가 끝나면 닫힌다.
func (s *Scenario) Done() <-chan struct{} { return s.done }

// ExitErr는 프로세스의 Wait 결과를 돌려준다. Done이 닫힌 뒤에만 읽는다.
func (s *Scenario) ExitErr() error { return s.exit }

// Alive는 이 시나리오가 기록한 PID가 아직 실행 중인지 알려 준다.
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
	s.StopWithin(10 * time.Second)
}

// StopWithin은 Stop과 같지만 watcher가 끝나기를 timeout만큼 기다린다.
func (s *Scenario) StopWithin(timeout time.Duration) {
	s.t.Helper()
	if err := gracefulStop(s.cmd); err != nil {
		s.t.Fatal(err)
	}
	s.Await("watcher exit", timeout, s.Exited)
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
	if report := s.dllInitFailureReport(); report != "" {
		_ = os.WriteFile(filepath.Join(s.Work, "dll-init-failed.log"), []byte(report), 0644)
		s.t.Log(strings.TrimSuffix(report, "\n"))
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
