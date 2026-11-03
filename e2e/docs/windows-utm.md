# Running E2E on a UTM Windows VM

🇺🇸 English | 🇰🇷 [한국어](windows-utm.ko.md)

This starts a Windows 11 ARM64 VM in UTM on a Mac, runs the `./e2e/scenarios` E2E inside the guest with the source of the current working tree, and brings the results back to the Mac. The runner is in `e2e/windows` and builds only on macOS.

## Prerequisites

You do the setup below only once. The runner does not change the VM configuration.

1. Install UTM at `/Applications/UTM.app`. If you installed it elsewhere, pass `-utmctl=<UTM.app>/Contents/MacOS/utmctl` when running. The runner rejects symbolic links such as Homebrew's `/opt/homebrew/bin/utmctl`.
2. Create a Windows 11 ARM64 VM in UTM and finish the Windows installation and license agreement yourself. This step is not automated.
3. Install [UTM Windows Guest Tools](https://docs.getutm.app/guest-support/windows/) in the guest. If `/Applications/UTM.app/Contents/MacOS/utmctl ip-address <VM>` prints an IPv4 address while the VM is running, the QEMU Guest Agent is working. If the screen shows `Display output is not active.` after installing Guest Tools, on the Mac where this project was verified, changing the DirectX Driver to DXMT in the VM's Display settings and restarting the VM normally fixed it.
4. Install Go with the MSI in the guest, at or above the version in `go.mod`. The runner uses the default install path `C:\Program Files\Go\bin\go.exe`; pass a different path with `-guest-go`. You do not need to change the PowerShell execution policy.
5. Create a clone without display and sound devices by following [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm) below.

### Creating a dedicated E2E VM

**A VM that still has a display device shows its VM window on screen even when started with `utmctl start --hide`.** `--hide` applies only to the UTM main window, so run the E2E on a clone with the display device removed. The original VM is left unchanged. Clone and change settings while both VMs are shut down.

1. Clone the original VM. The Guest Tools and Go installed in the guest are cloned as well.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl clone <original VM name or UUID> --name Windows-E2E
   ```

2. Empty the clone's display list. Remove the Display device in the UTM settings screen, or run the following AppleScript.

   ```sh
   osascript -e 'tell application "UTM"
   set vm to virtual machine named "Windows-E2E"
   set config to configuration of vm
   set displays of config to {}
   update configuration of vm with config
   end tell'
   ```

3. Remove the clone's sound device in the UTM settings screen. If you leave it, Windows plays its boot sound even when started headless. UTM scripting has no sound setting, so the runner cannot check this item; the check in step 4 below covers it instead.
4. Start the clone with `make e2e-windows-up ARGS=-vm=Windows-E2E` and check it. If a display device remains, this command does not start the VM and fails, pointing to this procedure. Once the VM is running, the output of the following command must contain `-vga none`, and the `-device` list must contain no sound device.

   ```sh
   ps -ww -o args= -p "$(pgrep -f qemu-aarch64-softmmu)" | tr ' ' '\n' | grep -A1 -E '^-(vga|device|audio)$'
   ```

## Choosing the VM

Set the full name or UUID shown by `utmctl list` in the `DW_E2E_UTM_VM` environment variable or in `ARGS="-vm=<VM>"`. Without it, every command ends with exit code 2.

```sh
export DW_E2E_UTM_VM=Windows-E2E
```

## Run commands

Starting and shutting down the VM takes time, so when testing several times, run `up` → `e2e-windows` (repeated) → `down` in that order. For a single run, use `make e2e-windows-all`.

| Command | Behavior |
|---|---|
| `make e2e-windows-up` | Opens UTM if it is not running, starts the VM if it is off, then waits until the Guest Agent responds. For a VM that is already running, it only checks readiness. |
| `make e2e-windows` | Sends the source, runs the tests, and collects the results on the running VM. It does not start or stop UTM or the VM, and fails with `run make e2e-windows-up first` if the VM is not ready. |
| `make e2e-windows-down` | Requests a graceful shutdown of the VM and waits until it is off, then quits UTM if no VM is running. |
| `make e2e-windows-all` | Runs up, test, and down in order, and stops only the VM it started and the UTM it opened. A VM and UTM that were already running are left as they are. |

**`make e2e-windows-down` quits UTM whenever no VM is running, regardless of who opened UTM.** A UTM you opened yourself is closed too. If another VM is running, UTM and that VM keep running.

The runner opens UTM with `open -g` and closes the UTM main window right away, and no VM window appears because the VM has no display device. Do not open or click the UTM window yourself during a run ([UTM crashes](#utm-crashes)).

On success, the go test output shows `PASS` for each test and the command exits with 0. Test failures, timeouts, communication failures, and missing results all end with a nonzero exit code. Through make, the exit code is 2. Failing to remove the guest run directory does not change the exit code and is reported only as a warning ([Guest run directory is left behind](#guest-run-directory-is-left-behind)).

### Test selection, repetition, and time limits

In `ARGS`, write the runner options first and the go test arguments after `--`. The guest runs `go test -count=1 -v <passed arguments> ./e2e/scenarios`, so a `-count` you append replaces the default. If you add package paths after `--`, the tests in those packages also run along with `./e2e/scenarios`. For example, `make e2e-windows-all ARGS="-vm=Windows-E2E -- ./internal/app/service"` runs `go test -count=1 -v ./internal/app/service ./e2e/scenarios` in the guest. You cannot leave out `./e2e/scenarios`.

```sh
make e2e-windows ARGS="-deadline=10m -- -count=3 -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$'"
DW_E2E_FAULT=failure make e2e-windows ARGS="-- -timeout=2m -run '^TestProcessLifecycle$'"
```

`DW_E2E_FAULT` set on the Mac is passed to the guest, and the second command above is expected to fail. When you pass a Windows path as a value, wrap it in single quotes as in `ARGS="-guest-dir='D:\dw-e2e'"`. Without the quotes, the shell that make runs strips the `\` and the path becomes `D:dw-e2e`.

| Option | Default | Meaning |
|---|---|---|
| `-vm` | `$DW_E2E_UTM_VM` | VM name or UUID |
| `-utmctl` | `/Applications/UTM.app/Contents/MacOS/utmctl` | utmctl inside UTM.app |
| `-guest-dir` | `C:\dw-e2e` | Parent path on the guest's internal disk where per-run directories are created |
| `-guest-go` | `C:\Program Files\Go\bin\go.exe` | Go in the guest |
| `-deadline` | `20m` | Time limit for the guest go test. When exceeded, go and its child processes are terminated and the run fails. |
| `-ready-timeout` | `5m` | Time to wait for the Guest Agent to respond after startup |
| `-stop-timeout` | `3m` | Time to wait for the VM to shut down after the shutdown request |

`up` and `down` do not accept go test arguments after `--`.

## Checking results

`Windows diagnostics: <Mac path>` is printed at the start and end of the command. The following files are left in this directory as UTF-8 text. The `guest.exe` left alongside them is the executable sent to the guest.

| File | Contents |
|---|---|
| `runner.log` | Runner progress log |
| `source.txt` | Commit, size and SHA-256 of the source tar that was sent, go test arguments, list of uncommitted changes |
| `request.json`, `result.json` | Request sent to the guest and the guest's result. The result contains the run ID, go exit code, whether it timed out, leftover processes, Windows and Go versions, and the run directory and its drive type. |
| `stdout.log`, `stderr.log` | Output of the guest go test |
| `diagnostics/` | Guest E2E diagnostics files. Analyze `cleanup.log` and `records.log.processes` as in [Windows failure analysis](windows.md#failure-analysis). |

The runner judges a run successful only when the result's run ID matches this run, there are no errors, timeouts, or leftover processes, the go exit code is 0, and the run directory is on an internal disk (`fixed`). Uncommitted modifications, additions, and deletions in the working tree are sent as they are, and files excluded by `.gitignore` are not sent.

The guest's per-run directory `<guest-dir>\<run ID>` is deleted once collection finishes. If collection fails, it is left in place and the error message shows its path. If the directory cannot be removed after collection, the test result and the exit code stay as they are, and a warning starting with `Windows E2E cleanup:` shows the remaining path and the command that removes that directory.

Tests against the same VM from the same Mac can run at the same time, and each uses a different run directory. While a test is running, both a VM start by `up` and `down` are refused with `another Windows E2E run on this Mac is using the VM`. Runs from other user accounts or other Macs are not coordinated.

## Shutdown behavior

`down` and `all` request a graceful shutdown from Windows with `utmctl stop --request`, and repeat the request every 30 seconds if the VM does not shut down. They never force the VM's power off; if the VM does not shut down within `-stop-timeout`, they fail and leave the VM running.

Every request checked on 2026-11-03 was a real shutdown. The guest logged a power-off by winlogon (event 1074, reason 0x500ff) and a clean stop of the event log (6006), and the next boot was a cold boot (Kernel-Boot 27, boot type 0x0). Hibernation is off in this guest, so Fast Startup cannot be used. In some cases the first request was ignored and the VM shut down on the second request; the cause was not determined.

## Differences between Windows ARM64 and x64

The UTM VM runs Windows ARM64, and the guest Go is `windows/arm64`. Go's race detector supports only `windows/amd64` on Windows ([requirements](https://go.dev/doc/articles/race_detector#Requirements)), so `-race` verification is not possible on this VM. `-race` verification and Windows x64 behavior are still checked by `test (windows-latest)` in GitHub Actions ([CI steps](windows.md#ci-steps)). A passing UTM run does not substitute for a Windows x64 CI result.

## Troubleshooting

### start does not respond

`utmctl start` may keep failing in the same UTM process, ending after 2 minutes with `OSStatus error -1712` without starting the VM ([UTM #8006](https://github.com/utmapp/UTM/issues/8006)). If every registered VM is off, `up` quits UTM gracefully once, relaunches it, and starts the VM again. If another VM is running, it fails without restarting UTM; shut that VM down, quit UTM, and run `up` again. `OSStatus error -10004` mixed into the output appears whether or not the start succeeded, so you can ignore it.

### UTM crashes

In one case, clicking the UTM window during a run made UTM exit with SIGSEGV, and the pending request ended with `OSStatus error -609`. Do not open or click the UTM window during a run. Crash reports are in `~/Library/Logs/DiagnosticReports/UTM-*.ips`.

### QEMU aborts during shutdown

If a new `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips` appears after the shutdown request, `down` and `all` fail with `QEMU aborted while the VM was stopping` even if the VM shut down. On the original VM with a display, 7 of 11 shutdowns ended this way, and on the dedicated VM 0 of 18 did (2026-11-03). The cause is on UTM's QEMU side and has not been determined. The report does not identify the VM, so a QEMU abort of another VM at the same time is also judged a failure.

### Guest run directory is left behind

`Windows E2E cleanup: guest run directory <path> is still there after ...` means the directory remained although the runner repeated `rmdir` for 1 minute. The cause is a guest process that keeps a file in the directory open or uses a directory inside it as its current directory. On 2026-11-03, a `hookrec.exe` still running after the tests ended held `src\e2e\scenarios`. `could not confirm that guest run directory <path> was removed` means the guest did not report the removal result; check whether the directory remains with step 4 below.

The output and exit code that `utmctl exec` returns may not be those of the guest command, so the commands below write their results to a file in the guest and read it with `utmctl file pull`. `C:\dw-e2e` in the commands is the default `-guest-dir`.

1. Check the guest's process list. Look for the `directory-watcher.exe` and `hookrec.exe` that the E2E builds and runs, and for `go.exe` and `*.test.exe`. The output is encoded in the guest's code page.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c 'tasklist > C:\dw-e2e\tasks.txt'
   /Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> 'C:\dw-e2e\tasks.txt'
   ```

2. End the remaining processes.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c taskkill /f /pid <PID>
   ```

The directories of other tests running on the same VM at the same time are also under `C:\dw-e2e`. Remove only the path shown in the warning.

3. Run the command shown in the warning to remove the directory.
4. If the run ID from the warning is not in the listing of `C:\dw-e2e`, the directory has been removed.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c 'dir /b C:\dw-e2e > C:\dw-e2e\list.txt'
   /Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> 'C:\dw-e2e\list.txt'
   ```

### The VM window appears or the boot sound plays

`up` refuses a VM that still has a display device. If the VM starts without a window but makes a sound, a sound device remains; follow step 3 of [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm).
