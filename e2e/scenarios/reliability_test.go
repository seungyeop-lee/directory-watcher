package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestFastInflowSlowHook(t *testing.T) {
	t.Parallel()
	for _, deliver := range []string{"last", "each"} {
		for _, interrupt := range []string{"none", "oldest"} {
			t.Run(fmt.Sprintf("deliver=%s/interrupt=%s", deliver, interrupt), func(t *testing.T) {
				t.Parallel()
				// each는 기다리지 않고 받은 이벤트마다 실행하게 하고, last는 기다린 뒤 마지막 경로 하나만 실행하게 한다.
				each, wait := deliver == "each", 30
				sequential := each && interrupt == "none"
				if each {
					wait = 0
				}
				s := harness.NewScenario(t)
				s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      watch:
        exclude: ['*.probe', '*.swp']
        fileEvent: CU
      hookBatch:
        waitMillisecond: %d
        deliver: %s
      run:
        interrupt: %s
`, s.Watch, s.Hook("--delay=80ms", "--line=DONE:{{.Path}}"), s.Hook("--line=FINISH"), wait, deliver, interrupt))
				initial := filepath.Join(s.Watch, "initial.txt")
				s.Write(initial, []byte("initial"))
				s.Await("slow hook starts", 5*time.Second, func() bool { return harness.DelayedHookBegun(s.Lines(), "DONE:"+initial) })
				started := time.Now()
				var paths []string
				shared := filepath.Join(s.Watch, "shared.txt")
				for n := range 24 {
					path := filepath.Join(s.Watch, fmt.Sprintf("%02d.txt", n))
					paths = append(paths, path)
					s.Write(path, []byte(fmt.Sprint(n)))
					s.Write(shared, []byte(fmt.Sprint(n)))
					s.Write(filepath.Join(s.Watch, "excluded.swp"), []byte(fmt.Sprint(n)))
					time.Sleep(10 * time.Millisecond)
				}
				elapsed := time.Since(started)
				deadline := time.Now().Add(30 * time.Second)
				inflowPaths := append([]string{initial, shared}, paths...)
				s.AwaitChangesComplete("DONE:", inflowPaths, fsnotify.Create|fsnotify.Write, sequential, time.Until(deadline))
				last := filepath.Join(s.Watch, "last.txt")
				s.Write(last, []byte("last"))
				s.Write(filepath.Join(s.Watch, "excluded.swp"), []byte("after isolated valid event"))
				s.AwaitLine("isolated valid change completes after inflow", time.Until(deadline), "DONE:"+last)
				lines := s.Lines()
				if sequential {
					for _, path := range paths {
						if harness.CountRecord(lines, "DONE:"+path) == 0 {
							t.Errorf("sequential accepted change not completed: %s", path)
						}
					}
				}
				for _, line := range lines {
					if strings.Contains(line, "excluded.swp") || strings.Contains(line, "readiness.probe") {
						t.Errorf("excluded hook: %s", line)
					}
				}
				// 이벤트가 더 들어오고 느린 명령이 실행되는 동안 정지한다.
				s.Write(filepath.Join(s.Watch, "shutdown.txt"), []byte("stop"))
				s.Await("shutdown event received", 5*time.Second, func() bool { return strings.Contains(s.Log(), "shutdown.txt") })
				s.Stop()
				lines = s.Lines()
				finish := -1
				for n, line := range lines {
					if line == "FINISH" {
						finish = n
					}
				}
				if finish < 0 {
					t.Fatal("finish hook missing")
				}
				for _, line := range lines[finish+1:] {
					if strings.HasPrefix(line, "BEGIN|") {
						t.Fatalf("hook after finish: %s", line)
					}
				}
				s.AwaitDelayedHooksGone()
				t.Logf("load: 24 unique creates + 24 shared writes + 25 excluded writes; interval=10ms duration=%s hook=80ms wait=%dms records=%d", elapsed, wait, len(lines))
			})
		}
	}
}

// TestTargetsRunIndependently는 빠른 대상의 hook이 500ms 안에 끝나기를 요구한다. 이 한도는 다른 시나리오와 CPU를
// 나눠 쓰면 넘을 수 있으므로 병렬로 실행하지 않는다.
func TestTargetsRunIndependently(t *testing.T) {
	s := harness.NewScenario(t)
	other := filepath.Join(s.Work, "other")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
`, s.Watch, s.Hook("--delay=800ms", "--line=SLOW"), other, s.Hook("--line=FAST")))
	otherProbe := filepath.Join(other, "readiness.probe")
	s.Await("second target readiness", 5*time.Second, func() bool {
		s.Write(otherProbe, nil)
		return strings.Contains(s.Log(), strconv.Quote(otherProbe))
	})
	s.Write(filepath.Join(s.Watch, "slow.txt"), nil)
	s.Await("slow target executing", 5*time.Second, func() bool { return harness.DelayedHookBegun(s.Lines(), "SLOW") })
	s.Write(filepath.Join(other, "fast.txt"), nil)
	s.AwaitLine("independent target completes", 500*time.Millisecond, "FAST")
	if harness.CountRecord(s.Lines(), "SLOW") != 0 {
		t.Fatal("fast target waited for slow target")
	}
	s.Stop()
	s.AwaitDelayedHooksGone()
}

func TestBoundedOverloadCLI(t *testing.T) {
	t.Parallel()
	s := harness.NewScenario(t)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
      queue:
        size: 64
`, s.Watch, s.Hook("--depth=2")))
	s.AwaitReadyGenerations("startup tree occupies sequential worker", 1)
	started := time.Now()
	for n := range 300 {
		s.Write(filepath.Join(s.Watch, fmt.Sprintf("load-%03d.txt", n)), nil)
	}
	s.Await("bounded overload exits", 10*time.Second, s.Exited)
	if s.ExitErr() == nil {
		t.Fatal("queue saturation exited successfully")
	}
	log := s.Log()
	if !strings.Contains(log, "dispatch queue full (capacity 64)") || !strings.Contains(log, strconv.Quote(s.Watch)) {
		t.Fatalf("queue loss target/cause missing: %s", log)
	}
	records := s.ProcessRecords()
	s.AwaitProcessesGone("overloaded process tree disappears", records)
	t.Logf("actual application overload: 300 creates duration=%s pending-bound=64 active-generation=1 descendants-alive=0 exit=%v", time.Since(started), s.ExitErr())
}
