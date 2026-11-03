package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestFastInflowSlowHook(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		for _, interruptible := range []bool{false, true} {
			t.Run(fmt.Sprintf("noWait=%t/interruptible=%t", noWait, interruptible), func(t *testing.T) {
				s := newScenario(t)
				s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      noWait: %t
      interruptible: %t
      waitMillisecond: 30
      watchEvent: CU
      excludeSuffix: [.probe, .swp]
`, s.watch, s.hook("--delay=80ms", "--line=DONE:{{.Path}}"), s.hook("--line=FINISH"), noWait, interruptible))
				initial := filepath.Join(s.watch, "initial.txt")
				s.write(initial, []byte("initial"))
				s.await("slow hook starts", 5*time.Second, func() bool { return delayedHookBegun(s.lines(), "DONE:"+initial) })
				started := time.Now()
				var paths []string
				shared := filepath.Join(s.watch, "shared.txt")
				for n := range 24 {
					path := filepath.Join(s.watch, fmt.Sprintf("%02d.txt", n))
					paths = append(paths, path)
					s.write(path, []byte(fmt.Sprint(n)))
					s.write(shared, []byte(fmt.Sprint(n)))
					s.write(filepath.Join(s.watch, "excluded.swp"), []byte(fmt.Sprint(n)))
					time.Sleep(10 * time.Millisecond)
				}
				elapsed := time.Since(started)
				deadline := time.Now().Add(30 * time.Second)
				inflowPaths := append([]string{initial, shared}, paths...)
				s.awaitChangesComplete("DONE:", inflowPaths, fsnotify.Create|fsnotify.Write, noWait && !interruptible, time.Until(deadline))
				last := filepath.Join(s.watch, "last.txt")
				s.write(last, []byte("last"))
				s.write(filepath.Join(s.watch, "excluded.swp"), []byte("after isolated valid event"))
				s.awaitLine("isolated valid change completes after inflow", time.Until(deadline), "DONE:"+last)
				lines := s.lines()
				if noWait && !interruptible {
					for _, path := range paths {
						if countRecord(lines, "DONE:"+path) == 0 {
							t.Errorf("sequential accepted change not completed: %s", path)
						}
					}
				}
				for _, line := range lines {
					if strings.Contains(line, "excluded.swp") || strings.Contains(line, "readiness.probe") {
						t.Errorf("excluded hook: %s", line)
					}
				}
				// Stop while more events arrive and a slow command is running.
				s.write(filepath.Join(s.watch, "shutdown.txt"), []byte("stop"))
				s.await("shutdown event received", 5*time.Second, func() bool { return strings.Contains(s.log.String(), "shutdown.txt") })
				s.stop()
				lines = s.lines()
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
				s.awaitDelayedHooksGone()
				t.Logf("load: 24 unique creates + 24 shared writes + 25 excluded writes; interval=10ms duration=%s hook=80ms debounce=30ms records=%d", elapsed, len(lines))
			})
		}
	}
}

func TestTargetsRunIndependently(t *testing.T) {
	s := newScenario(t)
	other := filepath.Join(s.work, "other")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, s.hook("--delay=800ms", "--line=SLOW"), other, s.hook("--line=FAST")))
	otherProbe := filepath.Join(other, "readiness.probe")
	s.await("second target readiness", 5*time.Second, func() bool {
		s.write(otherProbe, nil)
		return strings.Contains(s.log.String(), strconv.Quote(otherProbe))
	})
	s.write(filepath.Join(s.watch, "slow.txt"), nil)
	s.await("slow target executing", 5*time.Second, func() bool { return delayedHookBegun(s.lines(), "SLOW") })
	s.write(filepath.Join(other, "fast.txt"), nil)
	s.awaitLine("independent target completes", 500*time.Millisecond, "FAST")
	if countRecord(s.lines(), "SLOW") != 0 {
		t.Fatal("fast target waited for slow target")
	}
	s.stop()
	s.awaitDelayedHooksGone()
}

func TestBoundedOverloadCLI(t *testing.T) {
	s := newScenario(t)
	s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, s.hook("--depth=2")))
	s.awaitReadyGenerations("startup tree occupies sequential worker", 1)
	started := time.Now()
	for n := range 300 {
		s.write(filepath.Join(s.watch, fmt.Sprintf("load-%03d.txt", n)), nil)
	}
	s.await("bounded overload exits", 10*time.Second, s.exited)
	if s.exit == nil {
		t.Fatal("queue saturation exited successfully")
	}
	log := s.log.String()
	if !strings.Contains(log, "dispatch queue full (capacity 256)") || !strings.Contains(log, strconv.Quote(s.watch)) {
		t.Fatalf("queue loss target/cause missing: %s", log)
	}
	records := s.processRecords()
	s.awaitProcessesGone("overloaded process tree disappears", records)
	t.Logf("actual application overload: 300 creates duration=%s pending-bound=256 active-generation=1 descendants-alive=0 exit=%v", time.Since(started), s.exit)
}
