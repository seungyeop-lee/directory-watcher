package harness

import (
	"fmt"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// 실제 watcher의 hook이 0xc0000142로 끝나면, 보고에 그 종료와 하네스와 watcher가 보낸 CTRL+BREAK가 남아야 한다.
func TestDLLInitFailureReportFromWatcher(t *testing.T) {
	s := NewScenario(t)
	// cmd.exe는 exit의 음수 인자를 32비트 종료 코드로 그대로 돌려주므로, -1073741502는 0xc0000142다.
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - 'exit -1073741502'
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
`, s.Watch, s.Hook("--delay=30s", "--line=SLOW")))
	s.Write(filepath.Join(s.Watch, "a.txt"), nil)
	s.Await("slow hook begins", 10*time.Second, func() bool { return DelayedHookBegun(s.Lines(), "SLOW") })
	s.Stop()

	report := s.dllInitFailureReport()
	watcher := s.cmd.Process.Pid
	for _, want := range []string{
		fmt.Sprintf(`(?m) exit pid=\d+ code=0xc0000142 started=\S+ from=watcher:%d$`, watcher),
		fmt.Sprintf(`(?m) CTRL\+BREAK pid=%d from=harness$`, watcher),
	} {
		if !regexp.MustCompile(want).MatchString(report) {
			t.Errorf("report does not match %s:\n%s", want, report)
		}
	}
	canceled := regexp.MustCompile(fmt.Sprintf(`(?m) CTRL\+BREAK pid=(\d+) from=watcher:%d$`, watcher)).FindStringSubmatch(report)
	if canceled == nil {
		t.Fatalf("report has no CTRL+BREAK from the watcher:\n%s", report)
	}
	if !regexp.MustCompile(fmt.Sprintf(`(?m) exit pid=%s code=0x[0-9a-f]+ started=\S+ from=watcher:%d$`, canceled[1], watcher)).MatchString(report) {
		t.Errorf("report has no exit of canceled hook %s:\n%s", canceled[1], report)
	}
}
