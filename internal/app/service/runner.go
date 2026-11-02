package service

import (
	"errors"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type runner struct {
	global       *globalRunner
	watchTargets watchTargetRunners
	errors       chan error
	stopOnce     sync.Once
	errorOnce    sync.Once
	done         chan struct{}
}

func NewRunner(commandSet domain.CommandSet, logger helper.Logger) *runner {
	result := runner{errors: make(chan error, 1), done: make(chan struct{})}
	result.global = NewGlobalRunner(commandSet.Global, logger)
	for _, watchTarget := range commandSet.WatchTargets {
		result.watchTargets = append(result.watchTargets, NewWatchTargetRunner(commandSet.Global, watchTarget, logger))
	}
	return &result
}

func (r *runner) Run() {
	defer close(r.done)
	r.global.CallOnStartWatch()

	var targets sync.WaitGroup
	for _, t := range r.watchTargets {
		targets.Add(1)
		go func(target *watchTargetRunner) {
			defer targets.Done()
			if err := target.Run(); err != nil {
				r.errorOnce.Do(func() {
					for _, target := range r.watchTargets {
						target.stopAccepting()
					}
					if errors.Is(err, fsnotify.ErrEventOverflow) {
						loggerMessage := fmt.Sprintf("%v; possible event loss (count unknown)", err)
						r.global.logger.Error(loggerMessage)
					} else {
						r.global.logger.Error(err.Error())
					}
					r.errors <- err
					if errors.Is(err, fsnotify.ErrEventOverflow) {
						r.global.CallOnError(err)
					}
				})
			}
		}(t)
	}
	targets.Wait()
}

func (r *runner) Errors() <-chan error { return r.errors }

func (r *runner) Stop() {
	r.stopOnce.Do(func() {
		setsWg := sync.WaitGroup{}
		for _, target := range r.watchTargets {
			setsWg.Add(1)
			go target.Stop(&setsWg)
		}
		setsWg.Wait()
		<-r.done
		r.global.CallOnFinishWatch()
	})
}
