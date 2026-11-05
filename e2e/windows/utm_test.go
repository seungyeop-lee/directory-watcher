//go:build darwin

package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

const listOutput = `UUID                                 Status   Name
C2E2EC92-7712-460C-BB1A-617717849200 stopped  Windows
0B6D7A43-36A3-4A4B-9E0B-6A7D2F7B1C11 started  Windows Server 2025
`

func TestListFindsVMsByUUIDOrCompleteName(t *testing.T) {
	entries := parseList([]byte(listOutput))
	byName, ok := findVM(entries, "Windows Server 2025")
	if !ok || byName.uuid != "0B6D7A43-36A3-4A4B-9E0B-6A7D2F7B1C11" || byName.status != "started" {
		t.Fatalf("find by complete name = %+v, %v", byName, ok)
	}
	byUUID, ok := findVM(entries, "c2e2ec92-7712-460c-bb1a-617717849200")
	if !ok || byUUID.name != "Windows" || byUUID.status != "stopped" {
		t.Fatalf("find by UUID = %+v, %v", byUUID, ok)
	}
	if _, ok := findVM(entries, "Win"); ok {
		t.Fatal("a partial name matched")
	}
}

func TestRunningVMsCountsEveryVMThatIsNotStopped(t *testing.T) {
	for name, tc := range map[string]struct {
		list string
		want int
	}{
		"none registered":         {"UUID Status Name\n", 0},
		"all stopped":             {"UUID Status Name\nA stopped One\nB stopped Two\n", 0},
		"another started":         {listOutput, 1},
		"paused and transitional": {"UUID Status Name\nA paused One\nB starting Two\nC stopping Three\nD stopped Four\n", 3},
	} {
		t.Run(name, func(t *testing.T) {
			if got := runningVMs(parseList([]byte(tc.list))); got != tc.want {
				t.Fatalf("runningVMs = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCallJudgesByOutputNotExitCode(t *testing.T) {
	noise := `echo 'Error from event: The operation couldn’t be completed. (OSStatus error -10004.)' >&2`
	u := fakeUTMCtl(t, `case "$1" in
status) `+noise+`; `+noise+`; echo started ;;
pull) echo 'Error from event: (OSStatus error -2700.)' >&2; echo "failed to open file 'C:\\missing'" >&2 ;;
hang) exec sleep 5 ;;
esac
exit 0`)
	if out, err := u.call(time.Second, nil, "status", "vm"); err != nil || strings.TrimSpace(string(out)) != "started" {
		t.Fatalf("status with -10004 noise = %q, %v; want started without error", out, err)
	}
	if _, err := u.call(time.Second, nil, "pull", "vm"); err == nil || !strings.Contains(err.Error(), "failed to open file") {
		t.Fatalf("pull error = %v; want the stderr message although the exit code is 0", err)
	}
	var log strings.Builder
	u.log = &log
	began := time.Now()
	if _, err := u.call(200*time.Millisecond, nil, "hang"); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("hang error = %v", err)
	}
	if !strings.Contains(log.String(), "utmctl hang did not answer within 200ms; killed utmctl") {
		t.Fatalf("log = %q; want the killed request in the runner log", log.String())
	}
	if elapsed := time.Since(began); elapsed > 3*time.Second {
		t.Fatalf("hang returned after %s", elapsed)
	}
}

func TestStartFailsWhenTheVMStaysStoppedAfterAnExitCodeOfZero(t *testing.T) {
	shortWaits(t)
	u := fakeUTMCtl(t, `case "$1" in
status) echo stopped ;;
esac
exit 0`)
	err := u.start("Windows")
	if err == nil || !strings.Contains(err.Error(), `status "stopped"`) {
		t.Fatalf("start error = %v; want the VM reported as still stopped", err)
	}
}

func TestGuestAgentReadyNeedsAnIPv4Address(t *testing.T) {
	ready := fakeUTMCtl(t, "printf '192.168.64.2\\nfd19::1\\n'")
	if err := ready.guestAgentReady("vm"); err != nil {
		t.Fatalf("ready agent rejected: %v", err)
	}
	notRunning := fakeUTMCtl(t, `echo 'Error from event: (OSStatus error -2700.)' >&2; echo 'QEMU guest agent is not running or not installed on the guest.' >&2`)
	if err := notRunning.guestAgentReady("vm"); err == nil {
		t.Fatal("an agent that is not running was reported ready")
	}
	ipv6Only := fakeUTMCtl(t, "echo fe80::1%3")
	if err := ipv6Only.guestAgentReady("vm"); err == nil {
		t.Fatal("an IPv6-only answer was reported ready")
	}
}

func TestDisplaysReadsTheCountAndRejectsOtherAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		script  string
		want    int
		wantErr bool
	}{
		"headless":     {"echo 0", 0, false},
		"one display":  {"echo 1", 1, false},
		"unknown VM":   {`echo 'execution error: (-1728)' >&2; exit 1`, 0, true},
		"other answer": {"echo missing value", 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			// AppleScript로 보낸 요청은 utmctl이 실행 중이면 -600으로 실패한다(utm.runJXA 참고).
			appleScriptFails := `[ "$1 $2" = "-l JavaScript" ] || { echo "execution error: UTM got an error: Application isn't running. (-600)" >&2; exit 1; }` + "\n"
			u := utm{osascript: fakeCommand(t, "osascript", appleScriptFails+tc.script), log: io.Discard}
			got, err := u.displays("vm")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("displays = %d, %v; want %d, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
