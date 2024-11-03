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

	runStartHook  func()
	runChangeHook func(event domain.Event)
	runFinishHook func()

	excludeDir      domain.Paths
	excludeSuffix   domain.PathSuffixes
	waitMillisecond domain.Millisecond
	watchSubDir     bool
	watchEvent      domain.WatchEvent
	noWait          bool

	logger  helper.Logger
	watcher *fsnotify.Watcher
}

func NewWatchTargetRunner(globalCommandSet domain.GlobalCommandSet, commandSet domain.WatchTargetsCommandSet, logger helper.Logger) *watchTargetRunner {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error(err.Error())
	}
	hookFuncBuilder := commandSet.Option.Interruptible.BuildHookFuncBuilder(domain.HookInfo{
		OnStartWatch:   commandSet.LifeCycle.OnStartWatch,
		OnBeforeChange: globalCommandSet.LifeCycle.OnBeforeChange,
		OnChange:       commandSet.LifeCycle.OnChange,
		OnAfterChange:  globalCommandSet.LifeCycle.OnAfterChange,
		OnFinishWatch:  commandSet.LifeCycle.OnFinishWatch,
	})
	hookFunc := hookFuncBuilder(commandSet.Path)

	return &watchTargetRunner{
		path:            commandSet.Path,
		runStartHook:    hookFunc.RunStartHook,
		runChangeHook:   hookFunc.RunChangeHook,
		runFinishHook:   hookFunc.RunFinishHook,
		excludeDir:      commandSet.Option.ExcludeDir,
		excludeSuffix:   commandSet.Option.ExcludeSuffix,
		waitMillisecond: commandSet.Option.WaitMillisecond,
		watchSubDir:     commandSet.Option.WatchSubDir,
		watchEvent:      commandSet.Option.WatchEvent,
		noWait:          commandSet.Option.NoWait,
		logger:          logger,
		watcher:         watcher,
	}
}

func (r watchTargetRunner) Run() {
	r.runStartHook()

	r.addDir()

	event := make(chan domain.Event)

	go r.selectEventHandler()(event)

	for {
		select {
		case ev := <-r.watcher.Events:
			r.printEventLog(ev)

			if ev.Op.Has(fsnotify.Create) {
				if domain.FileName(ev.Name).IsDefaultExcludeFile() {
					break
				}

				// 생성된 리소스가 directory인 경우, 설정에 따라 감시 대상으로 추가 한다.
				if r.watchSubDir && domain.Path(ev.Name).IsDir() && !r.excludeDir.Equal(domain.Path(ev.Name)) {
					_ = r.watcher.Add(ev.Name)
				}
			} else if ev.Op.Has(fsnotify.Remove) {
				_ = r.watcher.Remove(ev.Name)
			}

			if r.watchEvent.IsListening(ev) {
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

func (r watchTargetRunner) selectEventHandler() func(evChan chan domain.Event) {
	if r.noWait {
		return func(evChan chan domain.Event) {
			for e := range evChan {
				if r.excludeSuffix.Contain(e.Path) {
					continue
				}
				r.runChangeHook(e)
			}
		}
	} else {
		return func(evChan chan domain.Event) {
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
					r.runChangeHook(e.event)
				}
			}
		}
	}
}

func (r watchTargetRunner) printEventLog(ev fsnotify.Event) {
	r.logger.Debug(fmt.Sprintf("event: %s", ev.String()))
	if ev.Op.Has(fsnotify.Create) {
		r.logger.Info(fmt.Sprintf("%s has created", ev.Name))
	}
	if ev.Op.Has(fsnotify.Write) {
		r.logger.Info(fmt.Sprintf("%s has modified", ev.Name))
	}
	if ev.Op.Has(fsnotify.Remove) {
		r.logger.Info(fmt.Sprintf("%s has removed", ev.Name))
	}
}

func (r watchTargetRunner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()

	r.runFinishHook()
}
