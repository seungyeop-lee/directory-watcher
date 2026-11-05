package command_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

// 셸 테스트는 이 OS에서 쓸 수 있는 셸만 실행한다. sh는 macOS와 Linux에서, cmd와 powershell은
// Windows에서 확인한다.
func testedShells(t *testing.T) []command.Shell {
	t.Helper()
	names := []string{"sh"}
	if runtime.GOOS == "windows" {
		names = []string{"cmd", "powershell"}
	}
	var shells []command.Shell
	for _, name := range names {
		shell, err := command.LookupShell(name)
		if err != nil {
			t.Fatal(err)
		}
		shells = append(shells, shell)
	}
	return shells
}

// capturedContext는 훅 출력이 가는 표준 출력을 파일로 바꾼 뒤 만든 실행 context와, 그 파일의 내용을 읽는 함수를 돌려준다.
// hook이 낸 출력만 보도록, 명령을 실행하기 전에 출력하지 않는 context를 만든다.
func capturedContext(t *testing.T) (*process.WatcherContext, func() string) {
	t.Helper()
	base, output := capturedBase(t, process.CommandMode{Silent: true})
	return base.NewRun(), output
}

// capturedBase는 capturedContext와 같지만, 명령 실행 방식 mode를 정하고 NewRun이나 NewTimeoutRun으로 실행 context를
// 직접 만들 수 있게 바탕 context를 돌려준다.
func capturedBase(t *testing.T, mode process.CommandMode) (*process.WatcherContext, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = file
	base := process.NewWatcherContext("[test]", mode)
	os.Stdout = original
	t.Cleanup(func() {
		base.Cancel()
		_ = file.Close()
	})
	return base, func() string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
}

// skipRaceExitSleep은 이 테스트 바이너리를 다시 실행하는 hook에 GORACE의 atexit_sleep_ms=0을 넘긴다.
// -race로 빌드한 바이너리는 종료 코드 0으로 끝날 때 1초를 기다리므로, 넘기지 않으면 hook을 실행할 때마다 1초씩 늘어난다.
func skipRaceExitSleep(t *testing.T) {
	t.Helper()
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
}

var ansiCode = regexp.MustCompile("\x1b\\[[0-9;]*m")

// hookLines는 capturedContext가 모은 출력에서 각 줄의 출력 주체와 색상 코드를 빼고 hook이 쓴 글자만 돌려준다.
// Windows의 echo가 붙이는 줄 끝 공백도 뺀다.
func hookLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		_, text, ok := strings.Cut(ansiCode.ReplaceAllString(line, ""), " | ")
		if ok {
			lines = append(lines, strings.TrimRight(text, " \r"))
		}
	}
	return lines
}

func equalLines(got, want []string) bool {
	return strings.Join(got, "\n") == strings.Join(want, "\n") && len(got) == len(want)
}
