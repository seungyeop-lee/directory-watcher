package service

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"runtime"
	"slices"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type watchTargetRunners []*watchTargetRunner

// kqueueOS는 fsnotify가 kqueue로 감시하는 운영체제인지 나타낸다.
var kqueueOS = slices.Contains([]string{"darwin", "dragonfly", "freebsd", "netbsd", "openbsd"}, runtime.GOOS)

// inotifyOS는 fsnotify가 inotify로 감시하는 운영체제인지 나타낸다.
var inotifyOS = runtime.GOOS == "linux"

// FatalError는 종료가 필요한 실패가 일어난 감시 대상과 그 원인을 보존한다.
type FatalError struct {
	Path  domain.Path
	Cause error
}

func (e *FatalError) Error() string { return fmt.Sprintf("watch target %q: %v", e.Path, e.Cause) }
func (e *FatalError) Unwrap() error { return e.Cause }

type watchTargetRunner struct {
	path domain.Path
	// watching은 감시를 시작했는지를 나타낸다. Run이 done을 닫기 전에만 쓰므로 Stop은 done을 기다린 뒤 읽는다.
	watching bool

	tree         *watchTree
	renamePairer *renamePairer
	dirBatcher   *dirBatcher
	hookBatcher  *hookBatcher
	dispatcher   *dispatcher
	removedDirs  *removedDirRecords

	logger     helper.Logger
	initErr    error
	stop       chan struct{}
	done       chan struct{}
	stopOnce   sync.Once
	finishOnce sync.Once
}

func NewWatchTargetRunner(globalCommandSet domain.GlobalCommandSet, commandSet domain.WatchTargetsCommandSet, logger helper.Logger, mode process.CommandMode) *watchTargetRunner {
	watcher, err := fsnotify.NewWatcher()
	hooks := newHookFunc(commandSet.Option.Run, commandSet.Path.Alias(), commandSet.Path, hookInfo{
		OnStartWatch:   commandSet.LifeCycle.OnStartWatch,
		OnBeforeChange: globalCommandSet.LifeCycle.OnBeforeChange,
		OnChange:       commandSet.LifeCycle.OnChange,
		OnAfterChange:  globalCommandSet.LifeCycle.OnAfterChange,
		OnFinishWatch:  commandSet.LifeCycle.OnFinishWatch,
	}, logger, mode)
	var warner *queueWarner
	if hook := globalCommandSet.LifeCycle.OnQueueWarning; hook != nil {
		warner = &queueWarner{hook: hook, target: commandSet.Path, logger: logger, mode: mode}
	}
	stop := make(chan struct{})

	result := &watchTargetRunner{
		path:        commandSet.Path,
		hookBatcher: newHookBatcher(commandSet.Option),
		dispatcher:  newDispatcher(hooks, commandSet.Option.Queue.Size, warner, stop),
		removedDirs: newRemovedDirRecords(),
		logger:      logger,
		initErr:     err,
		stop:        stop,
		done:        make(chan struct{}),
	}
	result.tree = newWatchTree(commandSet.Path, commandSet.Option.Watch, watcher, stop, logger)
	result.renamePairer = newRenamePairer(result.tree.renamedFrom, logger)
	result.dirBatcher = newDirBatcher(commandSet.Option.DirBatch, result.hookBatcher, result.tree.reAddUnder)
	return result
}

func (r *watchTargetRunner) Run() error {
	defer close(r.done)
	defer r.stopOnce.Do(func() { close(r.stop) })
	if r.initErr != nil {
		r.dispatcher.skip()
		return &FatalError{Path: r.path, Cause: r.initErr}
	}
	defer r.tree.close()
	if err := r.tree.add(); err != nil {
		r.dispatcher.skip()
		return &FatalError{Path: r.path, Cause: err}
	}
	r.dirBatcher.root = r.tree.root
	r.watching = true
	r.logger.Info("watch start: " + r.path.String())
	r.dispatcher.start()
	for {
		select {
		case <-r.stop:
			return nil
		default:
		}
		select {
		case <-r.stop:
			return nil
		case <-r.renamePairer.Timeout():
			if err := r.handleAll(r.renamePairer.release()); err != nil {
				return r.exitError(err)
			}
			if err := r.receiveRetired(); err != nil {
				return r.exitError(err)
			}
		case <-r.tree.LeakCheck():
			if err := r.tree.releaseLeakedWatches(); err != nil {
				return r.exitError(err)
			}
			if err := r.receiveRetired(); err != nil {
				return r.exitError(err)
			}
		case <-r.dirBatcher.Due():
			if err := r.enqueue(r.dirBatcher.flush()); err != nil {
				return r.exitError(err)
			}
		case <-r.hookBatcher.Due():
			if err := r.dispatcher.enqueue(r.hookBatcher.flush()); err != nil {
				return r.exitError(err)
			}
		case ev, ok := <-r.tree.Events():
			if !ok {
				return &FatalError{Path: r.path, Cause: errors.New("watcher event channel closed unexpectedly")}
			}
			if err := r.tree.reAddDirs(); err != nil {
				return &FatalError{Path: r.path, Cause: err}
			}
			if err := r.receive(ev); err != nil {
				return r.exitError(err)
			}
			if err := r.receiveRetired(); err != nil {
				return r.exitError(err)
			}
		case err, ok := <-r.tree.Errors():
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

// errStopped는 감시 루프가 r.stop이 닫혀 멈췄음을 알린다.
var errStopped = errors.New("watch stopped")

// exitError는 감시 루프를 멈추게 한 err를 Run의 반환값으로 바꾼다.
func (r *watchTargetRunner) exitError(err error) error {
	if errors.Is(err, errStopped) {
		return nil
	}
	return &FatalError{Path: r.path, Cause: err}
}

// receive는 fsnotify 이벤트 ev를 renamePairer에 넘기고, 짝을 맞춰 돌려받은 이벤트를 handleEvent로 넘긴다.
func (r *watchTargetRunner) receive(ev fsnotify.Event) error {
	return r.handleAll(r.renamePairer.receive(ev))
}

// receiveRetired는 watchTree가 갈아 끼운 예전 watcher의 남은 이벤트를 receive로 넘긴다.
func (r *watchTargetRunner) receiveRetired() error {
	for ev := range r.tree.retireWatcher() {
		if err := r.receive(ev); err != nil {
			return err
		}
	}
	return nil
}

// handleAll은 renamePairer가 돌려준 이벤트 events를 차례로 handleEvent로 넘긴다.
func (r *watchTargetRunner) handleAll(events iter.Seq[fsnotify.Event]) error {
	for ev := range events {
		if err := r.handleEvent(ev); err != nil {
			return err
		}
	}
	return nil
}

// handleEvent는 fsnotify 이벤트 ev를 감시 목록과 기록에 반영하고, 제외되지 않은 파일과 폴더의 생성, 수정,
// 삭제이면 dirBatcher로 넘긴다. 넘기는 이벤트에는 반영하기 전에 경로가 기록에 있었는지를 함께 적는다.
func (r *watchTargetRunner) handleEvent(ev fsnotify.Event) error {
	r.printEventLog(ev)
	// fsnotify의 kqueue 구현(macOS)은 감시 대상 안의 폴더 링크를 스스로 따라가, 링크가 가리키는
	// 대상 밖 경로나 빈 경로로 이벤트를 알린다. 폴더 링크는 따라가지 않으므로 hook 실행과 감시 목록
	// 반영 전에 버린다.
	if ev.Name == "" || !r.tree.contains(ev.Name) {
		return nil
	}
	if r.tree.isUnreleased(ev.Name) {
		return nil
	}
	var isDir, existed, recreated bool
	switch {
	case ev.Op.Has(fsnotify.Create):
		r.removedDirs.forgetUnder(ev.Name)
		r.renamePairer.forgetSelfRename(ev.Name)
		if r.tree.isUnreleasedDir(ev.Name) {
			// 같은 경로에 새로 생긴 폴더와 풀지 못한 옛 폴더의 감시는 같은 경로로 이벤트를 알리므로, 새 폴더를
			// 감시하기 전에 옛 감시를 없앤다.
			if err := r.tree.rebuildIfLeaked(); err != nil {
				return err
			}
		}
		_, existed = r.tree.recorded(ev.Name)
		// 폴더를 가리키는 링크는 따라가지 않으므로 폴더로 보지 않는다.
		info, err := os.Lstat(ev.Name)
		isDir = err == nil && info.IsDir()
		// 이미 사라진 항목은 기록하지 않는다. kqueue에서는 그 항목의 삭제 이벤트가 오지 않을 수 있다.
		if err == nil {
			if err := r.tree.added(ev.Name, isDir); err != nil {
				return err
			}
		}
		ev.Op = fsnotify.Create
	case ev.Op.Has(fsnotify.Remove) || ev.Op.Has(fsnotify.Rename):
		if r.renamePairer.isSelfRename(ev) {
			return nil
		}
		// 사라진 경로는 디스크에서 폴더인지 알 수 없으므로 기록에 적어 둔 폴더인지로 판단한다.
		var wasDir bool
		wasDir, existed = r.tree.recorded(ev.Name)
		if r.removedDirs.isRepeatedRemove(ev.Name, existed) {
			return nil
		}
		isDir = r.tree.unwatchTree(ev.Name) || wasDir
		if isDir {
			r.removedDirs.add(ev.Name)
		}
		if isDir && inotifyOS && ev.Op.Has(fsnotify.Rename) {
			r.renamePairer.addSelfRename(ev.Name)
		}
		// 이름을 바꾸거나 옮긴 파일의 옛 경로는 지운 파일처럼 사라졌으므로 삭제로 알린다.
		ev.Op = fsnotify.Remove
	case ev.Op.Has(fsnotify.Write):
		var stale bool
		if stale, recreated = r.removedDirs.classifyWrite(ev.Name); stale {
			return nil
		}
		// Windows는 폴더 안의 항목이 바뀌면 그 폴더의 Write를 보낸다. 폴더 안의 변경은 그 항목의 이벤트로 알린다.
		if r.tree.isDir(ev.Name) {
			return nil
		}
		// 생성 이벤트를 놓친 파일은 수정 이벤트에서 기록에 넣는다. 그래야 이번에는 생성으로, 다음부터는 수정으로 넘긴다.
		if _, existed = r.tree.recorded(ev.Name); !existed {
			if _, err := os.Lstat(ev.Name); err == nil {
				r.tree.addEntry(ev.Name, false)
			}
		}
		ev.Op = fsnotify.Write
	default:
		return nil
	}
	if r.tree.isExcluded(ev.Name) {
		return nil
	}
	return r.enqueue(r.dirBatcher.add(change{Event: domain.Event{Path: domain.Path(ev.Name), Operation: domain.Operation(ev.Op), IsDir: isDir}, existed: existed, recreated: recreated}))
}

// enqueue는 dirBatcher가 돌려준 실행 runs를 dispatcher의 대기열에 넣고, dirBatcher가 runs 뒤의 변경을 넘기다 겪은
// 오류 err를 돌려준다.
func (r *watchTargetRunner) enqueue(runs []domain.Event, err error) error {
	if enqueueErr := r.dispatcher.enqueue(runs); enqueueErr != nil {
		return enqueueErr
	}
	return err
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

func (r *watchTargetRunner) stopAccepting() {
	r.stopOnce.Do(func() { close(r.stop) })
	r.dispatcher.cancel()
}

func (r *watchTargetRunner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()
	r.stopAccepting()
	r.finishOnce.Do(func() {
		<-r.done
		r.dispatcher.finish(r.watching)
	})
}
