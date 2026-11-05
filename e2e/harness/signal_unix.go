//go:build !windows

package harness

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

// processExit는 Unix에서 종료를 돌려주지 않는다. 0xc0000142 보고는 Windows에서만 생긴다.
func processExit(*exec.Cmd) (consoleEvent, bool) { return consoleEvent{}, false }

func forceStop(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// processObserver는 Unix에서 프로세스별 상태를 두지 않는다: signal 0과 프로세스 그룹
// kill이 기록된 PID를 직접 대상으로 삼는다.
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
