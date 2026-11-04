package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// fileID는 파일을 장치와 inode로 가리킨다. 이름을 바꿔도 그대로다.
type fileID struct{ dev, ino uint64 }

const (
	// leakCheckDelay는 폴더의 감시를 풀지 못한 뒤 releaseLeakedWatches를 실행하기까지 기다리는 시간이다.
	// 지운 폴더의 감시는 fsnotify가 스스로 풀므로, 그동안 기다려 지운 폴더 때문에 watcher를 다시 만들지 않는다.
	leakCheckDelay = 100 * time.Millisecond
)

// retiredWatcher는 rebuildWatcher가 닫은 예전 watcher의 이벤트 채널과, 그 watcher에서 감시를 풀지 못한 폴더의
// 경로다.
type retiredWatcher struct {
	events     <-chan fsnotify.Event
	unreleased []string
}

// markWatched는 감시 목록에 넣은 폴더 dir을 watchedDirs에 적는다.
func (r *watchTargetRunner) markWatched(dir string) {
	if _, ok := r.watchedDirs[dir]; ok {
		return
	}
	var entries map[string]fileID
	if kqueueOS {
		entries = map[string]fileID{}
	}
	r.watchedDirs[dir] = entries
}

// addEntry는 kqueue에서 감시 중인 폴더 안에 있는 항목 path를 그 폴더의 항목으로 적는다.
func (r *watchTargetRunner) addEntry(path string) {
	entries := r.watchedDirs[filepath.Dir(path)]
	if entries == nil {
		return
	}
	var id fileID
	if info, err := os.Lstat(path); err == nil {
		id, _ = fileIDOf(info)
	}
	entries[filepath.Base(path)] = id
	if id != (fileID{}) {
		r.entryPaths[id] = path
	}
}

// forgetEntry는 사라진 항목 path를 addEntry의 기록에서 지운다.
func (r *watchTargetRunner) forgetEntry(path string) {
	entries := r.watchedDirs[filepath.Dir(path)]
	id, ok := entries[filepath.Base(path)]
	if !ok {
		return
	}
	delete(entries, filepath.Base(path))
	if r.entryPaths[id] == path {
		delete(r.entryPaths, id)
	}
}

// renamedFrom은 kqueue에서 path에 새로 생긴 항목이 감시 중인 폴더 안의 다른 항목의 이름을 바꾼 것이면 그
// 옛 경로를 돌려준다. 같은 장치와 inode의 항목이 옛 경로에 더는 없으면 이름을 바꾼 것으로 본다.
func (r *watchTargetRunner) renamedFrom(path string) (string, bool) {
	if len(r.entryPaths) == 0 {
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
	old, ok := r.entryPaths[id]
	if !ok || old == path {
		return "", false
	}
	// 하드 링크를 만들면 옛 경로에 같은 파일이 그대로 있다.
	if oldInfo, err := os.Lstat(old); err == nil && os.SameFile(info, oldInfo) {
		return "", false
	}
	return old, true
}

// renameHoldLimit은 receive가 미뤄 둔 Create를 옛 경로의 Rename 없이 넘기기까지 기다리는 가장 긴 시간이다.
// 옛 경로의 감시가 남아 있으면 Rename은 반드시 오므로, 이 시간은 그 감시가 예상과 달리 풀렸을 때만 쓰인다.
const renameHoldLimit = time.Second

// receive는 fsnotify 이벤트 ev를 받은 순서대로 handleEvent로 넘기되, kqueue에서 이름을 바꾼 항목의 Create는
// 옛 경로의 Rename을 먼저 넘긴 뒤에 넘긴다. 그 Rename이 아직 오지 않았으면 Create와 뒤의 이벤트를 미뤄 둔다.
//
// fsnotify의 kqueue 구현은 이름 바꾸기와 감시 대상 안의 옮기기를 새 경로의 Create 다음에 옛 경로의 Rename으로
// 알리기도 하고, 둘 사이에 다른 이벤트가 끼거나 수십 ms가 걸리기도 한다. 다른 운영체제처럼 옛 경로의 삭제를
// 먼저 넘기려고 Create를 미룬다. 옛 경로의 감시는 Rename을 처리하며 fsnotify가 풀므로, Rename을 받기 전에
// 새 경로를 Add하면 fsnotify가 닫는 중인 감시와 겹칠 수 있다. 미루면 이 경합도 생기지 않는다.
func (r *watchTargetRunner) receive(ev fsnotify.Event, events chan<- domain.Event) error {
	r.held = append(r.held, ev)
	return r.drainHeld(events, false)
}

// drainHeld는 미뤄 둔 이벤트를 앞에서부터 handleEvent로 넘긴다. timedOut이 true이면 맨 앞의 Create를 옛 경로의
// Rename을 기다리지 않고 넘긴다.
func (r *watchTargetRunner) drainHeld(events chan<- domain.Event, timedOut bool) error {
	for len(r.held) > 0 {
		head := r.held[0]
		if r.renameSource == "" && !timedOut && head.Op.Has(fsnotify.Create) {
			r.renameSource, _ = r.renamedFrom(head.Name)
		}
		if r.renameSource != "" && !timedOut {
			i := slices.IndexFunc(r.held[1:], func(e fsnotify.Event) bool {
				return e.Name == r.renameSource && (e.Op.Has(fsnotify.Rename) || e.Op.Has(fsnotify.Remove))
			})
			if i < 0 {
				if r.holdTimeout == nil {
					r.holdTimeout = time.After(renameHoldLimit)
				}
				return nil
			}
			rename := r.held[1+i]
			r.held = slices.Delete(r.held, 1+i, 2+i)
			if err := r.handleEvent(rename, events); err != nil {
				return err
			}
		}
		r.held = r.held[1:]
		r.renameSource, r.holdTimeout, timedOut = "", nil, false
		if err := r.handleEvent(head, events); err != nil {
			return err
		}
	}
	return nil
}

// releaseHeld는 renameHoldLimit 안에 옛 경로의 Rename이 오지 않으면 미뤄 둔 이벤트를 받은 순서대로 넘긴다.
func (r *watchTargetRunner) releaseHeld(events chan<- domain.Event) error {
	r.logger.Debug(fmt.Sprintf("rename of %s did not arrive in %v", r.renameSource, renameHoldLimit))
	return r.drainHeld(events, true)
}

// selfRenameTTL은 addSelfRename이 적은 경로를 isSelfRename이 두 번째 Rename으로 보는 시간이다.
const selfRenameTTL = time.Second

// addSelfRename은 inotify에서 감시 중인 폴더 path를 옮긴 Rename을 받았음을 적는다.
//
// inotify는 감시 중인 폴더를 옮기면 부모 폴더의 Rename 다음에 그 폴더 자신의 Rename을 한 번 더 보낸다. 다만
// 두 번째 Rename은 unwatchTree가 그 폴더의 감시를 먼저 풀면 fsnotify가 버리므로 오지 않을 수도 있다. 그래서
// selfRenameTTL이 지난 기록은 버린다.
func (r *watchTargetRunner) addSelfRename(path string) {
	now := time.Now()
	for p, at := range r.selfRenames {
		if now.Sub(at) > selfRenameTTL {
			delete(r.selfRenames, p)
		}
	}
	r.selfRenames[path] = now
}

// isSelfRename은 ev가 addSelfRename이 적은 폴더의 두 번째 Rename이나 Remove인지 판단하고, 맞으면 기록을 지운다.
func (r *watchTargetRunner) isSelfRename(ev fsnotify.Event) bool {
	at, ok := r.selfRenames[ev.Name]
	if !ok {
		return false
	}
	delete(r.selfRenames, ev.Name)
	return time.Since(at) <= selfRenameTTL
}

// unwatchTree는 사라진 경로 path와 그 아래에서 감시 목록에 넣은 폴더의 감시를 모두 풀고, path가 감시 중인
// 폴더였는지 돌려준다.
//
// 감시 중인 폴더를 옮기면 fsnotify는 그 폴더의 감시만 풀고, 아래 폴더의 감시는 옛 경로로 남겨 옮겨 간 곳의
// 변경을 옛 경로로 알린다. kqueue 구현은 폴더 안 항목마다 연 감시도 남기는데, 옛 폴더를 Remove해도 이미
// 감시가 풀렸다며 이 감시를 풀지 않는다(fsnotify #791). 남은 감시는 같은 경로에 다시 생긴 항목의 생성
// 이벤트를 막으므로 watchedDirs에 모은 항목 이름으로 하나씩 푼다. Windows 구현은 옮겨 나간 폴더의 감시를
// 옛 경로로 찾지 못해 풀지 못하므로, 그 경로를 unreleasedDirs에 남겨 releaseLeakedWatches가 처리하게 한다.
func (r *watchTargetRunner) unwatchTree(path string) bool {
	r.forgetEntry(path)
	if _, ok := r.watchedDirs[path]; !ok {
		return false
	}
	removed := map[string]bool{}
	for dir, entries := range r.watchedDirs {
		if !domain.Path(path).Contains(domain.Path(dir)) {
			continue
		}
		_ = r.watcher.Remove(dir)
		removed[filepath.Clean(dir)] = true
		for name, id := range entries {
			_ = r.watcher.Remove(filepath.Join(dir, name))
			if r.entryPaths[id] == filepath.Join(dir, name) {
				delete(r.entryPaths, id)
			}
		}
		delete(r.watchedDirs, dir)
	}
	// Windows 구현은 옛 경로에 이미 다른 폴더가 생겼으면 그 폴더의 감시를 찾으므로, Remove의 결과가 아니라
	// 감시 목록으로 풀렸는지 확인한다.
	for _, p := range r.watcher.WatchList() {
		if removed[filepath.Clean(p)] {
			r.unreleasedDirs[filepath.Clean(p)] = struct{}{}
			r.leakCheck = time.After(leakCheckDelay)
		}
	}
	return true
}

// isUnreleased는 감시 대상 안의 경로 name이 감시를 풀지 못한 폴더의 아래에 있는지 판단한다. 그 폴더의 남은
// 감시는 옮겨 간 곳의 변경을 옛 경로로 알린다.
func (r *watchTargetRunner) isUnreleased(name string) bool {
	if len(r.unreleasedDirs) == 0 {
		return false
	}
	for dir := filepath.Dir(name); ; dir = filepath.Dir(dir) {
		if _, ok := r.unreleasedDirs[dir]; ok {
			return true
		}
		if dir == r.watchRoot || dir == filepath.Dir(dir) {
			return false
		}
	}
}

// releaseLeakedWatches는 감시를 풀지 못한 폴더가 watcher에 남았으면 watcher를 새로 만들어 버린다.
func (r *watchTargetRunner) releaseLeakedWatches(events chan<- domain.Event) error {
	if err := r.rebuildIfLeaked(); err != nil {
		return err
	}
	return r.retireWatcher(events)
}

// rebuildIfLeaked는 watcher의 감시 목록에 watchedDirs 밖의 폴더가 있거나 같은 폴더가 두 번 있으면
// rebuildWatcher를 실행하고, 그렇지 않으면 unreleasedDirs를 비운다. 옛 경로에 새 폴더가 생기면 남은 감시와 새
// 감시가 같은 경로로 나온다.
func (r *watchTargetRunner) rebuildIfLeaked() error {
	if r.retiring != nil {
		return nil
	}
	unlisted := make(map[string]bool, len(r.watchedDirs))
	for dir := range r.watchedDirs {
		unlisted[filepath.Clean(dir)] = true
	}
	for _, p := range r.watcher.WatchList() {
		if !unlisted[filepath.Clean(p)] {
			return r.rebuildWatcher()
		}
		delete(unlisted, filepath.Clean(p))
	}
	clear(r.unreleasedDirs)
	return nil
}

// rebuildWatcher는 watchedDirs의 폴더만 감시하는 새 watcher로 갈아 끼우고 예전 watcher를 닫는다. 예전
// watcher가 이미 받은 이벤트는 retireWatcher가 넘기도록 r.retiring에 둔다.
//
// fsnotify의 Windows 구현은 감시 대상 밖으로 옮긴 폴더의 감시를 경로로 찾아 풀므로, 옛 경로에 그 폴더가 없으면
// Remove가 실패하고 감시가 watcher를 닫을 때까지 남는다. 남은 감시만 풀 방법이 없어 watcher를 통째로 바꾼다.
func (r *watchTargetRunner) rebuildWatcher() error {
	next, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	nextEvents, nextErrors := pumpEvents(next, r.stop)
	for dir := range r.watchedDirs {
		// 사이에 지워진 폴더의 Remove 이벤트는 예전 watcher가 retireWatcher에서 넘긴다.
		if err := next.Add(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = next.Close()
			return err
		}
	}
	old := &retiredWatcher{events: r.watchEvents}
	for dir := range r.unreleasedDirs {
		old.unreleased = append(old.unreleased, dir)
	}
	// 예전 watcher의 오류는 닫는 동안 생긴 것이므로 버린다.
	go func(errs <-chan error) {
		for range errs {
		}
	}(r.watchErrors)
	if err := r.watcher.Close(); err != nil {
		r.logger.Debug(fmt.Sprintf("close replaced watcher: %v", err))
	}
	r.retiring = old
	r.watcher, r.watchEvents, r.watchErrors = next, nextEvents, nextErrors
	r.logger.Debug("rebuilt watcher to release watches of folders moved out")
	return nil
}

// retireWatcher는 rebuildWatcher가 닫은 예전 watcher가 닫히기 전까지 받은 이벤트를 넘긴다.
//
// 새 watcher는 폴더를 모두 넣은 뒤에 생긴 변경만 받으므로, 그 전의 변경은 예전 watcher의 이벤트로 넘긴다.
// 새 watcher에 폴더를 넣은 뒤 예전 watcher를 닫기까지 생긴 변경은 hook에 두 번 갈 수 있다.
func (r *watchTargetRunner) retireWatcher(events chan<- domain.Event) error {
	old := r.retiring
	if old == nil {
		return nil
	}
	r.retiring = nil
	defer func() {
		for _, dir := range old.unreleased {
			delete(r.unreleasedDirs, dir)
		}
	}()
	for ev := range old.events {
		if err := r.receive(ev, events); err != nil {
			return err
		}
	}
	return nil
}

// pumpEvents는 Windows에서 w의 이벤트와 오류를 쉬지 않고 받아 두었다가 돌려주는 채널로 차례로 넘기고, 다른
// 운영체제에서는 w의 채널을 그대로 돌려준다. w를 닫으면 받아 둔 이벤트를 모두 넘긴 뒤 두 채널을 닫고, 받아 둔
// 오류는 버린다. stop이 닫히면 받는 이벤트를 넘기지 않고 버린다.
//
// fsnotify의 Windows 구현은 Add와 Remove를 이벤트를 보내는 goroutine에서 처리하고, 그 goroutine은 이벤트를
// 받을 때까지 다른 일을 하지 않는다. 감시 루프가 Add나 Remove를 부르는 동안 이벤트를 받지 않으면 서로를 기다려
// 멈추므로, 이벤트를 따로 받아 둔다.
func pumpEvents(w *fsnotify.Watcher, stop <-chan struct{}) (<-chan fsnotify.Event, <-chan error) {
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
