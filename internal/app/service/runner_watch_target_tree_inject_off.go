//go:build !dw_e2e_inject

package service

import "github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"

// injectWatchErrors는 dw_e2e_inject build tag 없이 빌드하면 errs를 그대로 돌려준다. 오류를 주입하는 구현은
// runner_watch_target_tree_inject.go에 있다.
func injectWatchErrors(_ domain.Path, errs <-chan error, _ <-chan struct{}) <-chan error { return errs }
