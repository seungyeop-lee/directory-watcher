//go:build darwin

// Command windows runs the Go E2E scenarios in a Windows VM of UTM on a Mac.
// Booting and stopping the VM take long, so up, test, and down are separate
// commands; all runs them in order and stops only what it started.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = `usage: go run ./e2e/windows <up|test|down|all> [flags] [-- go test flags]

  up    open UTM if needed, start the VM if it is stopped, wait for the Guest Agent
  test  run the E2E scenarios in the started VM; never starts or stops UTM or the VM
  down  request the VM to shut down, then quit UTM if no VM is running
  all   up, test, and down; stops the VM and quits UTM only if this run started them
`

type config struct {
	vm, utmctl                          string
	guestDir, guestGo                   string
	deadline, readyTimeout, stopTimeout time.Duration
	goTestArgs                          []string
}

type runner struct {
	cfg config
	utm utm
	log io.Writer
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	command := args[0]
	var cfg config
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage); flags.PrintDefaults() }
	flags.StringVar(&cfg.vm, "vm", os.Getenv("DW_E2E_UTM_VM"), "UTM VM name or UUID (default $DW_E2E_UTM_VM)")
	flags.StringVar(&cfg.utmctl, "utmctl", "/Applications/UTM.app/Contents/MacOS/utmctl", "utmctl inside UTM.app, not a symlink to it")
	flags.StringVar(&cfg.guestDir, "guest-dir", `C:\dw-e2e`, "guest directory on its internal disk that holds the run directories")
	flags.StringVar(&cfg.guestGo, "guest-go", `C:\Program Files\Go\bin\go.exe`, "go.exe in the guest")
	flags.DurationVar(&cfg.deadline, "deadline", 20*time.Minute, "time limit of go test in the guest")
	flags.DurationVar(&cfg.readyTimeout, "ready-timeout", 5*time.Minute, "time for the started VM's Guest Agent to answer")
	flags.DurationVar(&cfg.stopTimeout, "stop-timeout", 3*time.Minute, "time for the VM to stop after the shutdown request")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	cfg.goTestArgs = flags.Args()
	if err := validate(command, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	r := runner{cfg: cfg, utm: utm{ctl: cfg.utmctl, osascript: "osascript", log: os.Stdout}, log: os.Stdout}
	var err error
	switch command {
	case "up":
		var state upState
		state, err = r.up()
		if err != nil && !state.startedVM {
			// A refused or failed up that started nothing does not leave
			// behind the UTM it opened. A started VM stays for down.
			err = errors.Join(err, r.teardown(state))
		}
	case "test":
		err = r.test()
	case "down":
		err = r.down()
	case "all":
		err = r.all()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Windows E2E:", err)
		return 1
	}
	return 0
}

func validate(command string, cfg config) error {
	switch command {
	case "up", "down":
		if len(cfg.goTestArgs) > 0 {
			return fmt.Errorf("%s takes no go test flags: %q", command, cfg.goTestArgs)
		}
	case "test", "all":
	default:
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
	if cfg.vm == "" {
		return errors.New("no VM selected: set DW_E2E_UTM_VM or -vm to a name or UUID that utmctl list shows")
	}
	info, err := os.Lstat(cfg.utmctl)
	if err != nil {
		return fmt.Errorf("utmctl not found: %v; install UTM or set -utmctl to UTM.app/Contents/MacOS/utmctl", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("-utmctl %s is a symlink; use the utmctl inside UTM.app, because calls through the Homebrew symlink left start unanswered", cfg.utmctl)
	}
	return nil
}

// upState records what up changed, so that all stops only those.
type upState struct {
	openedUTM, startedVM bool
	uuid                 string
}

func (r runner) up() (upState, error) {
	var state upState
	if !utmRunning() {
		if err := r.utm.open(); err != nil {
			return state, err
		}
		state.openedUTM = true
	}
	vm, err := r.findVM()
	if err != nil {
		return state, err
	}
	state.uuid = vm.uuid
	if err := r.requireHeadless(vm.uuid); err != nil {
		return state, err
	}
	switch vm.status {
	case "started":
		fmt.Fprintln(r.log, "VM is already started; checking the Guest Agent")
	case "stopped":
		lock, err := lockVM(vm.uuid, true)
		if err != nil {
			return state, err
		}
		defer lock.Close()
		state.startedVM = true
		if err := r.startVM(vm.uuid); err != nil {
			return state, err
		}
	default:
		return state, fmt.Errorf("VM %s is %s; wait until it is started or stopped", r.cfg.vm, vm.status)
	}
	if err := r.utm.awaitGuestAgent(vm.uuid, r.cfg.readyTimeout); err != nil {
		return state, err
	}
	fmt.Fprintln(r.log, "VM and Guest Agent are ready")
	return state, nil
}

// requireHeadless refuses a VM that has a display device, because UTM opens
// a window for each display whether start has --hide or not: on 2026-11-03
// the VM window came up over the user's work at every start of the original
// VM, while --hide closed at most UTM's main window. UTM's QEMU backend runs
// a VM without display devices with no window (UTM #6188,
// https://github.com/utmapp/UTM/issues/6188). A clone with its displays
// removed booted with no UTM window, no QEMURenderServer, and -vga none in
// its QEMU arguments. The runner does not change the configuration; the clone
// is prepared once by hand. Its sound device must be removed too, so that
// boot makes no sound, but UTM's AppleScript dictionary (UTM.sdef) has no
// sound setting to check, so only the documentation covers it.
func (r runner) requireHeadless(uuid string) error {
	n, err := r.utm.displays(uuid)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("VM %s has %d display device(s), so starting it would open a VM window on the screen; "+
			"run the E2E in a clone without display and sound devices, prepared as in e2e/docs/windows-utm.md", r.cfg.vm, n)
	}
	return nil
}

// startVM restarts UTM once when start does not take effect, but only when
// every registered VM is stopped. Restarting UTM cleared the unanswered start
// in the 2026-11-03 experiment and in irgo-windows-vm PR #9
// (https://github.com/joeblew999/irgo-windows-vm/pull/9).
func (r runner) startVM(uuid string) error {
	err := r.utm.start(uuid)
	if err == nil {
		return nil
	}
	entries, listErr := r.utm.list()
	if listErr != nil {
		return fmt.Errorf("%v; listing VMs before restarting UTM failed: %v", err, listErr)
	}
	if n := runningVMs(entries); n > 0 {
		return fmt.Errorf("%v; UTM was not restarted because %d VM(s) are not stopped: stop them, quit UTM, and run up again", err, n)
	}
	fmt.Fprintln(r.log, "start did not take effect; restarting UTM once:", err)
	if err := r.utm.quit(); err != nil {
		return err
	}
	if err := r.utm.open(); err != nil {
		return err
	}
	return r.utm.start(uuid)
}

func (r runner) down() error {
	if !utmRunning() {
		fmt.Fprintln(r.log, "UTM is not running, so the VM is stopped")
		return nil
	}
	vm, err := r.findVM()
	if err != nil {
		return err
	}
	err = r.stopVM(vm)
	if err != nil && !errors.Is(err, errQEMUAborted) {
		return err
	}
	return errors.Join(err, r.quitUTMIfIdle())
}

// stopVM requests the guest OS to shut down and waits for stopped. utmctl
// stop without --request powers the VM off, so it is never used.
//
// The request is repeated every shutdownRetry until the VM stops. On
// 2026-11-03 the original VM ignored a request twice and logged no shutdown
// event for it: once while Windows Update was installing updates, and once
// 20 seconds after a restart that Windows Update had started. A second
// request stopped it in 60 and 7 seconds. The headless clone ignored the
// first request once in 18 shutdowns, about a minute after test runs, and
// the second request stopped it in 8 seconds; its other shutdowns took 4 to
// 24 seconds. Why requests are ignored is not known. Each completed request
// was a full shutdown in both VMs: the guest logged a power-off by
// winlogon.exe (event 1074, reason 0x500ff) and a clean event log stop
// (6006), the next boot was a cold boot (Kernel-Boot event 27, boot type
// 0x0), and fast startup was unavailable because hibernation is disabled in
// that guest.
func (r runner) stopVM(vm vmEntry) error {
	if vm.status == "stopped" {
		fmt.Fprintln(r.log, "VM is already stopped")
		return nil
	}
	lock, err := lockVM(vm.uuid, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	requested := time.Now()
	deadline := requested.Add(r.cfg.stopTimeout)
	for {
		if _, err := r.utm.call(requestLimit, nil, "stop", "--request", vm.uuid); err != nil {
			fmt.Fprintln(r.log, "stop request:", err)
		}
		err := r.utm.await(vm.uuid, "stopped", min(shutdownRetry, time.Until(deadline)))
		if err == nil {
			if err := qemuCrash(requested); err != nil {
				return err
			}
			fmt.Fprintln(r.log, "VM stopped")
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%v; the VM was not forced off: run down again, or stop it in the UTM app", err)
		}
		fmt.Fprintln(r.log, "repeating the shutdown request:", err)
	}
}

const shutdownRetry = 30 * time.Second

var errQEMUAborted = errors.New("QEMU aborted while the VM was stopping")

// qemuCrash reports QEMU crash reports written since the shutdown request,
// because a VM whose QEMU process aborted is stopped without a confirmed
// graceful shutdown. On 2026-11-03 QEMULauncher of the original VM, which
// had a display, aborted (SIGABRT from an assertion in hvf_vcpu_exec) in 7
// of 11 shutdowns, and macOS showed a dialog saying that it quit
// unexpectedly. Every abort came at the end of a shutdown, none during
// start, file transfer, or tests. In the three aborts checked in the guest,
// it had logged its shutdown (events 1074 and 6006) 2 to 6 seconds before,
// and the next boot logged no unexpected shutdown (events 41 and 6008). The
// headless clone did not abort in 18 shutdowns that day, which suggests a
// link to the display but does not prove it. The cause lies in UTM's QEMU
// and is not known. A report written by another VM's QEMU in the same
// interval also fails the check, because reports do not name the VM.
func qemuCrash(since time.Time) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	reports, _ := filepath.Glob(filepath.Join(home, "Library", "Logs", "DiagnosticReports", "QEMULauncher-*.ips"))
	var recent []string
	for _, report := range reports {
		if info, err := os.Stat(report); err == nil && info.ModTime().After(since) {
			recent = append(recent, report)
		}
	}
	if len(recent) == 0 {
		return nil
	}
	return fmt.Errorf("%w; the VM is stopped but its shutdown was not confirmed: %s", errQEMUAborted, strings.Join(recent, ", "))
}

func (r runner) quitUTMIfIdle() error {
	entries, err := r.utm.list()
	if err != nil {
		return err
	}
	if n := runningVMs(entries); n > 0 {
		fmt.Fprintf(r.log, "keeping UTM open: %d VM(s) are not stopped\n", n)
		return nil
	}
	return r.utm.quit()
}

// all stops the VM and quits UTM only when this run started them, and
// reports a teardown failure without hiding the test result.
func (r runner) all() error {
	state, err := r.up()
	if err == nil {
		err = r.test()
	}
	if teardownErr := r.teardown(state); teardownErr != nil {
		fmt.Fprintln(os.Stderr, "Windows E2E teardown:", teardownErr)
		if err == nil {
			err = errors.New("teardown failed")
		}
	}
	return err
}

func (r runner) teardown(state upState) error {
	if state.startedVM {
		vm, err := r.findVM()
		if err != nil {
			return err
		}
		err = r.stopVM(vm)
		if errors.Is(err, errVMBusy) {
			fmt.Fprintln(r.log, "leaving the VM started:", err)
			return nil
		}
		if err != nil && !errors.Is(err, errQEMUAborted) {
			return err
		}
		if state.openedUTM {
			return errors.Join(err, r.quitUTMIfIdle())
		}
		return err
	}
	if state.openedUTM {
		return r.quitUTMIfIdle()
	}
	return nil
}

func (r runner) findVM() (vmEntry, error) {
	entries, err := r.utm.list()
	if err != nil {
		return vmEntry{}, err
	}
	vm, ok := findVM(entries, r.cfg.vm)
	if !ok {
		return vmEntry{}, fmt.Errorf("VM %q is not registered in UTM; utmctl list shows %d VM(s)", r.cfg.vm, len(entries))
	}
	return vm, nil
}
