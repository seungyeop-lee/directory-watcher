package helper

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

type WatcherContext struct {
	ctx    context.Context
	cancel context.CancelFunc

	m    sync.Mutex
	cmds []*exec.Cmd
	w    *sync.WaitGroup
}

func NewWatcherContext() *WatcherContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &WatcherContext{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (w *WatcherContext) CancelAndNew() {
	w.m.Lock()
	defer w.m.Unlock()

	w.cancel()
	for _, cmd := range w.cmds {
		GlobalLogger.Info("wait cancel command: " + cmd.String())
		if cmd.Process != nil {
			// cmd.Wait()를 할 경우, cancel을 했는데도 불구하고 계속 대기하는 문제가 있음
			_, err := cmd.Process.Wait()
			if err != nil {
				GlobalLogger.Error("wait cancel error:" + err.Error())
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
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

	GlobalLogger.Info(fmt.Sprintf("SetWaitNumber: %v", num))

	w.w = &sync.WaitGroup{}
	w.w.Add(num)
}

func (w *WatcherContext) MarkStart() {
	if w.isWaitGroupNull() {
		return
	}

	GlobalLogger.Info("MarkStart")
	w.w.Done()
}

func (w *WatcherContext) WaitStart() {
	if w.isWaitGroupNull() {
		return
	}

	GlobalLogger.Info("WaitStart")
	w.w.Wait()
}

func (w *WatcherContext) isWaitGroupNull() bool {
	w.m.Lock()
	defer w.m.Unlock()
	return w.w == nil
}
