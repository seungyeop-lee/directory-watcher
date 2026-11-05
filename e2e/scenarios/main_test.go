package scenarios

import (
	"os"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// 시나리오는 실행 폴더와 watcher를 따로 가지므로 t.Parallel로 함께 실행한다. t.Parallel을 부르지 않은 최상위
// 테스트는 병렬 테스트가 시작되기 전에 혼자 실행되므로, 시간 한도가 빠듯하거나 콘솔처럼 프로세스 밖의 상태를
// 바꾸는 시나리오는 t.Parallel을 부르지 않는다.
func TestMain(m *testing.M) { os.Exit(harness.Run(m)) }
