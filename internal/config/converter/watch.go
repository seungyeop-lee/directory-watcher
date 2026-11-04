package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// WatchConverter는 option.watch 그룹을 바꾼다. fileEvent와 dirEvent가 둘 다 none이면 onChange가 실행될 일이 없으므로
// 오류를 낸다.
type WatchConverter struct {
	key       string
	subDir    *bool
	exclude   config.Exclude
	fileEvent *config.WatchEvent
	dirEvent  *config.WatchEvent
}

func NewWatchConverter(subDir *bool, exclude config.Exclude, fileEvent, dirEvent *config.WatchEvent) *WatchConverter {
	return &WatchConverter{key: "watch", subDir: subDir, exclude: exclude, fileEvent: fileEvent, dirEvent: dirEvent}
}

func (c WatchConverter) Convert() (domain.WatchOption, error) {
	subDir, err := NewSubDirConverter(c.subDir).Convert()
	if err != nil {
		return domain.WatchOption{}, WithKey(c.key, err)
	}
	exclude, err := NewExcludeConverter(c.exclude).Convert()
	if err != nil {
		return domain.WatchOption{}, WithKey(c.key, err)
	}
	fileEvent, err := NewFileEventConverter(c.fileEvent).Convert()
	if err != nil {
		return domain.WatchOption{}, WithKey(c.key, err)
	}
	dirEvent, err := NewDirEventConverter(c.dirEvent).Convert()
	if err != nil {
		return domain.WatchOption{}, WithKey(c.key, err)
	}
	if fileEvent.IsNone() && dirEvent.IsNone() {
		return domain.WatchOption{}, fmt.Errorf("%s: fileEvent and dirEvent are both none; onChange would never run", c.key)
	}
	return domain.WatchOption{SubDir: subDir, Exclude: exclude, FileEvent: fileEvent, DirEvent: dirEvent}, nil
}
