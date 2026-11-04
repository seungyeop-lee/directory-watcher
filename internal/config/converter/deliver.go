package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

var deliverValues = map[string]domain.Deliver{
	"last":        domain.DeliverLast,
	"lastPerPath": domain.DeliverLastPerPath,
	"each":        domain.DeliverEach,
}

// DeliverConverter는 option.hookBatch.deliver 값을 바꾼다. 값을 적지 않았으면 last를 쓴다.
type DeliverConverter struct {
	deliver *string
}

func NewDeliverConverter(deliver *string) *DeliverConverter {
	return &DeliverConverter{deliver: deliver}
}

func (c DeliverConverter) Convert() (domain.Deliver, error) {
	if c.deliver == nil {
		return domain.DeliverLast, nil
	}
	deliver, ok := deliverValues[*c.deliver]
	if !ok {
		return 0, fmt.Errorf("deliver: unknown value %q; use last, lastPerPath, or each", *c.deliver)
	}
	return deliver, nil
}
