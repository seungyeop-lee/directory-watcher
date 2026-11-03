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
