package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestProcessLifecycle(t *testing.T) {
	s := harness.NewScenario(t)
	tree := s.Hook("--depth=2")
	s.Start(fmt.Sprintf(`global:
  lifeCycle:
    onFinishWatch:
      - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 100
      interruptible: true
`, s.Hook("--line=GLOBAL:onFinishWatch"), s.Watch, tree, tree, s.Hook("--line=TARGET:onFinishWatch")))
	first := s.AwaitReadyGenerations("1 ready process trees", 1)
	readyCount := 0
	for _, r := range first {
		if r.State == "ready" {
			readyCount++
		}
	}
	if readyCount != 3 {
		t.Fatalf("expected real parent, child and grandchild readiness: %v", first)
	}
	s.AssertRunning("process before restart", first)
	if fault := os.Getenv("DW_E2E_FAULT"); fault != "" {
		if fault == "timeout" {
			s.Await("intentional timeout", 100*time.Millisecond, func() bool { return false })
		}
		t.Fatal("intentional failure after process tree readiness")
	}
	target := filepath.Join(s.Watch, "restart.txt")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	all := s.AwaitReadyGenerations("2 ready process trees", 2)
	var replacement []harness.ProcessRecord
	for _, r := range all {
		if r.Generation != first[0].Generation {
			replacement = append(replacement, r)
		}
	}
	s.AssertRunning("replacement process before shutdown", replacement)
	assertCompletedBeforeRestart(t, all, first[0].Generation)
	s.AwaitProcessesGone("previous tree disappears", first)
	s.Stop()
	all = s.ProcessRecords()
	harness.AssertGracefulCompletion(t, all)
	lines := s.Lines()
	if len(lines) < 2 || lines[len(lines)-2] != "TARGET:onFinishWatch" || lines[len(lines)-1] != "GLOBAL:onFinishWatch" {
		t.Fatalf("finish hooks must follow current process completion: %v", lines)
	}
	s.AwaitProcessesGone("all process generations disappear", all)
}

func TestStartHookShutdownModes(t *testing.T) {
	for _, deliver := range []string{"last", "each"} {
		for _, interruptible := range []bool{false, true} {
			t.Run(fmt.Sprintf("deliver=%s/interruptible=%t", deliver, interruptible), func(t *testing.T) {
				// each는 기다리지 않고 받은 이벤트마다 실행하게 하고, last는 기다린 뒤 마지막 경로 하나만 실행하게 한다.
				wait := 10
				if deliver == "each" {
					wait = 0
				}
				s := harness.NewScenario(t)
				s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: %d
        deliver: %s
      interruptible: %t
`, s.Watch, s.Hook("--depth=2"), s.Hook("--depth=2"), wait, deliver, interruptible))
				s.AssertRunning("startup process", s.AwaitReadyGenerations("startup process tree ready", 1))
				s.Write(filepath.Join(s.Watch, "during-start.txt"), nil)
				if interruptible {
					s.AwaitReadyGenerations("replacement tree ready", 2)
				}
				beforeStop := s.ProcessRecords()
				currentRun := beforeStop[len(beforeStop)-1].Generation
				var current []harness.ProcessRecord
				for _, r := range beforeStop {
					if r.Generation == currentRun {
						current = append(current, r)
					}
				}
				s.AssertRunning("current process before stop", current)
				s.Stop()
				records := s.ProcessRecords()
				harness.AssertGracefulCompletion(t, records)
				s.AwaitProcessesGone("startup/replacement descendants disappear", records)
				if !interruptible {
					for _, r := range records {
						if r.Depth == 2 && r.State == "start" && r.Generation != records[0].Generation {
							t.Fatal("sequential startup was replaced")
						}
					}
				}
			})
		}
	}
}

// assertCompletedBeforeRestart는 대체 generation이 시작되기 전에 이전 generation의
// 부모, 자식, 손자가 complete를 기록했는지 확인한다.
func assertCompletedBeforeRestart(t *testing.T, records []harness.ProcessRecord, oldRun string) {
	t.Helper()
	complete := map[int]bool{}
	for _, r := range records {
		if r.Generation == oldRun && r.State == "complete" {
			complete[r.Depth] = true
		}
		if r.Generation != oldRun && r.State == "start" {
			if len(complete) != 3 {
				t.Fatalf("new run started before parent/child/grandchild completion: %v", records)
			}
			return
		}
	}
	t.Fatal("replacement run not recorded")
}
