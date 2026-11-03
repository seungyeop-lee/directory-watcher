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

// lockVM coordinates runs on this Mac that use the same VM. Test runs share
// the lock; starting or stopping the VM needs it alone, so that down or all
// never stops the VM under another run's test. The lock is released when the
// returned file is closed or the process exits.
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
