package service

import (
	"os"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// change는 감시 루프가 hook에 넘기기 전에 모으는 이벤트다. existed는 handleEvent가 이 이벤트를 기록에 반영하기
// 직전에 경로가 기록에 있었는지다.
type change struct {
	domain.Event
	existed bool
}

// changeBatch는 hook에 넘기려고 한 번 기다리는 동안 받은 이벤트를 받은 순서대로 모은다.
type changeBatch struct {
	changes []change
	// start는 첫 이벤트를 받은 때다.
	start time.Time
}

// batchForHook은 e를 hook에 넘길 실행으로 만든다. hookWait가 0이면 e 하나로 곧바로 실행을 만들어 대기열 events에
// 넣는다. block은 enqueue를 본다.
//
// hookWait가 0이 아니면 e를 모으고, 마지막 이벤트 뒤 hookWait 동안 이벤트가 없거나 첫 이벤트부터 hookMaxWait가
// 지나면 flushHookBatch가 모은 이벤트로 실행을 만든다. 대기열에 넣기 전에 모으므로 hook이 도는 동안에도 기다리는
// 시간이 끝날 때마다 실행이 대기열에 쌓이고, 다른 때 만든 실행끼리는 합치지 않는다.
func (r *watchTargetRunner) batchForHook(e change, events chan<- domain.Event, block bool) error {
	if r.hookWait == 0 {
		return r.enqueue(r.runsOf([]change{e}), events, block)
	}
	now := time.Now()
	if r.hookBatch == nil {
		r.hookBatch = &changeBatch{start: now}
	}
	r.hookBatch.changes = append(r.hookBatch.changes, e)
	r.hookBatchDue = time.After(min(r.hookWait, r.hookBatch.start.Add(r.hookMaxWait).Sub(now)))
	return nil
}

// flushHookBatch는 모은 이벤트로 만든 실행을 대기열 events에 넣는다. 대기열이 가득 차면 하나씩 받은 이벤트처럼
// 오류를 돌려준다.
func (r *watchTargetRunner) flushHookBatch(events chan<- domain.Event) error {
	batch := r.hookBatch
	r.hookBatch, r.hookBatchDue = nil, nil
	return r.enqueue(r.runsOf(batch.changes), events, false)
}

// runsOf는 한 번 기다리는 동안 모은 changes를 deliver에 따라 실행으로 합친다. watchFileEvent와 watchDirEvent에서
// 고르지 않은 종류의 실행은 뺀다.
//
// each는 이벤트마다 실행을 만든다. last와 lastPerPath는 경로마다 finalChange로 최종 변화를 계산하고, 경로가
// 마지막으로 바뀐 순서대로 둔다. last는 그 가운데 마지막 실행 하나만 남긴다.
func (r *watchTargetRunner) runsOf(changes []change) []domain.Event {
	var runs []domain.Event
	if r.deliver == domain.DeliverEach {
		for _, c := range changes {
			if r.listens(c.Event) {
				runs = append(runs, c.Event)
			}
		}
		return runs
	}
	type pathChanges struct {
		// existed는 경로의 첫 이벤트를 받기 직전에 경로가 기록에 있었는지이고, at은 경로의 마지막 이벤트가 changes에서
		// 몇 번째인지다.
		existed bool
		last    domain.Event
		at      int
	}
	byPath := map[domain.Path]*pathChanges{}
	for i, c := range changes {
		p, ok := byPath[c.Path]
		if !ok {
			p = &pathChanges{existed: c.existed}
			byPath[c.Path] = p
		}
		p.last, p.at = c.Event, i
	}
	paths := make([]*pathChanges, 0, len(byPath))
	for _, p := range byPath {
		paths = append(paths, p)
	}
	slices.SortFunc(paths, func(a, b *pathChanges) int { return a.at - b.at })
	for _, p := range paths {
		if run, ok := finalChange(p.existed, p.last); ok && r.listens(run) {
			runs = append(runs, run)
		}
	}
	if r.deliver == domain.DeliverLast && len(runs) > 1 {
		runs = runs[len(runs)-1:]
	}
	return runs
}

// finalChange는 경로가 한 번 기다리기 전에 기록에 있었는지 existed와 지금 디스크에 있는지로 최종 변화를 정한다.
// last는 그 경로의 마지막 이벤트다. 있던 폴더가 그대로 있거나, 없던 경로가 그대로 없으면 넘길 변화가 없다.
//
// 사라진 경로는 디스크에서 폴더인지 알 수 없으므로 마지막 이벤트로 판단한다.
func finalChange(existed bool, last domain.Event) (domain.Event, bool) {
	run := domain.Event{Path: last.Path, IsDir: last.IsDir}
	info, err := os.Lstat(last.Path.String())
	exists := err == nil
	if exists {
		run.IsDir = info.IsDir()
	}
	switch {
	case !existed && exists:
		run.Operation = domain.Operation(fsnotify.Create)
	case existed && exists && !run.IsDir:
		run.Operation = domain.Operation(fsnotify.Write)
	case existed && !exists:
		run.Operation = domain.Operation(fsnotify.Remove)
	default:
		return domain.Event{}, false
	}
	return run, true
}
