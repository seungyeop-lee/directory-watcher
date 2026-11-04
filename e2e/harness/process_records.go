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

// hookrec --depth=N은 실제 부모, 자식, 손자 프로세스 사슬을 시작한다. 각 프로세스는
// generation|PID|depth|state를 기록하는데, generation은 루트 부모의 PID이고 state는
// start, ready, signal, complete, parent-exit 중 하나다. --label을 주면 루트 부모가 start 앞에
// LABEL|generation|label을 기록한다. 프로세스가 살아 있는지는 시나리오의 processObserver가 답한다.

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

// GenerationsLabeled는 hookrec --label=label로 시작한 프로세스 트리의 generation을 시작한 순서대로 돌려준다.
func (s *Scenario) GenerationsLabeled(label string) []string {
	var generations []string
	for _, line := range s.Lines() {
		fields := strings.SplitN(line, "|", 3)
		if len(fields) == 3 && fields[0] == "LABEL" && fields[2] == label {
			generations = append(generations, fields[1])
		}
	}
	return generations
}

// generationReady는 r이 루트 부모의 ready 기록인지 알려 준다. hookrec은 자식이 ready를
// 기록한 뒤에야 부모의 ready를 기록하므로, 이 기록이 있으면 트리 전체가 준비된 것이다.
func generationReady(r ProcessRecord) bool {
	return r.State == "ready" && r.Generation == strconv.Itoa(r.PID)
}

// AwaitReadyGenerations는 준비된 프로세스 트리 generation이 count개 이상이 될 때까지
// 기다리고, 그 시점까지 본 기록을 돌려준다.
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

// AssertGracefulCompletion은 준비된 모든 프로세스가 signal을 기록한 뒤 complete를
// 기록했는지 확인한다. 이 순서는 처리된 정상 정지만 남긴다.
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

// SnapshotProcesses는 기록된 모든 프로세스가 살아 있는지를 실행 디렉터리의
// <phase>.log와 테스트 로그에 기록한다.
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

// rescueRecordedProcesses는 시나리오의 정상 정지 증거를 모은 뒤에, 이 시나리오가
// 기록한 프로세스만 강제로 멈추고 구조한 뒤의 상태를 cleanup.log에 쓴다. 이 정지는
// 정상 정지로 치지 않는다.
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
