package process

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// CommandMode는 hook 명령을 실행하기 직전에 그 명령을 출력할지와, 명령을 실제로 실행할지를 정한다.
// 두 필드를 모두 false로 두면 명령을 출력하고 실행한다.
type CommandMode struct {
	// Silent이면 명령을 출력하지 않는다. 명령이 내는 출력은 그대로 나온다.
	Silent bool
	// DryRun이면 명령을 만들고 작업 폴더까지 확인한 뒤, 실행하지 않고 넘어간다.
	DryRun bool
}

type WatcherContext struct {
	ctx         context.Context
	cancel      context.CancelFunc
	environment []string
	cleanupTree bool
	mode        CommandMode
	// hook은 이 context로 실행하는 hook의 이름이다. 명령을 출력할 때 앞에 붙인다.
	hook string
}

func NewWatcherContext(prefix string, mode CommandMode) *WatcherContext {
	_, out, _ := term.StdStreams()
	base := context.WithValue(context.Background(), helper.FormatterKey, helper.NewFormatter(prefix, helper.NextColor(), out))
	ctx, cancel := context.WithCancel(base)
	return &WatcherContext{ctx: ctx, cancel: cancel, mode: mode}
}

func (w *WatcherContext) NewRun() *WatcherContext {
	ctx, cancel := context.WithCancel(w.ctx)
	return &WatcherContext{ctx: ctx, cancel: cancel, environment: w.environment, mode: w.mode}
}

func (w *WatcherContext) NewTimeoutRun(timeout time.Duration, environment []string) *WatcherContext {
	ctx, cancel := context.WithTimeout(w.ctx, timeout)
	return &WatcherContext{ctx: ctx, cancel: cancel, environment: environment, cleanupTree: true, mode: w.mode}
}

// WithHook은 w와 함께 취소되고 hook 이름만 hook으로 바꾼 context를 돌려준다.
func (w *WatcherContext) WithHook(hook string) *WatcherContext {
	named := *w
	named.hook = hook
	return &named
}

func (w *WatcherContext) Cancel()                  { w.cancel() }
func (w *WatcherContext) Context() context.Context { return w.ctx }
func (w *WatcherContext) Mode() CommandMode        { return w.mode }
func (w *WatcherContext) Hook() string             { return w.hook }

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
