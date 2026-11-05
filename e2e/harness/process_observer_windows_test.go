package harness

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// 끝난 hook의 PID를 다른 프로그램이 이어받은 상황은, 살아 있는 다른 프로그램의 PID를 BEGIN 기록에 적어 재현한다.
func TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram(t *testing.T) {
	other := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Process.Kill(); _ = other.Wait() })
	s := NewScenario(t)
	s.Write(s.Records, []byte(fmt.Sprintf("BEGIN|%d|REUSED\nREUSED\n", other.Process.Pid)))
	s.AwaitDelayedHooksGone()
}

func TestDelayedHooksGoneFailsWhileHookRuns(t *testing.T) {
	skipShort(t)
	if os.Getenv("DW_E2E_LEFTOVER_HOOK") == "1" {
		s := NewScenario(t)
		hook := exec.Command(hookrecBin, "--file="+s.Records, "--delay=1m", "--line=LEFTOVER")
		if err := hook.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = hook.Process.Kill(); _ = hook.Wait() })
		s.Await("leftover hook begins", 5*time.Second, func() bool { return DelayedHookBegun(s.Lines(), "LEFTOVER") })
		s.AwaitDelayedHooksGone()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDelayedHooksGoneFailsWhileHookRuns$", "-test.v")
	cmd.Env = append(os.Environ(), "DW_E2E_KEEP=0", "DW_E2E_LEFTOVER_HOOK=1")
	out, err := cmd.CombinedOutput()
	output := string(out)
	if err == nil || !strings.Contains(output, "timeout: all slow hook processes disappear") {
		t.Fatalf("wait did not fail while the hook ran: err=%v\n%s", err, output)
	}
}
