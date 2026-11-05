//go:build !windows

package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCanceledRunEndsTermIgnoringDescendants(t *testing.T) {
	for _, ignoreParent := range []bool{false, true} {
		t.Run(fmt.Sprintf("ignoreParent=%t", ignoreParent), func(t *testing.T) {
			t.Parallel()
			run := NewWatcherContext("", CommandMode{}).NewRun()
			defer run.Cancel()
			script := `sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 120' sh "$1" & wait`
			if ignoreParent {
				script = `trap '' TERM; ` + script
			}
			cmd, childPID, done := runUnixDescendant(t, run, script)
			started := time.Now()
			run.Cancel()
			select {
			case <-done:
			case <-time.After(shutdownGrace + 5*time.Second):
				t.Fatal("canceled command exceeded the SIGTERM cleanup budget")
			}
			elapsed := time.Since(started)
			if elapsed < shutdownGrace-time.Second || elapsed > shutdownGrace+5*time.Second {
				t.Errorf("cancellation took %s; want about %s from SIGTERM", elapsed, shutdownGrace)
			}
			assertUnixProcessStopped(t, childPID)
			if cmd.ProcessState == nil {
				t.Fatal("command was not waited for")
			}
		})
	}
}

func TestCompletedRunKeepsBackgroundDescendant(t *testing.T) {
	run := NewWatcherContext("", CommandMode{}).NewRun()
	defer run.Cancel()
	_, childPID, done := runUnixDescendant(t, run, `sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 120' sh "$1" &`)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("normally completed command waited for its background descendant")
	}
	if err := syscall.Kill(childPID, 0); err != nil {
		t.Fatalf("normally completed command lost background descendant: %v", err)
	}
}

func TestTimeoutRunCleansBackgroundDescendant(t *testing.T) {
	t.Parallel()
	run := NewWatcherContext("", CommandMode{}).NewTimeoutRun(5*time.Second, nil)
	defer run.Cancel()
	started := time.Now()
	_, childPID, done := runUnixDescendant(t, run, `sh -c 'trap "" TERM; echo $$ > "$1"; exec sleep 120' sh "$1" & while [ ! -s "$1" ]; do sleep 0.01; done`)
	select {
	case <-done:
	case <-time.After(shutdownGrace + 5*time.Second):
		t.Fatal("timeout command exceeded its cleanup budget")
	}
	if elapsed := time.Since(started); elapsed > shutdownGrace+5*time.Second {
		t.Fatalf("timeout cleanup took %s", elapsed)
	}
	assertUnixProcessStopped(t, childPID)
}

func runUnixDescendant(t *testing.T, run *WatcherContext, script string) (*exec.Cmd, int, <-chan error) {
	t.Helper()
	record := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.CommandContext(run.Context(), "sh", "-c", script, "sh", record)
	SetupForOs(cmd)
	run.ApplyGracefulShutdown(cmd)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		done <- run.RunCommand(cmd)
		close(finished)
	}()
	var childPID int
	t.Cleanup(func() {
		if childPID != 0 {
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		}
		run.Cancel()
		<-finished
		if cmd.Process != nil {
			_ = signalGroup(cmd.Process.Pid, syscall.SIGKILL)
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(record)
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil {
				return cmd, childPID, done
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background descendant did not become ready")
	return nil, 0, nil
}

func assertUnixProcessStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		state := strings.TrimSpace(string(out))
		if (err != nil && state == "") || strings.HasPrefix(state, "Z") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("descendant %d still runs after command cleanup", pid)
}
