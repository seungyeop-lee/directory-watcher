//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var errVMBusy = errors.New("another Windows E2E run on this Mac is using the VM")

// lockVM은 이 Mac에서 같은 VM을 쓰는 실행들을 조율한다. 테스트 실행은 잠금을 함께
// 잡고, VM을 시작하거나 멈출 때는 잠금을 혼자 잡아야 하므로, down이나 all이 다른
// 실행의 테스트가 도는 중에 VM을 멈추지 않는다. 잠금은 돌려받은 파일을 닫거나
// 프로세스가 끝나면 풀린다.
func lockVM(uuid string, exclusive bool) (*os.File, error) {
	path := filepath.Join(os.TempDir(), "dw-e2e-windows-"+uuid+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), how|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w (lock %s)", errVMBusy, path)
		}
		return nil, err
	}
	return file, nil
}
