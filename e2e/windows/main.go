//go:build darwin

// Command windows는 Mac의 UTM에 있는 Windows VM에서 Go E2E 시나리오를 실행한다.
// VM을 부팅하고 멈추는 데 오래 걸리므로 up, test, down을 별도 명령으로 두며, all은
// 이 명령들을 차례로 실행하되 자신이 시작한 것만 멈춘다.
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
			// 거부되었거나 실패한 up이 아무것도 시작하지 않았다면, up이 연 UTM을
			// 남겨 두지 않는다. 시작한 VM은 down을 위해 남겨 둔다.
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

// upState는 up이 바꾼 것을 기록해, all이 그것만 멈추게 한다.
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

// requireHeadless는 디스플레이 장치가 있는 VM을 거부한다. start에 --hide를 붙이든
// 붙이지 않든 UTM은 디스플레이마다 창을 열기 때문이다: 2026-11-03에 원래 VM은
// 시작할 때마다 VM 창이 사용자의 작업 화면 위로 떴고, --hide는 기껏해야 UTM의 메인
// 창만 닫았다. UTM의 QEMU 백엔드는 디스플레이 장치가 없는 VM을 창 없이 실행한다
// (UTM #6188, https://github.com/utmapp/UTM/issues/6188). 디스플레이를 제거한
// 복제본은 UTM 창도 QEMURenderServer도 없이 부팅했고, QEMU 인자에는 -vga none이
// 들어 있었다. 실행기는 구성을 바꾸지 않으며, 복제본은 한 번 손으로 준비한다.
// 부팅할 때 소리가 나지 않도록 사운드 장치도 제거해야 하지만, UTM의 AppleScript
// 사전(UTM.sdef)에는 확인할 사운드 설정이 없으므로 이 조건은 문서에서만 다룬다.
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

// startVM은 start가 효과가 없으면 UTM을 한 번 다시 시작하되, 등록된 VM이 모두
// stopped일 때만 그렇게 한다. 2026-11-03 실험과 irgo-windows-vm PR #9
// (https://github.com/joeblew999/irgo-windows-vm/pull/9)에서는 UTM을 다시 시작하자
// 응답하지 않던 start가 풀렸다.
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

// stopVM은 게스트 OS에 종료를 요청하고 stopped가 될 때까지 기다린다. --request
// 없이 utmctl stop을 실행하면 VM의 전원을 끄므로, 그렇게는 호출하지 않는다.
//
// Windows는 종료 요청을 종료 이벤트도 남기지 않고 무시할 때가 있고(Windows Update
// 중이나 테스트 직후에 보았다), 다시 보낸 요청으로는 몇 초에서 1분 안에 멈췄다. 원인은
// 알려지지 않았으므로 VM이 멈출 때까지 shutdownRetry마다 요청을 되풀이한다. 완료된
// 요청은 매번 완전한 종료였다: 게스트는 winlogon.exe가 전원을 끈 기록(event 1074,
// reason 0x500ff)과 이벤트 로그의 정상 중지(6006)를 남겼고, 다음 부팅은 콜드 부팅
// (Kernel-Boot event 27, boot type 0x0)이었으며, 그 게스트는 최대 절전 모드가 꺼져
// 있어 빠른 시작을 쓸 수 없었다.
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

// qemuCrash는 종료를 요청한 뒤에 기록된 QEMU 충돌 보고서를 알려 준다. QEMU
// 프로세스가 중단된 VM은 정상 종료가 확인되지 않은 채 stopped가 되기 때문이다.
// 중단은 종료가 끝날 무렵 hvf_vcpu_exec의 assertion에서 SIGABRT로 일어났고, 디스플레이가
// 있던 VM은 종료 11번 중 7번 중단되었지만 headless 복제본은 18번 중 한 번도 중단되지
// 않았다. 디스플레이와 관련이 있어 보이지만 원인은 UTM의 QEMU 쪽에 있고 알려지지
// 않았다. 보고서에는 VM이 적히지 않으므로, 같은 구간에 다른 VM의 QEMU가 쓴 보고서도
// 검사를 실패시킨다.
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

// all은 이번 실행이 VM을 시작하고 UTM을 띄웠을 때만 VM을 멈추고 UTM을 종료하며,
// 정리 단계의 실패는 테스트 결과를 가리지 않게 함께 알린다.
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
	var err error
	if state.startedVM {
		vm, findErr := r.findVM()
		if findErr != nil {
			return findErr
		}
		err = r.stopVM(vm)
		if errors.Is(err, errVMBusy) {
			fmt.Fprintln(r.log, "leaving the VM started:", err)
			return nil
		}
		if err != nil && !errors.Is(err, errQEMUAborted) {
			return err
		}
	}
	if state.openedUTM {
		return errors.Join(err, r.quitUTMIfIdle())
	}
	return err
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
