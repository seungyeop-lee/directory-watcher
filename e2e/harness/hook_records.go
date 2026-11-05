package harness

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// hookrec은 hook마다 시나리오의 기록 파일에 아래 형식의 줄을 덧붙인다: 주어진 --line 텍스트,
// --delay를 기다리기 전의 BEGIN|PID|line, 기다리는 동안 SIGINT나 SIGTERM을 받았을 때만 --line
// 텍스트 대신 남기는 CANCEL|PID|line, --error-context의 ERROR|JSON(이름이 DW_ERROR_로 시작하는
// 환경 변수 전부와 workdir), --queue-context의 QUEUE|JSON(이름이 DW_QUEUE_로 시작하는 환경 변수
// 전부)이다. 프로세스 트리 기록은 process_records.go가 읽는다.

func ReadLinesLenient(path string) []string {
	data, _ := os.ReadFile(path)
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (s *Scenario) Lines() []string { return ReadLinesLenient(s.Records) }

// Hook은 주어진 인자로 hookrec을 실행해 이 시나리오의 기록 파일에 기록하는 YAML
// 명령을 작은따옴표로 감싸 돌려준다. 명령은 기본 셸(sh, cmd)이 해석하므로, 템플릿이 없는
// 인자는 경로에 공백이 있어도 인자 하나가 되게 큰따옴표로 감싼다. 템플릿이 든 인자는
// 템플릿이 따옴표 밖에 있어야 값이 인자 하나로 넘어가므로 감싸지 않는다. 그런 인자에는
// 셸이 해석하는 문자를 넣지 않는다.
func (s *Scenario) Hook(arguments ...string) string {
	var words []string
	for _, argument := range append([]string{hookrecBin, "--file=" + s.Records}, arguments...) {
		if !strings.Contains(argument, "{{") {
			argument = `"` + argument + `"`
		}
		words = append(words, argument)
	}
	return fmt.Sprintf("'%s'", strings.Join(words, " "))
}

func CountRecord(lines []string, value string) int {
	count := 0
	for _, line := range lines {
		if line == value {
			count++
		}
	}
	return count
}

func (s *Scenario) AwaitLine(label string, timeout time.Duration, line string) {
	s.t.Helper()
	s.Await(label, timeout, func() bool { return CountRecord(s.Lines(), line) > 0 })
}

type delayedHook struct {
	pid  int
	line string
}

// delayedHooks는 BEGIN을 기록한 지연 hook을 기록 순서대로 돌려준다.
func delayedHooks(lines []string) []delayedHook {
	var hooks []delayedHook
	for _, line := range lines {
		fields := strings.SplitN(line, "|", 3)
		if len(fields) != 3 || fields[0] != "BEGIN" {
			continue
		}
		if pid, err := strconv.Atoi(fields[1]); err == nil {
			hooks = append(hooks, delayedHook{pid, fields[2]})
		}
	}
	return hooks
}

func DelayedHookBegun(lines []string, line string) bool {
	for _, hook := range delayedHooks(lines) {
		if hook.line == line {
			return true
		}
	}
	return false
}

func receivedChanges(log string, paths map[string]bool, operations fsnotify.Op) []string {
	var received []string
	for _, line := range strings.Split(log, "\n") {
		_, event, ok := strings.Cut(line, "event: ")
		if !ok {
			continue
		}
		quote := strings.IndexByte(event, '"')
		if quote < 0 {
			continue
		}
		path, err := strconv.Unquote(event[quote:])
		if err != nil || !paths[path] {
			continue
		}
		op := strings.TrimSpace(event[:quote])
		if operations.Has(fsnotify.Create) && strings.Contains(op, "CREATE") ||
			operations.Has(fsnotify.Write) && strings.Contains(op, "WRITE") {
			received = append(received, path)
		}
	}
	return received
}

func changesCompleted(received, records []string, prefix string, sequential bool) bool {
	if len(received) == 0 {
		return false
	}
	begins, completed, cancelled := 0, 0, 0
	last := ""
	for _, line := range records {
		switch {
		case strings.HasPrefix(line, "BEGIN|"):
			begins++
		case strings.HasPrefix(line, "CANCEL|"):
			cancelled++
		case strings.HasPrefix(line, prefix):
			completed++
			last = strings.TrimPrefix(line, prefix)
		}
	}
	return last == received[len(received)-1] &&
		(begins == 0 || begins == completed+cancelled) &&
		(!sequential || completed == len(received))
}

// AwaitChangesComplete는 paths에서 받은 이벤트의 hook이 모두 끝나고 120ms 동안 조용할 때까지 기다린다. 이벤트 수와
// hook 하나의 비용에 따라 전체 시간이 달라지므로, 받은 이벤트나 기록이 늘어나는 동안은 기다리고 idle 동안 둘 다
// 그대로이면 실패한다. 그래서 받은 이벤트의 hook이 기록을 남기지 않으면 idle 뒤에 실패한다.
func (s *Scenario) AwaitChangesComplete(prefix string, paths []string, operations fsnotify.Op, sequential bool, idle time.Duration) {
	s.t.Helper()
	allowed := make(map[string]bool, len(paths))
	for _, path := range paths {
		allowed[path] = true
	}
	started := time.Now()
	recordsAtStart := len(s.Lines())
	quietSince := time.Now()
	previous := ""
	var received, records []string
	s.AwaitProgress("received inflow completes and settles", idle, func() (bool, int) {
		log := s.log.String()
		received = receivedChanges(log, allowed, operations)
		records = s.Lines()
		snapshot := log + strings.Join(records, "\n")
		if snapshot != previous {
			previous = snapshot
			quietSince = time.Now()
		}
		done := changesCompleted(received, records, prefix, sequential) && time.Since(quietSince) >= 120*time.Millisecond
		return done, len(received) + len(records)
	})
	s.t.Logf("inflow settled after %s: received=%d records=%d (records at wait start=%d) last-received=%s", time.Since(started), len(received), len(records), recordsAtStart, received[len(received)-1])
}
