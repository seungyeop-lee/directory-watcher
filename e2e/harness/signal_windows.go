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
	handle windows.Handle
	err    error
}

// processObserver belongs to one scenario. It keeps the handle opened at the
// first observation of each recorded PID until the scenario's cleanup, so a
// reused PID cannot be mistaken for, or terminated as, a recorded process.
type processObserver struct {
	observations map[int]processObservation
}

func newProcessObserver() *processObserver {
	return &processObserver{observations: map[int]processObservation{}}
}

func (o *processObserver) alive(pid int) bool {
	observation, known := o.observations[pid]
	if !known {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
		observation = processObservation{handle, err}
		o.observations[pid] = observation
	}
	if errors.Is(observation.err, windows.ERROR_INVALID_PARAMETER) {
		return false
	}
	if observation.err != nil {
		return true
	}
	state, err := windows.WaitForSingleObject(observation.handle, 0)
	return err != nil || state != windows.WAIT_OBJECT_0
}

func (o *processObserver) killGroup(pid int) {
	o.kill(pid)
}

func (o *processObserver) kill(pid int) {
	if !o.alive(pid) {
		return
	}
	observation := o.observations[pid]
	if observation.err == nil {
		var path [32768]uint16
		size := uint32(len(path))
		expected, expectedErr := os.Stat(hookrecBin)
		actualPath := windows.QueryFullProcessImageName(observation.handle, 0, &path[0], &size)
		actual, actualErr := os.Stat(windows.UTF16ToString(path[:size]))
		if actualPath == nil && expectedErr == nil && actualErr == nil && os.SameFile(expected, actual) {
			_ = windows.TerminateProcess(observation.handle, 1)
		}
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
	var created, exited, kernel, user windows.Filetime
	if observation.err == nil {
		if err := windows.GetProcessTimes(observation.handle, &created, &exited, &kernel, &user); err != nil {
			return fmt.Sprintf("pid=%d alive=%t query-error=%v", pid, alive, err)
		}
	}
	state, waitErr := windows.WaitForSingleObject(observation.handle, 0)
	return fmt.Sprintf("pid=%d alive=%t created=%d open-error=%v wait-state=%d wait-error=%v", pid, alive, created.Nanoseconds(), observation.err, state, waitErr)
}

func (o *processObserver) writeDiagnostics(file string) error {
	var diagnostics strings.Builder
	for _, record := range processRecords(file) {
		if record.State == "start" {
			fmt.Fprintf(&diagnostics, "run=%s depth=%d %s\n", record.Generation, record.Depth, o.describe(record.PID))
		}
	}
	return os.WriteFile(file+".processes", []byte(diagnostics.String()), 0644)
}
