package harness

import (
	"testing"
	"time"
)

func TestAwaitProgressWaitsWhileProgressContinues(t *testing.T) {
	const idle = 100 * time.Millisecond
	started := time.Now()
	calls := 0
	ok := awaitProgress(idle, func() (bool, int) {
		calls++
		return time.Since(started) >= 5*idle, calls
	})
	if !ok {
		t.Fatalf("failed after %s although progress changed on every check", time.Since(started))
	}
}

func TestAwaitProgressFailsWhenProgressStops(t *testing.T) {
	const idle = 100 * time.Millisecond
	started := time.Now()
	calls := 0
	ok := awaitProgress(idle, func() (bool, int) {
		calls++
		// 받은 이벤트 하나의 hook이 기록을 남기지 않아 완료 조건이 끝내 참이 되지 않는 경우처럼, 진행이 3에서 멈춘다.
		return false, min(calls, 3)
	})
	if elapsed := time.Since(started); ok || elapsed < idle {
		t.Fatalf("awaitProgress = %t after %s, want false after at least %s without progress", ok, elapsed, idle)
	}
}

// 진행 값이 바뀌지 않으면 고정 대기와 같이, 한도는 호출한 때부터 세고 condition을 확인한 직후에만 판정한다.
func TestAwaitProgressWithoutProgressMatchesFixedWait(t *testing.T) {
	const idle = 100 * time.Millisecond
	for _, tc := range []struct {
		name string
		done bool
		want bool
	}{
		{"condition met on a check that ends past the limit", true, true},
		{"limit passed during the first check", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ok := awaitProgress(idle, func() (bool, int) {
				calls++
				time.Sleep(idle + 50*time.Millisecond)
				return tc.done, 0
			})
			if ok != tc.want || calls != 1 {
				t.Fatalf("awaitProgress = %t after %d checks, want %t after 1 check", ok, calls, tc.want)
			}
		})
	}
}
