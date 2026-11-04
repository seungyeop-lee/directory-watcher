package converter

import (
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

// 기다리는 시간을 적지 않았을 때 쓰는 값이다.
const defaultWaitMillisecond, defaultMaxWaitMillisecond domain.Millisecond = 100, 1000

// batchWait는 dirBatch와 hookBatch의 기다리는 시간에 같은 규칙을 적용한다. key는 오류 메시지 앞에 붙이는 그룹 키다.
// 값을 적지 않은 것과 0을 구별하며, 0은 0 그대로 둔다. maxWaitMillisecond를 적지 않으면 waitMillisecond만 늘려 적어도
// 시작되도록 기본값 1000과 waitMillisecond 중 큰 값을 쓴다. 직접 적은 maxWaitMillisecond가 waitMillisecond보다 작으면
// 오류를 낸다.
func batchWait(key string, waitConfig, maxWaitConfig *config.Millisecond) (wait, maxWait domain.Millisecond, err error) {
	wait = defaultWaitMillisecond
	if waitConfig != nil {
		wait = domain.Millisecond(*waitConfig)
	}
	maxWait = max(defaultMaxWaitMillisecond, wait)
	if maxWaitConfig != nil {
		maxWait = domain.Millisecond(*maxWaitConfig)
	}
	if maxWait < wait {
		return 0, 0, fmt.Errorf("%s: maxWaitMillisecond %d is less than waitMillisecond %d", key, maxWait, wait)
	}
	return wait, maxWait, nil
}
