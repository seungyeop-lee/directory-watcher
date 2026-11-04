package domain

import (
	"github.com/fsnotify/fsnotify"
)

// Event는 hook에 넘기는 이벤트다. Operation은 fsnotify.Create, fsnotify.Write, fsnotify.Remove 중 하나다.
type Event struct {
	Path
	Operation
	// IsDir은 Path가 폴더인지를 나타낸다. 폴더 이벤트의 Operation은 Create와 Remove뿐이다.
	IsDir bool
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
