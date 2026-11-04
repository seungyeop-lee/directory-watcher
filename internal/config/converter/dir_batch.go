package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// DirBatchConverter는 option.dirBatch 그룹을 바꾼다.
type DirBatchConverter struct {
	key     string
	wait    *config.Millisecond
	maxWait *config.Millisecond
}

func NewDirBatchConverter(wait, maxWait *config.Millisecond) *DirBatchConverter {
	return &DirBatchConverter{key: "dirBatch", wait: wait, maxWait: maxWait}
}

func (c DirBatchConverter) Convert() (domain.DirBatchOption, error) {
	wait, maxWait, err := batchWait(c.key, c.wait, c.maxWait)
	if err != nil {
		return domain.DirBatchOption{}, err
	}
	return domain.DirBatchOption{WaitMillisecond: wait, MaxWaitMillisecond: maxWait}, nil
}
