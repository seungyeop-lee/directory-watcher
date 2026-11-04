package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// MillisecondConverter는 기다리는 시간 값을 바꾼다. 값을 적지 않았으면 fallback을 쓰고, 0은 0 그대로 둔다.
type MillisecondConverter struct {
	millisecond *config.Millisecond
	fallback    config.Millisecond
}

func NewMillisecondConverter(millisecond *config.Millisecond, fallback config.Millisecond) *MillisecondConverter {
	return &MillisecondConverter{millisecond: millisecond, fallback: fallback}
}

func (c MillisecondConverter) Convert() domain.Millisecond {
	if c.millisecond == nil {
		return domain.Millisecond(c.fallback)
	}
	return domain.Millisecond(*c.millisecond)
}
