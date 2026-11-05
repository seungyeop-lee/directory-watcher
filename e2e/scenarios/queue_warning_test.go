package scenarios

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// TestQueueWarningCLI는 대기열이 queue.size의 70%와 90%에 닿고 다시 내려갈 때 onQueueWarning이 DW_QUEUE_*
// 환경 변수로 대상, 단계, 방향, 기다리는 수, 용량, 실행이 몰린 경로를 받는지 확인한다.
func TestQueueWarningCLI(t *testing.T) {
	t.Parallel()
	s := harness.NewScenario(t)
	release := filepath.Join(s.Work, "release-start")
	s.Start(fmt.Sprintf(`global:
  lifeCycle:
    onQueueWarning:
      - %s
watchTargets:
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
      queue:
        size: 10
`, s.Hook("--queue-context"), s.Watch, s.Hook("--depth=0", "--release-file="+release), s.Hook("--line=CHANGE:{{.Path}}")))
	s.AwaitReadyGenerations("start hook holds the change hook", 1)
	var paths []string
	for n := range 9 {
		path := filepath.Join(s.Watch, fmt.Sprintf("load-%d.txt", n))
		paths = append(paths, path)
		s.Write(path, nil)
	}
	s.Await("warnings at 70% and 90%", 10*time.Second, func() bool { return len(queueContexts(t, s.Records)) >= 2 })
	up := queueContexts(t, s.Records)
	assertQueueContext(t, up[0], s.Watch, "70", "up", "7", paths[:5])
	assertQueueContext(t, up[1], s.Watch, "90", "up", "9", paths[:5])

	s.Write(release, nil)
	s.Await("queue drains below 60%", 10*time.Second, func() bool {
		contexts := queueContexts(t, s.Records)
		return len(contexts) > 2 && contexts[len(contexts)-1]["DW_QUEUE_LEVEL"] == "70" && contexts[len(contexts)-1]["DW_QUEUE_DIRECTION"] == "down"
	})
	for _, path := range paths {
		s.AwaitLine("queued change runs", 10*time.Second, "CHANGE:"+path)
	}
	s.Stop()
	contexts := queueContexts(t, s.Records)
	// 90 내려감 경고가 아직 실행 중일 때 70 내려감이 생기면 70 내려감만 남으므로, 90 내려감은 없을 수 있다.
	down := contexts[2:]
	if len(down) == 2 {
		assertQueueContext(t, down[0], s.Watch, "90", "down", "7", paths[2:7])
	} else if len(down) != 1 {
		t.Fatalf("warnings after the queue drained = %v, want 90 down then 70 down, or 70 down alone", down)
	}
	assertQueueContext(t, down[len(down)-1], s.Watch, "70", "down", "5", paths[4:9])
}

func queueContexts(t *testing.T, file string) []map[string]string {
	t.Helper()
	var contexts []map[string]string
	for _, line := range harness.ReadLinesLenient(file) {
		if strings.HasPrefix(line, "QUEUE|") {
			var context map[string]string
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "QUEUE|")), &context); err != nil {
				t.Fatalf("queue warning JSON: %v: %s", err, line)
			}
			contexts = append(contexts, context)
		}
	}
	return contexts
}

// assertQueueContext는 경고 하나가 받은 환경 변수를 확인한다. topPaths는 대기열에서 기다리던 경로이며, 실행이
// 하나씩이므로 DW_QUEUE_TOP_PATHS의 줄마다 개수는 1이다.
func assertQueueContext(t *testing.T, context map[string]string, target, level, direction, length string, topPaths []string) {
	t.Helper()
	var lines []string
	for _, path := range topPaths {
		lines = append(lines, "1 "+path)
	}
	for _, field := range [][2]string{
		{"DW_QUEUE_TARGET", target},
		{"DW_QUEUE_LEVEL", level},
		{"DW_QUEUE_DIRECTION", direction},
		{"DW_QUEUE_LENGTH", length},
		{"DW_QUEUE_CAPACITY", "10"},
		{"DW_QUEUE_TOP_PATHS", strings.Join(lines, "\n")},
	} {
		if key, want := field[0], field[1]; context[key] != want {
			t.Errorf("%s %s warning: %s = %q, want %q", level, direction, key, context[key], want)
		}
	}
}
