//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// minRunBeforeBreak는 프로세스가 시작된 뒤 CTRL+BREAK를 보내기까지 기다리는 최소 시간이다.
// Go 런타임은 콘솔 제어 처리기를 등록한 뒤 그 처리기가 쓸 스레드를 준비하는데, 그 사이에 CTRL+BREAK를
// 받은 Go 프로그램은 0xC0000005로 끝난다. UTM의 Windows 11 ARM64 VM에서 cmd.exe로 Go 프로그램을 시작해
// 재어 보니, 충돌은 cmd.exe가 시작된 뒤 15–36ms에 보낸 신호에서 나왔고 80ms까지는 신호가 Go 프로그램에
// 닿지 않고 사라지기도 했다. 100ms 이후에 보낸 370번에서는 둘 다 나오지 않았다.
const minRunBeforeBreak = 100 * time.Millisecond

// maxFirstBreakDelay는 stopJob이 첫 CTRL+BREAK를 늦추는 최대 시간이다. 취소된 명령이 자손을 계속 만들어도 이 시간이 지나면
// 막 만들어진 프로세스가 있더라도 첫 신호를 보낸다. 빌드 도구처럼 짧은 프로세스를 잇달아 띄우는 hook의 교체가 오래 늦어지지
// 않도록 breakGrace보다 훨씬 짧게 잡았다.
const maxFirstBreakDelay = time.Second

// breakGrace는 취소된 명령에 첫 CTRL+BREAK를 보낸 뒤 job의 프로세스를 강제로 끝내기까지 기다리는 시간이다.
// 테스트만 바꾼다.
var breakGrace = shutdownGrace

// jobKillWait는 job의 프로세스를 강제로 끝낸 뒤 모두 사라지기를 기다리는 최대 시간이다.
const jobKillWait = 5 * time.Second

// jobPollInterval은 job에 남은 프로세스와 새로 들어온 프로세스를 확인하는 간격이다.
const jobPollInterval = 10 * time.Millisecond

// terminateProcess는 cmd가 리더인 프로세스 그룹에 CTRL+BREAK를 보낸다. 그룹은 리더의 PID로 고르므로, 리더가 이미 끝났어도
// processTree가 리더의 handle을 쥐고 있어야 그 PID가 다른 프로세스에 재사용되지 않는다.
func terminateProcess(cmd *exec.Cmd) error {
	sent := time.Now()
	err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
	message := fmt.Sprintf("sent CTRL+BREAK to process group %d at %s", cmd.Process.Pid, debugTime(sent))
	if err != nil {
		message += fmt.Sprintf(" (error: %v)", err)
	}
	logDebug(message)
	return err
}

// logProcessExit는 끝난 cmd의 PID와 종료 코드, 시작·종료 시각을 DEBUG 수준으로 남긴다. 시각은 Wait가 프로세스 handle을
// 닫기 전에 읽은 값이다.
// e2e/harness/dll_init_failure.go는 이 줄과 terminateProcess가 남기는 줄을 읽어, 0xc0000142로 끝난 hook 앞뒤에 같은 콘솔에서
// 보낸 CTRL+BREAK를 보고한다. 두 줄의 형식을 바꾸면 그 파일의 정규식도 함께 바꾼다.
func logProcessExit(cmd *exec.Cmd) {
	if cmd.ProcessState == nil {
		return
	}
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		return
	}
	logDebug(fmt.Sprintf("process %d exited with code %#x (started %s, exited %s)", cmd.Process.Pid, uint32(cmd.ProcessState.ExitCode()),
		debugTime(time.Unix(0, usage.CreationTime.Nanoseconds())), debugTime(time.Unix(0, usage.ExitTime.Nanoseconds()))))
}

func debugTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func logDebug(message string) {
	if helper.GlobalLogger != nil {
		helper.GlobalLogger.Debug(message)
	}
}

// processTree는 명령을 넣은 job과 명령 프로세스(프로세스 그룹의 리더)의 handle이다. os/exec는 cmd.Wait에서 cmd.Cancel이
// 돌아오기를 기다리기 전에 자기 handle을 닫으므로, 이 handle은 cmd.Wait가 돌아온 뒤에 닫는다.
type processTree struct {
	job, leader windows.Handle
}

func (tree processTree) close() {
	for _, handle := range []windows.Handle{tree.job, tree.leader} {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
	}
}

// startProcessTree는 cmd를 새 job object 안에서 시작해 cmd의 자손도 모두 그 job에 들어가게 하고, cmd.Cancel이
// job의 프로세스를 모두 멈추게 한다(stopTreeOnCancel). killOnClose이면 job handle을 닫을 때 남은 프로세스도
// 끝낸다. 아니면 명령이 취소되지 않고 끝난 뒤에 남긴 프로세스는 계속 실행된다.
func startProcessTree(cmd *exec.Cmd, killOnClose bool) (processTree, error) {
	markStarted := stopTreeOnCancel(cmd)
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processTree{}, fmt.Errorf("create job object: %w", err)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	if killOnClose {
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	} else {
		// 스스로 job을 벗어나려고 CREATE_BREAKAWAY_FROM_JOB로 자식을 만드는 프로그램이 실패하지 않게 한다.
		// 이렇게 job을 벗어난 프로세스는 취소된 명령의 job을 끝낼 때 함께 끝나지 않는다.
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	}
	if ok, _, err := procSetInformationJobObject.Call(uintptr(job), windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 {
		_ = windows.CloseHandle(job)
		return processTree{}, fmt.Errorf("configure job object: %w", err)
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// 일시 중단된 부모는 job에 속하기 전에는 자손을 만들 수 없다.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return processTree{}, err
	}
	leader, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, leader)
	}
	if err == nil {
		err = resumeProcess(cmd.Process.Pid)
	}
	if err != nil {
		// cmd.Wait가 cmd.Cancel의 결과를 기다리므로, 그 전에 cmd.Cancel이 job 없이 끝나게 한다.
		markStarted(processTree{leader: leader})
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		processTree{job: job, leader: leader}.close()
		return processTree{}, fmt.Errorf("start in job object: %w", err)
	}
	tree := processTree{job: job, leader: leader}
	markStarted(tree)
	return tree, nil
}

func releaseProcessTree(tree processTree) {
	tree.close()
}

// stopTreeOnCancel은 cmd.Cancel을 감싸, 취소된 명령이 시작 후 minRunBeforeBreak가 지난 뒤에 첫 CTRL+BREAK를
// 받고 job의 프로세스가 모두 끝날 때까지 cmd.Cancel이 돌아오지 않게 한다(stopJob). 프로세스를 job에 넣고
// 다시 실행한 직후, 또는 그러지 못했을 때 돌려받은 함수에 processTree를 넘겨야 한다. job에 넣지 못했으면 job을 0으로 두고,
// 리더의 handle도 열지 못했으면 leader도 0으로 둔다. 리더의 handle이 없으면 PID가 재사용됐을 수 있으므로 CTRL+BREAK를
// 보내지 않는다. 이때 명령은 startProcessTree가 강제로 끝낸다.
func stopTreeOnCancel(cmd *exec.Cmd) (markStarted func(tree processTree)) {
	send := cmd.Cancel
	if send == nil {
		return func(processTree) {}
	}
	started := make(chan struct{})
	var tree processTree
	var startedAt time.Time
	cmd.Cancel = func() error {
		<-started
		switch {
		case tree.leader == 0:
			return nil
		case tree.job == 0:
			return send()
		}
		time.Sleep(time.Until(startedAt.Add(minRunBeforeBreak)))
		return stopJob(tree.job, send)
	}
	return func(t processTree) {
		tree, startedAt = t, time.Now()
		close(started)
	}
}

// stopJob은 send로 CTRL+BREAK를 보내고 job의 프로세스가 모두 끝날 때까지 기다린 뒤, 첫 send의 오류를 돌려준다.
// 첫 신호는 첫 신호를 기다리는 동안 들어온 프로세스까지 job의 프로세스가 모두 minRunBeforeBreak만큼 실행된 뒤에 보내되,
// maxFirstBreakDelay보다 늦추지는 않는다(waitBeforeFirstBreak). CTRL+BREAK는 보낸 순간 프로세스 그룹에 있던 프로세스에만
// 가므로, cmd.exe가 명령 목록의 다음 프로그램을 시작하는 것처럼 첫 신호 뒤에 job에 들어온 프로세스가 있으면 다시 보낸다.
// 들어온 프로세스가 minRunBeforeBreak만큼 실행된 뒤에 보내고, 새 프로세스가 없으면 다시 보내지 않아 이미 신호를 받고 정상
// 종료하는 프로그램에는 두 번째 신호가 가지 않는다.
// 첫 신호 뒤 breakGrace가 지나도 남은 프로세스는 job과 함께 끝낸다. 신호를 받지 못한 프로세스와 PowerShell처럼
// 신호로 끝나지 않는 프로세스가 여기서 끝난다. Windows PowerShell은 명령을 실행하는 중에 CTRL+BREAK를 받으면 끝나지
// 않고 디버거로 들어가며, 표준 입력이 콘솔이면 거기서 입력을 기다린다.
func stopJob(job windows.Handle, send func() error) error {
	signaled, err := waitBeforeFirstBreak(job)
	if err != nil {
		return errors.Join(send(), err)
	}
	sendErr := send()
	deadline := time.Now().Add(breakGrace)
	joined, lastJoin := signaled, time.Time{}
	for {
		accounting, err := queryJob(job)
		if err != nil {
			return errors.Join(sendErr, err)
		}
		if accounting.activeProcesses == 0 {
			return sendErr
		}
		now := time.Now()
		if !now.Before(deadline) {
			return errors.Join(sendErr, terminateJob(job, now.Add(jobKillWait)))
		}
		if accounting.totalProcesses != joined {
			joined, lastJoin = accounting.totalProcesses, now
		}
		if joined != signaled && now.Sub(lastJoin) >= minRunBeforeBreak {
			signaled = joined
			_ = send()
		}
		time.Sleep(jobPollInterval)
	}
}

// waitBeforeFirstBreak은 job에서 가장 늦게 만들어진 프로세스가 minRunBeforeBreak만큼 실행될 때까지 기다리고, 기다림을
// 마칠 때까지 job에 들어온 프로세스 수를 돌려준다. 만들어진 지 minRunBeforeBreak가 지나지 않은 프로세스는 CTRL+BREAK를
// 받지 못하거나 0xC0000005로 끝나기도 한다. 기다리는 동안 들어온 프로세스가 있으면 그 프로세스를 기준으로 다시 기다리되,
// 처음부터 maxFirstBreakDelay가 지나면 더 기다리지 않는다.
func waitBeforeFirstBreak(job windows.Handle) (uint32, error) {
	limit := time.Now().Add(maxFirstBreakDelay)
	accounting, err := queryJob(job)
	for err == nil {
		joined := accounting.totalProcesses
		var lastCreated time.Time
		if lastCreated, err = lastProcessCreation(job); err != nil {
			break
		}
		wake := lastCreated.Add(minRunBeforeBreak)
		if wake.After(limit) {
			wake = limit
		}
		time.Sleep(time.Until(wake))
		if accounting, err = queryJob(job); err == nil && (accounting.totalProcesses == joined || !time.Now().Before(limit)) {
			return accounting.totalProcesses, nil
		}
	}
	return 0, err
}

// jobProcessIDList는 JOBOBJECT_BASIC_PROCESS_ID_LIST다. 목록 크기는 hook 하나가 동시에 실행하는 프로세스 수보다
// 넉넉하게 잡았다.
type jobProcessIDList struct {
	assigned, listed uint32
	ids              [256]uintptr
}

// job 정보를 조회하고 설정할 때는 windows.QueryInformationJobObject와 windows.SetInformationJobObject 대신 아래
// 함수를 쓴다. windows 패키지의 두 함수는 버퍼를 uintptr로 받으므로, 버퍼가 스택에 있으면 호출하는 사이에 스택이
// 옮겨져 커널이 옛 주소를 읽거나 쓸 수 있다. UTM의 Windows 11 ARM64 VM에서 스택에 둔 프로세스 목록을
// windows.QueryInformationJobObject로 받았을 때는 첫 신호가 늦춰지지 않았다. LazyProc.Call은 uintptr로 바꾼
// 포인터의 대상을 힙에 두게 한다.
var (
	kernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	procQueryInformationJobObject = kernel32.NewProc("QueryInformationJobObject")
	procSetInformationJobObject   = kernel32.NewProc("SetInformationJobObject")
)

// lastProcessCreation은 job에서 실행 중인 프로세스 가운데 가장 늦게 만들어진 프로세스의 생성 시각을 돌려준다.
// 생성 시각을 읽기 전에 끝난 프로세스는 건너뛴다. 프로세스가 목록 크기보다 많으면 목록에 담기지 않은 프로세스가
// 방금 만들어졌다고 보고 지금 시각을 돌려준다.
func lastProcessCreation(job windows.Handle) (time.Time, error) {
	var list jobProcessIDList
	if ok, _, err := procQueryInformationJobObject.Call(uintptr(job), windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), unsafe.Sizeof(list), 0); ok == 0 {
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			return time.Now(), nil
		}
		return time.Time{}, fmt.Errorf("list job processes: %w", err)
	}
	var last time.Time
	for _, pid := range list.ids[:list.listed] {
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			continue
		}
		var creation, exit, kernel, user windows.Filetime
		err = windows.GetProcessTimes(process, &creation, &exit, &kernel, &user)
		_ = windows.CloseHandle(process)
		if created := time.Unix(0, creation.Nanoseconds()); err == nil && created.After(last) {
			last = created
		}
	}
	return last, nil
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
	defer tree.close()
	return terminateJob(tree.job, deadline)
}

// terminateJob은 job의 프로세스를 모두 끝내고, deadline까지 모두 사라지기를 기다린다.
func terminateJob(job windows.Handle, deadline time.Time) error {
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return fmt.Errorf("terminate job object: %w", err)
	}
	for {
		accounting, err := queryJob(job)
		if err != nil {
			return err
		}
		if accounting.activeProcesses == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("wait for job object cleanup: %w", context.DeadlineExceeded)
		}
		time.Sleep(jobPollInterval)
	}
}

func queryJob(job windows.Handle) (jobAccounting, error) {
	var accounting jobAccounting
	if ok, _, err := procQueryInformationJobObject.Call(uintptr(job), windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), unsafe.Sizeof(accounting), 0); ok == 0 {
		return accounting, fmt.Errorf("query job object: %w", err)
	}
	return accounting, nil
}

func SetupForOs(cmd *exec.Cmd) {
	// 새로운 프로세스 그룹을 생성합니다. cmd 셸이 정한 명령줄(CmdLine)은 그대로 둡니다.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}
