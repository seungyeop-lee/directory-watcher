# Running E2E on a UTM Windows VM

🇺🇸 English | 🇰🇷 [한국어](windows-utm.ko.md)

- This starts a Windows 11 ARM64 VM in UTM on a Mac, runs the `./e2e/scenarios` E2E inside the guest with the source of the current working tree, and brings the results back to the Mac.
- The runner is in `e2e/windows` and builds only on macOS.

## Prerequisites

- The runner does not change the VM configuration.

You do the setup below only once.

1. Install UTM at `/Applications/UTM.app`. If you installed it elsewhere, pass `-utmctl=<UTM.app>/Contents/MacOS/utmctl` when running. The runner rejects symbolic links such as Homebrew's `/opt/homebrew/bin/utmctl`.
2. Create a Windows 11 ARM64 VM in UTM and finish the Windows installation and license agreement yourself. This step is not automated.
3. Install [UTM Windows Guest Tools](https://docs.getutm.app/guest-support/windows/) in the guest. If `/Applications/UTM.app/Contents/MacOS/utmctl ip-address <VM>` prints an IPv4 address while the VM is running, the QEMU Guest Agent is working. If the screen shows `Display output is not active.` after installing Guest Tools, change the DirectX Driver to DXMT in the VM's Display settings and restart the VM normally.
4. Install Go with the MSI in the guest, at or above the version in `go.mod`. The runner uses the default install path `C:\Program Files\Go\bin\go.exe`; pass a different path with `-guest-go`. You do not need to change the PowerShell execution policy.
5. Create a clone without display and sound devices by following [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm) below.

### Creating a dedicated E2E VM

- **A VM that still has a display device shows its VM window on screen even when started with `utmctl start --hide`.**
- `--hide` applies only to the UTM main window, so run the E2E on a clone with the display device removed.
- The original VM is left unchanged.

Clone and change settings while both VMs are shut down.

1. Clone the original VM. The Guest Tools and Go installed in the guest are cloned as well.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl clone <original VM name or UUID> --name Windows-E2E
   ```

2. Empty the clone's display list. Remove the Display device in the UTM settings screen, or run the following JXA command. You can run it even while another E2E run is using `utmctl` (see [the -600 error](#applescript-fails-with--600-while-utm-is-running)).

   ```sh
   osascript -l JavaScript -e 'Application("com.utmapp.UTM").virtualMachines.byName("Windows-E2E").updateConfiguration({with: {displays: []}})'
   ```

   - Pass only the items to change in `with`. Sending back the whole configuration read with `configuration()` in JXA fails with error -1700 because the drive and network items do not keep their types.

3. Remove the clone's sound device in the UTM settings screen. If you leave it, Windows plays its boot sound even when started headless. UTM scripting has no sound setting, so the runner cannot check this item; the check in step 4 below covers it instead.
4. Start the clone with `make e2e-windows-up ARGS=-vm=Windows-E2E` and check it. If a display device remains, this command does not start the VM and fails, pointing to this procedure. Once the VM is running, the output of the following command must contain `-vga none`, and the `-device` list must contain no sound device.

   ```sh
   ps -ww -o args= -p "$(pgrep -f qemu-aarch64-softmmu)" | tr ' ' '\n' | grep -A1 -E '^-(vga|device|audio)$'
   ```

## Choosing the VM

Set the full name or UUID shown by `utmctl list` in the `DW_E2E_UTM_VM` environment variable or in `ARGS="-vm=<VM>"`.

```sh
export DW_E2E_UTM_VM=Windows-E2E
```

- Without it, every command ends with exit code 2.

## Run commands

- Starting and shutting down the VM takes time, so when testing several times, run `up` → `e2e-windows` (repeated) → `down` in that order.
- For a single run, use `make e2e-windows-all`.

| Command | Behavior |
|---|---|
| `make e2e-windows-up` | Opens UTM if it is not running, starts the VM if it is off, then waits until the Guest Agent responds. For a VM that is already running, it only checks readiness. |
| `make e2e-windows` | Sends the source, runs the tests, and collects the results on the running VM. It does not start or stop UTM or the VM, and fails with `run make e2e-windows-up first` if the VM is not ready. |
| `make e2e-windows-down` | Requests a graceful shutdown of the VM and waits until it is off, then quits UTM if no VM is running. |
| `make e2e-windows-all` | Runs up, test, and down in order, and stops only the VM it started and the UTM it opened. A VM and UTM that were already running are left as they are. |

- **`make e2e-windows-down` quits UTM whenever no VM is running, regardless of who opened UTM.**
- A UTM you opened yourself is closed too.
- If another VM is running, UTM and that VM keep running.
- The runner opens UTM with `open -g` and closes the UTM main window right away, and no VM window appears because the VM has no display device.
- Do not open or click the UTM window yourself during a run ([UTM crashes](#utm-crashes)).
- On success, the go test output shows `PASS` for each test and the command exits with 0.
- Test failures, timeouts, communication failures, and missing results all end with a nonzero exit code.
- Through make, the exit code is 2.
- Failing to remove the guest run directory does not change the exit code and is reported only as a warning ([Guest run directory is left behind](#guest-run-directory-is-left-behind)).

### Test selection, repetition, and time limits

- In `ARGS`, write the runner options first and the go test arguments after `--`.
- The guest runs `go test -count=1 -v <passed arguments> ./e2e/scenarios`, so a `-count` you append replaces the default.
- If you add package paths after `--`, the tests in those packages also run along with `./e2e/scenarios`.
- For example, `make e2e-windows-all ARGS="-vm=Windows-E2E -- ./internal/app/service"` runs `go test -count=1 -v ./internal/app/service ./e2e/scenarios` in the guest.
- You cannot leave out `./e2e/scenarios`.

```sh
make e2e-windows ARGS="-deadline=10m -- -count=3 -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$'"
DW_E2E_FAULT=failure make e2e-windows ARGS="-- -timeout=2m -run '^TestProcessLifecycle$'"
```

- `DW_E2E_FAULT` set on the Mac is passed to the guest, and the second command above is expected to fail.
- When you pass a Windows path as a value, wrap it in single quotes as in `ARGS="-guest-dir='D:\dw-e2e'"`.
- Without the quotes, the shell that make runs strips the `\` and the path becomes `D:dw-e2e`.

| Option | Default | Meaning |
|---|---|---|
| `-vm` | `$DW_E2E_UTM_VM` | VM name or UUID |
| `-utmctl` | `/Applications/UTM.app/Contents/MacOS/utmctl` | utmctl inside UTM.app |
| `-guest-dir` | `C:\dw-e2e` | Parent path on the guest's internal disk where per-run directories are created |
| `-guest-go` | `C:\Program Files\Go\bin\go.exe` | Go in the guest |
| `-deadline` | `20m` | Time limit for the guest go test. When exceeded, go and its child processes are terminated and the run fails. |
| `-ready-timeout` | `5m` | Time to wait for the Guest Agent to respond after startup |
| `-stop-timeout` | `3m` | Time to wait for the VM to shut down after the shutdown request |

- `up` and `down` do not accept go test arguments after `--`.

### Concurrent runs

- Tests against the same VM from the same Mac can run at the same time, and each uses a different run directory.
- While a test is running, both a VM start by `up` and `down` are refused with `another Windows E2E run on this Mac is using the VM`.
- Runs from other user accounts or other Macs are not coordinated.

## Checking results

- `Windows diagnostics: <Mac path>` is printed at the start and end of the command.
- The `guest.exe` left in this directory is the executable sent to the guest.

The following files are left in this directory as UTF-8 text.

| File | Contents |
|---|---|
| `runner.log` | Runner progress log |
| `source.txt` | Commit, size and SHA-256 of the source tar that was sent, go test arguments, list of uncommitted changes |
| `request.json`, `result.json` | Request sent to the guest and the guest's result. The request contains the run ID and the go test arguments, and the result contains guest-side errors, the go exit code, whether it timed out, leftover processes, Windows and Go versions, and the run directory and its drive type. |
| `stdout.log`, `stderr.log` | Output of the guest go test |
| `diagnostics/` | Guest E2E diagnostics files. Analyze `cleanup.log` and `records.log.processes` as in [Windows failure analysis](windows.md#failure-analysis), and `dll-init-failed.log` as in [A hook exits with 0xc0000142](#a-hook-exits-with-0xc0000142). |

- The runner judges a run successful only when the result has no guest-side error, timeout, or leftover process, the go exit code is 0, and the run directory is on an internal disk (`fixed`).
- When a run failed without a guest result, its `stdout.log`, `stderr.log`, and `diagnostics/` may hold the contents of another run on the same VM.
- The runner sends [the same working tree files as the Linux runner](diagnostics.md#parallel-runs-across-worktrees-and-isolation), except symbolic links such as `.agents/skills`.
- The guest's run directory `<guest-dir>\<run ID>` and the marker files `<guest-dir>\<run ID>.*` next to it are deleted once collection finishes.
- If collection fails, the run directory is left in place and the error message shows its path.

## Shutdown behavior

- `down` and `all` request a graceful shutdown from Windows with `utmctl stop --request`, and repeat the request every 30 seconds if the VM does not shut down.
  - Windows sometimes ignores the first request and shuts down on a repeated one; the cause has not been determined.
- They never force the VM's power off; if the VM does not shut down within `-stop-timeout`, they fail and leave the VM running.

## Differences between Windows ARM64 and x64

- The UTM VM runs Windows ARM64, and the guest Go is `windows/arm64`.
- Go's race detector supports only `windows/amd64` on Windows ([requirements](https://go.dev/doc/articles/race_detector#Requirements)), so `-race` verification is not possible on this VM.
- `-race` verification and Windows x64 behavior are checked by `test (windows-latest)` in GitHub Actions ([Checking CI results](windows.md#checking-ci-results)).
- A passing UTM run does not substitute for a Windows x64 CI result.

## Troubleshooting

### start does not respond

- `utmctl start` may keep failing in the same UTM process, ending after 2 minutes with `OSStatus error -1712` without starting the VM ([UTM #8006](https://github.com/utmapp/UTM/issues/8006)).
- If every registered VM is off, `up` quits UTM gracefully once, relaunches it, and starts the VM again.
- If another VM is running, `up` fails without restarting UTM; shut that VM down, quit UTM, and run `up` again.
- `OSStatus error -10004` mixed into the output appears whether or not the start succeeded, so you can ignore it.

### UTM crashes

- If a pending request ended with `OSStatus error -609` and UTM exited, clicking the UTM window during the run may have made UTM exit with SIGSEGV.
- Crash reports are in `~/Library/Logs/DiagnosticReports/UTM-*.ips`.

### AppleScript fails with -600 while UTM is running

- While any `utmctl` process is running, an AppleScript `tell application "UTM"` sends its events to that `utmctl` process instead of UTM and fails at once with `Application isn't running. (-600)`.
  - Pointing to UTM by bundle ID or app path gives the same result.
  - `utmctl` requests from other runs on the same VM, such as `file push`, last for seconds, so concurrent runs hit this often.
- The runner and the command in step 2 of [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm) send their events to UTM with JXA (`osascript -l JavaScript`), whose events reach UTM even while `utmctl` is running.
- When you send AppleScript to UTM yourself, run it when `pgrep -x utmctl` prints nothing.
- Setting `AEDebugSends=1` before `osascript` prints the target of each event; `target='psn '[utmctl]` means the event went to `utmctl`.

### QEMU aborts during shutdown

- If a new `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips` appears after the shutdown request, `down` and `all` fail with `QEMU aborted while the VM was stopping` even if the VM shut down.
- These aborts happen mostly on a VM with a display, and their cause on UTM's QEMU side has not been determined.
- The report does not identify the VM, so a QEMU abort of another VM at the same time is also judged a failure.

### Guest run directory is left behind

- When the runner cannot remove the guest run directory after collection, it prints a warning starting with `Windows E2E cleanup:`.
  - The last line of the warning holds only the command that removes that directory.
  - `guest run directory <path> is still there after ...` means the directory remained although the runner repeated `rmdir` for 1 minute.
  - `could not confirm that guest run directory <path> was removed` means the runner could not read a removal result within 1 minute; check whether the directory actually remains with step 4 below.
- The directory remains for one of three causes, and the warning alone does not tell which one, so check them in turn with the procedure below.
- **The directories of other tests running on the same VM at the same time are also under `C:\dw-e2e`, so remove only the path shown in the warning.**

Run each guest command in the procedure below with the following three lines in a single Mac shell (zsh or bash), putting the command of the step in `<guest command>`. `C:\dw-e2e` is the default `-guest-dir`.

```sh
id=cleanup-$(openssl rand -hex 8); out='C:\dw-e2e\'$id.txt
/Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "(echo $id& <guest command> & echo end $id) > $out 2>&1"
/Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> "$out"
```

- The guest command writes its result to a guest file, and the first and last lines show whether the result is its own.
  - `utmctl exec` does not wait for the guest command to finish, and may return the output of another guest command.
  - `utmctl file pull` may also return, without an error, the contents of a file that another request opened.
- Read the pulled contents as the command's result only when the first line is `$id` and the last line is `end $id`.
  - If the pull prints `failed to open file` or either line differs, run only the pull again after a few seconds.
  - `utmctl file pull` exits with code 0 even when it cannot open the file, so do not judge by the exit code.
- The output is encoded in the guest's code page.
- To run another guest command, start again from the first line so that it uses a new `id`.

After checking the result, delete the result file with the following command, and repeat the delete command and the pull until the pull prints `failed to open file`.

```sh
/Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "del $out"
```

1. Look at the guest's process list with `tasklist`, and check whether the `directory-watcher.exe` and `hookrec.exe` that the E2E builds and runs, or `go.exe` and `*.test.exe`, remain. When such a process keeps a file in the directory open or uses a directory inside it as its current directory, the directory cannot be removed.
2. If processes remain, end them with `taskkill /f /pid <PID>` and check the result that `taskkill` wrote for each PID. For several PIDs, repeat `/pid <PID>`.
3. Copy the last line of the warning as it is and run it in the Mac shell.
4. If the run ID from the warning is not in the listing of `dir /b /ad C:\dw-e2e`, the directory has been removed.
5. If the directory remains although no process remains, Windows cannot delete `guest.exe` for several minutes after the guest program ended. Wait a few minutes, then continue again from step 3.
   - Running `rmdir /s /q <path>` in the way above then prints `guest.exe - Access is denied.` (`액세스가 거부되었습니다.` on a Korean guest).
   - The process that holds this file has not been identified.
6. If the directory still remains a few minutes later, the QEMU Guest Agent did not close a file in the directory that it opened for `utmctl file pull` or `file push`. Such a file is closed only when the Guest Agent process ends, so after the other runs on the same VM end, restart the VM with `make e2e-windows-down` and `make e2e-windows-up`, then continue again from step 3.

### The VM window appears or the boot sound plays

- `up` refuses a VM that still has a display device.
- If the VM starts without a window but makes a sound, a sound device remains; follow step 3 of [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm).

### A hook exits with 0xc0000142

- When the harness cleans up a scenario in which a process exited with exit code 0xc0000142 (STATUS_DLL_INIT_FAILED), it leaves a report.
  - The report starts with a `STATUS_DLL_INIT_FAILED (0xc0000142):` line in that test's output in `stdout.log`.
  - The same content is also left in `dll-init-failed.log` in the scenario's run directory under `diagnostics/`. The runner passes `DW_E2E_KEEP=1` to the guest, so this file is kept even when the test passes.
- The watcher runs hooks with `cmd.exe`, so the hook PID and times in the report belong to `cmd.exe`.
  - `cmd.exe` returns the exit code of `hookrec.exe` unchanged, and `hookrec.exe` belongs to the process group of `cmd.exe`.
- When `./e2e/harness` runs as well, `TestDLLInitFailureReportFromWatcher` makes a report with a hook that exits with 0xc0000142 on purpose. The report of that test is not a real failure.
- On this VM, processes that exited with 0xc0000142 leave nothing in the Windows event log, so the event log cannot find this failure.

The lines after the first line of the report are written in UTC time order, and each has one of the following two forms.

| Line | Meaning |
|---|---|
| `<time> exit pid=<PID> code=<exit code> started=<start time> from=<recorder>` | A process exited at `<time>`. Only processes that exited with 0xc0000142 and processes whose PID received CTRL+BREAK are listed. |
| `<time> CTRL+BREAK pid=<target PID> from=<sender>` | After the scenario started, CTRL+BREAK was sent in this test binary's console to the process group of `<target PID>`. `error=<error>` is appended when sending failed, and `target=failed-process` is appended when the target is the PID of a process that exited with 0xc0000142. |

- `from=harness` is a signal the harness sent to stop a scenario process, or the exit of a scenario process the harness waited for.
- `from=watcher:<PID>` is a record that watcher left in its DEBUG log. In `watcher.log` in the same run directory, these are the lines containing `sent CTRL+BREAK to process group` and `exited with code`.

Read the report in the following order.

1. If a CTRL+BREAK line with `target=failed-process` falls between `started` and the exit time of an `exit` line with 0xc0000142, that signal went to the process group while it was starting.
2. If an `exit` line for the same PID comes before a CTRL+BREAK line, that signal went to the PID of a process that had already exited. If another process group in this console received that PID in between, the signal goes to that group.
3. If neither case appears, this report gives no evidence that CTRL+BREAK caused the failure.

The following signals and exits are not in the report.

- A watcher run at a log level below DEBUG does not record CTRL+BREAK or exits.
  - This applies to scenarios run at the default ERROR level because `s.Launch` was given no `--log-level`.
  - In that case the report has only a `logged exit status 3221225794 without exit records` line, without times.
- CTRL+BREAK sent by the test binaries of other packages added after `--` is not included. Whether those binaries use the same console has not been checked.
- CTRL+BREAK sent by the watchers of other scenarios running in parallel is not included. Those signals are in the `watcher.log` of that scenario's run directory.
- CTRL+BREAK sent before the scenario started or after the scenario was cleaned up is not included.
