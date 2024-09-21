package converter

import (
	"strings"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type WatchEventConverter struct {
	watchEvent config.WatchEvent
}

func NewWatchEventConverter(watchEvent config.WatchEvent) *WatchEventConverter {
	return &WatchEventConverter{watchEvent: watchEvent}
}

func (c WatchEventConverter) Convert() domain.WatchEvent {
	if c.watchEvent == "" {
		return domain.WatchAllEvent()
	}

	watchEventStr := string(c.watchEvent)
	create := strings.Contains(watchEventStr, "C")
	update := strings.Contains(watchEventStr, "U")
	del := strings.Contains(watchEventStr, "D")

	return domain.WatchEvent{
		Create: create,
		Update: update,
		Delete: del,
	}
}
