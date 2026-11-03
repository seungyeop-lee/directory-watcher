//go:build darwin

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Time limits for single utmctl requests. An unanswered request otherwise
// waits for the Apple Event timeout of 2 minutes: in the 2026-11-03
// experiment (macOS 27, UTM 5.0.8) start ended with OSStatus error -1712
// after 2 minutes without starting the VM, and UTM #8006
// (https://github.com/utmapp/UTM/issues/8006) reports the same stall. The
// cause is not known.
const (
	requestLimit = 15 * time.Second
	startLimit   = 30 * time.Second
	appLimit     = 30 * time.Second
	pullLimit    = 2 * time.Minute
)

// startedLimit is how long a VM may take to report started after start. It
// took 2 to 3 seconds in the experiment.
var startedLimit = time.Minute

// utm controls UTM through the utmctl inside UTM.app. Calling it through the
// Homebrew symlink /opt/homebrew/bin/utmctl left start unanswered in the
// 2026-11-03 experiment, and irgo-windows-vm PR #13
// (https://github.com/joeblew999/irgo-windows-vm/pull/13) measured the same;
// neither found the cause. main rejects a symlinked path for this reason.
type utm struct {
	ctl, osascript string
	log            io.Writer
}

// app returns UTM.app, which contains Contents/MacOS/utmctl.
func (u utm) app() string { return filepath.Dir(filepath.Dir(filepath.Dir(u.ctl))) }

// call runs utmctl under limit and judges success by its output. utmctl
// exited with 0 in every failure seen on 2026-11-03: start that left the VM
// stopped, file pull and push of a missing path that printed an error, and
// exec that does not wait for the guest command. Each such failure printed a
// message to stderr, so any stderr output is an error except the OSStatus
// error -10004 lines. Those came only from the --hide events, which every
// command with --hide printed twice, including a start that succeeded; see
// hideMainWindow.
func (u utm) call(limit time.Duration, stdin io.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, u.ctl, args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	request := "utmctl " + strings.Join(args, " ")
	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s did not answer within %s", request, limit)
	}
	var problems []string
	if err != nil {
		problems = append(problems, err.Error())
	}
	if problem := stderrProblem(stderr.String()); problem != "" {
		problems = append(problems, problem)
	}
	if len(problems) > 0 {
		return stdout.Bytes(), fmt.Errorf("%s: %s", request, strings.Join(problems, "; "))
	}
	return stdout.Bytes(), nil
}

func stderrProblem(stderr string) string {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "-10004") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, " / ")
}

type vmEntry struct{ uuid, status, name string }

func (u utm) list() ([]vmEntry, error) {
	out, err := u.call(requestLimit, nil, "list")
	if err != nil {
		return nil, err
	}
	return parseList(out), nil
}

// parseList reads the table printed by utmctl list: a header line, then one
// line per VM with its UUID, status, and name, which may contain spaces.
func parseList(out []byte) []vmEntry {
	var entries []vmEntry
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] == "UUID" {
			continue
		}
		entries = append(entries, vmEntry{uuid: fields[0], status: fields[1], name: strings.Join(fields[2:], " ")})
	}
	return entries
}

// findVM matches the identifier the way utmctl does: a UUID or the complete
// name.
func findVM(entries []vmEntry, id string) (vmEntry, bool) {
	for _, entry := range entries {
		if strings.EqualFold(entry.uuid, id) || entry.name == id {
			return entry, true
		}
	}
	return vmEntry{}, false
}

// runningVMs counts VMs that are not stopped. Paused and transitional VMs
// count too, because quitting or restarting UTM would affect them.
func runningVMs(entries []vmEntry) int {
	count := 0
	for _, entry := range entries {
		if entry.status != "stopped" {
			count++
		}
	}
	return count
}

func (u utm) status(vm string) (string, error) {
	out, err := u.call(requestLimit, nil, "status", vm)
	return strings.TrimSpace(string(out)), err
}

// start requests a stopped VM to start and waits until utmctl status reports
// started, because start exits with 0 even when the VM stays stopped.
func (u utm) start(vm string) error {
	if _, err := u.call(startLimit, nil, "start", "--hide", vm); err != nil {
		fmt.Fprintln(u.log, "start request:", err)
	}
	if err := u.await(vm, "started", startedLimit); err != nil {
		return err
	}
	u.hideMainWindow()
	return nil
}

// hideMainWindow does through AppleScript what utmctl --hide does: it turns
// off auto terminate and closes the window named "UTM"
// (https://github.com/utmapp/UTM/blob/main/utmctl/UTMCtl.swift). On
// 2026-11-03 both events failed from utmctl, which printed OSStatus error
// -10004 once for each, and the main window stayed on screen; the same
// events sent by osascript closed it. Why utmctl's events fail is not known.
// Neither closes a VM's display window; only a VM without display devices
// runs without one (see up). Auto terminate stays off so that UTM does not
// quit by itself when its last window closes.
func (u utm) hideMainWindow() {
	ctx, cancel := context.WithTimeout(context.Background(), appLimit)
	defer cancel()
	script := `tell application "UTM"
	set auto terminate to false
	close (every window whose name is "UTM") saving no
end tell`
	if out, err := exec.CommandContext(ctx, u.osascript, "-e", script).CombinedOutput(); err != nil {
		fmt.Fprintf(u.log, "hide the main UTM window: %v %s\n", err, out)
	}
}

// displays returns the number of display devices in the VM's configuration,
// read through UTM's AppleScript dictionary because utmctl does not show
// it. The one-line form length of (get displays of (configuration of ...))
// failed with error -1700 on 2026-11-03; the steps through variables
// worked. The tell launches UTM when it is not running, so call this only
// after open.
func (u utm) displays(vm string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), appLimit)
	defer cancel()
	script := `on run argv
	set vmID to item 1 of argv
	tell application "UTM"
		set cfg to configuration of virtual machine id vmID
		set ds to displays of cfg
		return length of ds
	end tell
end run`
	out, err := exec.CommandContext(ctx, u.osascript, "-e", script, vm).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("read the displays of VM %s: %v %s", vm, err, out)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("read the displays of VM %s: unexpected answer %q", vm, out)
	}
	return n, nil
}

// await polls utmctl status until it reports want.
func (u utm) await(vm, want string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		status, err := u.status(vm)
		if err == nil && status == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("VM is not %s after %s (status %q, %v)", want, limit, status, err)
		}
		time.Sleep(time.Second)
	}
}

// awaitGuestAgent waits until the Guest Agent reports an IPv4 address. A
// started VM does not mean the guest has booted: in the experiment the agent
// answered 7 to 10 seconds after started.
func (u utm) awaitGuestAgent(vm string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		err := u.guestAgentReady(vm)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Guest Agent did not answer within %s: %v", limit, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func (u utm) guestAgentReady(vm string) error {
	out, err := u.call(requestLimit, nil, "ip-address", vm)
	if err != nil {
		return err
	}
	for _, line := range strings.Fields(string(out)) {
		if ip := net.ParseIP(line); ip != nil && ip.To4() != nil {
			return nil
		}
	}
	return fmt.Errorf("no IPv4 address in %q", out)
}

// utmRunning reports whether the UTM app runs. It is checked before any
// utmctl request where UTM must not be launched, because a utmctl request
// launches UTM when it is not running.
var utmRunning = func() bool { return exec.Command("pgrep", "-x", "UTM").Run() == nil }

// open launches UTM in the background with open -g instead of letting utmctl
// or AppleScript launch it. UTM #8006 reports that UTM 5.0.8 on macOS 27
// crashes and start stalls when utmctl launches UTM, and irgo-windows-vm PR
// #13 measured that a request within 0.3 seconds of launch leaves start
// unanswered. Waiting 2 seconds before the first request made start work in
// the 2026-11-03 experiment; the cause is not known.
//
// UTM shows its main window within half a second of launch even with -g,
// which only keeps it behind the frontmost app, so the window is closed
// right away rather than after the wait; sending that event early did not
// leave start unanswered in 8 launches on 2026-11-03. open -j would launch
// UTM hidden, but start then left the VM stopped in one of two tries that
// day.
func (u utm) open() error {
	ctx, cancel := context.WithTimeout(context.Background(), appLimit)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "open", "-g", "-a", u.app()).CombinedOutput(); err != nil {
		return fmt.Errorf("open %s: %v %s", u.app(), err, out)
	}
	if err := awaitApp(true); err != nil {
		return err
	}
	u.hideMainWindow()
	fmt.Fprintln(u.log, "opened UTM; waiting 2s before the first request")
	time.Sleep(2 * time.Second)
	return nil
}

// quit asks UTM to quit normally and waits for it to exit. UTM is never
// killed; callers quit it only when no VM is running.
func (u utm) quit() error {
	ctx, cancel := context.WithTimeout(context.Background(), appLimit)
	defer cancel()
	if out, err := exec.CommandContext(ctx, u.osascript, "-e", `quit app "UTM"`).CombinedOutput(); err != nil {
		return fmt.Errorf("quit UTM: %v %s", err, out)
	}
	if err := awaitApp(false); err != nil {
		return err
	}
	fmt.Fprintln(u.log, "quit UTM")
	return nil
}

func awaitApp(running bool) error {
	deadline := time.Now().Add(appLimit)
	for utmRunning() != running {
		if time.Now().After(deadline) {
			return fmt.Errorf("UTM running=%v not reached within %s", running, appLimit)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}
