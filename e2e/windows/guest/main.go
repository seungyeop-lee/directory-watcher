//go:build windows

// Command guest performs one E2E run inside the Windows guest. The Mac runner
// in e2e/windows copies it into the run directory, starts it through the UTM
// Guest Agent with that directory as its only argument, and collects the files
// named in package guestrun.
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
	// utmctl exec returns 0 without waiting, so this file is the only sign
	// for the runner that the Guest Agent actually started this program.
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
	// ProductName still reads "Windows 10" on Windows 11, whose builds start
	// at 22000; the 2026-11-03 guest of build 26100 reported "Windows 10 Home".
	if v.BuildNumber >= 22000 {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	display, _, _ := key.GetStringValue("DisplayVersion")
	if ubr, _, err := key.GetIntegerValue("UBR"); err == nil {
		version += "." + strconv.FormatUint(ubr, 10)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s (%s)", product, display, version))
}

// extractSource checks the source archive against the checksum computed on
// the Mac before extracting it, because utmctl file push reports success even
// when the transfer fails.
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
	// The harness builds with "go" from PATH, and the Guest Agent's PATH does
	// not include Go. TEMP keeps the test binaries and run directories inside
	// this run directory, so leftover processes can be found by path.
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
		// taskkill /T follows parent PIDs from go, so it stops only this
		// run's tree. Orphans whose parent already exited are left to
		// stopLeftovers.
		killErr := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		<-done
		if killErr != nil {
			return -1, true, fmt.Errorf("stop go after the deadline: %w", killErr)
		}
		return -1, true, nil
	}
}

// stopLeftovers terminates processes whose executable is inside dir, other
// than this program, and reports them. Only this run's test binaries and the
// watcher and hookrec binaries it built live there.
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

// writeResult renames a complete file into place so that the runner, which
// polls for the result, never reads a partly written one.
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
