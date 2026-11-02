package helper

import (
	"context"
	"os/exec"
	"time"

	"github.com/moby/term"
)

type WatcherContext struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWatcherContext(prefix string) *WatcherContext {
	_, out, _ := term.StdStreams()
	base := context.WithValue(context.Background(), FormatterKey, NewFormatter(prefix, NextColor(), out))
	ctx, cancel := context.WithCancel(base)
	return &WatcherContext{ctx: ctx, cancel: cancel}
}

func (w *WatcherContext) NewRun() *WatcherContext {
	ctx, cancel := context.WithCancel(w.ctx)
	return &WatcherContext{ctx: ctx, cancel: cancel}
}

func (w *WatcherContext) Cancel()                  { w.cancel() }
func (w *WatcherContext) Context() context.Context { return w.ctx }

func (w *WatcherContext) ApplyGracefulShutdown(cmds []*exec.Cmd) {
	for _, cmd := range cmds {
		cmd.Cancel = func() error { return terminateProcess(cmd) }
		cmd.WaitDelay = 20 * time.Second
	}
}
