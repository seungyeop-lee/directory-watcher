package yaml_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	configyaml "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func TestDecodedOnErrorRunsStructuredCommand(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("DW_ERROR_CAUSE", "overflow 'quoted' {{.Path}} $HOME")
	t.Setenv("DW_CONFIG_NOTIFICATION_PROCESS", "1")
	// -race로 빌드한 이 바이너리는 종료 코드 0으로 끝날 때 1초를 기다리므로, hook으로 실행할 때는 끈다.
	t.Setenv("GORACE", strings.TrimSpace(os.Getenv("GORACE")+" atexit_sleep_ms=0"))
	data := fmt.Sprintf(`global:
  lifeCycle:
    onError:
      - dir: %q
        cmd: %q
watchTargets: []
`, workdir, fmt.Sprintf(`"%s" "-test.run=^TestConfigNotificationProcess$"`, os.Args[0]))
	commands := buildCommandSet(t, data)
	hook := commands.Global.LifeCycle.OnError
	if hook == nil {
		t.Fatal("decoded onError did not build a command")
	}
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	if err := hook.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(workdir, "notification.txt"))
	if err != nil {
		t.Fatal(err)
	}
	gotDir, cause, _ := strings.Cut(string(got), "\n")
	if !sameDir(t, gotDir, workdir) || cause != os.Getenv("DW_ERROR_CAUSE") {
		t.Fatalf("notification output = %q, want %q", got, workdir+"\n"+os.Getenv("DW_ERROR_CAUSE"))
	}
}

// sameDir는 두 경로가 같은 폴더인지 본다. Windows의 cmd.exe는 작업 폴더 경로의 대소문자를 디스크에 있는
// 그대로 바꿔(C:\WINDOWS → C:\Windows) 자식에게 넘기므로 문자열로 비교하지 않는다.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}

func TestDecodedLegacyConfigBuildsWithoutOnError(t *testing.T) {
	commands := buildCommandSet(t, "global:\n  lifeCycle: {}\nwatchTargets: []\n")
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnStartWatch.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestDecodedAbsentOnErrorRunsWithoutNotification(t *testing.T) {
	commands := buildCommandSet(t, "watchTargets: []\n")
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnError.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestConfigNotificationProcess(t *testing.T) {
	if os.Getenv("DW_CONFIG_NOTIFICATION_PROCESS") != "1" {
		return
	}
	workdir, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile("notification.txt", []byte(workdir+"\n"+os.Getenv("DW_ERROR_CAUSE")), 0600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestOmittedShellUsesOSDefault(t *testing.T) {
	dir := t.TempDir()
	// sh는 $0을 sh로, cmd는 %OS%를 Windows_NT로 펼치고 다른 쪽 문법은 그대로 둔다.
	commands := buildCommandSet(t, fmt.Sprintf("global:\n  lifeCycle:\n    onStartWatch: [{dir: %q, cmd: 'echo \"$0\" %%OS%%>shell.txt'}]\nwatchTargets: []\n", dir))
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnStartWatch.Run(ctx, "", nil); err != nil {
		t.Fatal(err)
	}
	want := "sh %OS%"
	if runtime.GOOS == "windows" {
		want = `"$0" Windows_NT`
	}
	if got := readTrimmed(t, filepath.Join(dir, "shell.txt")); got != want {
		t.Fatalf("hook printed %q, want %q", got, want)
	}
}

func TestConfiguredShellRunsEveryHook(t *testing.T) {
	shell, probe, want := "sh", `echo "$0" > %s.txt`, "sh"
	if runtime.GOOS == "windows" {
		shell, probe, want = "powershell", `[IO.File]::WriteAllText("$PWD\%s.txt", $PSVersionTable.PSEdition)`, "Desktop"
	}
	dir := t.TempDir()
	cmd := func(name string) string { return fmt.Sprintf("%q", fmt.Sprintf(probe, name)) }
	commands := buildCommandSet(t, fmt.Sprintf(`shell: %s
global:
  lifeCycle:
    onStartWatch: [%s]
    onBeforeChange: [{dir: %q, cmd: %s}]
    onAfterChange: [{dir: %q, cmd: [%s, %s]}]
    onFinishWatch: [%s]
    onError: [{dir: %q, cmd: %s}]
    onQueueWarning: [{dir: %q, cmd: %s}]
watchTargets:
  - path: %q
    lifeCycle:
      onStartWatch: [%s]
      onChange: [{dir: %q, cmd: [%s]}]
      onFinishWatch: [%s]
`, shell, cmd("globalStart"), dir, cmd("before"), dir, cmd("after1"), cmd("after2"), cmd("globalFinish"), dir, cmd("error"), dir, cmd("queueWarning"),
		dir, cmd("targetStart"), dir, cmd("change"), cmd("targetFinish")))
	global, target := commands.Global.LifeCycle, commands.WatchTargets[0].LifeCycle
	event := &domain.Event{Path: domain.Path(filepath.Join(dir, "b.txt"))}
	runs := []struct {
		hook  domain.Cmd
		event *domain.Event
	}{
		{global.OnStartWatch, nil}, {global.OnBeforeChange, event}, {global.OnAfterChange, event}, {global.OnFinishWatch, nil}, {global.OnError, nil}, {global.OnQueueWarning, nil},
		{target.OnStartWatch, nil}, {target.OnChange, event}, {target.OnFinishWatch, nil},
	}
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	for _, run := range runs {
		if err := run.hook.Run(ctx, domain.Path(dir), run.event); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"globalStart", "before", "after1", "after2", "globalFinish", "error", "queueWarning", "targetStart", "change", "targetFinish"} {
		if got := readTrimmed(t, filepath.Join(dir, name+".txt")); got != want {
			t.Errorf("%s ran in %q, want %q", name, got, want)
		}
	}
}

func TestUnknownShellStopsBuild(t *testing.T) {
	_, err := configyaml.NewConfig([]byte("shell: bash\nwatchTargets: []\n")).BuildCommandSet()
	if err == nil || !strings.Contains(err.Error(), `"bash"`) {
		t.Fatalf("BuildCommandSet error = %v, want an error naming bash", err)
	}
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestUnknownTemplateVariableInEventHookStopsBuild(t *testing.T) {
	const available = "available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event, IsDir"
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "old name in onChange",
			data: "watchTargets:\n  - path: other\n  - path: site\n    lifeCycle:\n      onChange: ['echo {{.Path}}', 'echo {{.DirPath}}']\n",
			want: `watchTargets[1].lifeCycle.onChange[1]: unknown template variable "DirPath"; ` + available,
		},
		{
			name: "typo in a cmd list",
			data: "watchTargets:\n  - path: site\n    lifeCycle:\n      onChange: [{dir: site, cmd: ['echo {{.Path}}', 'echo {{.FilePath}}']}]\n",
			want: `watchTargets[0].lifeCycle.onChange[0].cmd[1]: unknown template variable "FilePath"; ` + available,
		},
		{
			name: "old name in a dir command",
			data: "watchTargets:\n  - path: site\n    lifeCycle:\n      onChange: [{dir: site, cmd: 'echo {{.FileName}}'}]\n",
			want: `watchTargets[0].lifeCycle.onChange[0]: unknown template variable "FileName"; ` + available,
		},
		{
			name: "global onBeforeChange",
			data: "global:\n  lifeCycle:\n    onBeforeChange: ['echo {{.DirAbsPath}}']\nwatchTargets: []\n",
			want: `global.lifeCycle.onBeforeChange[0]: unknown template variable "DirAbsPath"; ` + available,
		},
		{
			name: "global onAfterChange",
			data: "global:\n  lifeCycle:\n    onAfterChange: ['echo {{$.Path}} {{$.Unknown}}']\nwatchTargets: []\n",
			want: `global.lifeCycle.onAfterChange[0]: unknown template variable "Unknown"; ` + available,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configyaml.NewConfig([]byte(tt.data)).BuildCommandSet()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestConditionInEventHookStopsBuild(t *testing.T) {
	data := "watchTargets:\n  - path: site\n    lifeCycle:\n      onChange: ['echo {{if eq .Event \"C\"}}created{{else}}other{{end}}', 'echo second-ran']\n"
	_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
	const want = `watchTargets[0].lifeCycle.onChange[0]: unsupported template action {{if eq .Event "C"}}created{{else}}other{{end}}; only template variables such as {{.Path}} are supported`
	if err == nil || err.Error() != want {
		t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
	}
}

// 이벤트가 없는 hook은 템플릿을 해석하지 않으므로, 쓸 수 있는 변수든 모르는 변수든 검사하지 않고 글자 그대로 실행한다.
func TestHookWithoutEventRunsTemplateTextAsIs(t *testing.T) {
	dir := t.TempDir()
	commands := buildCommandSet(t, fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch: [{dir: %q, cmd: 'echo {{.Path}}> start.txt'}]
    onFinishWatch: [{dir: %q, cmd: 'echo {{.Unknown}}> finish.txt'}]
    onError: [{dir: %q, cmd: 'echo {{.Name}}> error.txt'}]
    onQueueWarning: [{dir: %q, cmd: 'echo {{.Path}}> queueWarning.txt'}]
watchTargets:
  - path: %q
    lifeCycle:
      onStartWatch: [{dir: %q, cmd: 'echo {{.ParentPath}}> targetStart.txt'}]
      onFinishWatch: [{dir: %q, cmd: 'echo {{.FilePath}}> targetFinish.txt'}]
`, dir, dir, dir, dir, dir, dir, dir))
	global, target := commands.Global.LifeCycle, commands.WatchTargets[0].LifeCycle
	ctx := process.NewWatcherContext("config-test", process.CommandMode{})
	defer ctx.Cancel()
	for name, run := range map[string]struct {
		hook domain.Cmd
		want string
	}{
		"start":        {global.OnStartWatch, "{{.Path}}"},
		"finish":       {global.OnFinishWatch, "{{.Unknown}}"},
		"error":        {global.OnError, "{{.Name}}"},
		"queueWarning": {global.OnQueueWarning, "{{.Path}}"},
		"targetStart":  {target.OnStartWatch, "{{.ParentPath}}"},
		"targetFinish": {target.OnFinishWatch, "{{.FilePath}}"},
	} {
		if err := run.hook.Run(ctx, "", nil); err != nil {
			t.Fatal(err)
		}
		if got := readTrimmed(t, filepath.Join(dir, name+".txt")); got != run.want {
			t.Errorf("%s hook printed %q, want %q", name, got, run.want)
		}
	}
}
