package domain

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type Cmd interface {
	Run(ctx *helper.WatcherContext, runDir Path, event *Event) error
}
