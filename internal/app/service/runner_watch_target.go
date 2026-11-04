package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type watchTargetRunners []*watchTargetRunner

const dispatchCapacity = 256

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
	// watchRoot는 감시 목록에 넣는 감시 대상 폴더의 경로다. path가 폴더 링크이면 링크를 푼 경로다.
	watchRoot string
	// watching은 감시를 시작했는지를 나타낸다. Run이 done을 닫기 전에만 쓰므로 Stop은 done을 기다린 뒤 읽는다.
	watching bool
	// dirsToReAdd는 감시 루프가 다음 이벤트를 받을 때 감시 목록에 다시 넣을 폴더다. reAddDirs를 본다.
	dirsToReAdd []string
	// watchedDirs는 감시 목록에 넣은 폴더다. kqueue에서는 값에 그 폴더 안 항목의 이름과 fileID를 모은다.
	// fsnotify의 kqueue 구현은 감시 중인 폴더 안의 항목마다 감시를 따로 열기 때문이다. unwatchTree를 본다.
	watchedDirs map[string]map[string]fileID
	// entryPaths는 kqueue에서 watchedDirs에 모은 항목의 경로를 fileID로 찾는다. renamedFrom을 본다.
	entryPaths map[fileID]string
	// held는 receive가 아직 handleEvent로 넘기지 않은 이벤트다. 맨 앞이 kqueue에서 이름을 바꾼 항목의
	// Create이면 renameSource에 옛 경로를 두고, holdTimeout에 그 Rename을 기다리는 시간이 끝남을 알린다.
	held         []fsnotify.Event
	renameSource string
	holdTimeout  <-chan time.Time
	// selfRenames는 inotify에서 자신의 Rename이 한 번 더 올 수 있는, 옮긴 폴더의 옛 경로와 옮긴 때다.
	selfRenames map[string]time.Time
	// unreleasedDirs는 감시를 풀지 못한 폴더의 옛 경로다. releaseLeakedWatches를 본다.
	unreleasedDirs map[string]struct{}
	// leakCheck는 releaseLeakedWatches를 실행할 때를 알린다. 실행할 일이 없으면 nil이다.
	leakCheck <-chan time.Time
	// childDirs는 watchSubDir가 false일 때 감시 대상 바로 아래에 있는 폴더다. 감시 목록에 넣지 않는 이 폴더도
	// 지우거나 옮기면 폴더 이벤트로 알리려고 적어 둔다.
	childDirs map[string]struct{}
	// batch는 폴더 이벤트를 모으는 동안 받은 이벤트이고, 모으지 않을 때는 nil이다. batchDue는 batch를 내보낼
	// 때를 알린다. collect를 본다.
	batch    *eventBatch
	batchDue <-chan time.Time
	// retiring은 rebuildWatcher가 갈아 끼운 뒤 아직 닫지 않은 예전 watcher다. retireWatcher를 본다.
	retiring *retiredWatcher

	runStartHook  func()
	runChangeHook func(event domain.Event)
	runFinishHook func()
	cancelHook    func()

	exclude         domain.ExcludePatterns
	waitMillisecond domain.Millisecond
	watchSubDir     bool
	watchFileEvent  domain.WatchEvent
	watchDirEvent   domain.WatchEvent
	dirEventWait    time.Duration
	dirEventMaxWait time.Duration
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
	}, logger)
	hookFunc := hookFuncBuilder(commandSet.Path)

	result := &watchTargetRunner{
		path:            commandSet.Path,
		runStartHook:    hookFunc.RunStartHook,
		runChangeHook:   hookFunc.RunChangeHook,
		runFinishHook:   hookFunc.RunFinishHook,
		cancelHook:      hookFunc.Cancel,
		exclude:         commandSet.Option.Exclude,
		waitMillisecond: commandSet.Option.WaitMillisecond,
		watchSubDir:     commandSet.Option.WatchSubDir,
		watchFileEvent:  commandSet.Option.WatchFileEvent,
		watchDirEvent:   commandSet.Option.WatchDirEvent,
		dirEventWait:    durationOr(commandSet.Option.DirEventWait, defaultDirEventWait),
		dirEventMaxWait: durationOr(commandSet.Option.DirEventMaxWait, defaultDirEventMaxWait),
		noWait:          commandSet.Option.NoWait,
		logger:          logger,
		watcher:         watcher,
		watchedDirs:     map[string]map[string]fileID{},
		entryPaths:      map[fileID]string{},
		selfRenames:     map[string]time.Time{},
		unreleasedDirs:  map[string]struct{}{},
		childDirs:       map[string]struct{}{},
		initErr:         err,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		handlerDone:     make(chan struct{}),
	}
	if watcher != nil {
		result.watchEvents, result.watchErrors = pumpEvents(watcher, result.stop)
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
	// rebuildWatcher가 watcher를 갈아 끼우므로, 끝날 때의 watcher를 닫는다.
	defer func() { _ = r.watcher.Close() }()
	if err := r.addDir(); err != nil {
		close(r.handlerDone)
		return &FatalError{Path: r.path, Cause: err}
	}
	r.watching = true
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
		case <-r.holdTimeout:
			if err := r.releaseHeld(events); err != nil {
				return r.exitError(err)
			}
		case <-r.leakCheck:
			r.leakCheck = nil
			if err := r.releaseLeakedWatches(events); err != nil {
				return r.exitError(err)
			}
		case <-r.batchDue:
			if err := r.flushBatch(events); err != nil {
				return r.exitError(err)
			}
		case ev, ok := <-r.watchEvents:
			if !ok {
				return &FatalError{Path: r.path, Cause: errors.New("watcher event channel closed unexpectedly")}
			}
			if err := r.reAddDirs(); err != nil {
				return &FatalError{Path: r.path, Cause: err}
			}
			if err := r.receive(ev, events); err != nil {
				return r.exitError(err)
			}
			if err := r.retireWatcher(events); err != nil {
				return r.exitError(err)
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

// errStopped는 감시 루프가 r.stop이 닫혀 멈췄음을 알린다.
var errStopped = errors.New("watch stopped")

// exitError는 감시 루프를 멈추게 한 err를 Run의 반환값으로 바꾼다.
func (r *watchTargetRunner) exitError(err error) error {
	if errors.Is(err, errStopped) {
		return nil
	}
	return &FatalError{Path: r.path, Cause: err}
}

// handleEvent는 fsnotify 이벤트 ev를 감시 목록에 반영하고, 제외되지 않은 파일과 폴더의 생성, 수정, 삭제이면
// collect로 넘긴다.
func (r *watchTargetRunner) handleEvent(ev fsnotify.Event, events chan<- domain.Event) error {
	r.printEventLog(ev)
	// fsnotify의 kqueue 구현(macOS)은 감시 대상 안의 폴더 링크를 스스로 따라가, 링크가 가리키는
	// 대상 밖 경로나 빈 경로로 이벤트를 알린다. 폴더 링크는 따라가지 않으므로 hook 실행과 감시 목록
	// 반영 전에 버린다.
	if ev.Name == "" || !domain.Path(r.watchRoot).Contains(domain.Path(ev.Name)) {
		return nil
	}
	if r.isUnreleased(ev.Name) {
		return nil
	}
	var isDir bool
	switch {
	case ev.Op.Has(fsnotify.Create):
		delete(r.selfRenames, ev.Name)
		if _, ok := r.unreleasedDirs[ev.Name]; ok {
			// 같은 경로에 새로 생긴 폴더와 풀지 못한 옛 폴더의 감시는 같은 경로로 이벤트를 알리므로, 새 폴더를
			// 감시하기 전에 옛 감시를 없앤다.
			if err := r.rebuildIfLeaked(); err != nil {
				return err
			}
		}
		r.addEntry(ev.Name)
		// 폴더를 가리키는 링크는 따라가지 않으므로 폴더로 보지 않는다.
		info, err := os.Lstat(ev.Name)
		isDir = err == nil && info.IsDir()
		if isDir && !r.watchSubDir {
			r.childDirs[ev.Name] = struct{}{}
		}
		if !r.isExcluded(ev.Name) && r.watchSubDir {
			if err := r.addDirTree(ev.Name); err != nil {
				return err
			}
		}
		ev.Op = fsnotify.Create
	case ev.Op.Has(fsnotify.Remove) || ev.Op.Has(fsnotify.Rename):
		if r.isSelfRename(ev) {
			return nil
		}
		// 사라진 경로는 디스크에서 폴더인지 알 수 없으므로 감시 목록에 적어 둔 폴더인지로 판단한다.
		_, isChildDir := r.childDirs[ev.Name]
		delete(r.childDirs, ev.Name)
		isDir = r.unwatchTree(ev.Name) || isChildDir
		if isDir && inotifyOS && ev.Op.Has(fsnotify.Rename) {
			r.addSelfRename(ev.Name)
		}
		// 이름을 바꾸거나 옮긴 파일의 옛 경로는 지운 파일처럼 사라졌으므로 삭제로 알린다.
		ev.Op = fsnotify.Remove
	case ev.Op.Has(fsnotify.Write):
		// Windows는 폴더 안의 항목이 바뀌면 그 폴더의 Write를 보낸다. 폴더 안의 변경은 그 항목의 이벤트로 알린다.
		if r.isDir(ev.Name) {
			return nil
		}
		ev.Op = fsnotify.Write
	default:
		return nil
	}
	if r.isExcluded(ev.Name) {
		return nil
	}
	return r.collect(domain.Event{Path: domain.Path(ev.Name), Operation: domain.Operation(ev.Op), IsDir: isDir}, events)
}

// dispatch는 e가 감시하는 이벤트 종류이면 hook이 받도록 events에 넣는다. events가 가득 찼으면 넣지 않고
// 오류를 돌려준다.
func (r *watchTargetRunner) dispatch(e domain.Event, events chan<- domain.Event) error {
	if !r.listens(e) {
		return nil
	}
	select {
	case <-r.stop:
		return errStopped
	case events <- e:
		return nil
	default:
		return fmt.Errorf("dispatch queue full (capacity %d): %w", dispatchCapacity, fsnotify.ErrEventOverflow)
	}
}

// listens는 e가 watchFileEvent나 watchDirEvent에서 고른 이벤트 종류인지 판단한다.
func (r *watchTargetRunner) listens(e domain.Event) bool {
	if e.IsDir {
		return r.watchDirEvent.Has(e.Operation)
	}
	return r.watchFileEvent.Has(e.Operation)
}

func (r *watchTargetRunner) addDir() error {
	if !helper.IsExist(r.path.String()) {
		return fmt.Errorf("not exist path: %s", r.path)
	}
	if err := r.resolveLinkedPath(); err != nil {
		return err
	}

	// watchSubDir이 false이면 하위 폴더를 감시하지 않는다.
	if !r.watchSubDir {
		r.logger.Debug(fmt.Sprint("add path:", r.watchRoot))
		if err := r.watcher.Add(r.watchRoot); err != nil {
			return err
		}
		r.markWatched(r.watchRoot)
		entries, err := os.ReadDir(r.watchRoot)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				r.childDirs[filepath.Join(r.watchRoot, entry.Name())] = struct{}{}
			}
		}
		return nil
	}

	return r.addDirTree(r.watchRoot)
}

// resolveLinkedPath는 path가 폴더 링크이면 링크를 푼 경로를 watchRoot로 정한다. 링크를 그대로 두면
// filepath.WalkDir은 시작 경로의 링크를 따라가지 않아 아무 폴더도 감시하지 않고, macOS의 fsnotify는 링크에
// 적힌 대상 경로로 이벤트를 알려 exclude 패턴을 맞출 상대 경로의 기준이 어긋난다.
func (r *watchTargetRunner) resolveLinkedPath() error {
	r.watchRoot = r.path.String()
	info, err := os.Lstat(r.watchRoot)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(r.watchRoot)
	if err != nil {
		return err
	}
	r.watchRoot = resolved
	return nil
}

// addDirTree는 root와 그 아래에서 감시 규칙을 통과한 폴더를 감시 목록에 넣는다.
// 감시를 시작할 때와 감시 중에 폴더가 생기거나 옮겨 올 때 모두 이 함수를 써서, 폴더가 언제
// 생겼는지와 관계없이 같은 규칙을 적용한다.
func (r *watchTargetRunner) addDirTree(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// 감시 중에 생긴 폴더는 감시 목록에 넣기 전에 지워질 수 있다.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		r.addEntry(p)
		// 폴더를 가리키는 링크는 폴더로 보지 않아 따라가지 않는다. 링크 순환과 감시 대상 밖 폴더의 감시를 막는다.
		if !d.IsDir() {
			return nil
		}
		if r.isExcluded(p) {
			return filepath.SkipDir
		}

		r.logger.Debug(fmt.Sprint("add path:", p))
		// fsnotify의 Windows 구현은 감시 대상 안의 다른 곳으로 옮긴 폴더의 감시를 옛 경로로 남겨 두고,
		// 옮긴 폴더를 Add해도 그 감시를 옛 경로 그대로 다시 쓴다. 새 경로로 알리도록 남은 감시를 먼저 푼다.
		if runtime.GOOS == "windows" {
			_ = r.watcher.Remove(p)
		}
		if err := r.watcher.Add(p); err != nil {
			return err
		}
		r.markWatched(p)
		// reAddDirs가 막는 경합은 fsnotify의 kqueue 구현에만 있다.
		if kqueueOS {
			r.dirsToReAdd = append(r.dirsToReAdd, p)
		}
		return nil
	})
}

// reAddDirs는 addDirTree가 감시 목록에 넣은 폴더를 다시 넣는다.
//
// fsnotify의 kqueue 구현은 감시 중인 폴더의 항목이 바뀔 때마다 그 폴더 안의 하위 폴더를 내부에서 다시
// 등록한다. 이 등록은 하위 폴더의 감시 설정을 먼저 읽고 나중에 등록하므로, 그 사이에 같은 하위 폴더를 Add하면
// Add 전에 읽은 설정이 Add의 등록을 덮어쓴다. 그러면 그 하위 폴더 안에 새로 생기는 파일의 이벤트가 오지 않는다.
// fsnotify는 이벤트를 한 goroutine에서 차례로 보내므로, Add 뒤에 다음 이벤트를 받았다면 Add와 겹친 내부
// 등록은 이미 끝났다. 그래서 다음 이벤트를 받을 때 같은 폴더를 한 번 더 Add해 덮어쓴 등록을 되돌린다.
// 이 두 번째 Add와 다시 겹친 내부 등록은 되돌리지 못한다.
func (r *watchTargetRunner) reAddDirs() error {
	dirs := r.dirsToReAdd
	r.dirsToReAdd = nil
	return r.reAdd(dirs)
}

// reAdd는 감시 목록에 넣은 폴더 dirs를 다시 넣는다.
func (r *watchTargetRunner) reAdd(dirs []string) error {
	for _, dir := range dirs {
		// 다시 넣기 전에 폴더가 지워졌거나 옮겨졌으면 넣지 않는다. fsnotify는 그 폴더의 Remove나 Rename을
		// 처리하며 감시를 닫는데, 닫는 사이에 Add하면 이미 닫은 감시 번호를 다시 닫아 그 번호를 새로 받은 다른
		// 파일까지 닫는다. Add는 감시 중인 경로의 존재를 확인하지 않으므로 먼저 확인한다.
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := r.watcher.Add(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// isExcluded는 감시 대상 안의 경로 name이 기본 제외 규칙이나 option.exclude에 따라 감시와 hook 실행에서
// 빠지는지 판단한다. 감시 대상 경로는 사용자가 직접 지정했으므로 빼지 않는다.
func (r *watchTargetRunner) isExcluded(name string) bool {
	if name == r.watchRoot {
		return false
	}
	if domain.Path(name).IsDefaultExcludeFile() {
		return true
	}
	if len(r.exclude) == 0 {
		return false
	}
	rel, ok := domain.Path(r.watchRoot).RelOf(domain.Path(name))
	return ok && r.exclude.Excludes(filepath.ToSlash(rel))
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
				if r.isExcluded(event.Path.String()) {
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

func (r *watchTargetRunner) stopAccepting() {
	r.stopOnce.Do(func() { close(r.stop) })
	if r.cancelHook != nil {
		r.cancelHook()
	}
}

func (r *watchTargetRunner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()
	r.stopAccepting()
	r.finishOnce.Do(func() {
		<-r.done
		<-r.handlerDone
		// 감시를 시작하지 못한 대상은 onStartWatch를 실행하지 않았으므로 그 짝인 onFinishWatch도 실행하지 않는다.
		if r.watching {
			r.runFinishHook()
		}
	})
}
