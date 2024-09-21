package domain

import (
	"github.com/fsnotify/fsnotify"
)

type WatchEvent struct {
	Create bool
	Update bool
	Delete bool
}

func (e WatchEvent) IsListening(op fsnotify.Op) bool {
	if e.Create && op.Has(fsnotify.Create) {
		return true
	}
	if e.Update && op.Has(fsnotify.Write) {
		return true
	}
	if e.Delete && op.Has(fsnotify.Remove) {
		return true
	}
	return false
}

func WatchAllEvent() WatchEvent {
	return WatchEvent{
		Create: true,
		Update: true,
		Delete: true,
	}
}
