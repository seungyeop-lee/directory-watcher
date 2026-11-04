package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// HookBatchConverter는 option.hookBatch 그룹을 바꾼다.
type HookBatchConverter struct {
	key     string
	wait    *config.Millisecond
	maxWait *config.Millisecond
	deliver *string
}

func NewHookBatchConverter(wait, maxWait *config.Millisecond, deliver *string) *HookBatchConverter {
	return &HookBatchConverter{key: "hookBatch", wait: wait, maxWait: maxWait, deliver: deliver}
}

func (c HookBatchConverter) Convert() (domain.HookBatchOption, error) {
	wait, maxWait, err := batchWait(c.key, c.wait, c.maxWait)
	if err != nil {
		return domain.HookBatchOption{}, err
	}
	deliver, err := NewDeliverConverter(c.deliver).Convert()
	if err != nil {
		return domain.HookBatchOption{}, WithKey(c.key, err)
	}
	return domain.HookBatchOption{WaitMillisecond: wait, MaxWaitMillisecond: maxWait, Deliver: deliver}, nil
}
