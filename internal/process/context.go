package process

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type WatcherContext struct {
	ctx         context.Context
	cancel      context.CancelFunc
	environment []string
	cleanupTree bool
}

func NewWatcherContext(prefix string) *WatcherContext {
	_, out, _ := term.StdStreams()
	base := context.WithValue(context.Background(), helper.FormatterKey, helper.NewFormatter(prefix, helper.NextColor(), out))
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

// shutdownGrace는 취소된 명령이 정상 종료하기를 기다리는 최대 시간이다. 이 시간이 지나도 끝나지 않으면 강제로 끝낸다.
const shutdownGrace = 20 * time.Second

func (w *WatcherContext) RunCommand(cmd *exec.Cmd) error {
	tree, err := startProcessTree(cmd, w.cleanupTree)
	if err != nil {
		return err
	}
	err = cmd.Wait()
	if !w.cleanupTree {
		releaseProcessTree(tree)
		return err
	}
	deadline := time.Now().Add(shutdownGrace)
	if limit, ok := w.ctx.Deadline(); ok && limit.Add(shutdownGrace).Before(deadline) {
		deadline = limit.Add(shutdownGrace)
	}
	cleanupErr := cleanupProcessTree(cmd, tree, deadline)
	if cleanupErr == nil {
		return err
	}
	return errors.Join(err, cleanupErr)
}

func (w *WatcherContext) ApplyGracefulShutdown(cmd *exec.Cmd) {
	if len(w.environment) != 0 {
		cmd.Env = append(cmd.Environ(), w.environment...)
	}
	cmd.Cancel = func() error { return terminateProcess(cmd) }
	cmd.WaitDelay = shutdownGrace
}
