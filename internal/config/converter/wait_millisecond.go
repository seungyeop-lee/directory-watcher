package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"
)

const defaultWaitMillisecondValue config.Millisecond = 100

type WaitMillisecondConverter struct {
	millisecond config.Millisecond
}

func NewWaitMillisecondConverter(millisecond config.Millisecond) *WaitMillisecondConverter {
	return &WaitMillisecondConverter{millisecond: millisecond}
}

func (c WaitMillisecondConverter) Convert() domain.Millisecond {
	if c.millisecond == 0 {
		return domain.Millisecond(defaultWaitMillisecondValue)
	} else {
		return domain.Millisecond(c.millisecond)
	}
}
