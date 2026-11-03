//go:build windows

package helper

import (
	"context"
	"fmt"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// minRunBeforeBreak는 hook 프로세스가 시작된 뒤 CTRL+BREAK를 보내기까지 기다리는 최소 시간이다.
// Go 런타임은 콘솔 제어 처리기를 등록한 뒤 그 처리기가 쓸 스레드를 준비하는데, 그 사이에 CTRL+BREAK를
// 받은 Go 프로그램은 0xC0000005로 끝난다. UTM의 Windows 11 ARM64 VM에서 cmd.exe로 Go 프로그램을 시작해
// 재어 보니, 충돌은 cmd.exe가 시작된 뒤 15–36ms에 보낸 신호에서 나왔고 80ms까지는 신호가 Go 프로그램에
// 닿지 않고 사라지기도 했다. 100ms 이후에 보낸 370번에서는 둘 다 나오지 않았다.
const minRunBeforeBreak = 100 * time.Millisecond

// holdEarlyCancel은 cmd.Cancel이 hook 프로세스가 시작된 뒤 minRunBeforeBreak가 지나야 신호를 보내게
// 감싼다. 프로세스를 시작한 직후 돌려받은 함수를 불러 시작 시각을 알려야 한다. 그 전에 취소되면
// 취소된 때부터 minRunBeforeBreak를 기다린다.
func holdEarlyCancel(cmd *exec.Cmd) (markStarted func()) {
	cancel := cmd.Cancel
	if cancel == nil {
		return func() {}
	}
	var started atomic.Pointer[time.Time]
	cmd.Cancel = func() error {
		wait := minRunBeforeBreak
		if at := started.Load(); at != nil {
			wait -= time.Since(*at)
		}
		time.Sleep(wait)
		return cancel()
	}
	return func() {
		now := time.Now()
		started.Store(&now)
	}
}

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
