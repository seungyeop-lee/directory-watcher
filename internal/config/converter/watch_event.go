package converter

import (
	"fmt"
	"strings"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// noWatchEvent는 이벤트를 하나도 받지 않음을 나타내는 값이다.
const noWatchEvent = "none"

// FileEventKinds와 DirEventKinds는 fileEvent와 dirEvent에 쓸 수 있는 글자다.
// 폴더 이벤트에는 U가 없다. 폴더 안의 변경은 그 안의 파일 이벤트로 알린다.
const (
	FileEventKinds = "CUD"
	DirEventKinds  = "CD"
)

// WatchEventConverter는 option.watch.fileEvent나 option.watch.dirEvent 값을 hook에 넘길 이벤트 종류로 바꾼다.
type WatchEventConverter struct {
	key      string
	value    *config.WatchEvent
	kinds    string // 쓸 수 있는 글자
	noun     string // 오류 메시지에서 이벤트를 부르는 말
	fallback string // 값을 적지 않았을 때 쓰는 값
}

func NewFileEventConverter(value *config.WatchEvent) *WatchEventConverter {
	return &WatchEventConverter{key: "fileEvent", value: value, kinds: FileEventKinds, noun: "file", fallback: "CUD"}
}

func NewDirEventConverter(value *config.WatchEvent) *WatchEventConverter {
	return &WatchEventConverter{key: "dirEvent", value: value, kinds: DirEventKinds, noun: "folder", fallback: "CD"}
}

func (c WatchEventConverter) Convert() (domain.WatchEvent, error) {
	value := c.fallback
	if c.value != nil {
		value = string(*c.value)
	}
	if value == noWatchEvent {
		return domain.WatchEvent{}, nil
	}
	if value == "" {
		return domain.WatchEvent{}, fmt.Errorf("%s: empty value; use %s, or none", c.key, c.kindList())
	}
	for _, kind := range value {
		if !strings.ContainsRune(c.kinds, kind) {
			return domain.WatchEvent{}, fmt.Errorf("%s: %c is not a %s event; use %s, or none", c.key, kind, c.noun, c.kindList())
		}
	}
	return domain.WatchEvent{
		Create: strings.Contains(value, "C"),
		Update: strings.Contains(value, "U"),
		Delete: strings.Contains(value, "D"),
	}, nil
}

// kindList는 쓸 수 있는 글자를 "C, U, D"처럼 쉼표로 이어 돌려준다.
func (c WatchEventConverter) kindList() string {
	return strings.Join(strings.Split(c.kinds, ""), ", ")
}
