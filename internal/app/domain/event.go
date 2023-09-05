package domain

import (
	"github.com/fsnotify/fsnotify"
)

type Event struct {
	Path
	Operation
}

func NewEventByFsnotify(input fsnotify.Event) Event {
	return Event{
		Path:      Path(input.Name),
		Operation: NewOperationByFsnotify(input.Op),
	}
}

const (
	Unknown           = 0
	Create  Operation = 1 << iota
	Write
	Remove
	Rename
	Chmod
)

type Operation uint

func NewOperationByFsnotify(input fsnotify.Op) Operation {
	switch input {
	case fsnotify.Create:
		return Create
	case fsnotify.Write:
		return Write
	case fsnotify.Remove:
		return Remove
	case fsnotify.Rename:
		return Rename
	case fsnotify.Chmod:
		return Chmod
	default:
		return Unknown
	}
}
