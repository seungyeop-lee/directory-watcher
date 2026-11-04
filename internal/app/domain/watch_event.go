package domain

import (
	"github.com/fsnotify/fsnotify"
)

// WatchEvent는 hook에 넘길 이벤트 종류다. 파일 이벤트와 폴더 이벤트를 따로 둔다.
type WatchEvent struct {
	Create bool
	Update bool
	Delete bool
}

// Has는 이벤트 종류 op를 hook에 넘기는지 판단한다.
func (e WatchEvent) Has(op Operation) bool {
	switch fsnotify.Op(op) {
	case fsnotify.Create:
		return e.Create
	case fsnotify.Write:
		return e.Update
	case fsnotify.Remove:
		return e.Delete
	default:
		return false
	}
}

// IsNone은 어떤 이벤트 종류도 hook에 넘기지 않는지 판단한다.
func (e WatchEvent) IsNone() bool {
	return !e.Create && !e.Update && !e.Delete
}

func WatchAllEvent() WatchEvent {
	return WatchEvent{
		Create: true,
		Update: true,
		Delete: true,
	}
}
