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

func prepareConsole() error { return nil }

func forceStop(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func killRecordedGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func killRecordedProcess(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func releaseRecordedProcess(pid int) {}

func writeProcessDiagnostics(file string) error { return nil }
