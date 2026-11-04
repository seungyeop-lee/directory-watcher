package command_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

// shellSyntax는 셸마다 같은 일을 하는 명령이다. 각 셸의 문법 규칙대로 해석될 때만 want가 나온다.
var shellSyntax = map[string]map[string]struct {
	command string
	want    []string
}{
	"sh": {
		"quotes":       {`echo 'single  $HOME' "double  quoted"`, []string{"single  $HOME double  quoted"}},
		"substitution": {`echo "x$(echo inner)"`, []string{"xinner"}},
		"and":          {`echo first && echo second`, []string{"first", "second"}},
		"redirection":  {`echo redirected > out.txt && cat out.txt`, []string{"redirected"}},
	},
	"cmd": {
		"quotes":       {`echo "a & b"`, []string{`"a & b"`}},
		"substitution": {`for /f "delims=" %i in ('echo inner') do @echo x%i`, []string{"xinner"}},
		"and":          {`echo first&& echo second`, []string{"first", "second"}},
		"redirection":  {`echo redirected>out.txt&& type out.txt`, []string{"redirected"}},
	},
	// Windows PowerShell 5.1에는 &&가 없으므로, 앞 명령이 성공했을 때만 뒤 명령을 실행하는 if ($?)로 확인한다.
	"powershell": {
		"quotes":       {`Write-Output 'single $HOME' "double $(1+1)"`, []string{"single $HOME", "double 2"}},
		"substitution": {`Write-Output "x$(Write-Output inner)"`, []string{"xinner"}},
		"and":          {`Write-Output first; if ($?) { Write-Output second }`, []string{"first", "second"}},
		"redirection":  {`Write-Output redirected > out.txt; Get-Content out.txt`, []string{"redirected"}},
	},
}

func TestShellInterpretsCommandWithItsOwnSyntax(t *testing.T) {
	for _, shell := range testedShells(t) {
		for name, tc := range shellSyntax[shell.String()] {
			t.Run(shell.String()+"/"+name, func(t *testing.T) {
				ctx, output := capturedContext(t)
				runDir := t.TempDir()
				cmd := command.NewManager(command.SingleCmd{Shell: shell, Cmd: tc.command})
				if err := cmd.Run(ctx, domain.Path(runDir), nil); err != nil {
					t.Fatal(err)
				}
				if got := hookLines(output()); !equalLines(got, tc.want) {
					t.Fatalf("output = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

func TestLookupShellRejectsUnknownName(t *testing.T) {
	_, err := command.LookupShell("bash")
	if err == nil || !strings.Contains(err.Error(), `"bash"`) || !strings.Contains(err.Error(), "sh, cmd, powershell") {
		t.Fatalf("error does not name the value and the choices: %v", err)
	}
}

func TestWindowsShellsAreRejectedOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	for _, name := range []string{"cmd", "powershell"} {
		if _, err := command.LookupShell(name); err == nil || !strings.Contains(err.Error(), "only on Windows") {
			t.Errorf("LookupShell(%q) error = %v, want a Windows-only error", name, err)
		}
	}
}

func TestDefaultShellMatchesOS(t *testing.T) {
	want := "sh"
	if runtime.GOOS == "windows" {
		want = "cmd"
	}
	if got := command.DefaultShell().String(); got != want {
		t.Fatalf("default shell = %q, want %q", got, want)
	}
}
