package cmd_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	command "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain/cmd"
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

// TestShellArgsProcess는 템플릿 값 테스트가 인자를 받는 실제 프로그램으로 쓰려고 이 테스트 바이너리를
// 다시 실행한다. 테스트 플래그 뒤의 인자를 JSON 배열 한 줄로 쓴다.
func TestShellArgsProcess(t *testing.T) {
	if os.Getenv("DW_SHELL_ARGS_HELPER") != "1" {
		return
	}
	data, _ := json.Marshal(os.Args[2:])
	fmt.Println("args=" + string(data))
	os.Exit(0)
}

// 각 셸에서 명령을 이어 붙이거나 치환하는 문법이 들어 있어, 셸이 값을 명령으로 해석하면 pwned 폴더가 생긴다.
// %OS%는 cmd가 값을 다시 펼치면 Windows_NT로 바뀐다.
const hostileFileName = `a b;mkdir pwned1;$(mkdir pwned2)'&mkdir pwned3&%OS%.txt`

func argsCommand(t *testing.T, shell command.Shell, placement string) command.SingleCmd {
	t.Setenv("DW_SHELL_ARGS_HELPER", "1")
	program := fmt.Sprintf(`"%s" "-test.run=^TestShellArgsProcess$"`, os.Args[0])
	if shell.String() == "powershell" {
		program = "& " + program
	}
	return command.SingleCmd{Shell: shell, Cmd: program + " " + placement}
}

func TestTemplateValueIsNeverRunAsCommand(t *testing.T) {
	placements := map[string]string{
		"outside quotes":       `{{.Path}}`,
		"inside single quotes": `'{{.Path}}'`,
		"inside double quotes": `"{{.Path}}"`,
	}
	event := &domain.Event{Path: hostileFileName, Operation: domain.Operation(fsnotify.Create)}
	for _, shell := range testedShells(t) {
		for name, placement := range placements {
			t.Run(shell.String()+"/"+name, func(t *testing.T) {
				ctx, output := capturedContext(t)
				runDir := t.TempDir()
				cmd := command.NewManager(argsCommand(t, shell, placement))
				err := cmd.Run(ctx, domain.Path(runDir), event)
				if made, _ := filepath.Glob(filepath.Join(runDir, "pwned*")); len(made) != 0 {
					t.Fatalf("file name ran as a command and made %q; output %q", made, output())
				}
				if err != nil {
					t.Fatal(err)
				}
				if placement != `{{.Path}}` {
					return
				}
				if got, want := receivedArgs(t, output()), []string{hostileFileName}; !equalLines(got, want) {
					t.Fatalf("program received %q, want %q", got, want)
				}
			})
		}
	}
}

func receivedArgs(t *testing.T, output string) []string {
	t.Helper()
	for _, line := range hookLines(output) {
		if data, ok := strings.CutPrefix(line, "args="); ok {
			var args []string
			if err := json.Unmarshal([]byte(data), &args); err != nil {
				t.Fatal(err)
			}
			return args
		}
	}
	t.Fatalf("program printed no arguments: %q", output)
	return nil
}

func TestTemplateVariablesAreReplacedWithTheirValues(t *testing.T) {
	event := &domain.Event{Path: domain.Path(filepath.Join("dir", "b.txt")), Operation: domain.Operation(fsnotify.Write)}
	templates := `{{.Path}} {{.AbsPath}} {{.FileName}} {{.ExtName}} {{.DirPath}} {{.DirAbsPath}} {{.Event}}`
	want := []string{event.Path.String(), event.Path.Abs(), "b.txt", ".txt", "dir", event.Path.DirAbsPath(), "U"}
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			ctx, output := capturedContext(t)
			cmd := command.NewManager(argsCommand(t, shell, templates))
			if err := cmd.Run(ctx, domain.Path(t.TempDir()), event); err != nil {
				t.Fatal(err)
			}
			if got := receivedArgs(t, output()); !equalLines(got, want) {
				t.Fatalf("program received %q, want %q", got, want)
			}
		})
	}
}

// 셸이 따옴표로 감싼 인자가 \로 끝나면, 프로그램은 \"를 따옴표 글자로 읽는다. 드라이브나 공유 폴더 바로
// 아래 파일의 {{.DirPath}}가 C:\이나 \\server\share\처럼 \로 끝난다.
func TestTemplateValueEndingInBackslashIsOneArgument(t *testing.T) {
	values := []string{`C:\`, `C:\My Dir\`, `\\server\my share\`, `C:\a\\`}
	for _, shell := range testedShells(t) {
		for _, value := range values {
			t.Run(shell.String()+"/"+value, func(t *testing.T) {
				ctx, output := capturedContext(t)
				event := &domain.Event{Path: domain.Path(value), Operation: domain.Operation(fsnotify.Create)}
				cmd := command.NewManager(argsCommand(t, shell, `{{.Path}} end`))
				if err := cmd.Run(ctx, domain.Path(t.TempDir()), event); err != nil {
					t.Fatal(err)
				}
				if got, want := receivedArgs(t, output()), []string{value, "end"}; !equalLines(got, want) {
					t.Fatalf("program received %q, want %q", got, want)
				}
			})
		}
	}
}

// 확장자가 없는 파일의 {{.ExtName}}처럼 빈 값도 빈 인자 하나로 넘어가야 한다. Windows PowerShell 5.1은
// 빈 문자열 인자를 프로그램에 넘기지 않으므로 powershell은 확인하지 않는다(USAGE에 적은 제약).
func TestEmptyTemplateValueIsOneEmptyArgument(t *testing.T) {
	event := &domain.Event{Path: domain.Path(filepath.Join("dir", "noext")), Operation: domain.Operation(fsnotify.Create)}
	for _, shell := range testedShells(t) {
		if shell.String() == "powershell" {
			continue
		}
		t.Run(shell.String(), func(t *testing.T) {
			ctx, output := capturedContext(t)
			cmd := command.NewManager(argsCommand(t, shell, `{{.ExtName}} end`))
			if err := cmd.Run(ctx, domain.Path(t.TempDir()), event); err != nil {
				t.Fatal(err)
			}
			if got, want := receivedArgs(t, output()), []string{"", "end"}; !equalLines(got, want) {
				t.Fatalf("program received %q, want %q", got, want)
			}
		})
	}
}

func TestFailureNamesCommandWithTemplateValuesFilledIn(t *testing.T) {
	readMissing := map[string]string{"sh": "cat", "cmd": "type", "powershell": "Get-Content"}
	event := &domain.Event{Path: "missing b.txt", Operation: domain.Operation(fsnotify.Create)}
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			ctx, _ := capturedContext(t)
			program := readMissing[shell.String()]
			cmd := command.NewManager(command.SingleCmd{Shell: shell, Cmd: program + " {{.Path}}"})
			err := cmd.Run(ctx, domain.Path(t.TempDir()), event)
			if want := "(with values: " + program + " missing b.txt)"; err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("failure %v does not contain %q", err, want)
			}
		})
	}
}

func TestFailureWithoutTemplatesShowsOnlyTheCommand(t *testing.T) {
	ctx, _ := capturedContext(t)
	event := &domain.Event{Path: "b.txt", Operation: domain.Operation(fsnotify.Create)}
	err := command.NewManager(command.SingleCmd{Shell: command.DefaultShell(), Cmd: "exit 3"}).Run(ctx, domain.Path(t.TempDir()), event)
	if err == nil || strings.Contains(err.Error(), "with values") {
		t.Fatalf("failure of a command without templates = %v, want no filled-in command", err)
	}
}

func TestCommandListSkipsRestAfterFailureInEveryShell(t *testing.T) {
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			ctx, output := capturedContext(t)
			cmd := command.NewManager(command.MultiLineCmd{Cmds: []command.SingleCmd{
				{Shell: shell, Cmd: "exit 3"},
				{Shell: shell, Cmd: "echo after"},
			}})
			if err := cmd.Run(ctx, domain.Path(t.TempDir()), nil); err == nil {
				t.Fatal("failing first command returned no error")
			}
			if got := hookLines(output()); len(got) != 0 {
				t.Fatalf("command after the failed one ran: %q", got)
			}
		})
	}
}

func TestErrorEnvironmentReachesCommandInEveryShell(t *testing.T) {
	reference := map[string]string{
		"sh":         `echo "$DW_ERROR_KIND"`,
		"cmd":        `echo %DW_ERROR_KIND%`,
		"powershell": `Write-Output $env:DW_ERROR_KIND`,
	}
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			base, output := capturedBase(t)
			ctx := base.NewTimeoutRun(10*time.Second, []string{"DW_ERROR_KIND=event_loss"})
			defer ctx.Cancel()
			cmd := command.NewManager(command.SingleCmd{Shell: shell, Cmd: reference[shell.String()]})
			if err := cmd.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
				t.Fatal(err)
			}
			if got, want := hookLines(output()), []string{"event_loss"}; !equalLines(got, want) {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestLookupShellRejectsUnknownName(t *testing.T) {
	_, err := command.LookupShell("bash")
	if err == nil || !strings.Contains(err.Error(), `"bash"`) || !strings.Contains(err.Error(), "sh, cmd, powershell") {
		t.Fatalf("error does not name the value and the choices: %v", err)
	}
}

// schema의 shell.enum은 코드의 셸 목록을 손으로 옮겨 적은 것이라, 셸을 추가하거나 바꿀 때 함께 고쳐야 한다.
func TestSchemaListsSameShellsAsCode(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "directory-watcher.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Shell struct {
				Enum []string `json:"enum"`
			} `json:"shell"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	got, want := schema.Properties.Shell.Enum, command.ShellNames()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("schema shell.enum = %q, want %q", got, want)
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
