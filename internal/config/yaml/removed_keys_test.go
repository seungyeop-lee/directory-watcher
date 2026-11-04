package yaml_test

import (
	"testing"

	configyaml "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
)

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
