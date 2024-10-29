//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

func terminateProcess(pid int) error {
	//-cmd.Process.Pid를 사용하여 프로세스 그룹 전체에 SIGINT 신호 발송 (음수 PID 사용)
	return syscall.Kill(-pid, syscall.SIGINT)
}

func setupForOs(cmd *exec.Cmd) {
	//Setpgid: true로 설정하면 프로세스 그룹이 만들어지고, 자식 프로세스들도 이 그룹에 포함 됨
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
