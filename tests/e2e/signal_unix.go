//go:build !windows

package e2e

import (
	"os/exec"
	"syscall"
)

func setupProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func gracefulStop(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}

const canVerifyFinishHooks = true
