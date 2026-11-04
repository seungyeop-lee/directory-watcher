package service

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// isDir은 감시 대상 안의 경로 name이 폴더인지 판단한다. 기록이나 감시 목록에 폴더로 적혀 있지 않으면 디스크를 본다.
func (r *watchTargetRunner) isDir(name string) bool {
	if isDir, _ := r.recorded(name); isDir {
		return true
	}
	if _, ok := r.watchedDirs[name]; ok {
		return true
	}
	info, err := os.Lstat(name)
	return err == nil && info.IsDir()
}

// eventBatch는 폴더 이벤트를 모으는 동안 감시 대상에서 받은 이벤트를 받은 순서대로 모은다. 묶음 안의 폴더
// 이벤트 아래 경로의 이벤트는 그 폴더 이벤트에 흡수해 모으지 않는다.
type eventBatch struct {
	events []change
	// start는 묶음의 첫 이벤트를 받은 때다.
	start time.Time
}

// add는 e를 묶음에 넣는다. 폴더를 지운 이벤트이면 앞서 모은 그 폴더 아래 경로의 이벤트를 흡수한다. 폴더를
// 통째로 지우면 운영체제는 폴더 안의 항목을 지운 이벤트를 폴더를 지운 이벤트보다 먼저 보내기 때문이다.
func (b *eventBatch) add(e change) {
	if slices.ContainsFunc(b.events, func(dir change) bool { return absorbs(dir.Event, e.Event) }) {
		return
	}
	if e.IsDir && fsnotify.Op(e.Operation) == fsnotify.Remove {
		b.events = slices.DeleteFunc(b.events, func(prev change) bool { return isUnder(prev.Path, e.Path) })
	}
	b.events = append(b.events, e)
}

// absorbs는 묶음 안의 이벤트 dir이 폴더 이벤트이고, 뒤에 받은 이벤트 e를 흡수하는지 판단한다.
func absorbs(dir, e domain.Event) bool {
	if !dir.IsDir {
		return false
	}
	if dir.Path == e.Path {
		// Windows는 지운 폴더의 Remove를 두 번 보내고, 그 폴더의 Write를 Remove 뒤에 보내기도 한다. 폴더 목록에서
		// 이미 지운 뒤라 두 번째 이벤트는 파일 이벤트로 온다. 지운 폴더를 다시 만든 Create는 흡수하지 않는다.
		return dir.Operation == e.Operation || (fsnotify.Op(dir.Operation) == fsnotify.Remove && fsnotify.Op(e.Operation) == fsnotify.Write)
	}
	return isUnder(e.Path, dir.Path)
}

// isUnder는 path가 dir 아래에 있는지 판단한다. path와 dir이 같으면 false다.
func isUnder(path, dir domain.Path) bool {
	return path != dir && dir.Contains(path)
}

// opensBatch는 e를 받으면 폴더 이벤트를 모으기 시작해야 하는지 판단한다.
//
// 폴더의 생성과 삭제 말고도, 감시 대상 바로 아래가 아닌 파일의 삭제로 시작한다. 폴더를 통째로 지우면 폴더 안의
// 파일을 지운 이벤트가 폴더를 지운 이벤트보다 먼저 오므로, 그 파일 이벤트부터 모아야 폴더 이벤트에 흡수할 수 있다.
func (r *watchTargetRunner) opensBatch(e domain.Event) bool {
	if e.IsDir {
		return true
	}
	return fsnotify.Op(e.Operation) == fsnotify.Remove && filepath.Dir(e.Path.String()) != r.watchRoot
}

// collect는 e를 batchForHook으로 넘긴다. 폴더 이벤트를 모으는 동안에는 e도 묶음에 넣어, 감시 대상의 이벤트가
// 생긴 순서대로 hook에 가게 한다. dirEventWait가 0이면 폴더 이벤트를 모으지 않는다.
//
// 묶음은 마지막 이벤트 뒤 dirEventWait 동안 이벤트가 없거나, 첫 이벤트부터 dirEventMaxWait가 지나면 flushBatch가
// 내보낸다. 폴더 이벤트를 대기열에 넣기 전에 모으므로, 큰 폴더를 지워도 대기열에는 폴더 이벤트 하나만 들어간다.
func (r *watchTargetRunner) collect(e change, events chan<- domain.Event) error {
	now := time.Now()
	if r.batch == nil {
		if r.dirEventWait == 0 || !r.opensBatch(e.Event) {
			return r.passOn(e, events, false)
		}
		r.batch = &eventBatch{start: now}
	}
	r.batch.add(e)
	wait := min(r.dirEventWait, r.batch.start.Add(r.dirEventMaxWait).Sub(now))
	r.batchDue = time.After(wait)
	return nil
}

// flushBatch는 모은 이벤트를 받은 순서대로 passOn으로 넘긴다.
//
// hookWait가 0이어서 이벤트를 곧바로 대기열에 넣을 때 자리가 없으면 자리가 날 때까지 기다린다. 묶음은 이벤트를
// 한꺼번에 내보내므로, 하나씩 받을 때는 넘치지 않던 대기열도 넘칠 수 있기 때문이다. 기다리는 동안 생긴 변경은
// 운영체제와 fsnotify가 받아 둔다.
func (r *watchTargetRunner) flushBatch(events chan<- domain.Event) error {
	batch := r.batch
	r.batch, r.batchDue = nil, nil
	for _, e := range batch.events {
		if err := r.passOn(e, events, true); err != nil {
			return err
		}
	}
	return nil
}

// passOn은 폴더 이벤트 모으기를 마친 e를 batchForHook으로 넘긴다. block은 enqueue를 본다.
//
// fsnotify의 kqueue 구현은 새 폴더의 감시 등록을 내부 재등록으로 덮어쓸 수 있다(reAddDirs 참고). 그래서 생긴
// 폴더의 이벤트를 넘기기 전에 그 아래의 감시 폴더를 다시 감시 목록에 넣어, hook이 폴더 생성을 받은 뒤 그 안에서
// 생기는 변경을 놓치지 않게 한다. 그 전에 폴더 안에 생긴 항목은 폴더 생성 이벤트를 받은 hook이 직접 읽는다.
func (r *watchTargetRunner) passOn(e change, events chan<- domain.Event, block bool) error {
	if kqueueOS && e.IsDir && fsnotify.Op(e.Operation) == fsnotify.Create {
		if err := r.reAdd(r.watchedDirsUnder(e.Path)); err != nil {
			return err
		}
	}
	return r.batchForHook(e, events, block)
}

// watchedDirsUnder는 감시 목록에 넣은 폴더 가운데 dir과 그 아래에 있는 폴더를 돌려준다.
func (r *watchTargetRunner) watchedDirsUnder(dir domain.Path) []string {
	var dirs []string
	for watched := range r.watchedDirs {
		if dir.Contains(domain.Path(watched)) {
			dirs = append(dirs, watched)
		}
	}
	return dirs
}
