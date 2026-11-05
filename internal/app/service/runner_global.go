package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type globalRunner struct {
	watchCtx *process.WatcherContext
	mode     process.CommandMode

	onStartWatch  domain.Cmd
	onFinishWatch domain.Cmd
	onError       domain.Cmd
	// onErrorTimeout은 onError hook을 한 번 실행할 때의 시간 제한이다. 제품에서는 notificationTimeout이고, 테스트만 줄인다.
	onErrorTimeout time.Duration

	logger helper.Logger
}

func NewGlobalRunner(commandSet domain.GlobalCommandSet, logger helper.Logger, mode process.CommandMode) *globalRunner {
	return &globalRunner{
		watchCtx:       process.NewWatcherContext("[GLOBAL]", mode),
		mode:           mode,
		onStartWatch:   commandSet.LifeCycle.OnStartWatch,
		onFinishWatch:  commandSet.LifeCycle.OnFinishWatch,
		logger:         logger,
		onError:        commandSet.LifeCycle.OnError,
		onErrorTimeout: notificationTimeout,
	}
}

func (g *globalRunner) CallOnStartWatch() {
	err := g.printInfo("CallOnStartWatch", g.onStartWatch, func() error {
		return g.onStartWatch.Run(g.watchCtx.WithHook("onStartWatch"), "", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

func (g *globalRunner) CallOnFinishWatch() {
	err := g.printInfo("CallOnFinishWatch", g.onFinishWatch, func() error {
		return g.onFinishWatch.Run(g.watchCtx.WithHook("onFinishWatch"), "", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

// notificationTimeout은 onError와 onQueueWarning hook을 한 번 실행할 때의 시간 제한이다.
const notificationTimeout = 5 * time.Second

// errorVariables는 onError hook에 치명적 오류의 정보를 넘기는 환경 변수다.
var errorVariables = []struct {
	name  string
	value func(fatal *FatalError) string
}{
	{"DW_ERROR_TARGET", func(fatal *FatalError) string { return fatal.Path.String() }},
	{"DW_ERROR_KIND", func(*FatalError) string { return "event_loss" }},
	{"DW_ERROR_CAUSE", func(fatal *FatalError) string { return fatal.Cause.Error() }},
	{"DW_ERROR_MESSAGE", func(fatal *FatalError) string { return fatal.Error() }},
	{"DW_ERROR_POSSIBLE_LOSS", func(*FatalError) string { return "true" }},
}

// ErrorVariableNames는 onError hook이 받는 환경 변수의 이름을 돌려준다.
func ErrorVariableNames() []string {
	names := make([]string, len(errorVariables))
	for i, v := range errorVariables {
		names[i] = v.name
	}
	return names
}

func (g *globalRunner) CallOnError(err error) {
	if g.onError == nil {
		return
	}
	var fatal *FatalError
	if !errors.As(err, &fatal) {
		return
	}
	base := process.NewWatcherContext("[ERROR HOOK]", g.mode)
	defer base.Cancel()
	var env []string
	for _, v := range errorVariables {
		env = append(env, v.name+"="+v.value(fatal))
	}
	ctx := base.NewTimeoutRun(g.onErrorTimeout, env)
	defer ctx.Cancel()
	if notifyErr := g.onError.Run(ctx.WithHook("onError"), "", nil); notifyErr != nil {
		if ctx.Context().Err() != nil {
			g.logger.Error(fmt.Sprintf("onError notification failed: %v: %v", ctx.Context().Err(), notifyErr))
		} else {
			g.logger.Error(fmt.Sprintf("onError notification failed: %v", notifyErr))
		}
	}
}

func (g *globalRunner) printInfo(methodName string, cmd domain.Cmd, fn func() error) error {
	g.logger.Debug(fmt.Sprintf("%s start: %v", methodName, cmd))
	err := fn()
	g.logger.Debug(fmt.Sprintf("%s finished: %v", methodName, cmd))

	return err
}
