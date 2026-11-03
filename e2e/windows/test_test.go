//go:build darwin

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fakeRunDir = `C:\dw-e2e\20261103-223804-8cbc351a9c24e6c8`

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

func shortRemoveLimit(t *testing.T) {
	t.Helper()
	limit := removeLimit
	t.Cleanup(func() { removeLimit = limit })
	removeLimit = 4 * time.Second
}

func TestRemoveGuestDirRetriesWithoutReadingInsideTheRunDirectory(t *testing.T) {
	shortRemoveLimit(t)
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
	shortRemoveLimit(t)
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
	shortRemoveLimit(t)
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
