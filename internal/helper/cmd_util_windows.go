//go:build windows

package helper

import (
	"os/exec"
	"strconv"
	"syscall"
)

func terminateProcess(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	// https://stackoverflow.com/a/44551450
	kill := exec.Command("TASKKILL", "/T", "/F", "/PID", strconv.Itoa(pid))
	return kill.Run()
}

func SetupForOs(cmd *exec.Cmd) error {
	// 새로운 프로세스 그룹을 생성합니다.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return nil
}
