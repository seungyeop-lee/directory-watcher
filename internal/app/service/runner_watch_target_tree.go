package service

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

const (
	// leakCheckDelay는 폴더의 감시를 풀지 못한 뒤 releaseLeakedWatches를 실행하기까지 기다리는 시간이다.
	// 지운 폴더의 감시는 fsnotify가 스스로 풀므로, 그동안 기다려 지운 폴더 때문에 watcher를 다시 만들지 않는다.
	leakCheckDelay = 100 * time.Millisecond
	// staleWatchRetries는 addWatch가 bad file descriptor로 실패한 Add를 다시 하는 최대 횟수이고,
	// staleWatchRetryDelay는 다시 하기 전에 기다리는 시간이다.
	staleWatchRetries    = 3
	staleWatchRetryDelay = time.Millisecond
)

// watchTree는 감시 대상의 감시 목록과 그 안 항목의 기록을 가지고, 감시를 풀지 못한 폴더가 남으면 watcher를 새로
// 만들어 갈아 끼운다. 감시 루프의 고루틴에서만 부른다.
//
// watcher를 갈아 끼우면 Events와 Errors가 새 watcher의 채널을 돌려주므로, 감시 루프는 기다릴 때마다 두 메서드로
// 채널을 다시 받는다.
type watchTree struct {
	path    domain.Path
	subDir  bool
	exclude domain.ExcludePatterns
	logger  helper.Logger
	// stop은 감시 루프가 끝났음을 알린다. pumpEvents를 본다.
	stop <-chan struct{}

	// root는 감시 목록에 넣는 감시 대상 폴더의 경로다. path가 폴더 링크이면 링크를 푼 경로다. add가 정한다.
	root    string
	watcher *fsnotify.Watcher
	events  <-chan fsnotify.Event
	errs    <-chan error
	// dirsToReAdd는 감시 루프가 다음 이벤트를 받을 때 감시 목록에 다시 넣을 폴더다. reAddDirs를 본다.
	dirsToReAdd []string
	// watchedDirs는 감시 목록에 넣은 폴더이고, 값은 그 폴더 안 항목의 이름별 기록이다. recorded를 본다.
	watchedDirs map[string]map[string]entryInfo
	// entryPaths는 kqueue에서 watchedDirs에 모은 항목의 경로를 fileID로 찾는다. renamedFrom을 본다.
	entryPaths map[fileID]string
	// unreleasedDirs는 감시를 풀지 못한 폴더의 옛 경로다. releaseLeakedWatches를 본다.
	unreleasedDirs map[string]struct{}
	// leakCheck는 releaseLeakedWatches를 실행할 때를 알린다. 실행할 일이 없으면 nil이다.
	leakCheck <-chan time.Time
	// retiring은 rebuildWatcher가 갈아 끼운 뒤 아직 닫지 않은 예전 watcher다. retireWatcher를 본다.
	retiring *retiredWatcher
}

// retiredWatcher는 rebuildWatcher가 닫은 예전 watcher의 이벤트 채널과, 그 watcher에서 감시를 풀지 못한 폴더의
// 경로다.
type retiredWatcher struct {
	events     <-chan fsnotify.Event
	unreleased []string
}

// newWatchTree는 watcher로 path를 감시하는 watchTree를 만든다. watcher가 nil이면 감시하지 않는다.
func newWatchTree(path domain.Path, option domain.WatchOption, watcher *fsnotify.Watcher, stop <-chan struct{}, logger helper.Logger) *watchTree {
	t := &watchTree{
		path:           path,
		subDir:         option.SubDir,
		exclude:        option.Exclude,
		logger:         logger,
		stop:           stop,
		watcher:        watcher,
		watchedDirs:    map[string]map[string]entryInfo{},
		entryPaths:     map[fileID]string{},
		unreleasedDirs: map[string]struct{}{},
	}
	if watcher != nil {
		t.events, t.errs = pumpEvents(watcher, stop)
		t.errs = injectWatchErrors(path, t.errs, stop)
	}
	return t
}

// Events는 지금 watcher의 이벤트 채널을 돌려준다.
func (t *watchTree) Events() <-chan fsnotify.Event {
	return t.events
}

// Errors는 지금 watcher의 오류 채널을 돌려준다.
func (t *watchTree) Errors() <-chan error {
	return t.errs
}

// LeakCheck는 releaseLeakedWatches를 부를 때를 알리는 채널을 돌려준다. 할 일이 없으면 nil 채널이라 알리지 않는다.
func (t *watchTree) LeakCheck() <-chan time.Time {
	return t.leakCheck
}

// close는 지금 watcher를 닫는다. rebuildWatcher가 watcher를 갈아 끼우므로 감시 루프가 끝날 때 부른다.
func (t *watchTree) close() {
	_ = t.watcher.Close()
}

// add는 감시 대상을 감시 목록에 넣고 그 안의 항목을 기록한다. subDir가 false이면 하위 폴더를 감시하지 않는다.
func (t *watchTree) add() error {
	if !helper.IsExist(t.path.String()) {
		return fmt.Errorf("not exist path: %s", t.path)
	}
	if err := t.resolveLinkedPath(); err != nil {
		return err
	}

	if !t.subDir {
		t.logger.Debug(fmt.Sprint("add path:", t.root))
		if err := addWatch(t.watcher.Add, t.root); err != nil {
			return err
		}
		t.markWatched(t.root)
		entries, err := os.ReadDir(t.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			t.addEntry(filepath.Join(t.root, entry.Name()), entry.IsDir())
		}
		return nil
	}

	return t.addDirTree(t.root)
}

// resolveLinkedPath는 path가 폴더 링크이면 링크를 푼 경로를 root로 정한다. 링크를 그대로 두면
// filepath.WalkDir은 시작 경로의 링크를 따라가지 않아 아무 폴더도 감시하지 않고, macOS의 fsnotify는 링크에
// 적힌 대상 경로로 이벤트를 알려 exclude 패턴을 맞출 상대 경로의 기준이 어긋난다.
func (t *watchTree) resolveLinkedPath() error {
	t.root = t.path.String()
	info, err := os.Lstat(t.root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(t.root)
	if err != nil {
		return err
	}
	t.root = resolved
	return nil
}

// added는 감시 중에 생긴 항목 path를 기록에 넣고, path가 감시 규칙을 통과한 폴더이면 그 아래까지 감시 목록에
// 넣는다. subDir가 false이면 폴더를 감시 목록에 넣지 않는다.
func (t *watchTree) added(path string, isDir bool) error {
	t.addEntry(path, isDir)
	if isDir && t.subDir && !t.isExcluded(path) {
		return t.addDirTree(path)
	}
	return nil
}

// addDirTree는 root와 그 아래에서 감시 규칙을 통과한 폴더를 감시 목록에 넣는다.
// 감시를 시작할 때와 감시 중에 폴더가 생기거나 옮겨 올 때 모두 이 함수를 써서, 폴더가 언제
// 생겼는지와 관계없이 같은 규칙을 적용한다.
func (t *watchTree) addDirTree(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// d가 nil이면 훑기 시작한 폴더가 이미 없어 감시 목록에 넣지 못한 것이다. 그 폴더가 감시 대상 폴더이면
				// 감시를 시작하지 못했으므로 오류를 돌려준다. 감시 중에 생긴 폴더이면 그 폴더의 삭제 이벤트가 오지 않을
				// 수 있어 기록에서 지운다. d가 있으면 감시 목록에 넣은 폴더를 읽지 못한 것이고, 그 폴더의 삭제 이벤트가
				// 기록을 지운다.
				if d == nil {
					if p == t.root {
						return err
					}
					t.forgetEntry(p)
				}
				return nil
			}
			return err
		}
		t.addEntry(p, d.IsDir())
		// 폴더를 가리키는 링크는 폴더로 보지 않아 따라가지 않는다. 링크 순환과 감시 대상 밖 폴더의 감시를 막는다.
		if !d.IsDir() {
			return nil
		}
		if t.isExcluded(p) {
			return filepath.SkipDir
		}

		t.logger.Debug(fmt.Sprint("add path:", p))
		// fsnotify의 Windows 구현은 감시 대상 안의 다른 곳으로 옮긴 폴더의 감시를 옛 경로로 남겨 두고,
		// 옮긴 폴더를 Add해도 그 감시를 옛 경로 그대로 다시 쓴다. 새 경로로 알리도록 남은 감시를 먼저 푼다.
		if runtime.GOOS == "windows" {
			_ = t.watcher.Remove(p)
		}
		if err := addWatch(t.watcher.Add, p); err != nil {
			// 훑은 뒤 Add하기 전에 지워진 폴더는 그 아래까지 건너뛰고 기록에서도 지운다. 감시 대상 폴더가 지워졌으면
			// 감시를 시작하지 못한 것이므로 오류를 돌려준다.
			if errors.Is(err, fs.ErrNotExist) && p != t.root {
				t.forgetEntry(p)
				return filepath.SkipDir
			}
			return err
		}
		t.markWatched(p)
		// reAddDirs가 막는 경합은 fsnotify의 kqueue 구현에만 있다.
		if kqueueOS {
			t.dirsToReAdd = append(t.dirsToReAdd, p)
		}
		return nil
	})
}

// addWatch는 add로 dir을 감시 목록에 넣는다. add에는 fsnotify.Watcher의 Add를 넘긴다.
//
// fsnotify의 kqueue 구현은 감시 중인 폴더가 옮겨지거나 지워지면 이벤트를 읽는 goroutine에서 그 폴더의 감시를
// 닫고 감시 목록에서 지우는데, 이 일과 Add를 함께 막는 lock이 없다. 감시를 닫은 뒤 목록에서 지우기 전에 같은
// 경로를 Add하면, Add는 목록에 남은 닫힌 감시 번호로 등록하다가 bad file descriptor로 실패한다. 닫은 쪽은 곧바로
// 목록에서 지우므로, 잠시 기다렸다가 다시 Add하면 같은 경로에 다시 만든 폴더를 새로 감시하고 폴더가 없으면
// ErrNotExist를 돌려받는다. 다시 해도 실패하면 마지막 오류를 돌려준다.
func addWatch(add func(string) error, dir string) error {
	err := add(dir)
	for range staleWatchRetries {
		if !kqueueOS || !errors.Is(err, syscall.EBADF) {
			break
		}
		time.Sleep(staleWatchRetryDelay)
		err = add(dir)
	}
	return err
}

// reAddDirs는 addDirTree가 감시 목록에 넣은 폴더를 다시 넣는다.
//
// fsnotify의 kqueue 구현은 감시 중인 폴더의 항목이 바뀔 때마다 그 폴더 안의 하위 폴더를 내부에서 다시
// 등록한다. 이 등록은 하위 폴더의 감시 설정을 먼저 읽고 나중에 등록하므로, 그 사이에 같은 하위 폴더를 Add하면
// Add 전에 읽은 설정이 Add의 등록을 덮어쓴다. 그러면 그 하위 폴더 안에 새로 생기는 파일의 이벤트가 오지 않는다.
// fsnotify는 이벤트를 한 goroutine에서 차례로 보내므로, Add 뒤에 다음 이벤트를 받았다면 Add와 겹친 내부
// 등록은 이미 끝났다. 그래서 다음 이벤트를 받을 때 같은 폴더를 한 번 더 Add해 덮어쓴 등록을 되돌린다.
// 이 두 번째 Add와 다시 겹친 내부 등록은 되돌리지 못한다.
func (t *watchTree) reAddDirs() error {
	dirs := t.dirsToReAdd
	t.dirsToReAdd = nil
	return t.reAdd(dirs)
}

// reAdd는 감시 목록에 넣은 폴더 dirs를 다시 넣는다.
func (t *watchTree) reAdd(dirs []string) error {
	for _, dir := range dirs {
		// 다시 넣기 전에 폴더가 지워졌거나 옮겨졌으면 넣지 않는다. fsnotify는 그 폴더의 Remove나 Rename을
		// 처리하며 감시를 닫는데, 닫는 사이에 Add하면 이미 닫은 감시 번호를 다시 닫아 그 번호를 새로 받은 다른
		// 파일까지 닫는다. Add는 감시 중인 경로의 존재를 확인하지 않으므로 먼저 확인한다.
		if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := addWatch(t.watcher.Add, dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// reAddUnder는 감시 목록에 넣은 폴더 가운데 dir과 그 아래에 있는 폴더를 다시 넣는다.
func (t *watchTree) reAddUnder(dir domain.Path) error {
	var dirs []string
	for watched := range t.watchedDirs {
		if dir.Contains(domain.Path(watched)) {
			dirs = append(dirs, watched)
		}
	}
	return t.reAdd(dirs)
}

// contains는 경로 name이 감시 대상 폴더이거나 그 안에 있는지 판단한다.
func (t *watchTree) contains(name string) bool {
	return domain.Path(t.root).Contains(domain.Path(name))
}

// isExcluded는 감시 대상 안의 경로 name이 기본 제외 규칙이나 option.exclude에 따라 감시와 hook 실행에서
// 빠지는지 판단한다. 감시 대상 경로는 사용자가 직접 지정했으므로 빼지 않는다.
func (t *watchTree) isExcluded(name string) bool {
	if name == t.root {
		return false
	}
	if domain.Path(name).IsDefaultExcludeFile() {
		return true
	}
	if len(t.exclude) == 0 {
		return false
	}
	rel, ok := domain.Path(t.root).RelOf(domain.Path(name))
	return ok && t.exclude.Excludes(filepath.ToSlash(rel))
}

// fileID는 파일을 장치와 inode로 가리킨다. 이름을 바꿔도 그대로다.
type fileID struct{ dev, ino uint64 }

// entryInfo는 감시 중인 폴더 안에 있는 항목 하나의 기록이다.
type entryInfo struct {
	// id는 kqueue에서만 채운다. renamedFrom을 본다.
	id  fileID
	dir bool
	// excluded는 exclude나 기본 제외에 걸린 항목이다. fsnotify의 kqueue 구현은 이런 항목에도 감시를 열므로,
	// unwatchTree가 그 감시를 풀 수 있게 kqueue에서만 적어 두고 recorded는 기록에 없는 항목으로 본다.
	excluded bool
}

// recorded는 감시 대상 안의 경로 path가 기록에 있는지와 폴더인지를 돌려준다.
//
// 기록은 감시를 시작할 때 감시 대상을 훑어 채우고, 항목의 생성과 삭제, 이름 바꾸기, 옮겨 오기와 옮겨 나가기
// 이벤트를 받을 때 고친다. exclude와 기본 제외에 걸린 경로와, subDir가 false일 때 감시 대상 바로 아래 폴더
// 안의 항목은 기록에 없다.
func (t *watchTree) recorded(path string) (isDir, ok bool) {
	if path == t.root {
		_, ok := t.watchedDirs[path]
		return ok, ok
	}
	e, ok := t.watchedDirs[filepath.Dir(path)][filepath.Base(path)]
	if !ok || e.excluded {
		return false, false
	}
	return e.dir, true
}

// isDir은 감시 대상 안의 경로 name이 폴더인지 판단한다. 기록이나 감시 목록에 폴더로 적혀 있지 않으면 디스크를 본다.
func (t *watchTree) isDir(name string) bool {
	if isDir, _ := t.recorded(name); isDir {
		return true
	}
	if _, ok := t.watchedDirs[name]; ok {
		return true
	}
	info, err := os.Lstat(name)
	return err == nil && info.IsDir()
}

// renamedFrom은 kqueue에서 path에 새로 생긴 항목이 감시 중인 폴더 안의 다른 항목의 이름을 바꾼 것이면 그
// 옛 경로를 돌려준다. 같은 장치와 inode의 항목이 옛 경로에 더는 없으면 이름을 바꾼 것으로 본다.
func (t *watchTree) renamedFrom(path string) (string, bool) {
	if !kqueueOS {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", false
	}
	id, ok := fileIDOf(info)
	if !ok {
		return "", false
	}
	old, ok := t.entryPaths[id]
	if !ok || old == path {
		return "", false
	}
	// 하드 링크를 만들면 옛 경로에 같은 파일이 그대로 있다.
	if oldInfo, err := os.Lstat(old); err == nil && os.SameFile(info, oldInfo) {
		return "", false
	}
	return old, true
}

// markWatched는 감시 목록에 넣은 폴더 dir을 watchedDirs에 적는다. dir 안의 항목은 addEntry가 적는다.
func (t *watchTree) markWatched(dir string) {
	if _, ok := t.watchedDirs[dir]; !ok {
		t.watchedDirs[dir] = map[string]entryInfo{}
	}
}

// addEntry는 감시 중인 폴더 안에 있는 항목 path를 그 폴더의 항목으로 적는다. 감시하지 않는 폴더 안의 항목과,
// kqueue가 아닌 운영체제에서 제외에 걸린 항목은 적지 않는다.
func (t *watchTree) addEntry(path string, isDir bool) {
	entries := t.watchedDirs[filepath.Dir(path)]
	if entries == nil {
		return
	}
	e := entryInfo{dir: isDir, excluded: t.isExcluded(path)}
	if e.excluded && !kqueueOS {
		return
	}
	if kqueueOS {
		if info, err := os.Lstat(path); err == nil {
			e.id, _ = fileIDOf(info)
		}
		if e.id != (fileID{}) {
			t.entryPaths[e.id] = path
		}
	}
	// filepath.Base가 돌려주는 이름은 path의 메모리를 그대로 가리키므로, 전체 경로를 붙잡지 않게 복사한다.
	entries[strings.Clone(filepath.Base(path))] = e
}

// forgetEntry는 사라진 항목 path를 addEntry의 기록에서 지운다.
func (t *watchTree) forgetEntry(path string) {
	entries := t.watchedDirs[filepath.Dir(path)]
	e, ok := entries[filepath.Base(path)]
	if !ok {
		return
	}
	delete(entries, filepath.Base(path))
	if t.entryPaths[e.id] == path {
		delete(t.entryPaths, e.id)
	}
}

// unwatchTree는 사라진 경로 path와 그 아래에서 감시 목록에 넣은 폴더의 감시를 모두 풀고, path가 감시 중인
// 폴더였는지 돌려준다.
//
// 감시 중인 폴더를 옮기면 fsnotify는 그 폴더의 감시만 풀고, 아래 폴더의 감시는 옛 경로로 남겨 옮겨 간 곳의
// 변경을 옛 경로로 알린다. kqueue 구현은 폴더 안 항목마다 연 감시도 남기는데, 옛 폴더를 Remove해도 이미
// 감시가 풀렸다며 이 감시를 풀지 않는다(fsnotify #791). 남은 감시는 같은 경로에 다시 생긴 항목의 생성
// 이벤트를 막으므로 watchedDirs에 모은 항목 이름으로 하나씩 푼다. Windows 구현은 옮겨 나간 폴더의 감시를
// 옛 경로로 찾지 못해 풀지 못하므로, 그 경로를 unreleasedDirs에 남겨 releaseLeakedWatches가 처리하게 한다.
func (t *watchTree) unwatchTree(path string) bool {
	t.forgetEntry(path)
	if _, ok := t.watchedDirs[path]; !ok {
		return false
	}
	removed := map[string]bool{}
	for dir, entries := range t.watchedDirs {
		if !domain.Path(path).Contains(domain.Path(dir)) {
			continue
		}
		_ = t.watcher.Remove(dir)
		removed[filepath.Clean(dir)] = true
		if kqueueOS {
			for name, e := range entries {
				_ = t.watcher.Remove(filepath.Join(dir, name))
				if t.entryPaths[e.id] == filepath.Join(dir, name) {
					delete(t.entryPaths, e.id)
				}
			}
		}
		delete(t.watchedDirs, dir)
	}
	// Windows 구현은 옛 경로에 이미 다른 폴더가 생겼으면 그 폴더의 감시를 찾으므로, Remove의 결과가 아니라
	// 감시 목록으로 풀렸는지 확인한다.
	for _, p := range t.watcher.WatchList() {
		if removed[filepath.Clean(p)] {
			t.unreleasedDirs[filepath.Clean(p)] = struct{}{}
			t.leakCheck = time.After(leakCheckDelay)
		}
	}
	return true
}

// isUnreleasedDir는 경로 dir이 감시를 풀지 못한 폴더의 옛 경로인지 판단한다.
func (t *watchTree) isUnreleasedDir(dir string) bool {
	_, ok := t.unreleasedDirs[dir]
	return ok
}

// isUnreleased는 감시 대상 안의 경로 name이 감시를 풀지 못한 폴더의 아래에 있는지 판단한다. 그 폴더의 남은
// 감시는 옮겨 간 곳의 변경을 옛 경로로 알린다.
func (t *watchTree) isUnreleased(name string) bool {
	if len(t.unreleasedDirs) == 0 {
		return false
	}
	for dir := filepath.Dir(name); ; dir = filepath.Dir(dir) {
		if _, ok := t.unreleasedDirs[dir]; ok {
			return true
		}
		if dir == t.root || dir == filepath.Dir(dir) {
			return false
		}
	}
}

// releaseLeakedWatches는 감시를 풀지 못한 폴더가 watcher에 남았으면 watcher를 새로 만들어 버린다. LeakCheck가
// 알릴 때 부른다.
func (t *watchTree) releaseLeakedWatches() error {
	t.leakCheck = nil
	return t.rebuildIfLeaked()
}

// rebuildIfLeaked는 watcher의 감시 목록에 watchedDirs 밖의 폴더가 있거나 같은 폴더가 두 번 있으면
// rebuildWatcher를 실행하고, 그렇지 않으면 unreleasedDirs를 비운다. 옛 경로에 새 폴더가 생기면 남은 감시와 새
// 감시가 같은 경로로 나온다.
func (t *watchTree) rebuildIfLeaked() error {
	if t.retiring != nil {
		return nil
	}
	unlisted := make(map[string]bool, len(t.watchedDirs))
	for dir := range t.watchedDirs {
		unlisted[filepath.Clean(dir)] = true
	}
	for _, p := range t.watcher.WatchList() {
		if !unlisted[filepath.Clean(p)] {
			return t.rebuildWatcher()
		}
		delete(unlisted, filepath.Clean(p))
	}
	clear(t.unreleasedDirs)
	return nil
}

// rebuildWatcher는 watchedDirs의 폴더만 감시하는 새 watcher로 갈아 끼우고 예전 watcher를 닫는다. 예전
// watcher가 이미 받은 이벤트는 retireWatcher가 돌려주도록 t.retiring에 둔다.
//
// fsnotify의 Windows 구현은 감시 대상 밖으로 옮긴 폴더의 감시를 경로로 찾아 풀므로, 옛 경로에 그 폴더가 없으면
// Remove가 실패하고 감시가 watcher를 닫을 때까지 남는다. 남은 감시만 풀 방법이 없어 watcher를 통째로 바꾼다.
func (t *watchTree) rebuildWatcher() error {
	next, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	nextEvents, nextErrors := pumpEvents(next, t.stop)
	nextErrors = injectWatchErrors(t.path, nextErrors, t.stop)
	for dir := range t.watchedDirs {
		// 사이에 지워진 폴더의 Remove 이벤트는 retireWatcher가 예전 watcher의 이벤트로 돌려준다.
		if err := addWatch(next.Add, dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = next.Close()
			return err
		}
	}
	old := &retiredWatcher{events: t.events}
	for dir := range t.unreleasedDirs {
		old.unreleased = append(old.unreleased, dir)
	}
	// 예전 watcher의 오류는 닫는 동안 생긴 것이므로 버린다.
	go func(errs <-chan error) {
		for range errs {
		}
	}(t.errs)
	if err := t.watcher.Close(); err != nil {
		t.logger.Debug(fmt.Sprintf("close replaced watcher: %v", err))
	}
	t.retiring = old
	t.watcher, t.events, t.errs = next, nextEvents, nextErrors
	t.logger.Debug("rebuilt watcher to release watches of folders moved out")
	return nil
}

// retireWatcher는 rebuildWatcher가 닫은 예전 watcher가 닫히기 전까지 받은 이벤트를 차례로 돌려준다. 다 돌려주거나
// 감시 루프가 순회를 멈추면, 그 watcher에서 감시를 풀지 못한 폴더의 기록을 지운다.
//
// 새 watcher는 폴더를 모두 넣은 뒤에 생긴 변경만 받으므로, 그 전의 변경은 예전 watcher의 이벤트로 넘긴다.
// 새 watcher에 폴더를 넣은 뒤 예전 watcher를 닫기까지 생긴 변경은 hook에 두 번 갈 수 있다.
//
// handleEvent와 releaseLeakedWatches가 rebuildWatcher를 부를 수 있으므로, 감시 루프의 select에서 둘을 부르는
// case는 모두 끝에 이 함수가 돌려준 이벤트를 넘긴다. 그래야 예전 watcher의 이벤트가 새 watcher의 다음 이벤트보다
// 먼저 간다.
func (t *watchTree) retireWatcher() iter.Seq[fsnotify.Event] {
	return func(yield func(fsnotify.Event) bool) {
		old := t.retiring
		if old == nil {
			return
		}
		t.retiring = nil
		defer func() {
			for _, dir := range old.unreleased {
				delete(t.unreleasedDirs, dir)
			}
		}()
		for ev := range old.events {
			if !yield(ev) {
				return
			}
		}
	}
}

// pumpEvents는 Windows에서 w의 이벤트와 오류를 쉬지 않고 받아 두었다가 돌려주는 채널로 차례로 넘긴다. inotify를
// 쓰는 Linux에서는 오류만 pumpErrors로 받아 두고, 다른 운영체제에서는 w의 채널을 그대로 돌려준다. Windows에서
// w를 닫으면 받아 둔 이벤트를 모두 넘긴 뒤 두 채널을 닫고, 받아 둔 오류는 버린다. stop이 닫히면 받는 이벤트를
// 넘기지 않고 버린다.
//
// fsnotify의 Windows 구현은 Add와 Remove를 이벤트를 보내는 goroutine에서 처리하고, 그 goroutine은 이벤트를
// 받을 때까지 다른 일을 하지 않는다. 감시 루프가 Add나 Remove를 부르는 동안 이벤트를 받지 않으면 서로를 기다려
// 멈추므로, 이벤트를 따로 받아 둔다.
func pumpEvents(w *fsnotify.Watcher, stop <-chan struct{}) (<-chan fsnotify.Event, <-chan error) {
	if inotifyOS {
		return w.Events, pumpErrors(w.Errors, stop)
	}
	if runtime.GOOS != "windows" {
		return w.Events, w.Errors
	}
	events, errs := make(chan fsnotify.Event), make(chan error)
	go func() {
		defer close(events)
		defer close(errs)
		var queue []fsnotify.Event
		var errQueue []error
		in, inErrs := w.Events, w.Errors
		for in != nil || len(queue) > 0 {
			var out chan<- fsnotify.Event
			var next fsnotify.Event
			if len(queue) > 0 {
				out, next = events, queue[0]
			}
			var errOut chan<- error
			var nextErr error
			if len(errQueue) > 0 {
				errOut, nextErr = errs, errQueue[0]
			}
			select {
			case <-stop:
				// 감시 루프가 끝났으므로 w가 닫힐 때까지 받기만 한다.
				if in != nil {
					for range in {
					}
				}
				return
			case ev, ok := <-in:
				if !ok {
					in, inErrs, errQueue = nil, nil, nil
					continue
				}
				queue = append(queue, ev)
			case err, ok := <-inErrs:
				if !ok {
					inErrs = nil
					continue
				}
				errQueue = append(errQueue, err)
			case out <- next:
				queue = queue[1:]
			case errOut <- nextErr:
				errQueue = errQueue[1:]
			}
		}
	}()
	return events, errs
}

// pumpErrors는 errs의 오류를 쉬지 않고 받아 두었다가 돌려주는 채널로 차례로 넘긴다. errs가 닫히면 받아 둔 오류를
// 버리고 돌려준 채널을 닫는다. stop이 닫히면 errs가 닫힐 때까지 받기만 한다.
//
// fsnotify의 inotify 구현은 감시 중인 폴더가 옮겨지면 Add와 Remove가 쓰는 잠금을 쥔 채 그 폴더의 감시를 풀다
// 생긴 오류를 보낸다. 감시 루프가 Add나 Remove를 부르는 동안 오류를 받지 않으면 서로를 기다려 멈추므로, 오류를
// 따로 받아 둔다.
func pumpErrors(errs <-chan error, stop <-chan struct{}) <-chan error {
	out := make(chan error)
	go func() {
		defer close(out)
		var queue []error
		for {
			var send chan<- error
			var next error
			if len(queue) > 0 {
				send, next = out, queue[0]
			}
			select {
			case <-stop:
				// 감시 루프가 끝났으므로 errs가 닫힐 때까지 받기만 한다.
				for range errs {
				}
				return
			case err, ok := <-errs:
				if !ok {
					return
				}
				queue = append(queue, err)
			case send <- next:
				queue = queue[1:]
			}
		}
	}()
	return out
}
