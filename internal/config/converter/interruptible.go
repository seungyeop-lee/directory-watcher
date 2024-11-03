package converter

import "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"

const defaultInterruptible bool = false

type InterruptibleConverter struct {
	interruptible *bool
}

func NewInterruptibleConverter(interruptible *bool) *InterruptibleConverter {
	return &InterruptibleConverter{
		interruptible: interruptible,
	}
}

func (c *InterruptibleConverter) Convert() domain.Interruptible {
	if c.interruptible == nil {
		return domain.Interruptible(defaultInterruptible)
	} else {
		return domain.Interruptible(*c.interruptible)
	}
}
