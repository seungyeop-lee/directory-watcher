package yaml_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	configyaml "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
)

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
