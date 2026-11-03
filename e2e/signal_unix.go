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

// processObserver keeps no per-process state on Unix: signal 0 and process
// group kills address recorded PIDs directly.
type processObserver struct{}

func newProcessObserver() *processObserver { return &processObserver{} }

func (*processObserver) alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func (*processObserver) killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

func (*processObserver) kill(pid int) {
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func (*processObserver) writeDiagnostics(file string) error { return nil }

func (*processObserver) close() {}
