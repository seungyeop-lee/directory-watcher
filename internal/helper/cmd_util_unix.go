//go:build !windows

package helper

import (
	"errors"
	"os/exec"
	"syscall"
	"time"
)

func terminateProcess(cmd *exec.Cmd) error {
	// 프로세스와 자식 프로세스에 SIGTERM 신호를 보냅니다.
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		//-cmd.Process.Pid를 사용하여 프로세스 그룹 전체에 SIGTERM 신호 발송 (음수 PID 사용)
		err := syscall.Kill(-pgid, syscall.SIGTERM)
		if err != nil {
			return err
		}
	} else {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}
	}

	return nil
}

type processTree struct{}

// startProcessTree는 Unix에서 cmd를 그대로 시작한다. 취소된 명령에는 시작 직후에도 SIGTERM을 바로 보내고,
// 자손은 SetupForOs가 만든 프로세스 그룹으로 함께 신호를 받는다.
func startProcessTree(cmd *exec.Cmd, _ bool) (processTree, error) {
	return processTree{}, cmd.Start()
}

func releaseProcessTree(processTree) {}

func cleanupProcessTree(cmd *exec.Cmd, _ processTree, deadline time.Time) error {
	// SetupForOs가 명령에 프로세스 그룹을 만들어 주며, 이 그룹은 그룹 리더가 끝난 뒤에도 남는다.
	pgid := -cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
		return nil
	}
	for {
		if errors.Is(syscall.Kill(pgid, 0), syscall.ESRCH) {
			return nil
		}
		if !time.Now().Before(deadline) {
			_ = syscall.Kill(pgid, syscall.SIGKILL)
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
