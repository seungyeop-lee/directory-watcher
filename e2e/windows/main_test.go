//go:build darwin

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpRefusesAVMWithADisplayBeforeStartingIt(t *testing.T) {
	defer func(running func() bool) { utmRunning = running }(utmRunning)
	utmRunning = func() bool { return true }
	started := filepath.Join(t.TempDir(), "started")
	for name, status := range map[string]string{"stopped": "stopped", "started": "started"} {
		t.Run(name, func(t *testing.T) {
			u := utm{
				ctl: fakeCommand(t, "utmctl", `case "$1" in
list) printf 'UUID Status Name\nC2E2EC92-7712-460C-BB1A-617717849200 `+status+` Windows\n' ;;
start) touch '`+started+`' ;;
esac`),
				osascript: fakeCommand(t, "osascript", "echo 1"),
				log:       io.Discard,
			}
			r := runner{cfg: config{vm: "Windows"}, utm: u, log: io.Discard}
			state, err := r.up()
			if err == nil || !strings.Contains(err.Error(), "e2e/docs/windows-utm.md") {
				t.Fatalf("up error = %v; want a refusal that points to the preparation guide", err)
			}
			if _, statErr := os.Stat(started); statErr == nil || state.startedVM {
				t.Fatal("up requested start for a VM with a display")
			}
		})
	}
}

func TestTeardownQuitsOnlyTheUTMThatUpOpened(t *testing.T) {
	defer func(running func() bool) { utmRunning = running }(utmRunning)
	utmRunning = func() bool { return false }
	const stopped = "C2E2EC92-7712-460C-BB1A-617717849200 stopped Windows\n"
	const otherStarted = stopped + "0B6D7A43-36A3-4A4B-9E0B-6A7D2F7B1C11 started Other\n"
	for name, tc := range map[string]struct {
		state    upState
		list     string
		wantQuit bool
	}{
		"nothing changed":                  {upState{}, stopped, false},
		"opened UTM":                       {upState{openedUTM: true}, stopped, true},
		"opened UTM while another VM runs": {upState{openedUTM: true}, otherStarted, false},
		"started the VM only":              {upState{startedVM: true}, stopped, false},
		"opened UTM and started the VM":    {upState{openedUTM: true, startedVM: true}, stopped, true},
	} {
		t.Run(name, func(t *testing.T) {
			quit := filepath.Join(t.TempDir(), "quit")
			u := utm{
				ctl:       fakeCommand(t, "utmctl", `[ "$1" = list ] && printf 'UUID Status Name\n`+tc.list+`'`),
				osascript: fakeCommand(t, "osascript", "touch '"+quit+"'"),
				log:       io.Discard,
			}
			r := runner{cfg: config{vm: "Windows"}, utm: u, log: io.Discard}
			if err := r.teardown(tc.state); err != nil {
				t.Fatalf("teardown = %v", err)
			}
			if _, err := os.Stat(quit); (err == nil) != tc.wantQuit {
				t.Fatalf("UTM quit = %v; want %v", err == nil, tc.wantQuit)
			}
		})
	}
}
