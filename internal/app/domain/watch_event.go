package domain

import (
	"github.com/fsnotify/fsnotify"
)

type WatchEvent struct {
	Create bool
	Update bool
	Delete bool
}

func (e WatchEvent) IsListening(ev fsnotify.Event) bool {
	// 삭제된 경우, 경로를 통해 디렉토리인지를 판단 할 수 없는 한계가 있음.
	if Path(ev.Name).IsDir() {
		return false
	}

	switch {
	case e.Create && ev.Op.Has(fsnotify.Create):
		return true
	case e.Update && ev.Op.Has(fsnotify.Write):
		return true
	case e.Delete && ev.Op.Has(fsnotify.Remove):
		return true
	default:
		return false
	}
}

func WatchAllEvent() WatchEvent {
	return WatchEvent{
		Create: true,
		Update: true,
		Delete: true,
	}
}
