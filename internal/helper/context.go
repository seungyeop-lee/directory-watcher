package helper

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/moby/term"
)

type WatcherContext struct {
	baseCtx context.Context

	ctx    context.Context
	cancel context.CancelFunc

	m    sync.Mutex
	cmds []*exec.Cmd
	w    *sync.WaitGroup
}

func NewWatcherContext(prefix string) *WatcherContext {
	_, out, _ := term.StdStreams()
	ctxWithFormatter := context.WithValue(context.Background(), FormatterKey, NewFormatter(prefix, NextColor(), out))
	ctx, cancel := context.WithCancel(ctxWithFormatter)
	return &WatcherContext{
		baseCtx: ctxWithFormatter,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (w *WatcherContext) CancelAndNew() {
	w.m.Lock()
	defer w.m.Unlock()

	w.cancel()
	for _, cmd := range w.cmds {
		GlobalLogger.Debug("wait cancel command: " + cmd.String())
		if cmd.Process != nil {
			if err := postProcessForCancel(cmd); err != nil {
				GlobalLogger.Error(err.Error())
			}
		}
	}

	ctx, cancel := context.WithCancel(w.baseCtx)
	w.ctx = ctx
	w.cancel = cancel
}

func (w *WatcherContext) Context() context.Context {
	return w.ctx
}

func (w *WatcherContext) ApplyGracefulShutdown(cmds []*exec.Cmd) {
	w.m.Lock()
	defer w.m.Unlock()

	for _, c := range cmds {
		c.Cancel = func() error {
			return terminateProcess(c)
		}
		c.WaitDelay = time.Second * 20
	}
	w.cmds = cmds
}

func (w *WatcherContext) SetWaitNumber(num int) {
	w.m.Lock()
	defer w.m.Unlock()

	GlobalLogger.Debug(fmt.Sprintf("SetWaitNumber: %v", num))

	w.w = &sync.WaitGroup{}
	w.w.Add(num)
}

func (w *WatcherContext) MarkStart() {
	if w.isWaitGroupNull() {
		return
	}

	GlobalLogger.Debug("MarkStart")
	w.w.Done()
}

func (w *WatcherContext) WaitStart() {
	if w.isWaitGroupNull() {
		return
	}

	GlobalLogger.Debug("WaitStart")
	w.w.Wait()
}

func (w *WatcherContext) isWaitGroupNull() bool {
	w.m.Lock()
	defer w.m.Unlock()
	return w.w == nil
}
