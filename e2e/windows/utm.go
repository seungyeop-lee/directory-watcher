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

// utmctl 요청 하나에 주는 시간 한도다. 한도가 없으면 응답하지 않는 요청은 Apple
// Event 시간 제한인 2분 동안 기다린다. 2026-11-03 실험(macOS 27, UTM 5.0.8)에서
// start는 VM을 시작하지 못한 채 2분 뒤 OSStatus error -1712로 끝났고, UTM #8006
// (https://github.com/utmapp/UTM/issues/8006)도 같은 멈춤을 보고한다. 원인은
// 알려지지 않았다.
const (
	requestLimit = 15 * time.Second
	startLimit   = 30 * time.Second
	appLimit     = 30 * time.Second
	pullLimit    = 2 * time.Minute
)

// startedLimit는 start 뒤에 VM이 started를 보고하기까지 기다리는 시간이다.
// 실험에서는 2초에서 3초가 걸렸다.
var startedLimit = time.Minute

// utm은 UTM.app 안에 있는 utmctl로 UTM을 제어한다. Homebrew 심볼릭 링크
// /opt/homebrew/bin/utmctl로 호출하면 2026-11-03 실험에서 start가 응답하지 않았고,
// irgo-windows-vm PR #13(https://github.com/joeblew999/irgo-windows-vm/pull/13)도
// 같은 결과를 측정했다. 두 곳 모두 원인은 찾지 못했다. 그래서 main은 심볼릭 링크
// 경로를 거부한다.
type utm struct {
	ctl, osascript string
	log            io.Writer
}

// app은 Contents/MacOS/utmctl을 담고 있는 UTM.app을 돌려준다.
func (u utm) app() string { return filepath.Dir(filepath.Dir(filepath.Dir(u.ctl))) }

// call은 limit 안에서 utmctl을 실행하고 출력으로 성공 여부를 판단한다. 2026-11-03에
// 본 실패에서 utmctl은 모두 0으로 끝났다: VM을 stopped로 남긴 start, 없는 경로를
// 지정해 오류를 출력한 file pull과 push, 게스트 명령을 기다리지 않는 exec가 그랬다.
// 이런 실패는 모두 stderr에 메시지를 출력했으므로, OSStatus error -10004 줄을 뺀
// stderr 출력은 모두 오류로 본다. 이 줄은 --hide 이벤트에서만 나왔고, --hide를 붙인
// 명령은 성공한 start까지 포함해 모두 이 줄을 두 번 출력했다. hideMainWindow를
// 참고한다.
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

// parseList는 utmctl list가 출력하는 표를 읽는다. 표는 머리글 한 줄 뒤에 VM마다
// UUID, 상태, 이름을 적은 줄이 이어지며, 이름에는 공백이 들어갈 수 있다.
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

// findVM은 utmctl과 같은 방식으로, UUID나 전체 이름이 식별자와 일치하는 VM을
// 찾는다.
func findVM(entries []vmEntry, id string) (vmEntry, bool) {
	for _, entry := range entries {
		if strings.EqualFold(entry.uuid, id) || entry.name == id {
			return entry, true
		}
	}
	return vmEntry{}, false
}

// runningVMs는 stopped가 아닌 VM을 센다. UTM을 종료하거나 다시 시작하면 일시 정지된
// VM과 상태가 바뀌는 중인 VM도 영향을 받으므로, 이런 VM도 센다.
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

// start는 stopped 상태인 VM에 시작을 요청하고, utmctl status가 started를 보고할
// 때까지 기다린다. VM이 stopped로 남아도 start는 0으로 끝나기 때문이다.
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

// hideMainWindow는 utmctl --hide가 하는 일을 AppleScript로 한다: auto terminate를
// 끄고 이름이 "UTM"인 창을 닫는다
// (https://github.com/utmapp/UTM/blob/main/utmctl/UTMCtl.swift). 2026-11-03에는
// utmctl이 보낸 두 이벤트가 모두 실패해 이벤트마다 OSStatus error -10004를 한 번씩
// 출력했고, 메인 창은 화면에 남았다. 같은 이벤트를 osascript로 보내자 창이 닫혔다.
// utmctl의 이벤트가 실패하는 원인은 알려지지 않았다. 두 방법 모두 VM의 디스플레이
// 창은 닫지 못하며, 디스플레이 장치가 없는 VM만 창 없이 실행된다(up 참고). auto
// terminate를 꺼 둔 채로 두어, 마지막 창이 닫혀도 UTM이 스스로 종료하지 않게 한다.
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

// displays는 VM 구성에 있는 디스플레이 장치 수를 돌려준다. utmctl은 이 값을 보여
// 주지 않으므로 UTM의 AppleScript 사전으로 읽는다. 한 줄로 쓴 length of (get
// displays of (configuration of ...))는 2026-11-03에 error -1700으로 실패했고,
// 변수를 거쳐 단계별로 읽는 형태는 동작했다. tell은 UTM이 실행 중이 아니면 UTM을
// 띄우므로, open 뒤에만 호출한다.
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

// await는 utmctl status가 want를 보고할 때까지 상태를 반복해서 확인한다.
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

// awaitGuestAgent는 Guest Agent가 IPv4 주소를 보고할 때까지 기다린다. VM이
// started라고 해서 게스트가 부팅을 마친 것은 아니다: 실험에서 agent는 started 뒤
// 7초에서 10초가 지나서야 응답했다.
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

// utmRunning은 UTM 앱이 실행 중인지 알려 준다. utmctl 요청은 UTM이 실행 중이
// 아니면 UTM을 띄우므로, UTM을 띄우면 안 되는 utmctl 요청을 보내기 전에 이 값을
// 확인한다.
var utmRunning = func() bool { return exec.Command("pgrep", "-x", "UTM").Run() == nil }

// open은 utmctl이나 AppleScript가 UTM을 띄우게 두지 않고, open -g로 UTM을
// 백그라운드에서 띄운다. UTM #8006은 macOS 27의 UTM 5.0.8을 utmctl이 띄우면 UTM이
// 충돌하고 start가 멈춘다고 보고하고, irgo-windows-vm PR #13은 UTM을 띄우고 0.3초
// 안에 요청을 보내면 start가 응답하지 않는다고 측정했다. 2026-11-03 실험에서는 첫
// 요청 전에 2초를 기다리자 start가 동작했지만, 원인은 알려지지 않았다.
//
// -g는 UTM을 맨 앞 앱 뒤에 둘 뿐이어서, UTM은 -g를 붙여도 실행 후 0.5초 안에 메인
// 창을 띄운다. 그래서 창은 대기가 끝난 뒤가 아니라 곧바로 닫는다. 2026-11-03에 8번
// 실행하는 동안 이 이벤트를 일찍 보내도 start가 응답하지 않는 일은 없었다. open
// -j는 UTM을 숨긴 채 띄우지만, 그날 두 번 중 한 번은 start가 VM을 stopped로
// 남겼다.
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

// quit은 UTM에 정상 종료를 요청하고 UTM이 끝날 때까지 기다린다. UTM을 강제로
// 종료하지는 않으며, 호출하는 쪽은 실행 중인 VM이 없을 때만 quit을 호출한다.
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
