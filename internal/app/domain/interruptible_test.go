package domain

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type hookCommand func(*helper.WatcherContext, *Event) error

func (f hookCommand) Run(ctx *helper.WatcherContext, _ Path, event *Event) error {
	return f(ctx, event)
}
func emptyHook(*helper.WatcherContext, *Event) error { return nil }

func TestReplacementWaitsForCanceledGeneration(t *testing.T) {
	helper.GlobalLogger = helper.NewBasicLogger(helper.LogLevelString("ERROR").GetLogLevel())
	started, canceled, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := hookCommand(func(ctx *helper.WatcherContext, _ *Event) error {
		if calls.Add(1) != 1 {
			return nil
		}
		original := ctx.Context()
		close(started)
		<-original.Done()
		close(canceled)
		<-release
		if ctx.Context() != original {
			t.Error("canceled generation received a replacement context")
		}
		close(completed)
		return original.Err()
	})
	hooks := Interruptible(true).BuildHookFuncBuilder("test", HookInfo{hookCommand(emptyHook), command, hookCommand(emptyHook), hookCommand(emptyHook), hookCommand(emptyHook)}, helper.GlobalLogger)(".")
	// The first command occupies onBeforeChange; a replacement must join it.
	hooks.RunChangeHook(Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.RunChangeHook(Event{}); close(replaced) }()
	<-canceled
	select {
	case <-replaced:
		t.Error("replacement returned before canceled generation completed")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement did not finish")
	}
	<-completed
	hooks.RunFinishHook()
}

func TestShutdownDuringReplacementPreventsNewGeneration(t *testing.T) {
	helper.GlobalLogger = helper.NewBasicLogger(helper.LogLevelString("ERROR").GetLogLevel())
	started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	command := hookCommand(func(ctx *helper.WatcherContext, _ *Event) error {
		if calls.Add(1) > 1 {
			return nil
		}
		close(started)
		<-ctx.Context().Done()
		close(canceled)
		<-release
		return ctx.Context().Err()
	})
	empty := hookCommand(emptyHook)
	hooks := Interruptible(true).BuildHookFuncBuilder("test", HookInfo{OnStartWatch: empty, OnBeforeChange: command, OnChange: empty, OnAfterChange: empty, OnFinishWatch: empty}, helper.GlobalLogger)(".")
	hooks.RunChangeHook(Event{})
	<-started
	replaced := make(chan struct{})
	go func() { hooks.RunChangeHook(Event{}); close(replaced) }()
	<-canceled
	hooks.Cancel()
	close(release)
	select {
	case <-replaced:
	case <-time.After(time.Second):
		t.Fatal("replacement remained blocked after stop")
	}
	hooks.RunFinishHook()
	if got := calls.Load(); got != 1 {
		t.Fatalf("new generation started after shutdown: %d", got)
	}
}
