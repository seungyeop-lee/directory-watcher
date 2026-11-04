package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

var interruptValues = map[string]domain.Interrupt{
	"none":     domain.InterruptNone,
	"samePath": domain.InterruptSamePath,
	"oldest":   domain.InterruptOldest,
}

// InterruptConverter는 option.run.interrupt 값을 바꾼다. 값을 적지 않았으면 none을 쓴다.
type InterruptConverter struct {
	interrupt *string
}

func NewInterruptConverter(interrupt *string) *InterruptConverter {
	return &InterruptConverter{interrupt: interrupt}
}

func (c InterruptConverter) Convert() (domain.Interrupt, error) {
	if c.interrupt == nil {
		return domain.InterruptNone, nil
	}
	interrupt, ok := interruptValues[*c.interrupt]
	if !ok {
		return 0, fmt.Errorf("interrupt: unknown value %q; use none, samePath, or oldest", *c.interrupt)
	}
	return interrupt, nil
}
