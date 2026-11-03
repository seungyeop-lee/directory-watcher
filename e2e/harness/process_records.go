package harness

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

type ProcessRecord struct {
	Generation string
	PID, Depth int
	State      string
}

func processRecords(file string) []ProcessRecord {
	var records []ProcessRecord
	for _, line := range ReadLinesLenient(file) {
		fields := strings.Split(line, "|")
		if len(fields) != 4 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[1])
		depth, e2 := strconv.Atoi(fields[2])
		if e1 == nil && e2 == nil {
			records = append(records, ProcessRecord{fields[0], pid, depth, fields[3]})
		}
	}
	return records
}

func (s *Scenario) ProcessRecords() []ProcessRecord { return processRecords(s.Records) }

// generationReady reports a root parent's ready record. hookrec records a
// parent ready only after its child recorded ready, so the whole tree is ready.
func generationReady(r ProcessRecord) bool {
	return r.State == "ready" && r.Generation == strconv.Itoa(r.PID)
}

// AwaitReadyGenerations waits until at least count process tree generations
// are ready and returns the records seen at that point.
func (s *Scenario) AwaitReadyGenerations(label string, count int) []ProcessRecord {
	s.t.Helper()
	var records []ProcessRecord
	s.Await(label, 10*time.Second, func() bool {
		records = s.ProcessRecords()
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

func (s *Scenario) AssertRunning(what string, records []ProcessRecord) {
	s.t.Helper()
	for _, r := range records {
		if r.State == "start" && !s.procs.alive(r.PID) {
			s.t.Fatalf("%s not alive: %+v", what, r)
		}
	}
}

func (s *Scenario) ProcessesGone(records []ProcessRecord) bool {
	for _, r := range records {
		if r.State == "start" && s.procs.alive(r.PID) {
			return false
		}
	}
	return true
}

func (s *Scenario) AwaitProcessesGone(label string, records []ProcessRecord) {
	s.t.Helper()
	s.Await(label, 5*time.Second, func() bool { return s.ProcessesGone(records) })
}

// AssertGracefulCompletion requires every ready process to record signal and
// then complete, which only a handled graceful stop produces.
func AssertGracefulCompletion(t *testing.T, records []ProcessRecord) {
	t.Helper()
	for _, ready := range records {
		if ready.State != "ready" {
			continue
		}
		signaled, completed := false, false
		for _, r := range records {
			if r.PID == ready.PID && r.Generation == ready.Generation {
				if r.State == "signal" {
					signaled = true
				}
				if r.State == "complete" {
					if !signaled {
						t.Fatalf("process completed before signal: %+v", r)
					}
					completed = true
				}
			}
		}
		if !signaled || !completed {
			t.Errorf("process %d missing signal or graceful completion: %v", ready.PID, records)
		}
	}
}

func (s *Scenario) AwaitDelayedHooksGone() {
	s.t.Helper()
	hooks := delayedHooks(s.Lines())
	s.Await("all slow hook processes disappear", 5*time.Second, func() bool {
		for _, hook := range hooks {
			if s.procs.alive(hook.pid) {
				return false
			}
		}
		return true
	})
	s.t.Logf("slow hook process observations: started=%d alive-after-stop=0", len(hooks))
}

// SnapshotProcesses records the liveness of every recorded process into
// <phase>.log in the run directory and the test log.
func (s *Scenario) SnapshotProcesses(phase string) {
	s.t.Helper()
	var snapshot strings.Builder
	fmt.Fprintf(&snapshot, "time=%s phase=%s\n", time.Now().UTC().Format(time.RFC3339Nano), phase)
	for _, record := range s.ProcessRecords() {
		if record.State == "start" {
			fmt.Fprintf(&snapshot, "run=%s depth=%d pid=%d alive=%t\n", record.Generation, record.Depth, record.PID, s.procs.alive(record.PID))
		}
	}
	if err := s.procs.writeDiagnostics(s.Records); err != nil {
		s.t.Fatal(err)
	}
	if detailed, err := os.ReadFile(s.Records + ".processes"); err == nil {
		snapshot.Write(detailed)
	}
	s.Write(filepath.Join(s.Work, phase+".log"), []byte(snapshot.String()))
	s.t.Log(snapshot.String())
}

// rescueRecordedProcesses force-stops only the processes recorded by this
// scenario, after its graceful-stop evidence was collected, and writes the
// post-rescue state to cleanup.log. It never counts as a graceful stop.
func (s *Scenario) rescueRecordedProcesses() bool {
	groups := map[int]bool{}
	records := s.ProcessRecords()
	for _, r := range records {
		if pid, err := strconv.Atoi(r.Generation); err == nil {
			groups[pid] = true
		}
	}
	for pid := range groups {
		s.procs.killGroup(pid)
	}
	for _, r := range records {
		if r.State == "start" {
			s.procs.kill(r.PID)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for !s.ProcessesGone(records) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	var diagnostics strings.Builder
	for _, r := range records {
		if r.State == "start" {
			fmt.Fprintf(&diagnostics, "pid=%d alive=%t\n", r.PID, s.procs.alive(r.PID))
		}
	}
	err := os.WriteFile(filepath.Join(s.Work, "cleanup.log"), []byte(diagnostics.String()), 0644)
	return err == nil && s.ProcessesGone(records)
}
