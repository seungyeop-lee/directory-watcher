package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

func TestTargetThatFailedToStartSkipsFinishHook(t *testing.T) {
	set := testCommandSet(filepath.Join(t.TempDir(), "missing-dir"), t.TempDir())
	var missingFinished, startedFinished, globalFinished atomic.Int32
	counter := func(calls *atomic.Int32) domain.Cmd {
		return runnerCommand(func(*process.WatcherContext, *domain.Event) error { calls.Add(1); return nil })
	}
	set.WatchTargets[0].LifeCycle.OnFinishWatch = counter(&missingFinished)
	set.WatchTargets[1].LifeCycle.OnFinishWatch = counter(&startedFinished)
	set.Global.LifeCycle.OnFinishWatch = counter(&globalFinished)
	r := NewRunner(set, newMockLogger())
	assertWatchDirectoriesRemovable(t, r.watchTargets[1])
	go r.Run()
	if err := <-r.Errors(); err == nil {
		t.Fatal("missing target path did not stop the watcher")
	}
	r.Stop()
	if missingFinished.Load() != 0 {
		t.Fatal("onFinishWatch ran for the target that failed to start watching")
	}
	if startedFinished.Load() != 1 || globalFinished.Load() != 1 {
		t.Fatalf("finish hooks of started target=%d global=%d, want 1 each", startedFinished.Load(), globalFinished.Load())
	}
}

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
