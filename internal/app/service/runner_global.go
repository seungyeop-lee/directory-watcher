package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type globalRunner struct {
	watchCtx *helper.WatcherContext

	onStartWatch  domain.Cmd
	onFinishWatch domain.Cmd
	onError       domain.Cmd

	logger helper.Logger
}

func NewGlobalRunner(commandSet domain.GlobalCommandSet, logger helper.Logger) *globalRunner {
	return &globalRunner{
		watchCtx:      helper.NewWatcherContext("[GLOBAL]"),
		onStartWatch:  commandSet.LifeCycle.OnStartWatch,
		onFinishWatch: commandSet.LifeCycle.OnFinishWatch,
		logger:        logger,
		onError:       commandSet.LifeCycle.OnError,
	}
}

func (g *globalRunner) CallOnStartWatch() {
	err := g.printInfo("CallOnStartWatch", g.onStartWatch, func() error {
		return g.onStartWatch.Run(g.watchCtx, "", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

func (g *globalRunner) CallOnFinishWatch() {
	err := g.printInfo("CallOnFinishWatch", g.onFinishWatch, func() error {
		return g.onFinishWatch.Run(g.watchCtx, "", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

func (g *globalRunner) CallOnError(err error) {
	if g.onError == nil {
		return
	}
	var fatal *FatalError
	if !errors.As(err, &fatal) {
		return
	}
	base := helper.NewWatcherContext("[ERROR HOOK]")
	defer base.Cancel()
	ctx := base.NewTimeoutRun(5*time.Second, []string{
		"DW_ERROR_TARGET=" + fatal.Path.String(),
		"DW_ERROR_KIND=event_loss",
		"DW_ERROR_CAUSE=" + fatal.Cause.Error(),
		"DW_ERROR_MESSAGE=" + fatal.Error(),
		"DW_ERROR_POSSIBLE_LOSS=true",
	})
	defer ctx.Cancel()
	if notifyErr := g.onError.Run(ctx, "", nil); notifyErr != nil {
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
