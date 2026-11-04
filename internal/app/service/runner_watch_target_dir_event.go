package service

import (
	"path/filepath"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// dirBatcher는 폴더 이벤트를 모으는 동안 감시 대상에서 받은 변경을 받은 순서대로 모으고, 모으기를 마친 변경을
// hookBatcher에 넘긴다. 감시 루프의 고루틴에서만 부른다.
//
// 모으는 동안에는 폴더 이벤트가 아닌 변경도 묶음에 넣어, 감시 대상의 변경이 생긴 순서대로 hook에 가게 한다. 묶음은
// 마지막 변경 뒤 wait 동안 변경이 없거나 첫 변경부터 maxWait가 지나면 Due가 알린다. 감시 루프는 그때 flush를
// 부른다. 대기열에 넣기 전에 모으므로, 큰 폴더를 지워도 대기열에는 폴더 이벤트 하나만 들어간다. wait가 0이면
// 폴더 이벤트를 모으지 않는다.
type dirBatcher struct {
	wait    time.Duration
	maxWait time.Duration
	// root는 감시 목록에 넣는 감시 대상 폴더의 경로다. runner가 감시를 시작할 때 링크를 푼 경로로 정한다.
	root        string
	hookBatcher *hookBatcher
	// reAddUnder는 감시 목록에 넣은 폴더 가운데 dir과 그 아래에 있는 폴더를 감시 목록에 다시 넣는다.
	reAddUnder func(dir domain.Path) error
	// batch는 폴더 이벤트를 모으는 동안 받은 변경이고, 모으지 않을 때는 nil이다. due는 batch를 넘길 때를 알린다.
	batch *eventBatch
	due   <-chan time.Time
}

func newDirBatcher(option domain.DirBatchOption, hookBatcher *hookBatcher, reAddUnder func(dir domain.Path) error) *dirBatcher {
	return &dirBatcher{
		wait:        option.WaitMillisecond.Duration(),
		maxWait:     option.MaxWaitMillisecond.Duration(),
		hookBatcher: hookBatcher,
		reAddUnder:  reAddUnder,
	}
}

// add는 변경 c를 받는다. 폴더 이벤트를 모으지 않으면 c를 곧바로 hookBatcher에 넘기고, hookBatcher가 곧바로
// 돌려준 실행을 돌려준다. 모으는 동안에는 c를 묶음에 넣고 nil을 돌려준다.
func (b *dirBatcher) add(c change) ([]domain.Event, error) {
	now := time.Now()
	if b.batch == nil {
		if b.wait == 0 || !b.opensBatch(c.Event) {
			return b.passOn(c)
		}
		b.batch = &eventBatch{start: now}
	}
	b.batch.add(c)
	b.due = time.After(min(b.wait, b.batch.start.Add(b.maxWait).Sub(now)))
	return nil, nil
}

// Due는 flush를 부를 때를 알리는 채널을 돌려준다. 모은 변경이 없으면 nil 채널이라 알리지 않는다.
func (b *dirBatcher) Due() <-chan time.Time {
	return b.due
}

// flush는 모은 변경을 받은 순서대로 hookBatcher에 넘기고, hookBatcher가 곧바로 돌려준 실행을 돌려준다. 넘기다
// 오류가 나면 그 앞의 변경으로 만든 실행과 오류를 돌려준다.
func (b *dirBatcher) flush() ([]domain.Event, error) {
	batch := b.batch
	b.batch, b.due = nil, nil
	var runs []domain.Event
	for _, c := range batch.events {
		next, err := b.passOn(c)
		if err != nil {
			return runs, err
		}
		runs = append(runs, next...)
	}
	return runs, nil
}

// passOn은 폴더 이벤트 모으기를 마친 c를 hookBatcher에 넘기고, hookBatcher가 곧바로 돌려준 실행을 돌려준다.
//
// fsnotify의 kqueue 구현은 새 폴더의 감시 등록을 내부 재등록으로 덮어쓸 수 있다(reAddDirs 참고). 그래서 생긴
// 폴더의 변경을 넘기기 전에 그 아래의 감시 폴더를 다시 감시 목록에 넣어, hook이 폴더 생성을 받은 뒤 그 안에서
// 생기는 변경을 놓치지 않게 한다. 그 전에 폴더 안에 생긴 항목은 폴더 생성 이벤트를 받은 hook이 직접 읽는다.
func (b *dirBatcher) passOn(c change) ([]domain.Event, error) {
	if kqueueOS && c.IsDir && fsnotify.Op(c.Operation) == fsnotify.Create {
		if err := b.reAddUnder(c.Path); err != nil {
			return nil, err
		}
	}
	return b.hookBatcher.add(c), nil
}

// opensBatch는 e를 받으면 폴더 이벤트를 모으기 시작해야 하는지 판단한다.
//
// 폴더의 생성과 삭제 말고도, 감시 대상 바로 아래가 아닌 파일의 삭제로 시작한다. 폴더를 통째로 지우면 폴더 안의
// 파일을 지운 이벤트가 폴더를 지운 이벤트보다 먼저 오므로, 그 파일 이벤트부터 모아야 폴더 이벤트에 흡수할 수 있다.
func (b *dirBatcher) opensBatch(e domain.Event) bool {
	if e.IsDir {
		return true
	}
	return fsnotify.Op(e.Operation) == fsnotify.Remove && filepath.Dir(e.Path.String()) != b.root
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
	for i := len(b.events) - 1; i >= 0; i-- {
		prev := b.events[i]
		if !e.recreated && absorbs(prev.Event, e.Event) {
			return
		}
		// 같은 경로의 재생성 뒤에는 앞선 폴더 삭제가 새 항목의 삭제를 흡수하지 않는다.
		if prev.Path == e.Path {
			break
		}
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
		// 같은 삭제 수명의 후행 REMOVE와 WRITE만 흡수하고, 재생성 CREATE는 흡수하지 않는다.
		return dir.Operation == e.Operation || (fsnotify.Op(dir.Operation) == fsnotify.Remove && fsnotify.Op(e.Operation) == fsnotify.Write)
	}
	return isUnder(e.Path, dir.Path)
}

// isUnder는 path가 dir 아래에 있는지 판단한다. path와 dir이 같으면 false다.
func isUnder(path, dir domain.Path) bool {
	return path != dir && dir.Contains(path)
}
