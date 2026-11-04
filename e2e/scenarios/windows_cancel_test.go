//go:build windows

package scenarios

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// windowsBreakGrace는 watcher가 취소된 hook에 첫 CTRL+BREAK를 보낸 뒤 프로세스 트리를 강제로 끝내기까지
// 기다리는 시간이다(docs/guide/running.md의 과부하와 종료).
const windowsBreakGrace = 20 * time.Second

// 명령 목록의 첫 프로그램이 실행 중일 때 취소하면, cmd.exe가 그 뒤에 시작한 다음 프로그램도 CTRL+BREAK를 받고 끝나야 한다.
func TestCanceledCommandListStopsNextProgram(t *testing.T) {
	for _, list := range []struct {
		separator       string
		firstCancelExit string
	}{
		{"&", "0"},
		{"&&", "0"},
		{"||", "1"},
	} {
		t.Run(fmt.Sprintf("separator=%s", list.separator), func(t *testing.T) {
			s := harness.NewScenario(t)
			first := unquotedHook(s, "--delay=30s", "--line=FIRST", "--cancel-exit-code="+list.firstCancelExit)
			second := unquotedHook(s, "--delay=30s", "--line=SECOND")
			s.Launch(harness.WatcherBin(), fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '%s %s %s'
`, s.Watch, first, list.separator, second))
			s.Await("first program of the command list begins", 10*time.Second, func() bool {
				return harness.DelayedHookBegun(s.Lines(), "FIRST")
			})
			stopping := time.Now()
			s.Stop()
			t.Logf("watcher exited %s after stop", time.Since(stopping))

			lines := s.Lines()
			for _, program := range []string{"FIRST", "SECOND"} {
				if !harness.DelayedHookCanceled(lines, program) || harness.CountRecord(lines, program) != 0 {
					t.Errorf("%s did not end on CTRL+BREAK: %v", program, lines)
				}
			}
			s.AwaitDelayedHooksGone()
		})
	}
}

// PowerShell hook을 시작 후 0–1000ms에 취소하면, PowerShell이 그때 시작 중이든 디버거에 들어가든 hook 프로그램이
// 끝까지 실행되지 않고 watcher가 유예 시간 안에 끝나야 한다.
func TestCanceledPowershellHookStops(t *testing.T) {
	for after := time.Duration(0); after <= time.Second; after += 100 * time.Millisecond {
		t.Run(fmt.Sprintf("after=%s", after), func(t *testing.T) {
			s := harness.NewScenario(t)
			s.Launch(harness.WatcherBin(), fmt.Sprintf(`shell: powershell
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '& %s'
`, s.Watch, unquotedHook(s, "--delay=30s", "--line=DONE")), "--log-level=INFO")
			// watcher는 이 로그를 남긴 직후에 onStartWatch를 시작한다.
			s.Await("watch start", 10*time.Second, func() bool { return strings.Contains(s.Log(), "watch start:") })
			time.Sleep(after)
			stopping := time.Now()
			s.StopWithin(windowsBreakGrace + 5*time.Second)
			t.Logf("watcher exited %s after stop; records: %v", time.Since(stopping), s.Lines())

			if harness.CountRecord(s.Lines(), "DONE") != 0 {
				t.Errorf("hook program ran to completion after cancel: %v", s.Lines())
			}
			s.AwaitDelayedHooksGone()
		})
	}
}

// unquotedHook은 s.Hook의 명령을 YAML 작은따옴표 없이 돌려준다. 명령 여러 개를 셸 문법으로 이어 붙일 때 쓴다.
func unquotedHook(s *harness.Scenario, arguments ...string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s.Hook(arguments...), "'"), "'")
}
