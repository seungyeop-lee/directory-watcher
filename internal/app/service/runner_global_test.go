package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

func TestGlobalHookFailureNamesMissingWorkingDirectory(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "missing-dir")
	missingDirCmd := command.NewManager(command.StructuredCmd{Cmd: command.SingleCmd{Shell: command.DefaultShell(), Cmd: fmt.Sprintf(`"%s" "-test.run=^$"`, os.Args[0])}, Dir: domain.Path(missingDir)})
	cases := map[string]func(*globalRunner){
		"onStartWatch":  (*globalRunner).CallOnStartWatch,
		"onFinishWatch": (*globalRunner).CallOnFinishWatch,
		"onError": func(g *globalRunner) {
			g.CallOnError(&FatalError{Path: "target", Cause: fsnotify.ErrEventOverflow})
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			logger := &notificationLog{}
			call(NewGlobalRunner(domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnStartWatch: missingDirCmd, OnFinishWatch: missingDirCmd, OnError: missingDirCmd}}, logger))
			if got := logger.String(); !strings.Contains(got, "working directory "+missingDir) {
				t.Fatalf("failure log lacks working directory: %q", got)
			}
		})
	}
}

// debugLog는 notificationLog가 모으는 ERROR 로그에 더해 DEBUG 로그를 따로 모은다.
type debugLog struct {
	notificationLog
	debug []string
}

func (l *debugLog) Debug(s string) { l.debug = append(l.debug, s) }

func TestGlobalHookDebugLogShowsCommand(t *testing.T) {
	logger := &debugLog{}
	start := command.NewManager(command.Cmds{command.SingleCmd{Shell: command.DefaultShell(), Cmd: "exit 0"}})
	finish := command.NewManager(command.EmptyCmd{})
	g := NewGlobalRunner(domain.GlobalCommandSet{LifeCycle: domain.GlobalLifeCycle{OnStartWatch: start, OnFinishWatch: finish}}, logger)

	g.CallOnStartWatch()
	g.CallOnFinishWatch()

	want := []string{
		"CallOnStartWatch start: exit 0",
		"CallOnStartWatch finished: exit 0",
		"CallOnFinishWatch start: (none)",
		"CallOnFinishWatch finished: (none)",
	}
	if strings.Join(logger.debug, "\n") != strings.Join(want, "\n") {
		t.Fatalf("debug log = %q, want %q", logger.debug, want)
	}
}
