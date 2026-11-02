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

func forceStop(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func killRecordedGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
