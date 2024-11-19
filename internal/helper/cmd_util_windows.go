//go:build windows

package helper

import (
	"errors"
	"os/exec"
	"syscall"
)

func terminateProcess(cmd *exec.Cmd) error {
	// 프로세스 그룹에 CTRL_BREAK_EVENT를 보냅니다.
	err := sendCtrlBreak(cmd.Process.Pid)
	if err != nil {
		// CTRL_BREAK_EVENT 실패 시 직접 종료
		return cmd.Process.Kill()
	}
	return nil
}

func sendCtrlBreak(pid int) error {
	dll, err := syscall.LoadDLL("kernel32.dll")
	if err != nil {
		return err
	}
	defer dll.Release()

	proc, err := dll.FindProc("GenerateConsoleCtrlEvent")
	if err != nil {
		return err
	}

	// CTRL_BREAK_EVENT를 프로세스 그룹에 보냅니다.
	r, _, err := proc.Call(uintptr(syscall.CTRL_BREAK_EVENT), uintptr(pid))
	if r == 0 {
		return err
	}
	return nil
}

func SetupForOs(cmd *exec.Cmd) error {
	// 새로운 프로세스 그룹을 생성합니다.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return nil
}

func postProcessForCancel(cmd *exec.Cmd) error {
	// cmd.Wait()를 호출하지 않고 프로세스를 종료합니다.
	err := terminateProcess(cmd)
	if err != nil {
		if err.Error() == "invalid argument" {
			return nil
		}
		return errors.New("terminate process error: " + err.Error())
	}
	return nil
}
