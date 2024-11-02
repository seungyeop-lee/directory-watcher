//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
	"time"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	terminateJobObject       = kernel32.NewProc("TerminateJobObject")
)

// 프로세스를 관리하기 위한 전역 변수
var job syscall.Handle

func terminateProcess(cmd *exec.Cmd) error {
	// 프로세스 그룹에 CTRL_BREAK_EVENT를 보냅니다.
	err := sendCtrlBreak(cmd.Process.Pid)
	if err != nil {
		return err
	}

	// 최대 30초 동안 정상 종료를 기다립니다.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// 프로세스가 종료됨
		if job != 0 {
			_ = syscall.CloseHandle(job)
			job = 0
		}
		return err
	case <-time.After(30 * time.Second):
		// 30초 내에 종료되지 않으면 프로세스 그룹을 강제 종료합니다.
		if job != 0 {
			_, _, _ = terminateJobObject.Call(uintptr(job), 1)
			_ = syscall.CloseHandle(job)
			job = 0
		} else {
			_ = cmd.Process.Kill()
		}
		return <-done
	}
}

func setupForOs(cmd *exec.Cmd) error {
	// 새로운 프로세스 그룹을 생성합니다.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if cmd.Process == nil {
		return nil // 프로세스가 아직 시작되지 않았으면 리턴
	}

	// 프로세스 핸들을 얻습니다.
	const PROCESS_ALL_ACCESS = 0x1F0FFF
	processHandle, err := syscall.OpenProcess(PROCESS_ALL_ACCESS, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}

	// Job Object를 생성하여 프로세스를 할당합니다.
	h, err := createJobObject()
	if err != nil {
		_ = syscall.CloseHandle(processHandle)
		return err
	}

	if err := assignToJob(h, processHandle); err != nil {
		_ = syscall.CloseHandle(processHandle)
		_ = syscall.CloseHandle(h)
		return err
	}

	_ = syscall.CloseHandle(processHandle) // 프로세스 핸들을 닫습니다.
	job = h
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

// Job Object를 생성하는 함수
func createJobObject() (syscall.Handle, error) {
	h, _, err := createJobObjectW.Call(0, 0)
	if h == 0 {
		return 0, err
	}
	return syscall.Handle(h), nil
}

// 프로세스를 Job Object에 할당하는 함수
func assignToJob(job syscall.Handle, process syscall.Handle) error {
	r, _, err := assignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r == 0 {
		return err
	}
	return nil
}
