package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

var deliverValues = choices[domain.Deliver]{
	{"last", domain.DeliverLast},
	{"lastPerPath", domain.DeliverLastPerPath},
	{"each", domain.DeliverEach},
}

// DeliverNames는 설정 파일의 deliver에 쓸 수 있는 값을 오류 메시지가 안내하는 순서대로 돌려준다.
func DeliverNames() []string { return deliverValues.names() }

// DeliverConverter는 option.hookBatch.deliver 값을 바꾼다. 값을 적지 않았으면 last를 쓴다.
type DeliverConverter struct {
	key     string
	deliver *string
}

func NewDeliverConverter(deliver *string) *DeliverConverter {
	return &DeliverConverter{key: "deliver", deliver: deliver}
}

func (c DeliverConverter) Convert() (domain.Deliver, error) {
	if c.deliver == nil {
		return domain.DeliverLast, nil
	}
	deliver, ok := deliverValues.lookup(*c.deliver)
	if !ok {
		return 0, fmt.Errorf("%s: unknown value %q; use %s", c.key, *c.deliver, deliverValues.list())
	}
	return deliver, nil
}
