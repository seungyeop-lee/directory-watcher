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
