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
	"strings"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/e2e/windows/guestrun"
	"github.com/seungyeop-lee/directory-watcher/v2/e2e/worktree"
)

// test runs the scenarios in a VM that up has prepared. It never opens UTM
// or starts the VM: it checks UTM's process before any utmctl request,
// because a request would launch UTM.
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

// guestTest is one run: its ID names both the guest run directory and the
// result the runner accepts.
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
	if _, err := g.utm.call(requestLimit, nil, "exec", g.vm, "--cmd", g.file("guest.exe"), g.dir); err != nil {
		return fmt.Errorf("start guest program: %w", err)
	}
	if err := g.awaitStarted(); err != nil {
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
	return errors.Join(testErr, collectErr, cleanupErr)
}

func (g guestTest) file(name string) string { return g.dir + `\` + name }

// send builds the guest program, archives the working tree, and pushes both
// with the request into a new guest run directory.
func (g guestTest) send() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if root, err = worktree.Root(root); err != nil {
		return err
	}
	// The guest program is built for amd64 so that one binary runs on both
	// Windows x64 and ARM64; Windows 11 on ARM64 runs it under emulation.
	// The Guest Agent itself ran as an x64 process in the ARM64 guest of the
	// 2026-11-03 experiment (PROCESSOR_ARCHITECTURE=AMD64 in its commands).
	// The tests still run with the guest's own Go toolchain and architecture.
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

// recordSource keeps on the Mac which source and test scope this run used.
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

// createGuestDir creates the run directory with mkdir through exec and
// confirms it by reading the request back. exec does not wait for the
// command, and push and pull exit with 0 on failure, so reading back is the
// only confirmation that the directory exists and file transfer works.
func (g guestTest) createGuestDir(request []byte) error {
	if _, err := g.utm.call(requestLimit, nil, "exec", g.vm, "--cmd", "cmd.exe", "/d", "/c", "mkdir", g.dir); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
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
			return fmt.Errorf("create guest run directory %s: %w", g.dir, err)
		}
		time.Sleep(time.Second)
	}
}

// push allows 30 seconds plus 128 KiB per second; pushes ran at about
// 570 KB per second in the experiment.
func (g guestTest) push(name string, data []byte) error {
	limit := 30*time.Second + time.Duration(len(data)/(128<<10))*time.Second
	_, err := g.utm.call(limit, bytes.NewReader(data), "file", "push", g.vm, g.file(name))
	return err
}

func (g guestTest) pull(name string, limit time.Duration) ([]byte, error) {
	return g.utm.call(limit, nil, "file", "pull", g.vm, g.file(name))
}

// awaitStarted confirms that exec actually started the guest program.
func (g guestTest) awaitStarted() error {
	deadline := time.Now().Add(time.Minute)
	for {
		data, err := g.pull(guestrun.StartedFile, requestLimit)
		if err == nil && string(data) == g.id {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("guest program did not start within 1m (%v); check that the Guest Agent can run %s", err, g.file("guest.exe"))
		}
		time.Sleep(time.Second)
	}
}

// awaitResult polls for the result the guest program writes last. Its
// deadline stops go test, so the result is due shortly after it. The result
// file is the only evidence of the outcome: utmctl exec returned 0 within
// 0.2 seconds on 2026-11-03 without waiting for the guest command or passing
// on its output or exit code, even for cmd.exe /c "exit 7". run accepts
// only a result that names this run's ID, so a result left by another run
// is not taken for this one.
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

// collect copies the result, if any, the logs, and the diagnostics to the
// Mac and prints the go test output.
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

// removeGuestDir deletes the guest run directory after its files reached the
// Mac, and confirms it by the request file becoming unreadable.
func (g guestTest) removeGuestDir() error {
	if _, err := g.utm.call(requestLimit, nil, "exec", g.vm, "--cmd", "cmd.exe", "/d", "/c", "rmdir", "/s", "/q", g.dir); err != nil {
		return fmt.Errorf("remove guest run directory: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := g.pull(guestrun.RequestFile, requestLimit); err != nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("guest run directory %s still exists 30s after rmdir", g.dir)
}
