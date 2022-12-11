package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/internal/helper"
)

type watchTargetRunners []*watchTargetRunner

type watchTargetRunner struct {
	path domain.Path

	onStartWatch   domain.Cmd
	onBeforeChange domain.Cmd
	onChange       domain.Cmd
	onAfterChange  domain.Cmd
	onFinishWatch  domain.Cmd

	excludeDir      domain.Paths
	excludeSuffix   domain.PathSuffixes
	waitMillisecond domain.Millisecond

	logger  Logger
	watcher *fsnotify.Watcher
}

func NewWatchTargetRunner(globalCommandSet domain.GlobalCommandSet, commandSet domain.WatchTargetsCommandSet, logger Logger) *watchTargetRunner {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error(err.Error())
	}

	return &watchTargetRunner{
		path:            commandSet.Path,
		onStartWatch:    commandSet.LifeCycle.OnStartWatch,
		onBeforeChange:  globalCommandSet.LifeCycle.OnBeforeChange,
		onChange:        commandSet.LifeCycle.OnChange,
		onAfterChange:   globalCommandSet.LifeCycle.OnAfterChange,
		onFinishWatch:   commandSet.LifeCycle.OnFinishWatch,
		excludeDir:      commandSet.Option.ExcludeDir,
		excludeSuffix:   commandSet.Option.ExcludeSuffix,
		waitMillisecond: commandSet.Option.WaitMillisecond,
		logger:          logger,
		watcher:         watcher,
	}
}

func (r watchTargetRunner) Run() {
	r.callOnStartWatch()

	r.addDir()

	event := make(chan domain.Event)

	go func(evChan chan domain.Event) {
		var threshold <-chan time.Time
		for {
			select {
			case ev := <-evChan:
				if r.excludeSuffix.Contain(ev.Path) {
					break
				}
				threshold = helper.CreateThreshold(r.waitMillisecond.Duration())
			case <-threshold:
				r.callOnBeforeChange()
				r.callOnChange()
				r.callOnAfterChange()
			}
		}
	}(event)

	for {
		select {
		case ev := <-r.watcher.Events:
			if ev.Op&fsnotify.Create == fsnotify.Create {
				r.logger.Info(fmt.Sprintf("%s has created", ev.Name))
				if domain.FileName(ev.Name).IsDefaultExcludeFile() {
					break
				}
				if domain.Path(ev.Name).IsDir() && !r.excludeDir.Equal(domain.Path(ev.Name)) {
					r.watcher.Add(ev.Name)
				}
			} else if ev.Op&fsnotify.Remove == fsnotify.Remove {
				r.logger.Info(fmt.Sprintf("%s has removed", ev.Name))
				r.watcher.Remove(ev.Name)
			}
			if ev.Op&fsnotify.Create == fsnotify.Create || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Remove == fsnotify.Remove {
				r.logger.Info(fmt.Sprintf("%s has changed", ev.Name))
				event <- domain.NewEventByFsnotify(ev)
			}
		case err := <-r.watcher.Errors:
			if v, ok := err.(*os.SyscallError); ok {
				// 인터럽트 발생은 에러로 처리하지 않는다.
				if v.Err == syscall.EINTR {
					continue
				}
				r.logger.Debug(fmt.Sprint("watcher.Error: SyscallError:", v))
			}
			r.logger.Debug(fmt.Sprint("watcher.Error:", err))
		}
	}
}

func (r watchTargetRunner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()

	r.callOnFinishWatch()
}

func (r watchTargetRunner) addDir() {
	if !helper.IsExist(r.path.String()) {
		r.logger.Error(fmt.Sprintf("not exist path: %s", r.path))
		return
	}

	err := filepath.Walk(r.path.String(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if r.excludeDir.Equal(domain.Path(path)) {
			return nil
		}
		if r.excludeDir.IsSubFolder(domain.Path(path)) {
			return nil
		}

		r.logger.Info(fmt.Sprint("add path:", path))
		return r.watcher.Add(path)
	})

	if err != nil {
		panic(err)
	}
}

func (r watchTargetRunner) callOnStartWatch() {
	err := r.printInfo("callOnStartWatch", r.onStartWatch, func() error {
		return r.onStartWatch.Run(r.path)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnBeforeChange() {
	err := r.printInfo("callOnBeforeChange", r.onBeforeChange, func() error {
		return r.onBeforeChange.Run(r.path)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnChange() {
	err := r.printInfo("callOnChange", r.onChange, func() error {
		return r.onChange.Run(r.path)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnAfterChange() {
	err := r.printInfo("callOnAfterChange", r.onAfterChange, func() error {
		return r.onAfterChange.Run(r.path)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnFinishWatch() {
	err := r.printInfo("callOnFinishWatch", r.onFinishWatch, func() error {
		return r.onFinishWatch.Run(r.path)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) printInfo(methodName string, cmd domain.Cmd, fn func() error) error {
	r.logger.Info(fmt.Sprintf("%s start: %v", methodName, cmd))
	err := fn()
	r.logger.Info(fmt.Sprintf("%s finished: %v", methodName, cmd))

	return err
}
