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
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

func TestDecodedOnErrorRunsStructuredCommand(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("DW_ERROR_CAUSE", "overflow 'quoted' {{.Path}} $HOME")
	t.Setenv("DW_CONFIG_NOTIFICATION_PROCESS", "1")
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
	ctx := helper.NewWatcherContext("config-test")
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
	ctx := helper.NewWatcherContext("config-test")
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnStartWatch.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestDecodedAbsentOnErrorRunsWithoutNotification(t *testing.T) {
	commands := buildCommandSet(t, "watchTargets: []\n")
	ctx := helper.NewWatcherContext("config-test")
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

func buildCommandSet(t *testing.T, data string) domain.CommandSet {
	t.Helper()
	commands, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
	if err != nil {
		t.Fatal(err)
	}
	return commands
}

func TestOmittedShellUsesOSDefault(t *testing.T) {
	dir := t.TempDir()
	// sh는 $0을 sh로, cmd는 %OS%를 Windows_NT로 펼치고 다른 쪽 문법은 그대로 둔다.
	commands := buildCommandSet(t, fmt.Sprintf("global:\n  lifeCycle:\n    onStartWatch: [{dir: %q, cmd: 'echo \"$0\" %%OS%%>shell.txt'}]\nwatchTargets: []\n", dir))
	ctx := helper.NewWatcherContext("config-test")
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
watchTargets:
  - path: %q
    lifeCycle:
      onStartWatch: [%s]
      onChange: [{dir: %q, cmd: [%s]}]
      onFinishWatch: [%s]
`, shell, cmd("globalStart"), dir, cmd("before"), dir, cmd("after1"), cmd("after2"), cmd("globalFinish"), dir, cmd("error"),
		dir, cmd("targetStart"), dir, cmd("change"), cmd("targetFinish")))
	global, target := commands.Global.LifeCycle, commands.WatchTargets[0].LifeCycle
	event := &domain.Event{Path: domain.Path(filepath.Join(dir, "b.txt"))}
	runs := []struct {
		hook  domain.Cmd
		event *domain.Event
	}{
		{global.OnStartWatch, nil}, {global.OnBeforeChange, event}, {global.OnAfterChange, event}, {global.OnFinishWatch, nil}, {global.OnError, nil},
		{target.OnStartWatch, nil}, {target.OnChange, event}, {target.OnFinishWatch, nil},
	}
	ctx := helper.NewWatcherContext("config-test")
	defer ctx.Cancel()
	for _, run := range runs {
		if err := run.hook.Run(ctx, domain.Path(dir), run.event); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"globalStart", "before", "after1", "after2", "globalFinish", "error", "targetStart", "change", "targetFinish"} {
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
