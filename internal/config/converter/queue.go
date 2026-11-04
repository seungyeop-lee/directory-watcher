package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// defaultQueueSize는 queue.size를 적지 않았을 때 쓰는 대기열 용량이다.
const defaultQueueSize = 2048

// QueueConverter는 option.queue 그룹을 바꾼다. size가 1보다 작으면 오류를 낸다.
type QueueConverter struct {
	key  string
	size *int
}

func NewQueueConverter(size *int) *QueueConverter {
	return &QueueConverter{key: "queue", size: size}
}

func (c QueueConverter) Convert() (domain.QueueOption, error) {
	size := defaultQueueSize
	if c.size != nil {
		size = *c.size
	}
	if size < 1 {
		return domain.QueueOption{}, fmt.Errorf("%s: size %d is less than 1", c.key, size)
	}
	return domain.QueueOption{Size: size}, nil
}
