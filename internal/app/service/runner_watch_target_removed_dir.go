package service

import (
	"errors"
	"os"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// dirRemoveRepeatOS는 지운 폴더의 REMOVE를 fsnotify가 한 번 더 보낼 수 있는 운영체제인지 나타낸다. 부모 폴더의
// 감시와 그 폴더 자신의 감시가 각각 REMOVE를 보내는 일이 Windows에서는 대부분, kqueue에서는 가끔 생긴다. inotify는
// 부모 폴더를 감시하면 폴더 자신의 삭제를 fsnotify가 버리므로 한 번만 보내고, 지운 폴더의 후행 WRITE도 보내지 않는다.
var dirRemoveRepeatOS = !inotifyOS

// removedDirTTL은 removedDirRecords가 지운 폴더를 기억하는 시간이다. 감시 루프가 처리한 시각으로 잰 원래 REMOVE와
// 중복 REMOVE·후행 WRITE의 간격은 Windows ARM64에서 가장 길게 6.3ms, macOS에서 0.6ms였다(2026-11-04). 다만 kqueue에서는
// renamePairer가 그 사이의 이벤트를 renameHoldLimit까지 미룰 수 있으므로 그보다 길게 둔다.
const removedDirTTL = 2 * time.Second

// removedDirRecords는 dirRemoveRepeatOS에서 지운 폴더와 지운 때를 removedDirTTL 동안 기억한다. 감시와 항목 기록을
// 지운 뒤 오는 중복 REMOVE와 WRITE는 버리게 하고, CREATE나 디스크에서 확인한 새 항목은 별도 수명으로 넘기게 한다.
// 감시 루프의 고루틴에서만 부른다.
type removedDirRecords struct {
	now func() time.Time
	at  map[string]time.Time
}

func newRemovedDirRecords() *removedDirRecords {
	return &removedDirRecords{now: time.Now, at: map[string]time.Time{}}
}

// add는 dirRemoveRepeatOS이면 지운 폴더 path를 적고, removedDirTTL이 지난 기록은 지운다.
func (d *removedDirRecords) add(path string) {
	if !dirRemoveRepeatOS {
		return
	}
	now := d.now()
	for recorded, at := range d.at {
		if now.Sub(at) > removedDirTTL {
			delete(d.at, recorded)
		}
	}
	d.at[path] = now
}

// isRepeatedRemove는 path의 REMOVE가 removedDirTTL 안에 지운 폴더의 중복 REMOVE인지 판단한다. recorded는 그 REMOVE를
// 받았을 때 path가 항목 기록에 있었는지이고, 기록에 있던 경로의 REMOVE는 중복으로 보지 않는다.
func (d *removedDirRecords) isRepeatedRemove(path string, recorded bool) bool {
	return !recorded && d.has(path)
}

// classifyWrite는 path의 WRITE가 removedDirTTL 안에 지운 폴더의 것일 때, 디스크에서 그 경로를 확인해 판단한다.
// 경로가 없으면 stale이 true라 버릴 WRITE이고, 있으면 recreated가 true라 CREATE를 놓친 새 항목의 WRITE이며 그
// 경로의 기록을 지운다. 지운 폴더가 아니거나 확인하지 못하면 둘 다 false다.
func (d *removedDirRecords) classifyWrite(path string) (stale, recreated bool) {
	if !d.has(path) {
		return false, false
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return true, false
	} else if err != nil {
		return false, false
	}
	delete(d.at, path)
	return false, true
}

// forgetUnder는 path에 항목이 새로 생겼으므로 path와 그 아래 경로의 기록을 지운다.
func (d *removedDirRecords) forgetUnder(path string) {
	for recorded := range d.at {
		if domain.Path(path).Contains(domain.Path(recorded)) {
			delete(d.at, recorded)
		}
	}
}

// has는 path가 removedDirTTL 안에 지운 폴더인지 판단한다.
func (d *removedDirRecords) has(path string) bool {
	at, ok := d.at[path]
	return ok && d.now().Sub(at) <= removedDirTTL
}
