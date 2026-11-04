package domain

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type Cmd interface {
	Run(ctx *process.WatcherContext, runDir Path, event *Event) error
}
