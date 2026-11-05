//go:build windows

package scenarios

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// hook 셸은 콘솔 출력 code page로 출력을 인코딩한다. 그 code page가 UTF-8이 아니어도 hook이 출력한 비ASCII 글자가
// watcher의 출력에 UTF-8로 그대로 나오고, watcher가 끝나면 콘솔 출력 code page가 원래 값으로 돌아와야 한다.
// 모든 watcher는 실행되는 동안 함께 쓰는 콘솔의 출력 code page를 바꾸므로, 이 테스트는 병렬로 실행하지 않는다.
func TestWindowsShellNonASCIIOutput(t *testing.T) {
	const text = "한글-줄 é ✓"
	// 437에는 한글과 ✓가 없어서, hook 셸이 이 code page로 출력하면 글자가 ?로 바뀐다.
	const consoleCP = 437
	for _, shell := range []string{"cmd", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			original, err := windows.GetConsoleOutputCP()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetConsoleOutputCP(consoleCP); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = windows.SetConsoleOutputCP(original) })
			command := "'echo " + text + "'"
			if shell == "powershell" {
				command = `"Write-Output '` + text + `'"`
			}
			s := harness.NewScenario(t)
			// --silent로 명령 안내 줄을 빼서, 출력에 text가 있으면 hook이 출력한 줄이게 한다.
			s.Launch(harness.WatcherBin(), fmt.Sprintf(`shell: %s
global:
  lifeCycle:
    onStartWatch:
      - %s
watchTargets:
  - path: '%s'
`, shell, command, s.Watch), "--silent", "--log-level=INFO")
			// 전역 onStartWatch가 끝나야 감시 대상이 시작하며 이 로그를 남긴다.
			s.Await("global start hook finished", 10*time.Second, func() bool { return strings.Contains(s.Log(), "watch start:") })
			s.Stop()

			output := s.Log()
			if !utf8.ValidString(output) || !strings.Contains(output, text+"\n") {
				t.Errorf("watcher output is not UTF-8 with the hook line %q:\n%q", text, output)
			}
			if got, err := windows.GetConsoleOutputCP(); err != nil || got != consoleCP {
				t.Errorf("console output code page after watcher exit = %d (%v), want %d", got, err, consoleCP)
			}
		})
	}
}
