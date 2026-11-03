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

// hookrec --depth=N starts a real parent, child and grandchild chain. Each
// process records generation|PID|depth|state, where the generation is the
// root parent's PID and the states are start, ready, signal, complete and
// parent-exit. Liveness is answered by the scenario's processObserver.

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

func (s *scenario) processRecords() []processRecord { return processRecords(s.records) }

// generationReady reports a root parent's ready record. hookrec records a
// parent ready only after its child recorded ready, so the whole tree is ready.
func generationReady(r processRecord) bool {
	return r.state == "ready" && r.run == strconv.Itoa(r.pid)
}

// awaitReadyGenerations waits until at least count process tree generations
// are ready and returns the records seen at that point.
func (s *scenario) awaitReadyGenerations(label string, count int) []processRecord {
	s.t.Helper()
	var records []processRecord
	s.await(label, 10*time.Second, func() bool {
		records = s.processRecords()
		ready := 0
		for _, r := range records {
			if generationReady(r) {
				ready++
			}
		}
		return ready >= count
	})
	return records
}

func (s *scenario) assertRunning(what string, records []processRecord) {
	s.t.Helper()
	for _, r := range records {
		if r.state == "start" && !s.procs.alive(r.pid) {
			s.t.Fatalf("%s not alive: %+v", what, r)
		}
	}
}

func (s *scenario) processesGone(records []processRecord) bool {
	for _, r := range records {
		if r.state == "start" && s.procs.alive(r.pid) {
			return false
		}
	}
	return true
}

func (s *scenario) awaitProcessesGone(label string, records []processRecord) {
	s.t.Helper()
	s.await(label, 5*time.Second, func() bool { return s.processesGone(records) })
}

// assertGracefulCompletion requires every ready process to record signal and
// then complete, which only a handled graceful stop produces.
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

func (s *scenario) awaitDelayedHooksGone() {
	s.t.Helper()
	hooks := delayedHooks(s.lines())
	s.await("all slow hook processes disappear", 5*time.Second, func() bool {
		for _, hook := range hooks {
			if s.procs.alive(hook.pid) {
				return false
			}
		}
		return true
	})
	s.t.Logf("slow hook process observations: started=%d alive-after-stop=0", len(hooks))
}

// snapshotProcesses records the liveness of every recorded process into
// <phase>.log in the run directory and the test log.
func (s *scenario) snapshotProcesses(phase string) {
	s.t.Helper()
	var snapshot strings.Builder
	fmt.Fprintf(&snapshot, "time=%s phase=%s\n", time.Now().UTC().Format(time.RFC3339Nano), phase)
	for _, record := range s.processRecords() {
		if record.state == "start" {
			fmt.Fprintf(&snapshot, "run=%s depth=%d pid=%d alive=%t\n", record.run, record.depth, record.pid, s.procs.alive(record.pid))
		}
	}
	if err := s.procs.writeDiagnostics(s.records); err != nil {
		s.t.Fatal(err)
	}
	if detailed, err := os.ReadFile(s.records + ".processes"); err == nil {
		snapshot.Write(detailed)
	}
	s.write(filepath.Join(s.work, phase+".log"), []byte(snapshot.String()))
	s.t.Log(snapshot.String())
}

// rescueRecordedProcesses force-stops only the processes recorded by this
// scenario, after its graceful-stop evidence was collected, and writes the
// post-rescue state to cleanup.log. It never counts as a graceful stop.
func (s *scenario) rescueRecordedProcesses() bool {
	groups := map[int]bool{}
	records := s.processRecords()
	for _, r := range records {
		if pid, err := strconv.Atoi(r.run); err == nil {
			groups[pid] = true
		}
	}
	for pid := range groups {
		s.procs.killGroup(pid)
	}
	for _, r := range records {
		if r.state == "start" {
			s.procs.kill(r.pid)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for !s.processesGone(records) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	var diagnostics strings.Builder
	for _, r := range records {
		if r.state == "start" {
			fmt.Fprintf(&diagnostics, "pid=%d alive=%t\n", r.pid, s.procs.alive(r.pid))
		}
	}
	err := os.WriteFile(filepath.Join(s.work, "cleanup.log"), []byte(diagnostics.String()), 0644)
	return err == nil && s.processesGone(records)
}
