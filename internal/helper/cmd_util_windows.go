//go:build windows

package helper

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func terminateProcess(cmd *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}

type processTree windows.Handle

func startProcessTree(cmd *exec.Cmd) (processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("create notification job: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("configure notification job: %w", err)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// 일시 중단된 부모는 job에 속하기 전에는 자손을 만들 수 없다.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err == nil {
		err = resumeProcess(cmd.Process.Pid)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("start notification job: %w", err)
	}
	return processTree(job), nil
}

func resumeProcess(pid int) error {
	// os/exec는 주 스레드 handle을 닫으므로, 프로세스가 일시 중단된 동안 handle을 찾는다.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		count, err := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("primary thread suspend count = %d, want 1", count)
		}
		return nil
	}
	return fmt.Errorf("find suspended primary thread: %w", err)
}

type jobAccounting struct {
	totalUserTime, totalKernelTime                     int64
	thisPeriodTotalUserTime, thisPeriodTotalKernelTime int64
	totalPageFaultCount, totalProcesses                uint32
	activeProcesses, totalTerminatedProcesses          uint32
}

func cleanupProcessTree(_ *exec.Cmd, tree processTree, deadline time.Time) error {
	job := windows.Handle(tree)
	defer windows.CloseHandle(job)
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return fmt.Errorf("terminate notification job: %w", err)
	}
	for {
		var accounting jobAccounting
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("query notification job: %w", err)
		}
		if accounting.activeProcesses == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("wait for notification job cleanup: %w", context.DeadlineExceeded)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func SetupForOs(cmd *exec.Cmd) error {
	// 새로운 프로세스 그룹을 생성합니다. cmd 셸이 정한 명령줄(CmdLine)은 그대로 둡니다.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	return nil
}
