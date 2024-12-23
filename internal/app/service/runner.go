package service

import (
	"sync"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type runner struct {
	global       *globalRunner
	watchTargets watchTargetRunners
}

func NewRunner(commandSet domain.CommandSet, logger helper.Logger) *runner {
	result := runner{}
	result.global = NewGlobalRunner(commandSet.Global, logger)
	for _, watchTarget := range commandSet.WatchTargets {
		result.watchTargets = append(result.watchTargets, NewWatchTargetRunner(commandSet.Global, watchTarget, logger))
	}
	return &result
}

func (r runner) Run() {
	r.global.CallOnStartWatch()

	for _, t := range r.watchTargets {
		go t.Run()
	}
}

func (r runner) Stop() {
	setsWg := sync.WaitGroup{}
	for _, r := range r.watchTargets {
		setsWg.Add(1)
		go r.Stop(&setsWg)
	}
	setsWg.Wait()

	r.global.CallOnFinishWatch()
}
