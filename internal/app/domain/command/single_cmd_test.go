package command_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

// TestShellArgsProcess는 템플릿 값 테스트가 인자를 받는 실제 프로그램으로 쓰려고 이 테스트 바이너리를
// 다시 실행한다. 테스트 플래그 뒤의 인자를 JSON 배열 한 줄로 쓰고, 이름이 DW_로 시작하는 환경 변수를
// JSON 객체 한 줄로 쓴다.
func TestShellArgsProcess(t *testing.T) {
	if os.Getenv("DW_SHELL_ARGS_HELPER") != "1" {
		return
	}
	data, _ := json.Marshal(os.Args[2:])
	fmt.Println("args=" + string(data))
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "DW_") {
			env[name] = value
		}
	}
	data, _ = json.Marshal(env)
	fmt.Println("env=" + string(data))
	os.Exit(0)
}

// 각 셸에서 명령을 이어 붙이거나 치환하는 문법이 들어 있어, 셸이 값을 명령으로 해석하면 pwned 폴더가 생긴다.
// %OS%는 cmd가 값을 다시 펼치면 Windows_NT로 바뀐다.
const hostileFileName = `a b;mkdir pwned1;$(mkdir pwned2)'&mkdir pwned3&%OS%.txt`

func argsCommand(t *testing.T, shell command.Shell, placement string) command.SingleCmd {
	t.Setenv("DW_SHELL_ARGS_HELPER", "1")
	skipRaceExitSleep(t)
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
	var args []string
	receivedJSON(t, output, "args=", &args)
	return args
}

func receivedEnv(t *testing.T, output string) map[string]string {
	t.Helper()
	var env map[string]string
	receivedJSON(t, output, "env=", &env)
	return env
}

func receivedJSON(t *testing.T, output, prefix string, v any) {
	t.Helper()
	for _, line := range hookLines(output) {
		if data, ok := strings.CutPrefix(line, prefix); ok {
			if err := json.Unmarshal([]byte(data), v); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("program printed no %q line: %q", prefix, output)
}

// 폴더 이벤트는 {{.IsDir}}가 true이고, 이름에 점이 있어도 {{.ExtName}}이 빈 값이다.
func TestTemplateVariablesAreReplacedWithTheirValues(t *testing.T) {
	file := &domain.Event{Path: domain.Path(filepath.Join("dir", "b.txt")), Operation: domain.Operation(fsnotify.Write)}
	folder := &domain.Event{Path: domain.Path(filepath.Join("dir", "pkg.v2")), Operation: domain.Operation(fsnotify.Create), IsDir: true}
	templates := `{{.Path}} {{.AbsPath}} {{.Name}} [{{.ExtName}}] {{.ParentPath}} {{.ParentAbsPath}} {{.Event}} {{.IsDir}}`
	cases := []struct {
		name  string
		event *domain.Event
		want  []string
	}{
		{"file", file, []string{file.Path.String(), file.Path.Abs(), "b.txt", "[.txt]", "dir", filepath.Dir(file.Path.Abs()), "U", "false"}},
		{"folder", folder, []string{folder.Path.String(), folder.Path.Abs(), "pkg.v2", "[]", "dir", filepath.Dir(folder.Path.Abs()), "C", "true"}},
	}
	for _, shell := range testedShells(t) {
		for _, tc := range cases {
			t.Run(shell.String()+"/"+tc.name, func(t *testing.T) {
				ctx, output := capturedContext(t)
				cmd := command.NewManager(argsCommand(t, shell, templates))
				if err := cmd.Run(ctx, domain.Path(t.TempDir()), tc.event); err != nil {
					t.Fatal(err)
				}
				if got := receivedArgs(t, output()); !equalLines(got, tc.want) {
					t.Fatalf("program received %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// hook은 템플릿 변수와 같은 값을 환경 변수로도 받는다. 이름을 바꾸기 전의 환경 변수는 cmd와 powershell이
// 쓰는 _ARG 변수까지 받지 않는다.
func TestEventEnvironmentUsesCurrentNamesOnly(t *testing.T) {
	event := &domain.Event{Path: domain.Path(filepath.Join("dir", "b.txt")), Operation: domain.Operation(fsnotify.Write)}
	want := map[string]string{
		"DW_PATH":            event.Path.String(),
		"DW_ABS_PATH":        event.Path.Abs(),
		"DW_NAME":            "b.txt",
		"DW_EXT_NAME":        ".txt",
		"DW_PARENT_PATH":     "dir",
		"DW_PARENT_ABS_PATH": filepath.Dir(event.Path.Abs()),
		"DW_EVENT":           "U",
		"DW_IS_DIR":          "false",
	}
	removed := []string{"DW_FILE_NAME", "DW_DIR_PATH", "DW_DIR_ABS_PATH"}
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			ctx, output := capturedContext(t)
			cmd := command.NewManager(argsCommand(t, shell, ""))
			if err := cmd.Run(ctx, domain.Path(t.TempDir()), event); err != nil {
				t.Fatal(err)
			}
			env := receivedEnv(t, output())
			for name, value := range want {
				if env[name] != value {
					t.Errorf("%s = %q, want %q", name, env[name], value)
				}
			}
			for _, name := range removed {
				for _, received := range []string{name, name + "_ARG"} {
					if value, ok := env[received]; ok {
						t.Errorf("program received removed variable %s=%q", received, value)
					}
				}
			}
		})
	}
}

// 셸이 따옴표로 감싼 인자가 \로 끝나면, 프로그램은 \"를 따옴표 글자로 읽는다. 드라이브나 공유 폴더 바로
// 아래 파일의 {{.ParentPath}}가 C:\이나 \\server\share\처럼 \로 끝난다.
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
// 빈 문자열 인자를 프로그램에 넘기지 않으므로 powershell은 확인하지 않는다(docs/guide/configuration.md에 적은 제약).
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
