package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessLifecycle(t *testing.T) {
	s := newScenario(t)
	tree := s.hook("--depth=2")
	s.start(fmt.Sprintf(`global:
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
      interruptible: true
      waitMillisecond: 100
      watchEvent: C
      excludeSuffix: [.probe]
`, s.hook("--line=GLOBAL:onFinishWatch"), s.watch, tree, tree, s.hook("--line=TARGET:onFinishWatch")))
	first := s.awaitReadyGenerations("1 ready process trees", 1)
	readyCount := 0
	for _, r := range first {
		if r.state == "ready" {
			readyCount++
		}
	}
	if readyCount != 3 {
		t.Fatalf("expected real parent, child and grandchild readiness: %v", first)
	}
	s.assertRunning("process before restart", first)
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
	all := s.awaitReadyGenerations("2 ready process trees", 2)
	var replacement []processRecord
	for _, r := range all {
		if r.run != first[0].run {
			replacement = append(replacement, r)
		}
	}
	s.assertRunning("replacement process before shutdown", replacement)
	assertCompletedBeforeRestart(t, all, first[0].run)
	s.awaitProcessesGone("previous tree disappears", first)
	s.stop()
	all = s.processRecords()
	assertGracefulCompletion(t, all)
	lines := s.lines()
	if len(lines) < 2 || lines[len(lines)-2] != "TARGET:onFinishWatch" || lines[len(lines)-1] != "GLOBAL:onFinishWatch" {
		t.Fatalf("finish hooks must follow current process completion: %v", lines)
	}
	s.awaitProcessesGone("all process generations disappear", all)
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
        - %s
      onChange:
        - %s
    option:
      noWait: %t
      interruptible: %t
      waitMillisecond: 10
      watchEvent: C
      excludeSuffix: [.probe]
`, s.watch, s.hook("--depth=2"), s.hook("--depth=2"), noWait, interruptible))
				s.assertRunning("startup process", s.awaitReadyGenerations("startup process tree ready", 1))
				s.write(filepath.Join(s.watch, "during-start.txt"), nil)
				if interruptible {
					s.awaitReadyGenerations("replacement tree ready", 2)
				}
				beforeStop := s.processRecords()
				currentRun := beforeStop[len(beforeStop)-1].run
				var current []processRecord
				for _, r := range beforeStop {
					if r.run == currentRun {
						current = append(current, r)
					}
				}
				s.assertRunning("current process before stop", current)
				s.stop()
				records := s.processRecords()
				assertGracefulCompletion(t, records)
				s.awaitProcessesGone("startup/replacement descendants disappear", records)
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

// assertCompletedBeforeRestart requires the old generation's parent, child and
// grandchild to record complete before the replacement generation starts.
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
