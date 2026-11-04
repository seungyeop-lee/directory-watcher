package service

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fileID는 파일을 장치와 inode로 가리킨다. 이름을 바꿔도 그대로다.
type fileID struct{ dev, ino uint64 }

// renamedFrom은 kqueue에서 path에 새로 생긴 항목이 감시 중인 폴더 안의 다른 항목의 이름을 바꾼 것이면 그
// 옛 경로를 돌려준다. 같은 장치와 inode의 항목이 옛 경로에 더는 없으면 이름을 바꾼 것으로 본다.
func (r *watchTargetRunner) renamedFrom(path string) (string, bool) {
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
	old, ok := r.tree.entryPath(id)
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
func (r *watchTargetRunner) receive(ev fsnotify.Event) error {
	r.held = append(r.held, ev)
	return r.drainHeld(false)
}

// drainHeld는 미뤄 둔 이벤트를 앞에서부터 handleEvent로 넘긴다. timedOut이 true이면 맨 앞의 Create를 옛 경로의
// Rename을 기다리지 않고 넘긴다.
func (r *watchTargetRunner) drainHeld(timedOut bool) error {
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
			if err := r.handleEvent(rename); err != nil {
				return err
			}
		}
		r.held = r.held[1:]
		r.renameSource, r.holdTimeout, timedOut = "", nil, false
		if err := r.handleEvent(head); err != nil {
			return err
		}
	}
	return nil
}

// releaseHeld는 renameHoldLimit 안에 옛 경로의 Rename이 오지 않으면 미뤄 둔 이벤트를 받은 순서대로 넘긴다.
func (r *watchTargetRunner) releaseHeld() error {
	r.logger.Debug(fmt.Sprintf("rename of %s did not arrive in %v", r.renameSource, renameHoldLimit))
	return r.drainHeld(true)
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
