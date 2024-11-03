package domain

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type Cmd interface {
	Run(ctx *helper.WatcherContext, runDir Path, event *Event) error
}
