package harness

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// statusDLLInitFailed는 Windows에서 프로세스가 DLL을 초기화하지 못해 시작하지 못했을 때의 종료 코드다.
const statusDLLInitFailed = 0xc0000142

// consoleEvent는 테스트 바이너리의 콘솔에서 일어난 CTRL+BREAK 전송 하나나 프로세스 종료 하나다.
// 전송이면 at이 보낸 시각이고, 종료면 at이 끝난 시각이다.
type consoleEvent struct {
	at      time.Time
	signal  bool
	pid     int
	code    uint32
	started time.Time
	from    string
	err     string
}

// harnessSignals는 하네스가 이 테스트 바이너리에서 보낸 CTRL+BREAK다. watcher가 보낸 CTRL+BREAK는 각 시나리오의
// watcher 로그에 남는다.
var harnessSignals struct {
	sync.Mutex
	events []consoleEvent
}

func recordHarnessSignal(at time.Time, pid int, err error) {
	event := consoleEvent{at: at, signal: true, pid: pid, from: "harness"}
	if err != nil {
		event.err = err.Error()
	}
	harnessSignals.Lock()
	defer harnessSignals.Unlock()
	harnessSignals.events = append(harnessSignals.events, event)
}

func harnessSignalsSince(start time.Time) []consoleEvent {
	harnessSignals.Lock()
	defer harnessSignals.Unlock()
	var events []consoleEvent
	for _, event := range harnessSignals.events {
		if !event.at.Before(start) {
			events = append(events, event)
		}
	}
	return events
}

// dllInitFailureReport는 이 시나리오에서 0xc0000142로 끝난 프로세스가 있으면, 시나리오를 시작한 뒤 이 콘솔에서 보낸
// CTRL+BREAK와 그 프로세스의 종료를 시각 순서로 적은 보고를 돌려준다. 없으면 빈 문자열을 돌려준다.
// 시나리오 프로세스가 끝난 뒤에 불러야 그 프로세스의 종료와 watcher 로그가 모두 담긴다.
func (s *Scenario) dllInitFailureReport() string {
	others := harnessSignalsSince(s.started)
	watcher := "watcher"
	if s.cmd != nil && s.cmd.Process != nil {
		watcher = fmt.Sprintf("watcher:%d", s.cmd.Process.Pid)
		if s.Exited() {
			if exit, ok := processExit(s.cmd); ok {
				others = append(others, exit)
			}
		}
	}
	return formatDLLInitFailureReport(s.log.String(), watcher, others)
}

// 아래 두 형식은 internal/process/process_windows.go가 DEBUG 수준으로 남기는 로그 줄이다.
var (
	watcherSignalLine = regexp.MustCompile(`sent CTRL\+BREAK to process group (\d+) at ([^\s)]+)(?: \(error: (.*)\))?`)
	watcherExitLine   = regexp.MustCompile(`process (\d+) exited with code 0x([0-9a-f]+) \(started ([^\s,]+), exited ([^\s)]+)\)`)
)

// unrecordedDLLInitFailure는 hook이 0xc0000142로 끝났을 때 watcher가 수준과 관계없이 남기는 오류의 일부다.
var unrecordedDLLInitFailure = "exit status " + strconv.FormatUint(statusDLLInitFailed, 10)

// formatDLLInitFailureReport는 watcherLog에서 읽은 기록과 others 가운데 0xc0000142로 끝난 프로세스가 있으면 보고를 만든다.
// 보고에는 CTRL+BREAK를 모두 담고, 종료는 0xc0000142로 끝난 프로세스와 CTRL+BREAK를 받은 PID의 것만 담는다.
func formatDLLInitFailureReport(watcherLog, watcher string, others []consoleEvent) string {
	events := append(watcherEvents(watcherLog, watcher), others...)
	failed, targets := map[int]bool{}, map[int]bool{}
	for _, event := range events {
		if event.signal {
			targets[event.pid] = true
		} else if event.code == statusDLLInitFailed {
			failed[event.pid] = true
		}
	}
	unrecorded := len(failed) == 0 && strings.Contains(watcherLog, unrecordedDLLInitFailure)
	if len(failed) == 0 && !unrecorded {
		return ""
	}
	slices.SortStableFunc(events, func(a, b consoleEvent) int { return a.at.Compare(b.at) })
	var report strings.Builder
	report.WriteString("STATUS_DLL_INIT_FAILED (0xc0000142): CTRL+BREAK sent in this console since the scenario started, and exits of the failed processes and of signal targets, in time order:\n")
	if unrecorded {
		fmt.Fprintf(&report, "%s logged %s without exit records; run it with --log-level=DEBUG to record exit times and the CTRL+BREAK it sends\n", watcher, unrecordedDLLInitFailure)
	}
	for _, event := range events {
		at := event.at.UTC().Format(time.RFC3339Nano)
		switch {
		case event.signal:
			fmt.Fprintf(&report, "%s CTRL+BREAK pid=%d from=%s", at, event.pid, event.from)
			if event.err != "" {
				fmt.Fprintf(&report, " error=%s", event.err)
			}
			if failed[event.pid] {
				report.WriteString(" target=failed-process")
			}
			report.WriteString("\n")
		case failed[event.pid] || targets[event.pid]:
			fmt.Fprintf(&report, "%s exit pid=%d code=%#x started=%s from=%s\n", at, event.pid, event.code, event.started.UTC().Format(time.RFC3339Nano), event.from)
		}
	}
	return report.String()
}

func watcherEvents(log, watcher string) []consoleEvent {
	var events []consoleEvent
	for _, line := range strings.Split(log, "\n") {
		if m := watcherSignalLine.FindStringSubmatch(line); m != nil {
			pid, _ := strconv.Atoi(m[1])
			at, err := time.Parse(time.RFC3339Nano, m[2])
			if err == nil {
				events = append(events, consoleEvent{at: at, signal: true, pid: pid, from: watcher, err: m[3]})
			}
		} else if m := watcherExitLine.FindStringSubmatch(line); m != nil {
			pid, _ := strconv.Atoi(m[1])
			code, _ := strconv.ParseUint(m[2], 16, 32)
			started, err1 := time.Parse(time.RFC3339Nano, m[3])
			at, err2 := time.Parse(time.RFC3339Nano, m[4])
			if err1 == nil && err2 == nil {
				events = append(events, consoleEvent{at: at, pid: pid, code: uint32(code), started: started, from: watcher})
			}
		}
	}
	return events
}
