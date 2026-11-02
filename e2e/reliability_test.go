//go:build !windows

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
        - '%s --file=%s --delay=80ms --line=DONE:{{.Path}}'
      onFinishWatch:
        - '%s --file=%s --line=FINISH'
    option:
      noWait: %t
      interruptible: %t
      waitMillisecond: 30
      watchEvent: CU
      excludeSuffix: [.probe, .swp]
`, s.watch, hookrecBin, s.records, hookrecBin, s.records, noWait, interruptible))
				initial := filepath.Join(s.watch, "initial.txt")
				s.write(initial, []byte("initial"))
				s.await("slow hook starts", 5*time.Second, func() bool { return strings.Contains(strings.Join(readLinesLenient(s.records), "\n"), "BEGIN|") })
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
				s.await("isolated valid change completes after inflow", time.Until(deadline), func() bool {
					return countRecord(readLinesLenient(s.records), "DONE:"+last) > 0
				})
				lines := readLinesLenient(s.records)
				if noWait && !interruptible {
					for _, path := range paths {
						found := false
						for _, line := range lines {
							if line == "DONE:"+path {
								found = true
							}
						}
						if !found {
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
				lines = readLinesLenient(s.records)
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
				assertLineProcessesGone(s)
				t.Logf("load: 24 unique creates + 24 shared writes + 25 excluded writes; interval=10ms duration=%s hook=80ms debounce=30ms records=%d", elapsed, len(lines))
			})
		}
	}
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

func TestStartHookShutdownModes(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		for _, interruptible := range []bool{false, true} {
			t.Run(fmt.Sprintf("noWait=%t/interruptible=%t", noWait, interruptible), func(t *testing.T) {
				s := newScenario(t)
				s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '%s --file=%s --depth=2'
      onChange:
        - '%s --file=%s --depth=2'
    option:
      noWait: %t
      interruptible: %t
      waitMillisecond: 10
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, hookrecBin, s.records, hookrecBin, s.records, noWait, interruptible))
				s.await("startup process tree ready", 10*time.Second, func() bool {
					for _, r := range processRecords(s.records) {
						if r.depth == 2 && r.state == "ready" {
							return true
						}
					}
					return false
				})
				s.write(filepath.Join(s.watch, "during-start.txt"), nil)
				if interruptible {
					s.await("replacement tree ready", 10*time.Second, func() bool {
						count := 0
						for _, r := range processRecords(s.records) {
							if r.depth == 2 && r.state == "ready" {
								count++
							}
						}
						return count >= 2
					})
				}
				s.stop()
				records := processRecords(s.records)
				s.await("startup/replacement descendants disappear", 5*time.Second, func() bool { return recordsGone(records) })
				if !interruptible {
					for _, r := range records {
						if r.depth == 2 && r.state == "start" && r.run != records[0].run {
							t.Fatal("sequential startup was replaced")
						}
					}
				}
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
        - '%s --file=%s --delay=800ms --line=SLOW'
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
  - path: '%s'
    lifeCycle:
      onChange:
        - '%s --file=%s --line=FAST'
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, hookrecBin, s.records, other, hookrecBin, s.records))
	s.await("second target readiness", 5*time.Second, func() bool {
		s.write(filepath.Join(other, "readiness.probe"), nil)
		return strings.Contains(s.log.String(), filepath.Join(other, "readiness.probe"))
	})
	s.write(filepath.Join(s.watch, "slow.txt"), nil)
	s.await("slow target executing", 5*time.Second, func() bool { return strings.Contains(strings.Join(readLinesLenient(s.records), "\n"), "BEGIN|") })
	s.write(filepath.Join(other, "fast.txt"), nil)
	s.await("independent target completes", 500*time.Millisecond, func() bool { return strings.Contains(strings.Join(readLinesLenient(s.records), "\n"), "FAST") })
	for _, line := range readLinesLenient(s.records) {
		if line == "SLOW" {
			t.Fatal("fast target waited for slow target")
		}
	}
	s.stop()
	assertLineProcessesGone(s)
}

func TestBoundedOverloadCLI(t *testing.T) {
	s := newScenario(t)
	s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '%s --file=%s --depth=2'
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, hookrecBin, s.records))
	s.await("startup tree occupies sequential worker", 10*time.Second, func() bool {
		for _, r := range processRecords(s.records) {
			if r.depth == 2 && r.state == "ready" {
				return true
			}
		}
		return false
	})
	started := time.Now()
	for n := range 300 {
		s.write(filepath.Join(s.watch, fmt.Sprintf("load-%03d.txt", n)), nil)
	}
	s.await("bounded overload exits", 10*time.Second, func() bool {
		select {
		case <-s.done:
			return true
		default:
			return false
		}
	})
	if s.exit == nil {
		t.Fatal("queue saturation exited successfully")
	}
	log := s.log.String()
	if !strings.Contains(log, "dispatch queue full (capacity 256)") || !strings.Contains(log, s.watch) {
		t.Fatalf("queue loss target/cause missing: %s", log)
	}
	records := processRecords(s.records)
	s.await("overloaded process tree disappears", 5*time.Second, func() bool { return recordsGone(records) })
	t.Logf("actual application overload: 300 creates duration=%s pending-bound=256 active-generation=1 descendants-alive=0 exit=%v", time.Since(started), s.exit)
}
