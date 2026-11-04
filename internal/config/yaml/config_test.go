package yaml_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
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
	ctx := helper.NewWatcherContext("config-test")
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

func TestGroupedOptionsBuildOption(t *testing.T) {
	// defaults는 설정 파일에 option을 적지 않았을 때의 값이다. 각 경우는 이 값에서 바뀌는 필드만 고친다.
	defaults := domain.WatchTargetsOption{
		Watch:     domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}},
		DirBatch:  domain.DirBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000},
		HookBatch: domain.HookBatchOption{WaitMillisecond: 100, MaxWaitMillisecond: 1000, Deliver: domain.DeliverLast},
		Queue:     domain.QueueOption{Size: 2048},
		Run:       domain.RunPolicy{Concurrency: 1, Interrupt: domain.InterruptNone},
	}
	with := func(change func(o *domain.WatchTargetsOption)) domain.WatchTargetsOption {
		o := defaults
		change(&o)
		return o
	}
	tests := []struct {
		name   string
		option string
		want   domain.WatchTargetsOption
	}{
		{name: "defaults", option: "{}", want: defaults},
		{
			name: "every group",
			option: `
      watch: {subDir: false, exclude: ['dist/**'], fileEvent: CU, dirEvent: D}
      dirBatch: {waitMillisecond: 300, maxWaitMillisecond: 5000}
      hookBatch: {waitMillisecond: 700, maxWaitMillisecond: 3000, deliver: lastPerPath}
      queue: {size: 4096}
      run: {concurrency: 3, interrupt: samePath}`,
			want: domain.WatchTargetsOption{
				Watch:     domain.WatchOption{SubDir: false, Exclude: domain.ExcludePatterns{"dist/**"}, FileEvent: domain.WatchEvent{Create: true, Update: true}, DirEvent: domain.WatchEvent{Delete: true}},
				DirBatch:  domain.DirBatchOption{WaitMillisecond: 300, MaxWaitMillisecond: 5000},
				HookBatch: domain.HookBatchOption{WaitMillisecond: 700, MaxWaitMillisecond: 3000, Deliver: domain.DeliverLastPerPath},
				Queue:     domain.QueueOption{Size: 4096},
				Run:       domain.RunPolicy{Concurrency: 3, Interrupt: domain.InterruptSamePath},
			},
		},
		{
			name:   "zero waits are kept",
			option: "{dirBatch: {waitMillisecond: 0, maxWaitMillisecond: 0}, hookBatch: {waitMillisecond: 0, deliver: each}}",
			want: with(func(o *domain.WatchTargetsOption) {
				o.HookBatch.WaitMillisecond, o.HookBatch.Deliver, o.DirBatch.WaitMillisecond, o.DirBatch.MaxWaitMillisecond = 0, domain.DeliverEach, 0, 0
			}),
		},
		{
			name:   "hookBatch max wait follows a wait above its default",
			option: "{hookBatch: {waitMillisecond: 2000}}",
			want: with(func(o *domain.WatchTargetsOption) {
				o.HookBatch.WaitMillisecond, o.HookBatch.MaxWaitMillisecond = 2000, 2000
			}),
		},
		{
			name:   "dirBatch max wait follows a wait above its default",
			option: "{dirBatch: {waitMillisecond: 2000}}",
			want: with(func(o *domain.WatchTargetsOption) {
				o.DirBatch.WaitMillisecond, o.DirBatch.MaxWaitMillisecond = 2000, 2000
			}),
		},
		{
			name:   "max wait keeps its default for a wait below it",
			option: "{dirBatch: {waitMillisecond: 500}, hookBatch: {waitMillisecond: 500}}",
			want:   with(func(o *domain.WatchTargetsOption) { o.HookBatch.WaitMillisecond, o.DirBatch.WaitMillisecond = 500, 500 }),
		},
		{
			name:   "queue of one",
			option: "{queue: {size: 1}}",
			want:   with(func(o *domain.WatchTargetsOption) { o.Queue.Size = 1 }),
		},
		{
			name:   "interrupt oldest",
			option: "{run: {interrupt: oldest}}",
			want:   with(func(o *domain.WatchTargetsOption) { o.Run.Interrupt = domain.InterruptOldest }),
		},
		{
			name:   "interrupt none",
			option: "{run: {concurrency: 1, interrupt: none}}",
			want:   defaults,
		},
		{
			name:   "deliver last",
			option: "{hookBatch: {deliver: last}}",
			want:   defaults,
		},
		{
			name:   "files only",
			option: "{watch: {fileEvent: CD, dirEvent: none}}",
			want: with(func(o *domain.WatchTargetsOption) {
				o.Watch.FileEvent, o.Watch.DirEvent = domain.WatchEvent{Create: true, Delete: true}, domain.WatchEvent{}
			}),
		},
		{
			name:   "folders only",
			option: "{watch: {fileEvent: none, dirEvent: D}}",
			want: with(func(o *domain.WatchTargetsOption) {
				o.Watch.FileEvent, o.Watch.DirEvent = domain.WatchEvent{}, domain.WatchEvent{Delete: true}
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCommandSet(t, "watchTargets:\n  - path: site\n    option: "+tt.option+"\n").WatchTargets[0].Option
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Option = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestInvalidHookBatchOptionStopsBuild(t *testing.T) {
	tests := []struct {
		name   string
		option string
		want   string
	}{
		{name: "unknown deliver", option: "deliver: latest", want: `option.hookBatch.deliver: unknown value "latest"; use last, lastPerPath, or each`},
		{name: "empty deliver", option: "deliver: ''", want: `option.hookBatch.deliver: unknown value ""; use last, lastPerPath, or each`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      hookBatch:\n        " + tt.option + "\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1]." + tt.want; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestMaxWaitBelowWaitStopsBuild(t *testing.T) {
	for _, group := range []string{"dirBatch", "hookBatch"} {
		t.Run(group, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      " + group + ": {waitMillisecond: 500, maxWaitMillisecond: 200}\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1].option." + group + ": maxWaitMillisecond 200 is less than waitMillisecond 500"; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestInvalidQueueSizeStopsBuild(t *testing.T) {
	for _, size := range []string{"0", "-1"} {
		t.Run(size, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      queue:\n        size: " + size + "\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1].option.queue: size " + size + " is less than 1"; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestInvalidRunOptionStopsBuild(t *testing.T) {
	tests := []struct {
		name   string
		option string
		want   string
	}{
		{name: "zero concurrency", option: "concurrency: 0", want: "option.run: concurrency 0 is less than 1"},
		{name: "negative concurrency", option: "concurrency: -2", want: "option.run: concurrency -2 is less than 1"},
		{name: "unknown interrupt", option: "interrupt: true", want: `option.run.interrupt: unknown value "true"; use none, samePath, or oldest`},
		{name: "lower case interrupt", option: "interrupt: samepath", want: `option.run.interrupt: unknown value "samepath"; use none, samePath, or oldest`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      run:\n        " + tt.option + "\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1]." + tt.want; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestRemovedOptionKeysStopBuild(t *testing.T) {
	const (
		toExclude   = `use option.watch.exclude (see USAGE "Moving from excludeDir and excludeSuffix")`
		toEvents    = `use option.watch.fileEvent and option.watch.dirEvent (see USAGE "Moving from watchEvent")`
		fromOptions = `(see USAGE "Moving from version 2 options")`
		toInterrupt = "use option.run.interrupt: oldest in place of interruptible: true, or option.run.interrupt: none in place of interruptible: false"
	)
	tests := []struct {
		name   string
		option string
		want   string
	}{
		{name: "excludeDir", option: "excludeDir: ['site/dist']", want: "option.excludeDir is no longer supported; " + toExclude},
		{name: "excludeSuffix", option: "excludeSuffix: ['~']", want: "option.excludeSuffix is no longer supported; " + toExclude},
		{name: "empty list", option: "excludeDir: []", want: "option.excludeDir is no longer supported; " + toExclude},
		{name: "null value", option: "excludeSuffix:", want: "option.excludeSuffix is no longer supported; " + toExclude},
		{name: "with watch.exclude", option: "{watch: {exclude: ['dist/**']}, excludeDir: ['site/dist']}", want: "option.excludeDir is no longer supported; " + toExclude},
		{name: "exclude outside watch", option: "exclude: ['dist/**']", want: "option.exclude is no longer supported; " + toExclude},
		{name: "watchEvent", option: "watchEvent: CUD", want: "option.watchEvent is no longer supported; " + toEvents},
		{name: "watchEvent with watch group", option: "{watch: {fileEvent: CUD}, watchEvent: ''}", want: "option.watchEvent is no longer supported; " + toEvents},
		{name: "watchSubDir", option: "watchSubDir: false", want: "option.watchSubDir is no longer supported; use option.watch.subDir " + fromOptions},
		{name: "waitMillisecond", option: "waitMillisecond: 500", want: "option.waitMillisecond is no longer supported; use option.hookBatch.waitMillisecond " + fromOptions},
		{name: "noWait true", option: "noWait: true", want: "option.noWait is no longer supported; use option.hookBatch.waitMillisecond: 0 and option.hookBatch.deliver: each in place of noWait: true, or remove noWait " + fromOptions},
		{name: "noWait false", option: "noWait: false", want: "option.noWait is no longer supported; use option.hookBatch.waitMillisecond: 0 and option.hookBatch.deliver: each in place of noWait: true, or remove noWait " + fromOptions},
		{name: "interruptible true", option: "interruptible: true", want: "option.interruptible is no longer supported; " + toInterrupt + " " + fromOptions},
		{name: "interruptible false", option: "interruptible: false", want: "option.interruptible is no longer supported; " + toInterrupt + " " + fromOptions},
		{name: "interruptible with run group", option: "{run: {interrupt: oldest}, interruptible: true}", want: "option.interruptible is no longer supported; " + toInterrupt + " " + fromOptions},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      " + tt.option + "\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1]." + tt.want; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestInvalidWatchEventOptionStopsBuild(t *testing.T) {
	tests := []struct {
		name   string
		option string
		want   string
	}{
		{name: "empty file events", option: "fileEvent: ''", want: "option.watch.fileEvent: empty value; use C, U, D, or none"},
		{name: "unknown file event", option: "fileEvent: CX", want: "option.watch.fileEvent: X is not a file event; use C, U, D, or none"},
		{name: "lower case", option: "fileEvent: cud", want: "option.watch.fileEvent: c is not a file event; use C, U, D, or none"},
		{name: "folder update", option: "dirEvent: U", want: "option.watch.dirEvent: U is not a folder event; use C, D, or none"},
		{name: "empty folder events", option: "dirEvent: ''", want: "option.watch.dirEvent: empty value; use C, D, or none"},
		{name: "none with letters", option: "dirEvent: Cnone", want: "option.watch.dirEvent: n is not a folder event; use C, D, or none"},
		{name: "both none", option: "{fileEvent: none, dirEvent: none}", want: "option.watch: fileEvent and dirEvent are both none; onChange would never run"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := "watchTargets:\n  - path: other\n  - path: site\n    option:\n      watch:\n        " + tt.option + "\n"
			_, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
			if want := "watchTargets[1]." + tt.want; err == nil || err.Error() != want {
				t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
			}
		})
	}
}

func TestExcludeBuildsPatterns(t *testing.T) {
	commands := buildCommandSet(t, "watchTargets:\n  - path: site\n    option:\n      watch:\n        exclude: ['dist/**', '**/*.log', '!important.log']\n")
	got := commands.WatchTargets[0].Option.Watch.Exclude
	if !slices.Equal(got, domain.ExcludePatterns{"dist/**", "**/*.log", "!important.log"}) {
		t.Fatalf("Exclude = %q", got)
	}
}

func TestInvalidExcludePatternStopsBuild(t *testing.T) {
	_, err := configyaml.NewConfig([]byte("watchTargets:\n  - path: site\n    option:\n      watch:\n        exclude: ['ok', '[a']\n")).BuildCommandSet()
	want := `watchTargets[0].option.watch.exclude[1]: invalid pattern "[a"`
	if err == nil || err.Error() != want {
		t.Fatalf("BuildCommandSet error = %v, want %q", err, want)
	}
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
			data: "global:\n  lifeCycle:\n    onBeforeChange: ['echo {{if .Event}}{{.DirAbsPath}}{{end}}']\nwatchTargets: []\n",
			want: `global.lifeCycle.onBeforeChange[0]: unknown template variable "DirAbsPath"; ` + available,
		},
		{
			name: "global onAfterChange",
			data: "global:\n  lifeCycle:\n    onAfterChange: ['echo {{$.Path}} {{printf \"%s\" $.Unknown}}']\nwatchTargets: []\n",
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
	ctx := helper.NewWatcherContext("config-test")
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
