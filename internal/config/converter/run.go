package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// defaultConcurrency는 run.concurrency를 적지 않았을 때 동시에 실행하는 hook의 수다.
const defaultConcurrency = 1

// RunConverter는 option.run 그룹을 바꾼다. concurrency가 1보다 작으면 오류를 낸다.
type RunConverter struct {
	key         string
	concurrency *int
	interrupt   *string
}

func NewRunConverter(concurrency *int, interrupt *string) *RunConverter {
	return &RunConverter{key: "run", concurrency: concurrency, interrupt: interrupt}
}

func (c RunConverter) Convert() (domain.RunPolicy, error) {
	concurrency := defaultConcurrency
	if c.concurrency != nil {
		concurrency = *c.concurrency
	}
	if concurrency < 1 {
		return domain.RunPolicy{}, fmt.Errorf("%s: concurrency %d is less than 1", c.key, concurrency)
	}
	interrupt, err := NewInterruptConverter(c.interrupt).Convert()
	if err != nil {
		return domain.RunPolicy{}, WithKey(c.key, err)
	}
	return domain.RunPolicy{Concurrency: concurrency, Interrupt: interrupt}, nil
}
