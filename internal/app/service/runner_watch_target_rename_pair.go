package service

import (
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

const (
	// renameHoldLimit은 renamePairer가 미뤄 둔 Create를 옛 경로의 Rename 없이 넘기기까지 기다리는 가장 긴 시간이다.
	// 옛 경로의 감시가 남아 있으면 Rename은 반드시 오므로, 이 시간은 그 감시가 예상과 달리 풀렸을 때만 쓰인다.
	renameHoldLimit = time.Second
	// selfRenameTTL은 addSelfRename이 적은 경로를 isSelfRename이 두 번째 Rename으로 보는 시간이다.
	selfRenameTTL = time.Second
)

// renamePairer는 fsnotify 이벤트를 받은 순서대로 돌려주되, kqueue에서 이름을 바꾼 항목의 Create는 옛 경로의
// Rename을 먼저 돌려준 뒤에 돌려준다. 그 Rename이 아직 오지 않았으면 Create와 뒤의 이벤트를 미뤄 둔다. inotify에서
// 옮긴 폴더의 두 번째 Rename을 알아보는 기록도 가진다. 감시 루프의 고루틴에서만 부른다.
//
// fsnotify의 kqueue 구현은 이름 바꾸기와 감시 대상 안의 옮기기를 새 경로의 Create 다음에 옛 경로의 Rename으로
// 알리기도 하고, 둘 사이에 다른 이벤트가 끼거나 수십 ms가 걸리기도 한다. 다른 운영체제처럼 옛 경로의 삭제를
// 먼저 넘기려고 Create를 미룬다. 옛 경로의 감시는 Rename을 처리하며 fsnotify가 풀므로, Rename을 받기 전에
// 새 경로를 Add하면 fsnotify가 닫는 중인 감시와 겹칠 수 있다. 미루면 이 경합도 생기지 않는다.
type renamePairer struct {
	logger helper.Logger
	// renamedFrom은 path에 새로 생긴 항목이 다른 항목의 이름을 바꾼 것이면 그 옛 경로를 돌려준다.
	renamedFrom func(path string) (string, bool)
	// held는 아직 돌려주지 않은 이벤트다. 맨 앞이 이름을 바꾼 항목의 Create이면 source에 옛 경로를 두고, timeout에
	// 그 Rename을 기다리는 시간이 끝남을 알린다.
	held    []fsnotify.Event
	source  string
	timeout <-chan time.Time
	// selfRenames는 inotify에서 자신의 Rename이 한 번 더 올 수 있는, 옮긴 폴더의 옛 경로와 옮긴 때다.
	selfRenames map[string]time.Time
}

func newRenamePairer(renamedFrom func(path string) (string, bool), logger helper.Logger) *renamePairer {
	return &renamePairer{
		logger:      logger,
		renamedFrom: renamedFrom,
		selfRenames: map[string]time.Time{},
	}
}

// receive는 fsnotify 이벤트 ev를 받고, 넘길 차례가 된 이벤트를 받은 순서대로 돌려준다.
//
// 돌려줄 이벤트는 하나씩 꺼낼 때마다 정하고, 그때 renamedFrom은 감시 대상 안 항목의 기록을 본다. 그래서 감시
// 루프는 꺼낸 이벤트를 기록에 반영한 뒤에 다음 이벤트를 꺼낸다.
func (p *renamePairer) receive(ev fsnotify.Event) iter.Seq[fsnotify.Event] {
	p.held = append(p.held, ev)
	return p.drain(false)
}

// Timeout은 미뤄 둔 Create가 옛 경로의 Rename을 기다리는 시간이 끝남을 알리는 채널을 돌려준다. 기다리는 Create가
// 없으면 nil 채널이라 알리지 않는다. 감시 루프는 알림을 받으면 release를 부른다.
func (p *renamePairer) Timeout() <-chan time.Time {
	return p.timeout
}

// release는 옛 경로의 Rename을 기다리던 Create를 그 Rename 없이 돌려주고, 미뤄 둔 뒤의 이벤트를 receive처럼
// 돌려준다.
func (p *renamePairer) release() iter.Seq[fsnotify.Event] {
	p.logger.Debug(fmt.Sprintf("rename of %s did not arrive in %v", p.source, renameHoldLimit))
	return p.drain(true)
}

// drain은 미뤄 둔 이벤트를 앞에서부터 돌려준다. timedOut이 true이면 맨 앞의 Create를 옛 경로의 Rename을 기다리지
// 않고 돌려준다.
func (p *renamePairer) drain(timedOut bool) iter.Seq[fsnotify.Event] {
	return func(yield func(fsnotify.Event) bool) {
		for len(p.held) > 0 {
			head := p.held[0]
			if p.source == "" && !timedOut && head.Op.Has(fsnotify.Create) {
				p.source, _ = p.renamedFrom(head.Name)
			}
			if p.source != "" && !timedOut {
				i := slices.IndexFunc(p.held[1:], func(e fsnotify.Event) bool {
					return e.Name == p.source && (e.Op.Has(fsnotify.Rename) || e.Op.Has(fsnotify.Remove))
				})
				if i < 0 {
					if p.timeout == nil {
						p.timeout = time.After(renameHoldLimit)
					}
					return
				}
				rename := p.held[1+i]
				p.held = slices.Delete(p.held, 1+i, 2+i)
				if !yield(rename) {
					return
				}
			}
			p.held = p.held[1:]
			p.source, p.timeout, timedOut = "", nil, false
			if !yield(head) {
				return
			}
		}
	}
}

// addSelfRename은 inotify에서 감시 중인 폴더 path를 옮긴 Rename을 받았음을 적는다.
//
// inotify는 감시 중인 폴더를 옮기면 부모 폴더의 Rename 다음에 그 폴더 자신의 Rename을 한 번 더 보낸다. 다만
// 두 번째 Rename은 unwatchTree가 그 폴더의 감시를 먼저 풀면 fsnotify가 버리므로 오지 않을 수도 있다. 그래서
// selfRenameTTL이 지난 기록은 버린다.
func (p *renamePairer) addSelfRename(path string) {
	now := time.Now()
	for recorded, at := range p.selfRenames {
		if now.Sub(at) > selfRenameTTL {
			delete(p.selfRenames, recorded)
		}
	}
	p.selfRenames[path] = now
}

// isSelfRename은 ev가 addSelfRename이 적은 폴더의 두 번째 Rename이나 Remove인지 판단하고, 맞으면 기록을 지운다.
func (p *renamePairer) isSelfRename(ev fsnotify.Event) bool {
	at, ok := p.selfRenames[ev.Name]
	if !ok {
		return false
	}
	delete(p.selfRenames, ev.Name)
	return time.Since(at) <= selfRenameTTL
}

// forgetSelfRename은 path에 항목이 새로 생겼으므로 addSelfRename이 적은 path의 기록을 지운다.
func (p *renamePairer) forgetSelfRename(path string) {
	delete(p.selfRenames, path)
}
