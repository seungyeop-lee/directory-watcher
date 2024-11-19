//go:build !windows

package helper

import (
	"os/exec"
	"syscall"
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

func SetupForOs(cmd *exec.Cmd) {
	//Setpgid: true로 설정하면 프로세스 그룹이 만들어지고, 자식 프로세스들도 이 그룹에 포함 됨
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func postProcessForCancel(cmd *exec.Cmd) error {
	// cmd.Wait()를 할 경우, cancel을 했는데도 불구하고 계속 대기하는 문제가 있음
	_, err := cmd.Process.Wait()
	if err != nil {
		if err.Error() == "wait: no child processes" {
			continue
		}
		GlobalLogger.Error("wait cancel error:" + err.Error())
	}
}
