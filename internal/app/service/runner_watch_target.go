package service

import (
	"errors"
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

const dispatchCapacity = 256

// FatalError preserves the watched target and cause of a failure requiring shutdown.
type FatalError struct {
	Path  domain.Path
	Cause error
}

func (e *FatalError) Error() string { return fmt.Sprintf("watch target %q: %v", e.Path, e.Cause) }
func (e *FatalError) Unwrap() error { return e.Cause }

type watchTargetRunner struct {
	path domain.Path

	runStartHook  func()
	runChangeHook func(event domain.Event)
	runFinishHook func()
	cancelHook    func()

	excludeDir      domain.Paths
	excludeSuffix   domain.PathSuffixes
	waitMillisecond domain.Millisecond
	watchSubDir     bool
	watchEvent      domain.WatchEvent
	noWait          bool

	logger      helper.Logger
	watcher     *fsnotify.Watcher
	watchEvents <-chan fsnotify.Event
	watchErrors <-chan error
	initErr     error
	stop        chan struct{}
	done        chan struct{}
	handlerDone chan struct{}
	stopOnce    sync.Once
	finishOnce  sync.Once
}

func NewWatchTargetRunner(globalCommandSet domain.GlobalCommandSet, commandSet domain.WatchTargetsCommandSet, logger helper.Logger) *watchTargetRunner {
	watcher, err := fsnotify.NewWatcher()
	hookFuncBuilder := commandSet.Option.Interruptible.BuildHookFuncBuilder(commandSet.Path.Alias(), domain.HookInfo{
		OnStartWatch:   commandSet.LifeCycle.OnStartWatch,
		OnBeforeChange: globalCommandSet.LifeCycle.OnBeforeChange,
		OnChange:       commandSet.LifeCycle.OnChange,
		OnAfterChange:  globalCommandSet.LifeCycle.OnAfterChange,
		OnFinishWatch:  commandSet.LifeCycle.OnFinishWatch,
	})
	hookFunc := hookFuncBuilder(commandSet.Path)

	result := &watchTargetRunner{
		path:            commandSet.Path,
		runStartHook:    hookFunc.RunStartHook,
		runChangeHook:   hookFunc.RunChangeHook,
		runFinishHook:   hookFunc.RunFinishHook,
		cancelHook:      hookFunc.Cancel,
		excludeDir:      commandSet.Option.ExcludeDir,
		excludeSuffix:   commandSet.Option.ExcludeSuffix,
		waitMillisecond: commandSet.Option.WaitMillisecond,
		watchSubDir:     commandSet.Option.WatchSubDir,
		watchEvent:      commandSet.Option.WatchEvent,
		noWait:          commandSet.Option.NoWait,
		logger:          logger,
		watcher:         watcher,
		initErr:         err,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		handlerDone:     make(chan struct{}),
	}
	if watcher != nil {
		result.watchEvents = watcher.Events
		result.watchErrors = watcher.Errors
	}
	return result
}

func (r *watchTargetRunner) Run() error {
	defer close(r.done)
	defer r.stopOnce.Do(func() { close(r.stop) })
	if r.initErr != nil {
		close(r.handlerDone)
		return &FatalError{Path: r.path, Cause: r.initErr}
	}
	defer r.watcher.Close()
	if err := r.addDir(); err != nil {
		close(r.handlerDone)
		return &FatalError{Path: r.path, Cause: err}
	}
	r.logger.Info("watch start: " + r.path.String())
	events := make(chan domain.Event, dispatchCapacity)
	defer func() {
		r.stopOnce.Do(func() { close(r.stop) })
		close(events)
	}()
	go func() {
		defer close(r.handlerDone)
		r.runStartHook()
		r.selectEventHandler()(events)
	}()
	for {
		select {
		case <-r.stop:
			return nil
		default:
		}
		select {
		case <-r.stop:
			return nil
		case ev, ok := <-r.watchEvents:
			if !ok {
				return &FatalError{Path: r.path, Cause: errors.New("watcher event channel closed unexpectedly")}
			}
			r.printEventLog(ev)
			if ev.Op.Has(fsnotify.Create) {
				if domain.FileName(ev.Name).IsDefaultExcludeFile() {
					continue
				}
				if r.watchSubDir && domain.Path(ev.Name).IsDir() && !r.excludeDir.Equal(domain.Path(ev.Name)) {
					if err := r.watcher.Add(ev.Name); err != nil {
						return &FatalError{Path: r.path, Cause: err}
					}
				}
			} else if ev.Op.Has(fsnotify.Remove) {
				_ = r.watcher.Remove(ev.Name)
			}
			if !r.watchEvent.IsListening(ev) || r.excludeSuffix.Contain(domain.Path(ev.Name)) {
				continue
			}
			select {
			case <-r.stop:
				return nil
			case events <- domain.NewEventByFsnotify(ev):
			default:
				return &FatalError{Path: r.path, Cause: fmt.Errorf("dispatch queue full (capacity %d): %w", dispatchCapacity, fsnotify.ErrEventOverflow)}
			}
		case err, ok := <-r.watchErrors:
			if !ok {
				return &FatalError{Path: r.path, Cause: errors.New("watcher error channel closed unexpectedly")}
			}
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				return &FatalError{Path: r.path, Cause: err}
			}
		}
	}
}

func (r *watchTargetRunner) addDir() error {
	if !helper.IsExist(r.path.String()) {
		return fmt.Errorf("not exist path: %s", r.path)
	}

	// watchSubDir이 false이면 하위 폴더를 감시하지 않는다.
	if !r.watchSubDir {
		path := r.path
		r.logger.Debug(fmt.Sprint("add path:", path))
		err := r.watcher.Add(path.String())
		if err != nil {
			return err
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

		r.logger.Debug(fmt.Sprint("add path:", p))
		return r.watcher.Add(p)
	})

	return err
}

func (r *watchTargetRunner) selectEventHandler() func(chan domain.Event) {
	return func(input chan domain.Event) {
		wait := r.waitMillisecond.Duration()
		if wait == 0 {
			wait = 100 * time.Millisecond
		}
		var timer *time.Timer
		if !r.noWait {
			timer = time.NewTimer(time.Hour)
			if !timer.Stop() {
				<-timer.C
			}
			defer timer.Stop()
		}
		var threshold <-chan time.Time
		var latest domain.Event
		for {
			select {
			case <-r.stop:
				return
			default:
			}
			select {
			case <-r.stop:
				return
			case event, ok := <-input:
				if !ok {
					return
				}
				if r.excludeSuffix.Contain(event.Path) {
					continue
				}
				if r.noWait {
					select {
					case <-r.stop:
						return
					default:
					}
					r.runChangeHook(event)
					continue
				}
				latest = event
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(wait)
				threshold = timer.C
			case <-threshold:
				threshold = nil
				select {
				case <-r.stop:
					return
				default:
				}
				r.runChangeHook(latest)
			}
		}
	}
}

func (r *watchTargetRunner) printEventLog(ev fsnotify.Event) {
	r.logger.Debug(fmt.Sprintf("event: %s", ev.String()))
	if ev.Op.Has(fsnotify.Create) {
		r.logger.Debug(fmt.Sprintf("%s has created", ev.Name))
	}
	if ev.Op.Has(fsnotify.Write) {
		r.logger.Debug(fmt.Sprintf("%s has modified", ev.Name))
	}
	if ev.Op.Has(fsnotify.Remove) {
		r.logger.Debug(fmt.Sprintf("%s has removed", ev.Name))
	}
}

func (r *watchTargetRunner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()
	r.stopOnce.Do(func() { close(r.stop) })
	r.finishOnce.Do(func() {
		if r.cancelHook != nil {
			r.cancelHook()
		}
		<-r.done
		<-r.handlerDone
		r.runFinishHook()
	})
}
