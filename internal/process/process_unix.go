//go:build !windows

package process

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

// terminateProcess는 cmd와 자식 프로세스에 SIGTERM을 보낸다. cmd의 프로세스 그룹을 찾지 못하면 cmd에만 보낸다.
func terminateProcess(cmd *exec.Cmd) error {
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
		return signalGroup(pgid, syscall.SIGTERM)
	}
	return cmd.Process.Signal(syscall.SIGTERM)
}

// signalGroup은 프로세스 그룹 pgid에 속한 프로세스 모두에 sig를 보낸다. kill에 음수 pid를 넘기면 그 그룹 전체가 신호를 받는다.
func signalGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

type processTree struct{}

// startProcessTree는 Unix에서 cmd를 그대로 시작한다. 취소된 명령에는 시작 직후에도 SIGTERM을 바로 보내고,
// 자손은 SetupForOs가 만든 프로세스 그룹으로 함께 신호를 받는다.
func startProcessTree(cmd *exec.Cmd, _ bool) (processTree, error) {
	return processTree{}, cmd.Start()
}

func releaseProcessTree(processTree) {}

// logProcessExit는 Unix에서 아무것도 남기지 않는다. 종료 기록은 Windows의 CTRL+BREAK 진단에만 쓴다.
func logProcessExit(*exec.Cmd) {}

func cleanupProcessTree(cmd *exec.Cmd, _ processTree, deadline time.Time) error {
	// SetupForOs가 명령의 pid를 id로 쓰는 프로세스 그룹을 만들어 주며, 이 그룹은 그룹 리더가 끝난 뒤에도 남는다.
	pgid := cmd.Process.Pid
	if err := signalGroup(pgid, syscall.SIGTERM); err != nil {
		return nil
	}
	for {
		if errors.Is(signalGroup(pgid, 0), syscall.ESRCH) {
			return nil
		}
		if !time.Now().Before(deadline) {
			_ = signalGroup(pgid, syscall.SIGKILL)
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func SetupForOs(cmd *exec.Cmd) {
	//Setpgid: true로 설정하면 프로세스 그룹이 만들어지고, 자식 프로세스들도 이 그룹에 포함 됨
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
