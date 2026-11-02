package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type processRecord struct {
	run        string
	pid, depth int
	state      string
}

func processRecords(file string) []processRecord {
	var records []processRecord
	for _, line := range readLinesLenient(file) {
		fields := strings.Split(line, "|")
		if len(fields) != 4 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[1])
		depth, e2 := strconv.Atoi(fields[2])
		if e1 == nil && e2 == nil {
			records = append(records, processRecord{fields[0], pid, depth, fields[3]})
		}
	}
	return records
}

func TestProcessLifecycle(t *testing.T) {
	s := newScenario(t)
	command := fmt.Sprintf("'%s --file=%s --depth=2'", hookrecBin, s.records)
	s.start(fmt.Sprintf(`global:
  lifeCycle:
    onFinishWatch:
      - '%s --file=%s --line=GLOBAL:onFinishWatch'
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - '%s --file=%s --line=TARGET:onFinishWatch'
    option:
      interruptible: true
      waitMillisecond: 100
      watchEvent: C
      excludeSuffix: [.probe]
`, hookrecBin, s.records, s.watch, command, command, hookrecBin, s.records))
	waitRun := func(count int) []processRecord {
		s.await(fmt.Sprintf("%d ready process trees", count), 10*time.Second, func() bool {
			n := 0
			for _, r := range processRecords(s.records) {
				if r.state == "ready" && r.depth == 2 {
					n++
				}
			}
			return n >= count
		})
		return processRecords(s.records)
	}
	first := waitRun(1)
	readyCount := 0
	for _, r := range first {
		if r.state == "ready" {
			readyCount++
		}
	}
	if readyCount != 3 {
		t.Fatalf("expected real parent, child and grandchild readiness: %v", first)
	}
	for _, r := range first {
		if r.state == "start" && !processAlive(r.pid) {
			t.Fatalf("process was not alive before restart: %+v", r)
		}
	}
	if fault := os.Getenv("DW_E2E_FAULT"); fault != "" {
		if fault == "timeout" {
			s.await("intentional timeout", 100*time.Millisecond, func() bool { return false })
		}
		t.Fatal("intentional failure after process tree readiness")
	}
	target := filepath.Join(s.watch, "restart.txt")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	all := waitRun(2)
	for _, r := range all {
		if r.run != first[0].run && r.state == "start" && !processAlive(r.pid) {
			t.Fatalf("replacement process was not alive before shutdown: %+v", r)
		}
	}
	assertCompletedBeforeRestart(t, all, first[0].run)
	s.await("previous tree disappears", 5*time.Second, func() bool { return recordsGone(first) })
	s.stop()
	all = processRecords(s.records)
	assertGracefulCompletion(t, all)
	lines := readLinesLenient(s.records)
	if len(lines) < 2 || lines[len(lines)-2] != "TARGET:onFinishWatch" || lines[len(lines)-1] != "GLOBAL:onFinishWatch" {
		t.Fatalf("finish hooks must follow current process completion: %v", lines)
	}
	s.await("all process generations disappear", 5*time.Second, func() bool { return recordsGone(all) })
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
				for _, r := range processRecords(s.records) {
					if r.state == "start" && !processAlive(r.pid) {
						t.Fatalf("startup process not alive: %+v", r)
					}
				}
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
				beforeStop := processRecords(s.records)
				currentRun := beforeStop[len(beforeStop)-1].run
				for _, r := range beforeStop {
					if r.run == currentRun && r.state == "start" && !processAlive(r.pid) {
						t.Fatalf("current process not alive before stop: %+v", r)
					}
				}
				s.stop()
				records := processRecords(s.records)
				assertGracefulCompletion(t, records)
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

func assertGracefulCompletion(t *testing.T, records []processRecord) {
	t.Helper()
	for _, ready := range records {
		if ready.state != "ready" {
			continue
		}
		signaled, completed := false, false
		for _, r := range records {
			if r.pid == ready.pid && r.run == ready.run {
				if r.state == "signal" {
					signaled = true
				}
				if r.state == "complete" {
					if !signaled {
						t.Fatalf("process completed before signal: %+v", r)
					}
					completed = true
				}
			}
		}
		if !signaled || !completed {
			t.Errorf("process %d missing signal or graceful completion: %v", ready.pid, records)
		}
	}
}

func assertCompletedBeforeRestart(t *testing.T, records []processRecord, oldRun string) {
	t.Helper()
	complete := map[int]bool{}
	for _, r := range records {
		if r.run == oldRun && r.state == "complete" {
			complete[r.depth] = true
		}
		if r.run != oldRun && r.state == "start" {
			if len(complete) != 3 {
				t.Fatalf("new run started before parent/child/grandchild completion: %v", records)
			}
			return
		}
	}
	t.Fatal("replacement run not recorded")
}

func recordsGone(records []processRecord) bool {
	for _, r := range records {
		if r.state == "start" && processAlive(r.pid) {
			return false
		}
	}
	return true
}

func cleanupRecordedProcesses(file string) bool {
	groups := map[int]bool{}
	records := processRecords(file)
	for _, r := range records {
		if pid, err := strconv.Atoi(r.run); err == nil {
			groups[pid] = true
		}
	}
	for pid := range groups {
		killRecordedGroup(pid)
	}
	for _, r := range records {
		if r.state == "start" {
			killRecordedProcess(r.pid)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for !recordsGone(records) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	var diagnostics strings.Builder
	for _, r := range records {
		if r.state == "start" {
			fmt.Fprintf(&diagnostics, "pid=%d alive=%t\n", r.pid, processAlive(r.pid))
		}
	}
	err := os.WriteFile(filepath.Join(filepath.Dir(file), "cleanup.log"), []byte(diagnostics.String()), 0644)
	gone := recordsGone(records)
	for _, r := range records {
		if r.state == "start" {
			releaseRecordedProcess(r.pid)
		}
	}
	return err == nil && gone
}
