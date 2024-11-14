package service

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type globalRunner struct {
	watchCtx *helper.WatcherContext

	onStartWatch  domain.Cmd
	onFinishWatch domain.Cmd

	logger helper.Logger
}

func NewGlobalRunner(commandSet domain.GlobalCommandSet, logger helper.Logger) *globalRunner {
	return &globalRunner{
		watchCtx:      helper.NewWatcherContext("[GLOBAL]"),
		onStartWatch:  commandSet.LifeCycle.OnStartWatch,
		onFinishWatch: commandSet.LifeCycle.OnFinishWatch,
		logger:        logger,
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

func (g *globalRunner) printInfo(methodName string, cmd domain.Cmd, fn func() error) error {
	g.logger.Info(fmt.Sprintf("%s start: %v", methodName, cmd))
	err := fn()
	g.logger.Info(fmt.Sprintf("%s finished: %v", methodName, cmd))

	return err
}
