package harness

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// hookrec appends one line per hook to the scenario's records file: the given
// --line text, BEGIN|PID|line and CANCEL|PID|line around a --delay, and
// ERROR|JSON for --error-context. Process tree records are read by
// process_records.go.

func ReadLinesLenient(path string) []string {
	data, _ := os.ReadFile(path)
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (s *Scenario) Lines() []string { return ReadLinesLenient(s.Records) }

// Hook returns a single-quoted YAML command that runs hookrec with the given
// arguments and records into this scenario's records file.
func (s *Scenario) Hook(arguments ...string) string {
	return fmt.Sprintf("'%s'", strings.Join(append([]string{hookrecBin, "--file=" + s.Records}, arguments...), " "))
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

// delayedHooks returns the delayed hooks that recorded BEGIN, in record order.
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

func (s *Scenario) AwaitChangesComplete(prefix string, paths []string, operations fsnotify.Op, sequential bool, timeout time.Duration) {
	s.t.Helper()
	allowed := make(map[string]bool, len(paths))
	for _, path := range paths {
		allowed[path] = true
	}
	quietSince := time.Now()
	previous := ""
	var received, records []string
	s.Await("received inflow completes and settles", timeout, func() bool {
		log := s.log.String()
		received = receivedChanges(log, allowed, operations)
		records = s.Lines()
		snapshot := log + strings.Join(records, "\n")
		if snapshot != previous {
			previous = snapshot
			quietSince = time.Now()
		}
		return changesCompleted(received, records, prefix, sequential) && time.Since(quietSince) >= 120*time.Millisecond
	})
	s.t.Logf("inflow settled: received=%d records=%d last-received=%s", len(received), len(records), received[len(received)-1])
}
