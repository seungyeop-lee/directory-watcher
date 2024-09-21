package service

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type globalRunner struct {
	onStartWatch  domain.Cmd
	onFinishWatch domain.Cmd

	logger Logger
}

func NewGlobalRunner(commandSet domain.GlobalCommandSet, logger Logger) *globalRunner {
	return &globalRunner{
		onStartWatch:  commandSet.LifeCycle.OnStartWatch,
		onFinishWatch: commandSet.LifeCycle.OnFinishWatch,
		logger:        logger,
	}
}

func (g globalRunner) CallOnStartWatch() {
	err := g.printInfo("CallOnStartWatch", g.onStartWatch, func() error {
		return g.onStartWatch.Run("", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

func (g globalRunner) CallOnFinishWatch() {
	err := g.printInfo("CallOnFinishWatch", g.onFinishWatch, func() error {
		return g.onFinishWatch.Run("", nil)
	})
	if err != nil {
		g.logger.Error(err.Error())
	}
}

func (g globalRunner) printInfo(methodName string, cmd domain.Cmd, fn func() error) error {
	g.logger.Info(fmt.Sprintf("%s start: %v", methodName, cmd))
	err := fn()
	g.logger.Info(fmt.Sprintf("%s finished: %v", methodName, cmd))

	return err
}
