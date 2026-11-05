package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestProcessLifecycle(t *testing.T) {
	t.Parallel()
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
      run:
        interrupt: oldest
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
	createWithoutWrite(t, filepath.Join(s.Watch, "restart.txt"))
	all := s.AwaitReadyGenerations("2 ready process trees", 2)
	var replacement []harness.ProcessRecord
	for _, r := range all {
		if r.Generation != first[0].Generation {
			replacement = append(replacement, r)
		}
	}
	s.AssertRunning("replacement process before shutdown", replacement)
	assertEndedBeforeStart(t, all, first[0].Generation, replacement[0].Generation)
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
	t.Parallel()
	for _, interrupt := range []string{"none", "oldest"} {
		t.Run("interrupt="+interrupt, func(t *testing.T) {
			t.Parallel()
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
        waitMillisecond: 10
        deliver: last
      run:
        interrupt: %s
`, s.Watch, s.Hook("--depth=2"), s.Hook("--depth=2"), interrupt))
			s.AssertRunning("startup process", s.AwaitReadyGenerations("startup process tree ready", 1))
			s.Write(filepath.Join(s.Watch, "during-start.txt"), nil)
			if interrupt == "oldest" {
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
			if interrupt == "none" {
				for _, r := range records {
					if r.Depth == 2 && r.State == "start" && r.Generation != records[0].Generation {
						t.Fatal("sequential startup was replaced")
					}
				}
			}
		})
	}
}

// run.concurrency: 2에서 함께 도는 두 프로세스 트리 hook이 감시를 멈출 때 모두 정상 종료하고, 그 뒤에 감시 대상
// onFinishWatch가 실행되는지 확인한다.
func TestConcurrentRunsShutdown(t *testing.T) {
	t.Parallel()
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
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
      run:
        concurrency: 2
`, s.Watch, s.Hook("--depth=2"), s.Hook("--line=TARGET:onFinishWatch")))
	s.Write(filepath.Join(s.Watch, "first.txt"), nil)
	s.Write(filepath.Join(s.Watch, "second.txt"), nil)
	// hookrec 트리는 신호를 받기 전에는 끝나지 않으므로, 두 generation이 함께 준비되면 두 실행이 동시에 돈다.
	s.AssertRunning("concurrent process trees before stop", s.AwaitReadyGenerations("2 concurrent process trees ready", 2))
	s.Stop()
	records := s.ProcessRecords()
	harness.AssertGracefulCompletion(t, records)
	if lines := s.Lines(); len(lines) == 0 || lines[len(lines)-1] != "TARGET:onFinishWatch" {
		t.Fatalf("finish hook must follow completion of every running tree: %v", lines)
	}
	s.AwaitProcessesGone("all process trees disappear", records)
}

// TestRunInterrupt는 run.concurrency: 2에서 새 실행이 run.interrupt에 따라 돌고 있는 실행 가운데 무엇을 취소하는지
// 실제 프로세스 트리로 확인한다. 취소한 트리가 정상 종료한 뒤에야 새 트리가 시작해야 한다.
func TestRunInterrupt(t *testing.T) {
	t.Parallel()
	t.Run("changes/interrupt=samePath", func(t *testing.T) {
		t.Parallel()
		s := startLabeledChangeTrees(t, "samePath")
		a, b := filepath.Join(s.Watch, "a.txt"), filepath.Join(s.Watch, "b.txt")
		createWithoutWrite(t, a)
		s.AwaitReadyGenerations("first a.txt tree ready", 1)

		// 자리가 비어 있어도 같은 경로의 실행은 취소한다.
		s.Write(a, []byte("1"))
		records := s.AwaitReadyGenerations("second a.txt tree ready", 2)
		aRuns := labeledGenerations(t, s, "a.txt", 2)
		assertEndedBeforeStart(t, records, aRuns[0], aRuns[1])
		s.AwaitProcessesGone("first a.txt tree disappears", generationRecords(records, aRuns[0]))

		// 같은 경로의 실행이 없으면 아무것도 취소하지 않는다.
		createWithoutWrite(t, b)
		records = s.AwaitReadyGenerations("b.txt tree ready", 3)
		bRun := labeledGenerations(t, s, "b.txt", 1)[0]
		assertNotCanceled(t, s, records, aRuns[1])

		// 두 자리가 모두 찼을 때도 a.txt의 실행만 취소한다.
		s.Write(a, []byte("2"))
		records = s.AwaitReadyGenerations("third a.txt tree ready", 4)
		aRuns = labeledGenerations(t, s, "a.txt", 3)
		assertEndedBeforeStart(t, records, aRuns[1], aRuns[2])
		assertNotCanceled(t, s, records, bRun)
		s.AwaitProcessesGone("second a.txt tree disappears", generationRecords(records, aRuns[1]))

		s.Stop()
		records = s.ProcessRecords()
		harness.AssertGracefulCompletion(t, records)
		s.AwaitProcessesGone("all process trees disappear", records)
	})

	t.Run("changes/interrupt=oldest", func(t *testing.T) {
		t.Parallel()
		s := startLabeledChangeTrees(t, "oldest")
		a, b := filepath.Join(s.Watch, "a.txt"), filepath.Join(s.Watch, "b.txt")
		createWithoutWrite(t, a)
		s.AwaitReadyGenerations("a.txt tree ready", 1)

		// 자리가 비어 있으면 아무것도 취소하지 않는다.
		createWithoutWrite(t, b)
		records := s.AwaitReadyGenerations("first b.txt tree ready", 2)
		aRun := labeledGenerations(t, s, "a.txt", 1)[0]
		assertNotCanceled(t, s, records, aRun)

		// 두 자리가 모두 찼으면 경로와 상관없이 먼저 시작한 a.txt의 실행을 취소한다.
		s.Write(b, []byte("1"))
		records = s.AwaitReadyGenerations("second b.txt tree ready", 3)
		bRuns := labeledGenerations(t, s, "b.txt", 2)
		assertEndedBeforeStart(t, records, aRun, bRuns[1])
		assertNotCanceled(t, s, records, bRuns[0])
		s.AwaitProcessesGone("a.txt tree disappears", generationRecords(records, aRun))

		s.Stop()
		records = s.ProcessRecords()
		harness.AssertGracefulCompletion(t, records)
		s.AwaitProcessesGone("all process trees disappear", records)
	})

	for _, interrupt := range []string{"samePath", "oldest"} {
		t.Run("onStartWatch/interrupt="+interrupt, func(t *testing.T) {
			t.Parallel()
			s := harness.NewScenario(t)
			release := filepath.Join(s.Work, "release-start")
			// fileEvent: C와 기다리지 않는 each로, 파일 하나를 만들면 그 이벤트를 받자마자 실행 하나가 생기게 한다.
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
        waitMillisecond: 0
        deliver: each
      run:
        concurrency: 2
        interrupt: %s
`, s.Watch, s.Hook("--depth=0", "--release-file="+release), s.Hook("--depth=2", "--label={{.Name}}"), interrupt))
			startRun := s.AwaitReadyGenerations("start hook ready", 1)[0].Generation
			a := filepath.Join(s.Watch, "a.txt")
			createWithoutWrite(t, a)
			if interrupt == "samePath" {
				// 실행이 생겼다는 기록은 없으므로, watcher가 이벤트를 받은 것을 보고 시작 hook을 끝낸다.
				s.Await("a.txt event received", 10*time.Second, func() bool { return strings.Contains(s.Log(), strconv.Quote(a)) })
				s.Write(release, nil)
			}
			records := s.AwaitReadyGenerations("a.txt tree ready", 2)
			aRun := labeledGenerations(t, s, "a.txt", 1)[0]
			assertEndedBeforeStart(t, records, startRun, aRun)
			startCanceled := slices.ContainsFunc(generationRecords(records, startRun), func(r harness.ProcessRecord) bool { return r.State == "signal" })
			if startCanceled != (interrupt == "oldest") {
				t.Fatalf("start hook canceled = %t, want %t: %v", startCanceled, interrupt == "oldest", records)
			}

			s.Stop()
			records = s.ProcessRecords()
			canceled := records
			if !startCanceled {
				// 취소되지 않은 시작 hook은 release 파일을 보고 스스로 끝났으므로 정상 종료 판정에서 뺀다.
				canceled = slices.DeleteFunc(slices.Clone(records), func(r harness.ProcessRecord) bool { return r.Generation == startRun })
			}
			harness.AssertGracefulCompletion(t, canceled)
			s.AwaitProcessesGone("all process trees disappear", records)
		})
	}
}

// startLabeledChangeTrees는 run.concurrency: 2와 interrupt로 변경마다 경로 이름을 label로 남기는 프로세스 트리를
// 실행하는 watcher를 시작한다.
func startLabeledChangeTrees(t *testing.T, interrupt string) *harness.Scenario {
	t.Helper()
	s := harness.NewScenario(t)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
      run:
        concurrency: 2
        interrupt: %s
`, s.Watch, s.Hook("--depth=2", "--label={{.Name}}"), interrupt))
	return s
}

// createWithoutWrite는 쓰기 이벤트 없이 생성 이벤트만 생기도록 빈 파일을 만든다.
func createWithoutWrite(t *testing.T, path string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// labeledGenerations는 label로 시작한 generation이 count개인지 확인하고 시작한 순서대로 돌려준다.
func labeledGenerations(t *testing.T, s *harness.Scenario, label string, count int) []string {
	t.Helper()
	generations := s.GenerationsLabeled(label)
	if len(generations) != count {
		t.Fatalf("%s generations = %v, want %d: %v", label, generations, count, s.Lines())
	}
	return generations
}

func generationRecords(records []harness.ProcessRecord, run string) []harness.ProcessRecord {
	var tree []harness.ProcessRecord
	for _, r := range records {
		if r.Generation == run {
			tree = append(tree, r)
		}
	}
	return tree
}

// assertNotCanceled는 generation run이 취소 신호를 받지 않고 살아 있는지 확인한다. 새 실행은 취소한 실행이 끝난
// 뒤에 시작하므로, 새 트리가 준비된 뒤에 부르면 그 실행이 취소하지 않았다는 증거가 된다.
func assertNotCanceled(t *testing.T, s *harness.Scenario, records []harness.ProcessRecord, run string) {
	t.Helper()
	tree := generationRecords(records, run)
	for _, r := range tree {
		if r.State == "signal" {
			t.Fatalf("generation %s was canceled: %v", run, records)
		}
	}
	s.AssertRunning("generation "+run, tree)
}

// assertEndedBeforeStart는 newRun generation이 시작되기 전에 oldRun generation의 모든 프로세스가
// complete나 parent-exit를 기록했는지 확인한다.
func assertEndedBeforeStart(t *testing.T, records []harness.ProcessRecord, oldRun, newRun string) {
	t.Helper()
	running := map[int]bool{}
	for _, r := range records {
		switch {
		case r.Generation == oldRun && r.State == "start":
			running[r.PID] = true
		case r.Generation == oldRun && (r.State == "complete" || r.State == "parent-exit"):
			delete(running, r.PID)
		case r.Generation == newRun && r.State == "start":
			if len(running) != 0 {
				t.Fatalf("generation %s started before generation %s ended: %v", newRun, oldRun, records)
			}
			return
		}
	}
	t.Fatalf("generation %s not recorded: %v", newRun, records)
}
