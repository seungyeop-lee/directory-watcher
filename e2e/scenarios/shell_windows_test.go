//go:build windows

package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestPowershellEventVariables(t *testing.T) {
	t.Parallel()
	s := harness.NewScenario(t)
	config := "shell: powershell\n" + strings.ReplaceAll(goldenConfig(s), "- '\"", "- '& \"")
	s.Start(config)
	s.AwaitLine("start hooks", 10*time.Second, "TARGET:onStartWatch")
	target := filepath.Join(s.Watch, goldenFileName)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("create hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "C"))
	s.Write(target, []byte("hello"))
	s.AwaitLine("update hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "U"))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("delete hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "D"))
	s.Stop()
	want := append(expectedSequence(target), "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
	if got := s.Lines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PowerShell hook sequence or eight event variables mismatch\nwant: %q\ngot: %q", want, got)
	}
	for _, dir := range []string{s.Work, s.Watch} {
		made, err := filepath.Glob(filepath.Join(dir, "pwned*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(made) != 0 {
			t.Fatalf("file name ran as a PowerShell command and made %q", made)
		}
	}
}

func TestWindowsShellEmptyArgument(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"cmd", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			s := harness.NewScenario(t)
			empty := s.Hook("--line", "{{.ExtName}}", "AFTER")
			end := s.Hook("--line=END")
			if shell == "powershell" {
				empty = "'& " + strings.Trim(empty, "'") + "'"
				end = "'& " + strings.Trim(end, "'") + "'"
			}
			s.Start(fmt.Sprintf(`shell: %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
`, shell, s.Watch, empty, end))
			s.Write(filepath.Join(s.Watch, "no-extension"), nil)
			s.AwaitLine("command after empty argument", 10*time.Second, "END")
			s.Stop()
			want := []string{"", "END"}
			if shell == "powershell" {
				want[0] = "AFTER"
			}
			if got := s.Lines(); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s empty argument mismatch\nwant: %q\ngot: %q", shell, want, got)
			}
		})
	}
}

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
