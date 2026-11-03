package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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
	s.launch(watcherBin, config, "--log-level=DEBUG")
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

func (s *scenario) launch(binary, config string, arguments ...string) {
	s.t.Helper()
	path := filepath.Join(s.work, "config.yml")
	s.write(path, []byte(config))
	s.cmd = exec.Command(binary, append([]string{"-c", path}, arguments...)...)
	s.cmd.Dir = s.work
	setupProcGroup(s.cmd)
	s.cmd.Stdout, s.cmd.Stderr = &s.log, &s.log
	s.cmd.WaitDelay = time.Second
	if err := s.cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	s.done = make(chan struct{})
	go func() { s.exit = s.cmd.Wait(); close(s.done) }()
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

func receivedChanges(log string, paths map[string]bool, operations fsnotify.Op) []string {
	var received []string
	for _, line := range strings.Split(log, "\n") {
		_, event, ok := strings.Cut(line, "event: ")
		if !ok {
			continue
		}
		quote := strings.IndexByte(event, '"')
		if quote < 0 {
			continue
		}
		path, err := strconv.Unquote(event[quote:])
		if err != nil || !paths[path] {
			continue
		}
		op := strings.TrimSpace(event[:quote])
		if operations.Has(fsnotify.Create) && strings.Contains(op, "CREATE") ||
			operations.Has(fsnotify.Write) && strings.Contains(op, "WRITE") {
			received = append(received, path)
		}
	}
	return received
}

func changesCompleted(received, records []string, prefix string, sequential bool) bool {
	if len(received) == 0 {
		return false
	}
	begins, completed, cancelled := 0, 0, 0
	last := ""
	for _, line := range records {
		switch {
		case strings.HasPrefix(line, "BEGIN|"):
			begins++
		case strings.HasPrefix(line, "CANCEL|"):
			cancelled++
		case strings.HasPrefix(line, prefix):
			completed++
			last = strings.TrimPrefix(line, prefix)
		}
	}
	return last == received[len(received)-1] &&
		(begins == 0 || begins == completed+cancelled) &&
		(!sequential || completed == len(received))
}

func (s *scenario) awaitChangesComplete(prefix string, paths []string, operations fsnotify.Op, sequential bool, timeout time.Duration) {
	s.t.Helper()
	allowed := make(map[string]bool, len(paths))
	for _, path := range paths {
		allowed[path] = true
	}
	quietSince := time.Now()
	previous := ""
	var received, records []string
	s.await("received inflow completes and settles", timeout, func() bool {
		log := s.log.String()
		received = receivedChanges(log, allowed, operations)
		records = readLinesLenient(s.records)
		snapshot := log + strings.Join(records, "\n")
		if snapshot != previous {
			previous = snapshot
			quietSince = time.Now()
		}
		return changesCompleted(received, records, prefix, sequential) && time.Since(quietSince) >= 120*time.Millisecond
	})
	s.t.Logf("inflow settled: received=%d records=%d last-received=%s", len(received), len(records), received[len(received)-1])
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
	if s.exit != nil {
		s.t.Fatalf("watcher exit: %v\n%s", s.exit, s.log.String())
	}
}

func (s *scenario) cleanup() {
	if err := writeProcessDiagnostics(s.records); err != nil {
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
	if !cleanupRecordedProcesses(s.records) {
		s.t.Error("process cleanup failed; inspect cleanup.log")
	}
	if s.t.Failed() || os.Getenv("DW_E2E_KEEP") == "1" {
		_ = os.WriteFile(filepath.Join(s.work, "watcher.log"), []byte(s.log.String()), 0644)
		s.t.Logf("diagnostics retained: %s", s.work)
	} else {
		if err := os.RemoveAll(s.work); err != nil {
			s.t.Errorf("remove run directory: %v", err)
		}
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

func assertLineProcessesGone(s *scenario) {
	s.t.Helper()
	var pids []int
	for _, line := range readLinesLenient(s.records) {
		fields := strings.Split(line, "|")
		if len(fields) >= 3 && fields[0] == "BEGIN" {
			pid, err := strconv.Atoi(fields[1])
			if err == nil {
				pids = append(pids, pid)
				defer releaseRecordedProcess(pid)
			}
		}
	}
	s.await("all slow hook processes disappear", 5*time.Second, func() bool {
		for _, pid := range pids {
			if processAlive(pid) {
				return false
			}
		}
		return true
	})
	s.t.Logf("slow hook process observations: started=%d alive-after-stop=0", len(pids))
}
