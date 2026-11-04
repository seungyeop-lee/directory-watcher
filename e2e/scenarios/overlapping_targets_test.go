package scenarios

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// overlapTarget은 겹치는 감시 대상 하나다. 대상의 onStartWatch는 START:<label>을 기록한다.
type overlapTarget struct {
	label, path string
	// onChange가 비어 있으면 onChange는 <label>:{{.Name}}을 기록한다.
	onChange []string
	// exclude는 '*.probe' 뒤에 덧붙일 watch.exclude 패턴이다.
	exclude string
}

// startOverlapping은 targets를 감시하는 watcher를 시작하고, 모든 대상이 감시를 시작해 onStartWatch를 기록할
// 때까지 기다린다. Start가 s.Watch의 이벤트로 준비를 확인해도, 그 이벤트를 받은 대상이 어느 것인지는 로그로
// 가릴 수 없다. withGlobal이면 전역 onBeforeChange와 onAfterChange가 BEFORE:{{.Name}}과 AFTER:{{.Name}}을
// 기록한다.
func startOverlapping(s *harness.Scenario, withGlobal bool, targets ...overlapTarget) {
	var config strings.Builder
	if withGlobal {
		fmt.Fprintf(&config, `global:
  lifeCycle:
    onBeforeChange:
      - %s
    onAfterChange:
      - %s
`, s.Hook("--line=BEFORE:{{.Name}}"), s.Hook("--line=AFTER:{{.Name}}"))
	}
	config.WriteString("watchTargets:\n")
	for _, target := range targets {
		onChange := target.onChange
		if len(onChange) == 0 {
			onChange = []string{s.Hook("--line=" + target.label + ":{{.Name}}")}
		}
		fmt.Fprintf(&config, `  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe'%s]
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
`, target.path, s.Hook("--line=START:"+target.label), strings.Join(onChange, "\n        - "), target.exclude)
	}
	s.Start(config.String())
	for _, target := range targets {
		s.AwaitLine(target.label+" target starts watching", 10*time.Second, "START:"+target.label)
	}
}

// awaitSentinel은 dir에 표지 파일을 만들고, labels의 대상이 모두 그 표지로 onChange를 실행할 때까지 기다린다.
// 대상은 한 폴더의 이벤트를 받은 순서대로 hook을 실행하므로, 표지보다 먼저 dir에서 한 동작이 그 대상의 hook을
// 실행했다면 그 기록은 이미 남아 있다.
func awaitSentinel(t *testing.T, s *harness.Scenario, dir string, labels ...string) {
	t.Helper()
	createEmpty(t, filepath.Join(dir, "zz-sentinel.txt"))
	s.Await("sentinel reaches "+strings.Join(labels, " and "), 10*time.Second, func() bool {
		lines := s.Lines()
		for _, label := range labels {
			if harness.CountRecord(lines, label+":zz-sentinel.txt") == 0 {
				return false
			}
		}
		return true
	})
}

// assertRunPerTarget은 name의 변경 하나가 labels의 대상마다 전역 onBeforeChange, 대상 onChange, 전역
// onAfterChange를 한 번씩 실행했는지 확인한다. 전역 hook의 기록은 대상을 가리지 않으므로, 대상마다
// BEFORE→onChange→AFTER 순서로 나눌 수 있는지를 앞에서부터 센 기록 수로 판정한다: 어느 지점에서도 onChange는
// BEFORE보다, AFTER는 onChange보다 많을 수 없다.
func assertRunPerTarget(t *testing.T, lines []string, name string, labels ...string) {
	t.Helper()
	var got []string
	before, change, after := 0, 0, 0
	for _, line := range lines {
		label, recordName, ok := strings.Cut(line, ":")
		if !ok || recordName != name {
			continue
		}
		got = append(got, line)
		switch label {
		case "BEFORE":
			before++
		case "AFTER":
			after++
		default:
			change++
		}
		if change > before || after > change {
			t.Fatalf("records of %s cannot split into onBeforeChange, onChange, onAfterChange per target: %q", name, got)
		}
	}
	if before != len(labels) || after != len(labels) {
		t.Errorf("global hooks for %s: onBeforeChange %d, onAfterChange %d, want %d each: %q", name, before, after, len(labels), got)
	}
	for _, label := range labels {
		if n := harness.CountRecord(lines, label+":"+name); n != 1 {
			t.Errorf("%s onChange for %s ran %d times, want 1: %q", label, name, n, got)
		}
	}
}

// stopWithoutErrors는 watcher가 종료 코드 0으로 끝났고 실행하는 동안 ERROR 로그를 남기지 않았는지 확인한다.
func stopWithoutErrors(t *testing.T, s *harness.Scenario) {
	t.Helper()
	s.Stop()
	if log := s.Log(); strings.Contains(log, "[ERROR]") {
		t.Fatalf("watcher logged an error with overlapping targets:\n%s", log)
	}
}

// TestOverlappingTargets는 감시 대상의 path가 같거나 상위·하위로 겹칠 때, 대상마다 따로 hook을 실행한다는 사용
// 안내의 동작을 확인한다. 상위 대상은 s.Watch이고 하위 대상은 그 안의 api 폴더다.
func TestOverlappingTargets(t *testing.T) {
	t.Run("same path runs each target once", func(t *testing.T) {
		s := harness.NewScenario(t)
		startOverlapping(s, true, overlapTarget{label: "FIRST", path: s.Watch}, overlapTarget{label: "SECOND", path: s.Watch})
		createEmpty(t, filepath.Join(s.Watch, "a.txt"))
		awaitSentinel(t, s, s.Watch, "FIRST", "SECOND")
		assertRunPerTarget(t, s.Lines(), "a.txt", "FIRST", "SECOND")
		stopWithoutErrors(t, s)
	})
	t.Run("parent and child paths run each target once", func(t *testing.T) {
		s := harness.NewScenario(t)
		api := filepath.Join(s.Watch, "api")
		mkdirAll(t, api)
		startOverlapping(s, true, overlapTarget{label: "PARENT", path: s.Watch}, overlapTarget{label: "CHILD", path: api})
		createEmpty(t, filepath.Join(api, "a.txt"))
		awaitSentinel(t, s, api, "PARENT", "CHILD")
		assertRunPerTarget(t, s.Lines(), "a.txt", "PARENT", "CHILD")
		stopWithoutErrors(t, s)
	})
	t.Run("slow parent hook does not block child", func(t *testing.T) {
		s := harness.NewScenario(t)
		api := filepath.Join(s.Watch, "api")
		mkdirAll(t, api)
		// 지연은 종료 신호로 취소되므로, 판정 대기보다 넉넉하게 길어도 테스트 시간이 늘지 않는다.
		startOverlapping(s, false,
			overlapTarget{label: "PARENT", path: s.Watch, onChange: []string{s.Hook("--delay=30s", "--line=PARENT:{{.Name}}")}},
			overlapTarget{label: "CHILD", path: api})
		createEmpty(t, filepath.Join(api, "a.txt"))
		s.Await("parent hook running", 10*time.Second, func() bool { return harness.DelayedHookBegun(s.Lines(), "PARENT:a.txt") })
		s.AwaitLine("child hook completes while parent hook runs", 10*time.Second, "CHILD:a.txt")
		if harness.CountRecord(s.Lines(), "PARENT:a.txt") != 0 {
			t.Fatal("parent hook completed before the child hook was checked; the check did not overlap the parent hook")
		}
		stopWithoutErrors(t, s)
		s.AwaitDelayedHooksGone()
	})
	t.Run("hook writing into another target runs its hooks", func(t *testing.T) {
		s := harness.NewScenario(t)
		api := filepath.Join(s.Watch, "api")
		mkdirAll(t, api)
		// hookrec은 --file의 파일을 만들어 한 줄을 쓰므로, 이 명령은 하위 대상 밖이면서 상위 대상 안인 out.txt를 만든다.
		writeOut := fmt.Sprintf(`'"%s" "--file=%s" "--line=written"'`, harness.HookrecBin(), filepath.Join(s.Watch, "out.txt"))
		startOverlapping(s, false,
			overlapTarget{label: "PARENT", path: s.Watch},
			overlapTarget{label: "CHILD", path: api, onChange: []string{s.Hook("--line=CHILD:{{.Name}}"), writeOut}})
		createEmpty(t, filepath.Join(api, "a.txt"))
		s.AwaitLine("child hook runs", 10*time.Second, "CHILD:a.txt")
		s.AwaitLine("file written by the child hook runs the parent hook", 10*time.Second, "PARENT:out.txt")
		stopWithoutErrors(t, s)
	})
	t.Run("parent excluding the child folder runs only the child", func(t *testing.T) {
		s := harness.NewScenario(t)
		api := filepath.Join(s.Watch, "api")
		mkdirAll(t, api)
		startOverlapping(s, false,
			overlapTarget{label: "PARENT", path: s.Watch, exclude: ", 'api/**'"},
			overlapTarget{label: "CHILD", path: api})
		createEmpty(t, filepath.Join(api, "a.txt"))
		s.AwaitLine("child hook runs", 10*time.Second, "CHILD:a.txt")
		// 상위 대상은 api 폴더를 감시하지 않아 그 안의 표지를 받을 수 없으므로, 상위 대상 바로 아래의 표지로 판정한다.
		awaitSentinel(t, s, s.Watch, "PARENT")
		lines := s.Lines()
		if n := harness.CountRecord(lines, "PARENT:a.txt"); n != 0 {
			t.Errorf("parent excluding api/** ran onChange %d times for api/a.txt: %q", n, lines)
		}
		if n := harness.CountRecord(lines, "CHILD:a.txt"); n != 1 {
			t.Errorf("child onChange for api/a.txt ran %d times, want 1: %q", n, lines)
		}
		stopWithoutErrors(t, s)
	})
}
