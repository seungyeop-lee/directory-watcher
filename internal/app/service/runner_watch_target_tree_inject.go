//go:build dw_e2e_inject

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// 이 파일은 dw_e2e_inject build tag로 빌드할 때만 들어간다. e2e/harness가 이 tag로 빌드한 watcher로
// TestInjectedLossNotificationCLI가 watcher 오류 채널로 들어온 오류의 알림과 종료 코드를 확인한다.
// 표지 파일 이름과 내용은 그 시나리오가 같은 값을 쓴다.

// injectMarker는 감시 대상 폴더에 생기면 오류를 보내게 하는 표지 파일의 이름이다.
const injectMarker = "dw-e2e-error.inject"

// injectedCauses는 표지 파일의 내용별로 보낼 첫 오류다. 유실 오류의 원인에는 템플릿과 셸 문자를 넣어,
// DW_ERROR_CAUSE로 그대로 넘어가는지 시나리오가 확인한다.
var injectedCauses = map[string]error{
	"loss":    fmt.Errorf("injected 'cause' {{.Path}} $HOME: %w", fsnotify.ErrEventOverflow),
	"nonloss": errors.New("injected non-loss failure"),
}

// errInjectedLater는 첫 오류 뒤에 보내는 오류다. 감시 루프는 첫 오류로 멈추므로 이 오류가 원인으로 보이면 안 된다.
var errInjectedLater = errors.New("later injected error must not replace the first")

// injectWatchErrors는 errs의 오류를 그대로 넘기는 채널을 돌려주고, path 폴더에 injectMarker가 생기면 그 내용에
// 맞는 injectedCauses의 오류와 errInjectedLater를 차례로 보낸다. 표지 파일은 10ms마다 읽고 한 번만 반응한다.
// errs가 닫히면 돌려준 채널도 닫고, stop이 닫히면 닫지 않고 멈춘다.
func injectWatchErrors(path domain.Path, errs <-chan error, stop <-chan struct{}) <-chan error {
	out := make(chan error)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		tick := ticker.C
		send := func(err error) bool {
			select {
			case out <- err:
				return true
			case <-stop:
				return false
			}
		}
		for {
			select {
			case <-stop:
				return
			case err, ok := <-errs:
				if !ok {
					close(out)
					return
				}
				if !send(err) {
					return
				}
			case <-tick:
				data, err := os.ReadFile(filepath.Join(path.String(), injectMarker))
				cause, known := injectedCauses[string(data)]
				if err != nil || !known {
					continue
				}
				tick = nil
				if !send(cause) || !send(errInjectedLater) {
					return
				}
			}
		}
	}()
	return out
}
