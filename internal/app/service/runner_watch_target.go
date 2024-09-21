package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
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
	watchSubDir     bool
	watchEvent      domain.WatchEvent

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
		watchSubDir:     commandSet.Option.WatchSubDir,
		watchEvent:      commandSet.Option.WatchEvent,
		logger:          logger,
		watcher:         watcher,
	}
}

func (r watchTargetRunner) Run() {
	r.callOnStartWatch()

	r.addDir()

	event := make(chan domain.Event)

	go func(evChan chan domain.Event) {
		var e struct {
			event     domain.Event
			threshold <-chan time.Time
		}
		for {
			select {
			case ev := <-evChan:
				if r.excludeSuffix.Contain(ev.Path) {
					break
				}
				e.event = ev
				e.threshold = helper.CreateThreshold(r.waitMillisecond.Duration())
			case <-e.threshold:
				r.callOnBeforeChange(e.event)
				r.callOnChange(e.event)
				r.callOnAfterChange(e.event)
			}
		}
	}(event)

	for {
		select {
		case ev := <-r.watcher.Events:
			r.printEventLog(ev)

			if ev.Op&fsnotify.Create == fsnotify.Create {
				if domain.FileName(ev.Name).IsDefaultExcludeFile() {
					break
				}

				// 생성된 리소스가 directory인 경우, 설정에 따라 감시 대상으로 추가 한다.
				if r.watchSubDir && domain.Path(ev.Name).IsDir() && !r.excludeDir.Equal(domain.Path(ev.Name)) {
					_ = r.watcher.Add(ev.Name)
				}
			} else if ev.Op&fsnotify.Remove == fsnotify.Remove {
				_ = r.watcher.Remove(ev.Name)
			}

			if r.watchEvent.IsListening(ev.Op) {
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

func (r watchTargetRunner) printEventLog(ev fsnotify.Event) {
	if ev.Op.Has(fsnotify.Create) {
		r.logger.Info(fmt.Sprintf("%s has created", ev.Name))
	}
	if ev.Op.Has(fsnotify.Write) {
		r.logger.Info(fmt.Sprintf("%s has changed", ev.Name))
	}
	if ev.Op.Has(fsnotify.Remove) {
		r.logger.Info(fmt.Sprintf("%s has removed", ev.Name))
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

	// watchSubDir이 false이면 하위 폴더를 감시하지 않는다.
	if !r.watchSubDir {
		path := r.path
		r.logger.Info(fmt.Sprint("add path:", path))
		err := r.watcher.Add(path.String())
		if err != nil {
			panic(err)
		}
	}

	err := filepath.Walk(r.path.String(), func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if r.excludeDir.Equal(domain.Path(p)) {
			return nil
		}
		if r.excludeDir.IsSubFolder(domain.Path(p)) {
			return nil
		}

		r.logger.Info(fmt.Sprint("add path:", p))
		return r.watcher.Add(p)
	})

	if err != nil {
		panic(err)
	}
}

func (r watchTargetRunner) callOnStartWatch() {
	err := r.printInfo("callOnStartWatch", r.onStartWatch, func() error {
		return r.onStartWatch.Run(r.path, nil)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnBeforeChange(event domain.Event) {
	err := r.printInfo("callOnBeforeChange", r.onBeforeChange, func() error {
		return r.onBeforeChange.Run(r.path, &event)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnChange(event domain.Event) {
	err := r.printInfo("callOnChange", r.onChange, func() error {
		return r.onChange.Run(r.path, &event)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnAfterChange(event domain.Event) {
	err := r.printInfo("callOnAfterChange", r.onAfterChange, func() error {
		return r.onAfterChange.Run(r.path, &event)
	})
	if err != nil {
		r.logger.Error(err.Error())
	}
}

func (r watchTargetRunner) callOnFinishWatch() {
	err := r.printInfo("callOnFinishWatch", r.onFinishWatch, func() error {
		return r.onFinishWatch.Run(r.path, nil)
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
