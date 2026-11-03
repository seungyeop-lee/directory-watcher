//go:build windows

package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareConsole() error {
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	var pid uint32
	if count, _, _ := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&pid)), 1); count != 0 {
		return nil
	}
	if ok, _, err := kernel.NewProc("AllocConsole").Call(); ok == 0 {
		return fmt.Errorf("AllocConsole: %w", err)
	}
	return nil
}

func setupProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func gracefulStop(cmd *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}

func forceStop(cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "TASKKILL", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("TASKKILL: %w: %s", err, out)
	}
	return nil
}

type processObservation struct {
	handle   windows.Handle
	err      error
	observed time.Time
}

// processObserver belongs to one scenario. It keeps the handle opened at the
// first observation of each recorded PID until the scenario's cleanup, so a
// PID reused after that observation cannot be mistaken for, or terminated as,
// a recorded process.
type processObserver struct {
	observations map[int]processObservation
}

// errOtherProgram는 처음 관찰할 때 이미 hookrec이 아닌 프로그램이 기록된 PID를
// 쓰고 있었다는 뜻이다. 기록된 프로세스는 그 전에 끝났으므로 사라진 것으로 판정한다.
var errOtherProgram = errors.New("PID reused by another program")

func newProcessObserver() *processObserver {
	return &processObserver{observations: map[int]processObservation{}}
}

func (o *processObserver) alive(pid int) bool {
	observation, known := o.observations[pid]
	if !known {
		observation = observeProcess(pid)
		o.observations[pid] = observation
	}
	if errors.Is(observation.err, windows.ERROR_INVALID_PARAMETER) || errors.Is(observation.err, errOtherProgram) {
		return false
	}
	if observation.err != nil {
		return true
	}
	state, err := windows.WaitForSingleObject(observation.handle, 0)
	return err != nil || state != windows.WAIT_OBJECT_0
}

// observeProcess는 PID를 처음 관찰할 때 handle을 연다. 기록된 프로세스는 모두
// hookrec이므로, 이미지가 hookrec이 아니라고 확인된 handle은 닫고 errOtherProgram으로
// 남긴다. 이미지를 확인하지 못하면 handle을 그대로 두고 종료 대기 상태로 판정한다.
func observeProcess(pid int) processObservation {
	observed := time.Now()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return processObservation{err: err, observed: observed}
	}
	if image, hookrec, err := hookrecIdentity(handle); err == nil && !hookrec {
		_ = windows.CloseHandle(handle)
		return processObservation{err: fmt.Errorf("%w: %s", errOtherProgram, image), observed: observed}
	}
	return processObservation{handle: handle, observed: observed}
}

// hookrecIdentity는 handle이 가리키는 프로세스의 이미지 경로와, 그 이미지가 이번
// 실행에서 빌드한 hookrec인지를 돌려준다. 두 파일 중 하나라도 확인하지 못하면 err를 돌려준다.
func hookrecIdentity(handle windows.Handle) (image string, hookrec bool, err error) {
	if image, err = imagePath(handle); err != nil {
		return "", false, err
	}
	expected, err := os.Stat(hookrecBin)
	if err != nil {
		return image, false, err
	}
	actual, err := os.Stat(image)
	if err != nil {
		return image, false, err
	}
	return image, os.SameFile(expected, actual), nil
}

func imagePath(handle windows.Handle) (string, error) {
	var path [32768]uint16
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(path[:size]), nil
}

func (o *processObserver) killGroup(pid int) {
	o.kill(pid)
}

func (o *processObserver) kill(pid int) {
	if !o.alive(pid) {
		return
	}
	observation := o.observations[pid]
	if observation.err != nil {
		return
	}
	if _, hookrec, err := hookrecIdentity(observation.handle); err == nil && hookrec {
		_ = windows.TerminateProcess(observation.handle, 1)
	}
}

func (o *processObserver) close() {
	for pid, observation := range o.observations {
		if observation.err == nil {
			_ = windows.CloseHandle(observation.handle)
		}
		delete(o.observations, pid)
	}
}

func (o *processObserver) describe(pid int) string {
	alive := o.alive(pid)
	observation := o.observations[pid]
	created, held := "-", "-"
	if observation.err == nil {
		var creation, exited, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(observation.handle, &creation, &exited, &kernel, &user); err != nil {
			return fmt.Sprintf("pid=%d alive=%t query-error=%v", pid, alive, err)
		}
		created = time.Unix(0, creation.Nanoseconds()).UTC().Format(time.RFC3339Nano)
		held = imageName(observation.handle)
	}
	state, waitErr := windows.WaitForSingleObject(observation.handle, 0)
	current := "-"
	if handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)); err != nil {
		current = "open-error:" + err.Error()
	} else {
		current = imageName(handle)
		_ = windows.CloseHandle(handle)
	}
	return fmt.Sprintf("pid=%d alive=%t observed=%s created=%s open-error=%v wait-state=%d wait-error=%v held-image=%s current-image=%s", pid, alive, observation.observed.UTC().Format(time.RFC3339Nano), created, observation.err, state, waitErr, held, current)
}

func imageName(handle windows.Handle) string {
	image, err := imagePath(handle)
	if err != nil {
		return "query-error:" + err.Error()
	}
	return image
}

func (o *processObserver) writeDiagnostics(file string) error {
	var diagnostics strings.Builder
	for _, record := range processRecords(file) {
		if record.State == "start" {
			fmt.Fprintf(&diagnostics, "run=%s depth=%d %s\n", record.Generation, record.Depth, o.describe(record.PID))
		}
	}
	for _, hook := range delayedHooks(ReadLinesLenient(file)) {
		fmt.Fprintf(&diagnostics, "delayed=%s %s now=%s\n", hook.line, o.describe(hook.pid), time.Now().UTC().Format(time.RFC3339Nano))
	}
	return os.WriteFile(file+".processes", []byte(diagnostics.String()), 0644)
}
