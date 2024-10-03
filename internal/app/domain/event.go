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
		Operation: Operation(input.Op),
	}
}

type Operation fsnotify.Op

func (o Operation) String() string {
	op := fsnotify.Op(o)
	switch {
	case op.Has(fsnotify.Create):
		return "C"
	case op.Has(fsnotify.Write):
		return "U"
	case op.Has(fsnotify.Remove):
		return "D"
	default:
		return "Unknown"
	}
}
