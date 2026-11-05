//go:build darwin

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/windows/guestrun"
)

const (
	fakeRunID  = "20261103-223804-8cbc351a9c24e6c8"
	fakeRunDir = `C:\dw-e2e\` + fakeRunID
)

// fakeGuest는 rmdir 요청을 받으면 처음 keptFor번은 디렉터리가 남았다고, 그 뒤로는
// 지워졌다고 요청의 마지막 인자인 표시 파일에 적는 utmctl을 만든다. answers가 false면
// exec는 아무것도 하지 않는다. 받은 요청은 state 디렉터리의 requests에 남는다.
func fakeGuest(t *testing.T, keptFor int, answers bool) (guestTest, string) {
	t.Helper()
	state := t.TempDir()
	script := `state='` + state + `'
printf '%s\n' "$*" >> "$state/requests"
for last; do :; done
case "$1" in
exec)
	case "$*" in
	*rmdir*)
		[ ` + strconv.FormatBool(answers) + ` = true ] || exit 0
		n=$(cat "$state/attempts" 2>/dev/null || echo 0); n=$((n + 1)); echo $n > "$state/attempts"
		if [ $n -gt ` + strconv.Itoa(keptFor) + ` ]; then printf 'removed\r\n'; else printf 'kept\r\n'; fi > "$state/${last##*\\}" ;;
	esac ;;
file)
	if [ -f "$state/${last##*\\}" ]; then cat "$state/${last##*\\}"; else echo 'Error from event: (OSStatus error -2700.)' >&2; fi ;;
esac
exit 0`
	g := guestTest{
		runner: runner{utm: fakeUTMCtl(t, script), log: io.Discard},
		vm:     "95B99283-8256-4360-AA1D-0563FCD5CE09",
		dir:    fakeRunDir,
	}
	return g, state
}

func requests(t *testing.T, state string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(state, "requests"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestRemoveGuestDirRetriesWithoutReadingInsideTheRunDirectory(t *testing.T) {
	shortRetryInterval(t)
	g, state := fakeGuest(t, 1, true)
	if err := g.removeGuestDir(); err != nil {
		t.Fatalf("removeGuestDir = %v; want the second rmdir to remove the directory", err)
	}
	var rmdirs, deletes int
	for _, request := range requests(t, state) {
		switch {
		case strings.HasPrefix(request, "file pull") && strings.Contains(request, fakeRunDir+`\`):
			// Guest Agent가 읽는 동안 연 파일은 rmdir가 지우지 못한다.
			t.Errorf("read a file inside the run directory while removing it: %s", request)
		case strings.Contains(request, "rmdir /s /q "+fakeRunDir):
			rmdirs++
		case strings.Contains(request, " del "):
			deletes++
		}
	}
	if rmdirs != 2 || deletes != 1 {
		t.Fatalf("sent %d rmdir and %d del requests; want 2 and 1", rmdirs, deletes)
	}
}

func TestRemoveGuestDirReportsADirectoryThatStays(t *testing.T) {
	shortWaits(t)
	g, state := fakeGuest(t, 1000, true)
	err := g.removeGuestDir()
	if err == nil || !strings.Contains(err.Error(), fakeRunDir+" is still there") {
		t.Fatalf("removeGuestDir = %v; want the remaining directory", err)
	}
	runRemoveCommand(t, g, err, state)
}

// runRemoveCommand는 경고의 마지막 줄을 그대로 복사해 Mac의 zsh에서 실행하고, utmctl이
// 남은 실행 디렉터리를 지우는 요청을 받았는지 확인한다.
func runRemoveCommand(t *testing.T, g guestTest, err error, state string) {
	t.Helper()
	lines := strings.Split(err.Error(), "\n")
	command := lines[len(lines)-1]
	if out, runErr := exec.Command("/bin/zsh", "-c", command).CombinedOutput(); runErr != nil {
		t.Fatalf("zsh could not run the last line of %q: %v\n%s", err, runErr, out)
	}
	got := requests(t, state)
	if want := "exec " + g.vm + " --cmd cmd.exe /d /c rmdir /s /q " + fakeRunDir; got[len(got)-1] != want {
		t.Fatalf("utmctl received %q from the last line of %q; want %q", got[len(got)-1], err, want)
	}
}

func TestRemoveGuestDirReportsAnUnansweredRemoval(t *testing.T) {
	shortWaits(t)
	g, state := fakeGuest(t, 0, false)
	began := time.Now()
	err := g.removeGuestDir()
	if err == nil || !strings.Contains(err.Error(), "could not confirm") || !strings.Contains(err.Error(), fakeRunDir) {
		t.Fatalf("removeGuestDir = %v; want an unconfirmed removal of %s", err, fakeRunDir)
	}
	if elapsed := time.Since(began); elapsed > removeLimit+3*time.Second {
		t.Fatalf("removeGuestDir returned after %s", elapsed)
	}
	runRemoveCommand(t, g, err, state)
}

func TestReportKeepsACleanupFailureOutOfTheTestResult(t *testing.T) {
	cleanupErr := errors.New("guest run directory " + fakeRunDir + " is still there")
	var warning bytes.Buffer
	if err := report(&warning, nil, nil, cleanupErr); err != nil {
		t.Fatalf("report = %v; want success for passing tests whose run directory stayed", err)
	}
	if got := warning.String(); !strings.HasPrefix(got, "Windows E2E cleanup: ") || !strings.Contains(got, fakeRunDir) {
		t.Fatalf("warning = %q; want a cleanup warning with the remaining directory", got)
	}
	testErr := errors.New("go test exited with 1")
	warning.Reset()
	if err := report(&warning, testErr, nil, cleanupErr); err == nil || strings.Contains(err.Error(), fakeRunDir) {
		t.Fatalf("report = %v; want only the test failure", err)
	}
	if !strings.Contains(warning.String(), fakeRunDir) {
		t.Fatalf("warning = %q; want the cleanup warning next to a test failure too", warning.String())
	}
}

// staleExecAnswer는 fakePreparingGuest의 exec가 돌려주는 오류에 들어가는 종료 상태다.
const staleExecAnswer = "exit status 13"

// fakePreparingGuest는 exec를 받을 때마다 앞서 실행한 다른 게스트 명령의 응답처럼 그
// 명령의 출력 BBB-4를 출력하고 13으로 끝나는 utmctl을 만든다. performs가 true면 exec는 응답과 관계없이
// mkdir로 실행 디렉터리를 만들거나, 게스트 프로그램처럼 실행 디렉터리 옆에 시작 표시
// 파일을 쓴다. 게스트 경로 C:\dw-e2e\<경로>는 state 디렉터리의 <경로>로 바꿔 다룬다.
// file push와 pull은 상위 디렉터리가 없거나 파일이 없으면 stderr에 오류를 출력하고
// 0으로 끝난다.
func fakePreparingGuest(t *testing.T, performs bool) guestTest {
	t.Helper()
	state := t.TempDir()
	script := `state='` + state + `'
for last; do :; done
local=$state/$(printf '%s' "$last" | sed -e 's|^C:\\dw-e2e\\||' | tr '\\' '/')
case "$1" in
exec)
	if [ ` + strconv.FormatBool(performs) + ` = true ]; then
		case "$*" in
		*mkdir*) mkdir -p "$local" ;;
		*guest.exe*) printf '%s' "${last##*\\}" > "$local.started" ;;
		esac
	fi
	echo 'BBB-4'
	exit 13 ;;
file)
	case "$2" in
	push) if [ -d "$(dirname "$local")" ]; then cat > "$local"; else echo "Error from event: failed to open file '$last'" >&2; fi ;;
	pull) if [ -f "$local" ]; then cat "$local"; else echo "Error from event: failed to open file '$last'" >&2; fi ;;
	esac ;;
esac
exit 0`
	shortWaits(t)
	return guestTest{
		runner: runner{utm: fakeUTMCtl(t, script), log: io.Discard},
		vm:     "95B99283-8256-4360-AA1D-0563FCD5CE09",
		id:     fakeRunID,
		dir:    fakeRunDir,
	}
}

func TestCreateGuestDirTrustsTheReadBackOverTheExecAnswer(t *testing.T) {
	g := fakePreparingGuest(t, true)
	if err := g.createGuestDir([]byte(`{"go":"go"}`)); err != nil {
		t.Fatalf("createGuestDir = %v; want success because the request was read back", err)
	}
}

func TestCreateGuestDirReportsADirectoryThatWasNotCreated(t *testing.T) {
	g := fakePreparingGuest(t, false)
	err := g.createGuestDir([]byte(`{"go":"go"}`))
	if err == nil || !strings.Contains(err.Error(), "create guest run directory "+fakeRunDir) ||
		!strings.Contains(err.Error(), "failed to open file") || !strings.Contains(err.Error(), staleExecAnswer) {
		t.Fatalf("createGuestDir = %v; want the failed read-back and the exec answer", err)
	}
}

func TestStartGuestProgramTrustsTheStartedFileOverTheExecAnswer(t *testing.T) {
	g := fakePreparingGuest(t, true)
	if err := g.startGuestProgram(); err != nil {
		t.Fatalf("startGuestProgram = %v; want success because the guest program wrote its started file", err)
	}
}

func TestStartGuestProgramReportsAProgramThatDidNotStart(t *testing.T) {
	g := fakePreparingGuest(t, false)
	err := g.startGuestProgram()
	if err == nil || !strings.Contains(err.Error(), "guest program did not start") || !strings.Contains(err.Error(), staleExecAnswer) {
		t.Fatalf("startGuestProgram = %v; want the missing started file and the exec answer", err)
	}
}

// otherRunID는 같은 VM에서 동시에 도는 다른 실행의 ID다.
const otherRunID = "20261105-172120-31f55126cb83b1ee"

// fakeCollectingGuest는 file pull을 받을 때마다 answers에서 그 파일 이름에 준비한 내용을
// 차례로 출력하고, 준비한 내용을 다 쓰면 마지막 내용을 계속 출력하는 utmctl을 만든다.
// 동시에 도는 다른 실행의 file pull과 응답이 섞여, 앞선 응답이 다른 실행의 파일 내용인
// 상황을 흉내 낸다. 내용을 준비하지 않은 파일은 stderr에 오류를 출력하고 0으로 끝난다.
func fakeCollectingGuest(t *testing.T, answers map[string][]string) guestTest {
	t.Helper()
	state := t.TempDir()
	for name, contents := range answers {
		for i, content := range contents {
			if err := os.WriteFile(filepath.Join(state, name+"."+strconv.Itoa(i+1)), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(state, name+".answers"), []byte(strconv.Itoa(len(contents))), 0644); err != nil {
			t.Fatal(err)
		}
	}
	script := `state='` + state + `'
for last; do :; done
name=${last##*\\}
n=$(cat "$state/$name.count" 2>/dev/null || echo 0); n=$((n + 1)); echo $n > "$state/$name.count"
k=$(cat "$state/$name.answers" 2>/dev/null || echo 0)
[ $n -le $k ] || n=$k
if [ -f "$state/$name.$n" ]; then cat "$state/$name.$n"; else echo "Error from event: failed to open file '$last'" >&2; fi
exit 0`
	return guestTest{
		runner: runner{utm: fakeUTMCtl(t, script), log: io.Discard},
		vm:     "95B99283-8256-4360-AA1D-0563FCD5CE09",
		local:  t.TempDir(),
		id:     fakeRunID,
		dir:    fakeRunDir,
	}
}

func encodeResult(t *testing.T, result guestrun.Result) string {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAwaitResultWaitsForItsOwnResultPastOtherContent(t *testing.T) {
	shortRetryInterval(t)
	own := encodeResult(t, guestrun.Result{RunID: fakeRunID, ExitCode: 1})
	g := fakeCollectingGuest(t, map[string][]string{guestrun.ResultFile: {
		encodeResult(t, guestrun.Result{RunID: otherRunID}),
		own[:len(own)/2],
		own,
	}})
	result, data, err := g.awaitResult()
	if err != nil || result.RunID != g.id || result.ExitCode != 1 || string(data) != own {
		t.Fatalf("awaitResult = %+v, %q, %v; want the result of this run", result, data, err)
	}
}

func TestAwaitResultReportsTheResultItRejected(t *testing.T) {
	shortWaits(t)
	g := fakeCollectingGuest(t, map[string][]string{guestrun.ResultFile: {encodeResult(t, guestrun.Result{RunID: otherRunID})}})
	_, _, err := g.awaitResult()
	if err == nil || !strings.Contains(err.Error(), "no guest result") || !strings.Contains(err.Error(), otherRunID) {
		t.Fatalf("awaitResult = %v; want a missing result that names the rejected run", err)
	}
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func diagnosticsArchive(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cleanup.log"), []byte("own cleanup"), 0644); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := guestrun.ArchiveDir(&archive, dir); err != nil {
		t.Fatal(err)
	}
	return archive.String()
}

func TestCollectReadsAFileAgainUntilItMatchesTheResult(t *testing.T) {
	shortRetryInterval(t)
	diagnostics := diagnosticsArchive(t)
	g := fakeCollectingGuest(t, map[string][]string{
		guestrun.StdoutFile:      {"other run stdout", "own stdout"},
		guestrun.StderrFile:      {""},
		guestrun.DiagnosticsFile: {"own std", diagnostics},
	})
	err := g.collect(nil, map[string]string{
		guestrun.StdoutFile:      sha256Hex("own stdout"),
		guestrun.StderrFile:      sha256Hex(""),
		guestrun.DiagnosticsFile: sha256Hex(diagnostics),
	})
	if err != nil {
		t.Fatalf("collect = %v; want the files of this run", err)
	}
	if got, err := os.ReadFile(filepath.Join(g.local, guestrun.StdoutFile)); err != nil || string(got) != "own stdout" {
		t.Fatalf("stdout.log = %q, %v; want the stdout of this run", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(g.local, "diagnostics", "cleanup.log")); err != nil || string(got) != "own cleanup" {
		t.Fatalf("diagnostics/cleanup.log = %q, %v; want the diagnostics of this run", got, err)
	}
}

func TestCollectReportsAFileThatNeverMatchesTheResult(t *testing.T) {
	shortRetryInterval(t)
	g := fakeCollectingGuest(t, map[string][]string{
		guestrun.StdoutFile:      {"other run stdout"},
		guestrun.StderrFile:      {""},
		guestrun.DiagnosticsFile: {diagnosticsArchive(t)},
	})
	err := g.collect(nil, map[string]string{guestrun.StdoutFile: sha256Hex("own stdout")})
	if err == nil || !strings.Contains(err.Error(), guestrun.StdoutFile) || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("collect = %v; want stdout.log reported as not matching the result", err)
	}
}

func TestArchiveSourceLeavesOutSymlinksSoTheGuestCanExtractIt(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "skills", "SKILL.md"), []byte("skill"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../.claude/skills", filepath.Join(root, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}
	var source bytes.Buffer
	if err := archiveSource(&source, root, []string{".agents/skills", ".claude/skills/SKILL.md", ""}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := guestrun.Extract(&source, dir); err != nil {
		t.Fatalf("Extract = %v; want the source to extract without the symlink", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "src", ".claude", "skills", "SKILL.md")); err != nil || string(got) != "skill" {
		t.Fatalf("SKILL.md = %q, %v; want the regular file", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src", ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatalf("symlink entry was extracted: %v", err)
	}
}
