//go:build darwin

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/windows/guestrun"
	"github.com/seungyeop-lee/directory-watcher/v3/e2e/worktree"
)

// test는 up이 준비한 VM에서 시나리오를 실행한다. UTM을 열거나 VM을 시작하지는
// 않는다: utmctl 요청은 UTM을 띄우므로, utmctl 요청을 보내기 전에 UTM 프로세스를
// 확인한다.
func (r runner) test() error {
	if !utmRunning() {
		return notUp("UTM is not running")
	}
	vm, err := r.findVM()
	if err != nil {
		return err
	}
	if vm.status != "started" {
		return notUp("VM is " + vm.status)
	}
	if err := r.utm.guestAgentReady(vm.uuid); err != nil {
		return notUp("Guest Agent does not answer: " + err.Error())
	}
	lock, err := lockVM(vm.uuid, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	local, err := os.MkdirTemp("", "dw-e2e-windows-*")
	if err != nil {
		return err
	}
	fmt.Println("Windows diagnostics:", local)
	logFile, err := os.Create(filepath.Join(local, "runner.log"))
	if err != nil {
		return err
	}
	defer logFile.Close()
	r.log = io.MultiWriter(os.Stdout, logFile)
	r.utm.log = r.log
	return guestTest{runner: r, vm: vm.uuid, local: local}.run()
}

func notUp(reason string) error {
	return fmt.Errorf("%s; run make e2e-windows-up first", reason)
}

// guestTest는 실행 한 번을 나타낸다. ID로 게스트 실행 디렉터리와, 실행기가 받아들일
// 결과를 모두 가리킨다.
type guestTest struct {
	runner
	vm, local, id, dir string
}

func (g guestTest) run() error {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	g.id = time.Now().Format("20060102-150405-") + hex.EncodeToString(id)
	g.dir = strings.TrimRight(g.cfg.guestDir, `\`) + `\` + g.id
	fmt.Fprintln(g.log, "run:", g.id, "guest directory:", g.dir)
	if err := g.send(); err != nil {
		return fmt.Errorf("prepare guest run: %w", err)
	}
	if err := g.startGuestProgram(); err != nil {
		return err
	}
	data, waitErr := g.awaitResult()
	collectErr := g.collect(data)
	if waitErr != nil {
		return fmt.Errorf("%w; guest files are kept in %s (partial copies: %v)", waitErr, g.dir, collectErr)
	}
	result, testErr := guestrun.Check(data, g.id)
	fmt.Fprintf(g.log, "guest: %s, %s %s, run directory %s on a %s drive\n",
		result.Guest.Windows, result.Guest.GoVersion, result.Guest.GoEnv, result.Guest.RunDir, result.Guest.DriveType)
	if testErr == nil && result.Guest.DriveType != "fixed" {
		testErr = fmt.Errorf("guest run directory is on a %q drive", result.Guest.DriveType)
	}
	var cleanupErr error
	if collectErr == nil {
		cleanupErr = g.removeGuestDir()
	} else {
		collectErr = fmt.Errorf("collect guest files: %w; they are kept in %s", collectErr, g.dir)
	}
	fmt.Fprintln(g.log, "Windows diagnostics:", g.local)
	return report(os.Stderr, testErr, collectErr, cleanupErr)
}

// report는 실행 결과를 돌려준다. 게스트 실행 디렉터리를 지우지 못한 것은 테스트 결과를
// 바꾸지 않으므로, 결과에 합치지 않고 warn에 경고로 알린다.
func report(warn io.Writer, testErr, collectErr, cleanupErr error) error {
	if cleanupErr != nil {
		fmt.Fprintln(warn, "Windows E2E cleanup:", cleanupErr)
	}
	return errors.Join(testErr, collectErr)
}

func (g guestTest) file(name string) string { return g.dir + `\` + name }

// withExecAnswer는 게스트의 파일로 확인한 실패에 그 명령을 보낸 exec의 오류를 덧붙인다.
func withExecAnswer(err, answer error) error {
	if err == nil || answer == nil {
		return err
	}
	return fmt.Errorf("%w (%v; utmctl exec may answer with the result of another guest command)", err, answer)
}

// send는 게스트 프로그램을 빌드하고 작업 트리를 아카이브로 묶은 뒤, 둘을 요청과 함께
// 새 게스트 실행 디렉터리로 보낸다.
func (g guestTest) send() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if root, err = worktree.Root(root); err != nil {
		return err
	}
	// 게스트 프로그램은 amd64용으로 빌드해 바이너리 하나가 Windows x64와 ARM64 양쪽에서
	// 실행되게 한다. ARM64의 Windows 11은 이 바이너리를 에뮬레이션으로 실행한다.
	// 2026-11-03 실험의 ARM64 게스트에서는 Guest Agent 자체도 x64 프로세스로
	// 실행되었다(Guest Agent가 실행한 명령에서 PROCESSOR_ARCHITECTURE=AMD64). 테스트는
	// 그래도 게스트에 설치된 Go 툴체인으로, 게스트의 아키텍처에서 실행한다.
	program := filepath.Join(g.local, "guest.exe")
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", program, "./e2e/windows/guest")
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build guest program: %v %s", err, out)
	}
	files, err := worktree.Files(root)
	if err != nil {
		return err
	}
	var source bytes.Buffer
	if err := worktree.Archive(&source, root, files); err != nil {
		return err
	}
	sum := sha256.Sum256(source.Bytes())
	request := guestrun.Request{
		Go:           g.cfg.guestGo,
		Args:         append(append([]string{"test", "-count=1", "-v"}, g.cfg.goTestArgs...), "./e2e/scenarios"),
		Env:          []string{"DW_E2E_KEEP=1"},
		Deadline:     g.cfg.deadline,
		SourceSHA256: hex.EncodeToString(sum[:]),
	}
	if fault := os.Getenv("DW_E2E_FAULT"); fault != "" {
		request.Env = append(request.Env, "DW_E2E_FAULT="+fault)
	}
	requestData, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	if err := g.recordSource(root, request, requestData, source.Len()); err != nil {
		return err
	}
	programData, err := os.ReadFile(program)
	if err != nil {
		return err
	}
	if err := g.createGuestDir(requestData); err != nil {
		return err
	}
	if err := g.push(guestrun.SourceFile, source.Bytes()); err != nil {
		return err
	}
	return g.push("guest.exe", programData)
}

// recordSource는 이번 실행이 쓴 소스와 테스트 범위를 Mac에 남긴다.
func (g guestTest) recordSource(root string, request guestrun.Request, requestData []byte, size int) error {
	head, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return err
	}
	status, err := exec.Command("git", "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("commit: %s\nsource.tar: %d bytes, sha256 %s\ngo %s\nuncommitted changes:\n%s",
		strings.TrimSpace(string(head)), size, request.SourceSHA256, strings.Join(request.Args, " "), status)
	fmt.Fprint(g.log, summary)
	if err := os.WriteFile(filepath.Join(g.local, "source.txt"), []byte(summary), 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.local, guestrun.RequestFile), requestData, 0644)
}

// guestDirLimit은 실행 디렉터리가 만들어졌는지 확인하는 시간이고, programStartLimit은
// 게스트 프로그램이 시작됐는지 확인하는 시간이다.
var (
	guestDirLimit     = 30 * time.Second
	programStartLimit = time.Minute
)

// createGuestDir는 exec로 mkdir를 실행해 실행 디렉터리를 만들고, 요청을 보낸 뒤 다시
// 읽어 만들어졌는지 확인한다. exec의 응답으로는 mkdir의 결과를 알 수 없고(utm.exec
// 참고) push와 pull은 실패해도 0으로 끝나므로, 디렉터리가 있는지와 파일 전송이
// 동작하는지는 다시 읽어 보는 방법으로만 확인할 수 있다.
func (g guestTest) createGuestDir(request []byte) error {
	answer := g.utm.exec(g.vm, "cmd.exe", "/d", "/c", "mkdir", g.dir)
	deadline := time.Now().Add(guestDirLimit)
	for {
		err := g.push(guestrun.RequestFile, request)
		if err == nil {
			var back []byte
			if back, err = g.pull(guestrun.RequestFile, requestLimit); err == nil && !bytes.Equal(back, request) {
				err = errors.New("request read back from the guest differs from the one sent")
			}
		}
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return withExecAnswer(fmt.Errorf("create guest run directory %s: %w", g.dir, err), answer)
		}
		time.Sleep(time.Second)
	}
}

// push는 30초에 128 KiB마다 1초를 더한 시간을 허용한다. 실험에서 push는 초당 약
// 570 KB로 전송되었다.
func (g guestTest) push(name string, data []byte) error {
	limit := 30*time.Second + time.Duration(len(data)/(128<<10))*time.Second
	_, err := g.utm.call(limit, bytes.NewReader(data), "file", "push", g.vm, g.file(name))
	return err
}

func (g guestTest) pull(name string, limit time.Duration) ([]byte, error) {
	return g.utm.call(limit, nil, "file", "pull", g.vm, g.file(name))
}

// startGuestProgram은 exec로 게스트 프로그램을 시작하고, 프로그램이 쓰는 시작 표시
// 파일로 시작했는지 확인한다. exec의 응답으로는 시작 여부를 알 수 없다(utm.exec 참고).
func (g guestTest) startGuestProgram() error {
	answer := g.utm.exec(g.vm, g.file("guest.exe"), g.dir)
	return withExecAnswer(g.awaitStarted(), answer)
}

func (g guestTest) awaitStarted() error {
	deadline := time.Now().Add(programStartLimit)
	for {
		data, err := g.pull(guestrun.StartedFile, requestLimit)
		if err == nil && string(data) == g.id {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("guest program did not start within %s (%v); check that the Guest Agent can run %s", programStartLimit, err, g.file("guest.exe"))
		}
		time.Sleep(time.Second)
	}
}

// awaitResult는 게스트 프로그램이 마지막에 쓰는 결과가 나올 때까지 반복해서
// 확인한다. 실행의 deadline이 go test를 멈추므로, 결과는 deadline 직후에 나와야
// 한다. exec는 게스트 프로그램이 끝나기를 기다리지 않으므로(utm.exec 참고), 결과
// 파일은 실행 결과를 알 수 있는 유일한 근거다. run은 이번 실행의 ID가 적힌 결과만
// 받아들이므로, 다른 실행이 남긴 결과를 이번 실행의 결과로 받지 않는다.
func (g guestTest) awaitResult() ([]byte, error) {
	limit := g.cfg.deadline + 2*time.Minute
	fmt.Fprintf(g.log, "waiting up to %s for the guest result\n", limit)
	start := time.Now()
	lastReport := start
	for {
		data, err := g.pull(guestrun.ResultFile, requestLimit)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		if time.Since(start) > limit {
			return nil, fmt.Errorf("no guest result within %s (%v)", limit, err)
		}
		if time.Since(lastReport) >= time.Minute {
			fmt.Fprintf(g.log, "guest run in progress (%s)\n", time.Since(start).Round(time.Second))
			lastReport = time.Now()
		}
		time.Sleep(3 * time.Second)
	}
}

// collect는 결과가 있으면 결과와 함께 로그와 진단 자료를 Mac으로 복사하고, go test
// 출력을 표시한다.
func (g guestTest) collect(result []byte) error {
	var problems []error
	if result != nil {
		if err := os.WriteFile(filepath.Join(g.local, guestrun.ResultFile), result, 0644); err != nil {
			problems = append(problems, err)
		}
	}
	for _, name := range []string{guestrun.StdoutFile, guestrun.StderrFile} {
		data, err := g.pull(name, pullLimit)
		if err == nil {
			err = os.WriteFile(filepath.Join(g.local, name), data, 0644)
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
			continue
		}
		os.Stdout.Write(data)
	}
	data, err := g.pull(guestrun.DiagnosticsFile, pullLimit)
	if err == nil {
		err = guestrun.Extract(bytes.NewReader(data), filepath.Join(g.local, "diagnostics"))
	}
	if err != nil {
		problems = append(problems, fmt.Errorf("%s: %w", guestrun.DiagnosticsFile, err))
	}
	return errors.Join(problems...)
}

// removeLimit은 게스트 실행 디렉터리를 지우는 데 주는 시간이다. 이 시간 안에서는
// 디렉터리가 남을 때마다 rmdir를 다시 보낸다.
var removeLimit = time.Minute

// removeGuestDir는 파일이 Mac에 도착한 뒤 게스트 실행 디렉터리를 지운다. rmdir를 실행한
// cmd.exe가 디렉터리가 남았는지를 실행 디렉터리 옆의 표시 파일에 적고, 실행기는 그
// 파일을 읽어 삭제를 확인한다. exec의 응답은 근거로 쓰지 않는다(utm.exec 참고).
//
// 삭제를 확인하려고 실행 디렉터리 안의 파일을 읽으면 안 된다. Guest Agent가 file
// pull로 열어 둔 파일은 rmdir가 지우지 못해서, 그날 rmdir가 지우는 동안 request.json을
// 읽자 디렉터리에 request.json만 남았다. 디렉터리를 현재 디렉터리로 쓰는 프로세스가
// 남아 있어도 rmdir는 디렉터리를 지우지 못한다.
func (g guestTest) removeGuestDir() error {
	defer g.deleteRemovalMarkers()
	deadline := time.Now().Add(removeLimit)
	for attempt := 1; ; attempt++ {
		marker := g.dir + ".rmdir-" + strconv.Itoa(attempt)
		if err := g.utm.exec(g.vm, "cmd.exe", "/d", "/c",
			"rmdir", "/s", "/q", g.dir, "&", "(if", "exist", g.dir, "(echo", "kept)", "else", "(echo", "removed))>", marker); err != nil {
			fmt.Fprintln(g.log, "rmdir request (utmctl exec may answer with the result of another guest command):", err)
		}
		switch g.awaitRemoval(marker, deadline) {
		case "removed":
			return nil
		case "kept":
			if time.Now().After(deadline) {
				return fmt.Errorf("guest run directory %s is still there after %d rmdir attempts in %s; "+
					"stop the guest processes that use it as e2e/docs/windows-utm.md describes, then remove it with this command:\n    %s",
					g.dir, attempt, removeLimit, g.removeCommand())
			}
			if attempt == 1 {
				fmt.Fprintf(g.log, "guest run directory is still there after rmdir; retrying for up to %s\n", removeLimit)
			}
			time.Sleep(time.Second)
		default:
			return fmt.Errorf("could not confirm that guest run directory %s was removed: rmdir attempt %d reported nothing within %s; "+
				"if it is still there (see e2e/docs/windows-utm.md), remove it with this command:\n    %s",
				g.dir, attempt, removeLimit, g.removeCommand())
		}
	}
}

// awaitRemoval은 rmdir 한 번이 표시 파일에 적은 결과를 deadline까지 기다린다. 결과가
// 없으면 빈 문자열을 돌려준다.
func (g guestTest) awaitRemoval(marker string, deadline time.Time) string {
	for {
		data, err := g.utm.call(requestLimit, nil, "file", "pull", g.vm, marker)
		if state := strings.TrimSpace(string(data)); err == nil && (state == "removed" || state == "kept") {
			return state
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(time.Second)
	}
}

// deleteRemovalMarkers는 이번 실행의 표시 파일을 지운다. 지우지 못한 표시 파일은 몇
// 바이트뿐이므로 확인하지 않는다.
func (g guestTest) deleteRemovalMarkers() {
	if err := g.utm.exec(g.vm, "cmd.exe", "/d", "/c", "del", "/q", g.dir+".rmdir-*"); err != nil {
		fmt.Fprintln(g.log, "delete rmdir markers request (utmctl exec may answer with the result of another guest command):", err)
	}
}

// removeCommand는 남은 실행 디렉터리를 Mac의 셸에서 지우는 명령이다. 사용자가 경고의
// 마지막 줄을 그대로 복사해 실행하므로, 셸에 그대로 넘길 수 있는 명령만 담는다.
func (g guestTest) removeCommand() string {
	return fmt.Sprintf("'%s' exec %s --cmd cmd.exe /d /c rmdir /s /q '%s'", g.utm.ctl, g.vm, g.dir)
}
