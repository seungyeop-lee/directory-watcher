//go:build darwin

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	result, data, waitErr := g.awaitResult()
	collectErr := g.collect(data, result.SHA256)
	if waitErr != nil {
		return fmt.Errorf("%w; guest files are kept in %s (partial copies: %v)", waitErr, g.dir, collectErr)
	}
	testErr := result.Check()
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

// execAnswerNote는 exec의 오류를 알릴 때 덧붙인다(utm.exec 참고).
const execAnswerNote = "utmctl exec may answer with the result of another guest command"

// withExecAnswer는 게스트의 파일로 확인한 실패에 그 명령을 보낸 exec의 오류를 덧붙인다.
func withExecAnswer(err, answer error) error {
	if err == nil || answer == nil {
		return err
	}
	return fmt.Errorf("%w (%v; %s)", err, answer, execAnswerNote)
}

// execLogged는 결과를 파일로 확인하는 게스트 명령을 exec로 보내고, exec의 오류는
// runner.log에만 남긴다.
func (g guestTest) execLogged(what string, command ...string) {
	if err := g.utm.exec(g.vm, command...); err != nil {
		fmt.Fprintf(g.log, "%s request (%s): %v\n", what, execAnswerNote, err)
	}
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
	program := filepath.Join(g.local, guestrun.ProgramFile)
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", program, "./e2e/windows/guest")
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build guest program: %v %s", err, out)
	}
	programData, err := os.ReadFile(program)
	if err != nil {
		return err
	}
	files, err := worktree.Files(root)
	if err != nil {
		return err
	}
	var source bytes.Buffer
	if err := archiveSource(&source, root, files); err != nil {
		return err
	}
	sent := map[string][]byte{guestrun.SourceFile: source.Bytes(), guestrun.ProgramFile: programData}
	request := g.newRequest(sent)
	requestData, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	if err := g.recordSource(root, request, requestData, source.Len()); err != nil {
		return err
	}
	if err := g.createGuestDir(requestData); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(sent)) {
		if err := g.push(name, sent[name]); err != nil {
			return err
		}
	}
	return nil
}

// newRequest는 files를 함께 보내는 이번 실행의 요청을 만든다.
func (g guestTest) newRequest(files map[string][]byte) guestrun.Request {
	request := guestrun.Request{
		RunID:    g.id,
		Go:       g.cfg.guestGo,
		Args:     append(append([]string{"test", "-count=1", "-v"}, g.cfg.goTestArgs...), "./e2e/scenarios"),
		Env:      []string{"DW_E2E_KEEP=1"},
		Deadline: g.cfg.deadline,
		SHA256:   map[string]string{},
	}
	if fault := os.Getenv("DW_E2E_FAULT"); fault != "" {
		request.Env = append(request.Env, "DW_E2E_FAULT="+fault)
	}
	for name, data := range files {
		request.SHA256[name] = guestrun.Digest(data)
	}
	return request
}

// archiveSource는 files에서 심볼릭 링크를 뺀 나머지를 worktree.Archive로 묶는다.
// 게스트의 guestrun.Extract는 일반 파일이 아닌 항목을 거부하고, 게스트에서 링크를
// 만드는 것은 Windows 권한에 따라 실패할 수 있다. 저장소의 링크(.agents/skills)는
// 게스트 테스트에 쓰이지 않는다.
func archiveSource(out io.Writer, root string, files []string) error {
	var sent []string
	for _, name := range files {
		info, err := os.Lstat(filepath.Join(root, name))
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		sent = append(sent, name)
	}
	return worktree.Archive(out, root, sent)
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
		strings.TrimSpace(string(head)), size, request.SHA256[guestrun.SourceFile], strings.Join(request.Args, " "), status)
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
// 읽어 만들어졌는지 확인한다. 요청에는 실행 ID가 들어 있어서, 다른 실행의 요청을 읽으면
// 보낸 요청과 다르다.
func (g guestTest) createGuestDir(request []byte) error {
	answer := g.utm.exec(g.vm, "cmd.exe", "/d", "/c", "mkdir", g.dir)
	sameRequest := func(back []byte) error {
		if !bytes.Equal(back, request) {
			return errors.New("request read back from the guest differs from the one sent")
		}
		return nil
	}
	err := retryUntil(time.Now().Add(guestDirLimit), retryInterval, func() error {
		if err := g.push(guestrun.RequestFile, request); err != nil {
			return err
		}
		_, err := g.pull(g.file(guestrun.RequestFile), sameRequest)
		return err
	})
	if err != nil {
		return withExecAnswer(fmt.Errorf("create guest run directory %s: %w", g.dir, err), answer)
	}
	return nil
}

// push는 게스트 실행 디렉터리에 파일을 쓴다. 다른 파일에 쓸 수 있으므로(pull 참고) 보낸
// 파일은 모두 게스트에서 확인한다: request.json은 createGuestDir가 다시 읽어 보고, 나머지는
// 게스트 프로그램이 요청에 적힌 SHA-256과 대조한다(guestrun.Request.Verify).
//
// push는 30초에 128 KiB마다 1초를 더한 시간을 허용한다. 실험에서 push는 초당 약
// 570 KB로 전송되었다.
func (g guestTest) push(name string, data []byte) error {
	limit := 30*time.Second + time.Duration(len(data)/(128<<10))*time.Second
	_, err := g.utm.call(limit, bytes.NewReader(data), "file", "push", g.vm, g.file(name))
	return err
}

// pull은 게스트 파일 path를 읽고, 읽은 내용이 이번 실행의 것이라고 check가 받아들일 때만
// 돌려준다. check가 거절한 내용은 runner.log에 남기고 오류로 돌려준다. 실행기가 게스트에서
// 읽는 파일은 모두 이 함수로 읽는다. 없는 파일을 읽은 오류는 결과를 기다리는 동안 늘
// 나오므로 남기지 않고, 그 밖의 오류는 runner.log에 남긴다.
//
// utmctl file pull과 push는 Guest Agent에 파일 열기, 읽기나 쓰기, 닫기를 따로 요청한다
// (UTM utmctl/UTMCtl.swift). UTM이 Guest Agent와 통신하는 QEMUKit(QEMUManager.m의
// qmp_rpc_call)은 요청에 ID를 붙이지 않고 도착한 응답을 그때 기다리는 요청에 넘기며,
// 10초 안에 응답이 오지 않은 요청을 시간 초과로 끝낸다. 늦게 온 그 응답은 다음 요청이
// 받으므로, 같은 VM을 다른 실행이 함께 쓰면 pull은 오류 없이 다른 실행이 연 파일의
// 내용을 돌려주고 push는 다른 실행이 연 파일에 쓸 수 있다. 시간 초과가 없을 때는 없는
// 파일을 읽는 pull 140번을 다른 pull 480번과 동시에 보내도 내용이 섞이지 않았다. 열기와
// 닫기 사이에 utmctl이 끝나거나 다른 요청이 열기 응답을 받으면, 그 파일은 Guest Agent
// 프로세스가 끝날 때까지 열린 채 남아 지울 수 없다.
func (g guestTest) pull(path string, check func([]byte) error) ([]byte, error) {
	data, err := g.utm.call(pullLimit, nil, "file", "pull", g.vm, path)
	if err != nil {
		if !strings.Contains(err.Error(), "failed to open file") {
			fmt.Fprintln(g.log, "pull:", err)
		}
		return nil, err
	}
	if err := check(data); err != nil {
		fmt.Fprintf(g.log, "ignored %s read from the guest (utmctl file pull may answer with another guest file): %v\n", path, err)
		return nil, err
	}
	return data, nil
}

// startGuestProgram은 exec로 게스트 프로그램을 시작하고, 프로그램이 시작 표시 파일에
// 이번 실행의 ID를 썼는지로 시작했는지 확인한다.
func (g guestTest) startGuestProgram() error {
	answer := g.utm.exec(g.vm, g.file(guestrun.ProgramFile), g.dir)
	startedByThisRun := func(data []byte) error {
		if string(data) != g.id {
			return fmt.Errorf("started file names run %q, not %q", data, g.id)
		}
		return nil
	}
	err := retryUntil(time.Now().Add(programStartLimit), retryInterval, func() error {
		_, err := g.pull(g.dir+guestrun.StartedSuffix, startedByThisRun)
		return err
	})
	if err != nil {
		err = fmt.Errorf("guest program did not start within %s (%v); check that the Guest Agent can run %s", programStartLimit, err, g.file(guestrun.ProgramFile))
	}
	return withExecAnswer(err, answer)
}

// resultInterval은 결과를 다시 확인하기 전에 기다리는 시간이고, resultGrace는 실행의
// deadline이 지난 뒤에도 결과를 더 기다리는 시간이다.
var (
	resultInterval = 3 * time.Second
	resultGrace    = 2 * time.Minute
)

// awaitResult는 게스트 프로그램이 마지막에 쓰는 이번 실행의 결과가 나올 때까지 반복해서
// 확인한다. 실행의 deadline이 go test를 멈추므로, 결과는 deadline 직후에 나와야 한다.
// exec는 게스트 프로그램이 끝나기를 기다리지 않으므로(utm.exec 참고), 결과 파일은 실행
// 결과를 알 수 있는 유일한 근거다.
func (g guestTest) awaitResult() (guestrun.Result, []byte, error) {
	limit := g.cfg.deadline + resultGrace
	fmt.Fprintf(g.log, "waiting up to %s for the guest result\n", limit)
	start := time.Now()
	lastReport := start
	var result guestrun.Result
	var data []byte
	err := retryUntil(start.Add(limit), resultInterval, func() (err error) {
		if time.Since(lastReport) >= time.Minute {
			fmt.Fprintf(g.log, "guest run in progress (%s)\n", time.Since(start).Round(time.Second))
			lastReport = time.Now()
		}
		data, err = g.pull(g.file(guestrun.ResultFile), func(data []byte) (err error) {
			result, err = guestrun.Decode(data, g.id)
			return err
		})
		return err
	})
	if err != nil {
		return guestrun.Result{}, nil, fmt.Errorf("no guest result within %s (%v)", limit, err)
	}
	return result, data, nil
}

// collect는 결과가 있으면 결과와 함께 로그와 진단 자료를 Mac으로 복사하고, go test
// 출력을 표시한다. digests는 결과에 적힌 파일별 SHA-256이고 결과가 없으면 nil이다. 결과에
// SHA-256이 없는 파일은 게스트에 없던 파일이므로 가져오지 않는다.
func (g guestTest) collect(result []byte, digests map[string]string) error {
	var problems []error
	if result != nil {
		if err := os.WriteFile(filepath.Join(g.local, guestrun.ResultFile), result, 0644); err != nil {
			problems = append(problems, err)
		}
	}
	for _, name := range guestrun.CollectedFiles {
		if digests != nil && digests[name] == "" {
			fmt.Fprintf(g.log, "%s: not collected because the guest result has no SHA-256 for it (the file was not in the guest)\n", name)
			continue
		}
		data, err := g.pullCollected(name, digests[name])
		if err == nil {
			err = g.keep(name, data)
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(problems...)
}

// collectAttempts는 회수할 파일 하나를 가져오는 횟수의 한도다.
const collectAttempts = 3

// pullCollected는 회수할 파일 name을 가져온다. want가 있으면 내용의 SHA-256이 want와
// 같을 때까지 다시 가져온다. 결과를 받지 못해 want가 없으면 대조할 값이 없으므로, 내용을
// 확인하지 않고 한 번만 가져온다.
func (g guestTest) pullCollected(name, want string) ([]byte, error) {
	if want == "" {
		return g.pull(g.file(name), func([]byte) error { return nil })
	}
	sameDigest := func(data []byte) error {
		if got := guestrun.Digest(data); got != want {
			return fmt.Errorf("SHA-256 %s differs from %s in the guest result", got, want)
		}
		return nil
	}
	for attempt := 1; ; attempt++ {
		data, err := g.pull(g.file(name), sameDigest)
		if err == nil || attempt == collectAttempts {
			return data, err
		}
		time.Sleep(retryInterval)
	}
}

// keep은 가져온 파일을 Mac에 둔다. 진단 자료는 풀어 두고, go test 출력은 표시도 한다.
func (g guestTest) keep(name string, data []byte) error {
	if name == guestrun.DiagnosticsFile {
		return guestrun.Extract(bytes.NewReader(data), filepath.Join(g.local, "diagnostics"))
	}
	if err := os.WriteFile(filepath.Join(g.local, name), data, 0644); err != nil {
		return err
	}
	os.Stdout.Write(data)
	return nil
}

// removeLimit은 게스트 실행 디렉터리를 지우는 데 주는 시간이다. 이 시간 안에서는
// 디렉터리가 남을 때마다 rmdir를 다시 보낸다.
var removeLimit = time.Minute

// removeGuestDir는 파일이 Mac에 도착한 뒤 게스트 실행 디렉터리를 지운다. rmdir를 실행한
// cmd.exe가 디렉터리가 남았는지를 실행 ID와 함께 실행 디렉터리 옆의 표시 파일에 적고,
// 실행기는 그 파일을 읽어 삭제를 확인한다. 실행 디렉터리 안의 파일을 읽어 확인하면, pull이
// 연 그 파일 때문에 rmdir가 디렉터리를 지우지 못한다(pull 참고).
func (g guestTest) removeGuestDir() error {
	defer g.deleteMarkers()
	deadline := time.Now().Add(removeLimit)
	for attempt := 1; ; attempt++ {
		marker := g.dir + ".rmdir-" + strconv.Itoa(attempt)
		g.execLogged("rmdir", "cmd.exe", "/d", "/c",
			"rmdir", "/s", "/q", g.dir, "&", "(if", "exist", g.dir, "(echo", g.id, "kept)", "else", "(echo", g.id, "removed))>", marker)
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
			time.Sleep(retryInterval)
		default:
			return fmt.Errorf("could not confirm that guest run directory %s was removed: rmdir attempt %d reported nothing within %s; "+
				"if it is still there (see e2e/docs/windows-utm.md), remove it with this command:\n    %s",
				g.dir, attempt, removeLimit, g.removeCommand())
		}
	}
}

// awaitRemoval은 rmdir 한 번이 표시 파일에 이번 실행의 ID와 함께 적은 결과를 deadline까지
// 기다려 "removed"나 "kept"를 돌려준다. 이번 실행의 결과를 읽지 못하면 빈 문자열을 돌려준다.
func (g guestTest) awaitRemoval(marker string, deadline time.Time) string {
	var outcome string
	thisRunsReport := func(data []byte) error {
		report := strings.TrimSpace(string(data))
		state, ok := strings.CutPrefix(report, g.id+" ")
		if !ok || (state != "removed" && state != "kept") {
			return fmt.Errorf("rmdir report %q is not this run's", report)
		}
		outcome = state
		return nil
	}
	retryUntil(deadline, retryInterval, func() error {
		_, err := g.pull(marker, thisRunsReport)
		return err
	})
	return outcome
}

// deleteMarkers는 이번 실행이 실행 디렉터리 옆에 둔 시작 표시 파일과 rmdir 표시 파일을
// 지운다. 지우지 못한 표시 파일은 몇 바이트뿐이므로 확인하지 않는다.
func (g guestTest) deleteMarkers() {
	g.execLogged("delete markers", "cmd.exe", "/d", "/c", "del", "/q", g.dir+guestrun.StartedSuffix, g.dir+".rmdir-*")
}

// removeCommand는 남은 실행 디렉터리를 Mac의 셸에서 지우는 명령이다. 사용자가 경고의
// 마지막 줄을 그대로 복사해 실행하므로, 셸에 그대로 넘길 수 있는 명령만 담는다.
func (g guestTest) removeCommand() string {
	return fmt.Sprintf("'%s' exec %s --cmd cmd.exe /d /c rmdir /s /q '%s'", g.utm.ctl, g.vm, g.dir)
}
