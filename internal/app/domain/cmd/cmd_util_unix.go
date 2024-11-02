//go:build !windows

package cmd

import (
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

	// 최대 30초 동안 정상 종료를 기다립니다.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// 프로세스가 종료됨
	case <-time.After(30 * time.Second):
		// 30초 내에 종료되지 않으면 강제 종료합니다.
		if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		<-done
	}

	return nil
}

func setupForOs(cmd *exec.Cmd) {
	//Setpgid: true로 설정하면 프로세스 그룹이 만들어지고, 자식 프로세스들도 이 그룹에 포함 됨
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
