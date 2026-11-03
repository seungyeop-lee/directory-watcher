//go:build windows

// Command guest는 Windows 게스트 안에서 E2E 실행 한 번을 수행한다. e2e/windows의
// Mac 실행기는 이 프로그램을 실행 디렉터리에 복사하고, 그 디렉터리를 유일한 인자로
// 주어 UTM Guest Agent로 시작한 뒤, package guestrun에 정의된 파일을 수집한다.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/seungyeop-lee/directory-watcher/v2/e2e/windows/guestrun"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: guest <run directory>")
		os.Exit(2)
	}
	os.Exit(run(os.Args[1]))
}

func run(dir string) int {
	result := guestrun.Result{RunID: filepath.Base(dir), ExitCode: -1, Started: time.Now()}
	// utmctl exec의 응답으로는 이 프로그램이 시작됐는지 알 수 없으므로(e2e/windows의
	// utm.exec 참고), 실행기에게는 이 파일이 Guest Agent가 실제로 이 프로그램을 시작했다는
	// 유일한 표시다.
	if err := os.WriteFile(filepath.Join(dir, guestrun.StartedFile), []byte(result.RunID), 0644); err != nil {
		return 1
	}
	var problems []string
	if err := test(dir, &result); err != nil {
		problems = append(problems, err.Error())
	}
	if err := archiveDiagnostics(dir); err != nil {
		problems = append(problems, "archive diagnostics: "+err.Error())
	}
	result.Error = strings.Join(problems, "; ")
	result.Finished = time.Now()
	if err := writeResult(dir, result); err != nil {
		return 1
	}
	return 0
}

func test(dir string, result *guestrun.Result) error {
	if err := os.MkdirAll(filepath.Join(dir, "diagnostics"), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, guestrun.RequestFile))
	if err != nil {
		return err
	}
	var req guestrun.Request
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	result.Guest = describe(dir, req.Go)
	if result.Guest.DriveType != "fixed" {
		return fmt.Errorf("run directory %s is on a %s drive; use a directory on the guest's internal disk", dir, result.Guest.DriveType)
	}
	if err := extractSource(dir, req.SourceSHA256); err != nil {
		return err
	}
	result.ExitCode, result.TimedOut, err = goTest(dir, req)
	result.Leftovers = stopLeftovers(dir)
	return err
}

func describe(dir, goPath string) guestrun.Guest {
	guest := guestrun.Guest{RunDir: dir, Windows: windowsVersion()}
	if out, err := exec.Command(goPath, "version").CombinedOutput(); err != nil {
		guest.GoVersion = fmt.Sprintf("%s: %v", goPath, err)
	} else {
		guest.GoVersion = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command(goPath, "env", "GOOS", "GOARCH").Output(); err == nil {
		guest.GoEnv = strings.Join(strings.Fields(string(out)), "/")
	}
	volume, err := windows.UTF16PtrFromString(filepath.VolumeName(dir) + `\`)
	if err != nil {
		guest.DriveType = "unknown"
		return guest
	}
	guest.DriveType = map[uint32]string{
		windows.DRIVE_REMOVABLE: "removable",
		windows.DRIVE_FIXED:     "fixed",
		windows.DRIVE_REMOTE:    "remote",
		windows.DRIVE_CDROM:     "cdrom",
		windows.DRIVE_RAMDISK:   "ramdisk",
	}[windows.GetDriveType(volume)]
	if guest.DriveType == "" {
		guest.DriveType = "unknown"
	}
	return guest
}

func windowsVersion() string {
	v := windows.RtlGetVersion()
	version := fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return version
	}
	defer key.Close()
	product, _, _ := key.GetStringValue("ProductName")
	// Windows 11에서도 ProductName은 "Windows 10"으로 읽히며, Windows 11의 빌드 번호는
	// 22000부터 시작한다. 2026-11-03에 빌드 26100인 게스트는 "Windows 10 Home"을 보고했다.
	if v.BuildNumber >= 22000 {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	display, _, _ := key.GetStringValue("DisplayVersion")
	if ubr, _, err := key.GetIntegerValue("UBR"); err == nil {
		version += "." + strconv.FormatUint(ubr, 10)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s (%s)", product, display, version))
}

// extractSource는 소스 아카이브를 풀기 전에 Mac에서 계산한 체크섬과 대조한다. utmctl
// file push는 전송에 실패해도 성공을 보고하기 때문이다.
func extractSource(dir, want string) error {
	data, err := os.ReadFile(filepath.Join(dir, guestrun.SourceFile))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("source checksum mismatch: got %s, want %s (%d bytes received)", got, want, len(data))
	}
	return guestrun.Extract(bytes.NewReader(data), dir)
}

func goTest(dir string, req guestrun.Request) (exitCode int, timedOut bool, err error) {
	tmp := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmp, 0755); err != nil {
		return -1, false, err
	}
	stdout, err := os.Create(filepath.Join(dir, guestrun.StdoutFile))
	if err != nil {
		return -1, false, err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(dir, guestrun.StderrFile))
	if err != nil {
		return -1, false, err
	}
	defer stderr.Close()
	cmd := exec.Command(req.Go, req.Args...)
	cmd.Dir = filepath.Join(dir, "src")
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// 하네스는 PATH에 있는 "go"로 빌드하는데, Guest Agent의 PATH에는 Go가 없다. TEMP를
	// 이 실행 디렉터리 안에 두면 테스트 바이너리와 실행 디렉터리도 그 안에 생기므로,
	// 남은 프로세스를 경로로 찾을 수 있다.
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(req.Go)+";"+os.Getenv("PATH"),
		"TEMP="+tmp, "TMP="+tmp,
		"DW_E2E_DIAGNOSTICS="+filepath.Join(dir, "diagnostics"))
	cmd.Env = append(cmd.Env, req.Env...)
	if err := cmd.Start(); err != nil {
		return -1, false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			return -1, false, err
		}
		return cmd.ProcessState.ExitCode(), false, nil
	case <-time.After(req.Deadline):
		// taskkill /T는 go에서부터 부모 PID를 따라가므로 이번 실행의 프로세스
		// 트리만 멈춘다. 부모가 이미 끝난 고아 프로세스는 stopLeftovers가
		// 처리한다.
		killErr := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		<-done
		if killErr != nil {
			return -1, true, fmt.Errorf("stop go after the deadline: %w", killErr)
		}
		return -1, true, nil
	}
}

// stopLeftovers는 실행 파일이 dir 안에 있는 프로세스 가운데 이 프로그램을 뺀 나머지를
// 종료하고 알린다. dir 안에는 이번 실행의 테스트 바이너리와, 그 바이너리가 빌드한
// watcher와 hookrec 바이너리만 있다.
func stopLeftovers(dir string) []string {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []string{"process snapshot: " + err.Error()}
	}
	defer windows.CloseHandle(snapshot)
	prefix := strings.ToLower(filepath.Clean(dir)) + `\`
	self := uint32(os.Getpid())
	var leftovers []string
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == self {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, entry.ProcessID)
		if err != nil {
			continue
		}
		path := make([]uint16, windows.MAX_LONG_PATH)
		size := uint32(len(path))
		if windows.QueryFullProcessImageName(handle, 0, &path[0], &size) == nil {
			image := windows.UTF16ToString(path[:size])
			if strings.HasPrefix(strings.ToLower(image), prefix) {
				leftover := fmt.Sprintf("pid=%d %s", entry.ProcessID, image)
				if err := windows.TerminateProcess(handle, 1); err != nil {
					leftover += " (terminate: " + err.Error() + ")"
				}
				leftovers = append(leftovers, leftover)
			}
		}
		windows.CloseHandle(handle)
	}
	return leftovers
}

func archiveDiagnostics(dir string) error {
	file, err := os.Create(filepath.Join(dir, guestrun.DiagnosticsFile))
	if err != nil {
		return err
	}
	archiveErr := guestrun.ArchiveDir(file, filepath.Join(dir, "diagnostics"))
	if err := file.Close(); archiveErr == nil {
		archiveErr = err
	}
	return archiveErr
}

// writeResult는 완성된 파일을 rename으로 제자리에 옮겨, 결과가 나올 때까지 반복해서
// 확인하는 실행기가 일부만 쓰인 파일을 읽지 않게 한다.
func writeResult(dir string, result guestrun.Result) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	partial := filepath.Join(dir, guestrun.ResultFile+".partial")
	if err := os.WriteFile(partial, data, 0644); err != nil {
		return err
	}
	return os.Rename(partial, filepath.Join(dir, guestrun.ResultFile))
}
