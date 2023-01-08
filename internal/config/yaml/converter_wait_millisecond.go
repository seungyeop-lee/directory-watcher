package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

const defaultWaitMillisecondValue Millisecond = 100

type WaitMillisecondConverter struct {
	millisecond Millisecond
}

func NewWaitMillisecondConverter(millisecond Millisecond) *WaitMillisecondConverter {
	return &WaitMillisecondConverter{millisecond: millisecond}
}

func (c WaitMillisecondConverter) Convert() domain.Millisecond {
	if c.millisecond == 0 {
		return domain.Millisecond(defaultWaitMillisecondValue)
	} else {
		return domain.Millisecond(c.millisecond)
	}
}
