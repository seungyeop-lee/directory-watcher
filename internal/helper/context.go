package helper

import (
	"context"
	"os/exec"
	"time"

	"github.com/moby/term"
)

type WatcherContext struct {
	ctx         context.Context
	cancel      context.CancelFunc
	environment []string
	cleanupTree bool
}

func NewWatcherContext(prefix string) *WatcherContext {
	_, out, _ := term.StdStreams()
	base := context.WithValue(context.Background(), FormatterKey, NewFormatter(prefix, NextColor(), out))
	ctx, cancel := context.WithCancel(base)
	return &WatcherContext{ctx: ctx, cancel: cancel}
}

func (w *WatcherContext) NewRun() *WatcherContext {
	ctx, cancel := context.WithCancel(w.ctx)
	return &WatcherContext{ctx: ctx, cancel: cancel, environment: w.environment}
}

func (w *WatcherContext) NewTimeoutRun(timeout time.Duration, environment []string) *WatcherContext {
	ctx, cancel := context.WithTimeout(w.ctx, timeout)
	return &WatcherContext{ctx: ctx, cancel: cancel, environment: environment, cleanupTree: true}
}

func (w *WatcherContext) Cancel()                  { w.cancel() }
func (w *WatcherContext) Context() context.Context { return w.ctx }

func (w *WatcherContext) RunCommand(cmd *exec.Cmd) error {
	err := cmd.Run()
	if w.cleanupTree && cmd.Process != nil {
		deadline := time.Now().Add(20 * time.Second)
		if limit, ok := w.ctx.Deadline(); ok && limit.Add(20*time.Second).Before(deadline) {
			deadline = limit.Add(20 * time.Second)
		}
		cleanupProcessTree(cmd, deadline)
	}
	return err
}

func (w *WatcherContext) ApplyGracefulShutdown(cmds []*exec.Cmd) {
	for _, cmd := range cmds {
		if len(w.environment) != 0 {
			cmd.Env = append(cmd.Environ(), w.environment...)
		}
		cmd.Cancel = func() error { return terminateProcess(cmd) }
		cmd.WaitDelay = 20 * time.Second
	}
}
