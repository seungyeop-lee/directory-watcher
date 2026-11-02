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
	s.start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
    option:
      interruptible: true
      waitMillisecond: 100
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, command, command))
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
	assertCompletedBeforeRestart(t, all, first[0].run)
	s.await("previous tree disappears", 5*time.Second, func() bool { return recordsGone(first) })
	s.stop()
	all = processRecords(s.records)
	for _, ready := range all {
		if ready.state != "ready" {
			continue
		}
		signal, complete := false, false
		for _, r := range all {
			if r.pid == ready.pid {
				signal = signal || r.state == "signal"
				complete = complete || r.state == "complete"
			}
		}
		if !signal || !complete {
			t.Errorf("process %d missing signal or graceful completion: %v", ready.pid, all)
		}
	}
	s.await("all process generations disappear", 5*time.Second, func() bool { return recordsGone(all) })
}

func assertCompletedBeforeRestart(t *testing.T, records []processRecord, oldRun string) {
	t.Helper()
	complete := map[int]bool{}
	for _, r := range records {
		if r.run == oldRun && r.state == "complete" {
			complete[r.depth] = true
		}
		if r.run != oldRun && r.state == "ready" {
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
	return err == nil && recordsGone(records)
}
