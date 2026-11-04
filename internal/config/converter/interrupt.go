package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

var interruptValues = choices[domain.Interrupt]{
	{"none", domain.InterruptNone},
	{"samePath", domain.InterruptSamePath},
	{"oldest", domain.InterruptOldest},
}

// InterruptNames는 설정 파일의 interrupt에 쓸 수 있는 값을 오류 메시지가 안내하는 순서대로 돌려준다.
func InterruptNames() []string { return interruptValues.names() }

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
	interrupt, ok := interruptValues.lookup(*c.interrupt)
	if !ok {
		return 0, fmt.Errorf("interrupt: unknown value %q; use %s", *c.interrupt, interruptValues.list())
	}
	return interrupt, nil
}
