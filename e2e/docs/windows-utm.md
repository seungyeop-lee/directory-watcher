# Running E2E on a UTM Windows VM

🇺🇸 English | 🇰🇷 [한국어](windows-utm.ko.md)

- This starts a Windows 11 ARM64 VM in UTM on a Mac, runs the `./e2e/scenarios` E2E inside the guest with the source of the current working tree, and brings the results back to the Mac.
- The runner is in `e2e/windows` and builds only on macOS.

## Prerequisites

- The runner does not change the VM configuration.

You do the setup below only once.

1. Install UTM at `/Applications/UTM.app`. If you installed it elsewhere, pass `-utmctl=<UTM.app>/Contents/MacOS/utmctl` when running. The runner rejects symbolic links such as Homebrew's `/opt/homebrew/bin/utmctl`.
2. Create a Windows 11 ARM64 VM in UTM and finish the Windows installation and license agreement yourself. This step is not automated.
3. Install [UTM Windows Guest Tools](https://docs.getutm.app/guest-support/windows/) in the guest. If `/Applications/UTM.app/Contents/MacOS/utmctl ip-address <VM>` prints an IPv4 address while the VM is running, the QEMU Guest Agent is working. If the screen shows `Display output is not active.` after installing Guest Tools, on the Mac where this project was verified, changing the DirectX Driver to DXMT in the VM's Display settings and restarting the VM normally fixed it.
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

## Checking results

- `Windows diagnostics: <Mac path>` is printed at the start and end of the command.
- The `guest.exe` left in this directory is the executable sent to the guest.

The following files are left in this directory as UTF-8 text.

| File | Contents |
|---|---|
| `runner.log` | Runner progress log |
| `source.txt` | Commit, size and SHA-256 of the source tar that was sent, go test arguments, list of uncommitted changes |
| `request.json`, `result.json` | Request sent to the guest and the guest's result. The request contains the run ID, go test arguments, and the SHA-256 of `source.tar` and `guest.exe` that were sent with it, and the guest program checks the request and both files against it before `go test` and fails if they differ. The result contains the run ID, go exit code, whether it timed out, leftover processes, Windows and Go versions, the run directory and its drive type, and the SHA-256 of `stdout.log`, `stderr.log`, and `diagnostics.tar` in the guest. |
| `stdout.log`, `stderr.log` | Output of the guest go test |
| `diagnostics/` | Guest E2E diagnostics files. Analyze `cleanup.log` and `records.log.processes` as in [Windows failure analysis](windows.md#failure-analysis), and `dll-init-failed.log` as in [A hook exits with 0xc0000142](#a-hook-exits-with-0xc0000142). |

- The runner judges a run successful only when the result's run ID matches this run, there are no errors, timeouts, or leftover processes, the go exit code is 0, and the run directory is on an internal disk (`fixed`).
- While other runs use the same VM, `utmctl file pull` can return the contents of a file that another run opened, and `utmctl file push` can write to a file that another run opened, both without an error.
  - When a `request.json`, start marker file, `result.json`, or `rmdir` removal result read from the guest does not carry this run's run ID or cannot be read, the runner ignores it, writes `ignored <guest path> read from the guest` and the reason to `runner.log`, and checks again.
  - After the result arrives, the runner reads those of `stdout.log`, `stderr.log`, and `diagnostics.tar` that have a SHA-256 in the result up to three times each until their SHA-256 matches the result, and collection fails if one still differs.
  - When no result arrives, the runner copies these files once without checking them, so they may belong to another run.
- Uncommitted modifications, additions, and deletions in the working tree are sent as they are, and files excluded by `.gitignore` and symbolic links such as `.agents/skills` are not sent.
- The guest's per-run directory `<guest-dir>\<run ID>` is deleted once collection finishes.
- As soon as it starts, the guest program writes its run ID to the start marker file `<guest-dir>\<run ID>.started` next to the run directory. The runner deletes this file in the step that removes the run directory, and does not warn if it cannot delete it.
- If collection fails, it is left in place and the error message shows its path.
- If the directory cannot be removed after collection, the test result and the exit code stay as they are, and a warning starting with `Windows E2E cleanup:` shows the remaining path and the command that removes that directory.
- Tests against the same VM from the same Mac can run at the same time, and each uses a different run directory.
- While a test is running, both a VM start by `up` and `down` are refused with `another Windows E2E run on this Mac is using the VM`.
- Runs from other user accounts or other Macs are not coordinated.

## Shutdown behavior

- `down` and `all` request a graceful shutdown from Windows with `utmctl stop --request`, and repeat the request every 30 seconds if the VM does not shut down.
- They never force the VM's power off; if the VM does not shut down within `-stop-timeout`, they fail and leave the VM running.
- Every request checked on 2026-11-03 was a real shutdown.
- The guest logged a power-off by winlogon (event 1074, reason 0x500ff) and a clean stop of the event log (6006), and the next boot was a cold boot (Kernel-Boot 27, boot type 0x0).
- Hibernation is off in this guest, so Fast Startup cannot be used.
- In some cases the first request was ignored and the VM shut down on the second request; the cause was not determined.

## Differences between Windows ARM64 and x64

- The UTM VM runs Windows ARM64, and the guest Go is `windows/arm64`.
- Go's race detector supports only `windows/amd64` on Windows ([requirements](https://go.dev/doc/articles/race_detector#Requirements)), so `-race` verification is not possible on this VM.
- `-race` verification and Windows x64 behavior are still checked by `test (windows-latest)` in GitHub Actions ([CI steps](windows.md#ci-steps)).
- A passing UTM run does not substitute for a Windows x64 CI result.

## Troubleshooting

### start does not respond

- `utmctl start` may keep failing in the same UTM process, ending after 2 minutes with `OSStatus error -1712` without starting the VM ([UTM #8006](https://github.com/utmapp/UTM/issues/8006)).
- If every registered VM is off, `up` quits UTM gracefully once, relaunches it, and starts the VM again.
- If another VM is running, `up` fails without restarting UTM; shut that VM down, quit UTM, and run `up` again.
- `OSStatus error -10004` mixed into the output appears whether or not the start succeeded, so you can ignore it.

### UTM crashes

- In one case, clicking the UTM window during a run made UTM exit with SIGSEGV, and the pending request ended with `OSStatus error -609`.
- Do not open or click the UTM window during a run.
- Crash reports are in `~/Library/Logs/DiagnosticReports/UTM-*.ips`.

### AppleScript fails with -600 while UTM is running

- While any `utmctl` process is running, an AppleScript `tell application "UTM"` sends its events to that `utmctl` process instead of UTM and fails at once with `Application isn't running. (-600)` (macOS 27.2, UTM 5.0.8, 2026-11-05).
  - Pointing to UTM by bundle ID or app path gives the same result.
  - `utmctl` requests from other runs on the same VM, such as `file push`, last for seconds, so concurrent runs hit this often.
- The runner and the command in step 2 of [Creating a dedicated E2E VM](#creating-a-dedicated-e2e-vm) send their events to UTM with JXA (`osascript -l JavaScript`), whose events reach UTM even while `utmctl` is running.
- When you send AppleScript to UTM yourself, run it when `pgrep -x utmctl` prints nothing.
- Setting `AEDebugSends=1` before `osascript` prints the target of each event; `target='psn '[utmctl]` means the event went to `utmctl`.

### QEMU aborts during shutdown

- If a new `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips` appears after the shutdown request, `down` and `all` fail with `QEMU aborted while the VM was stopping` even if the VM shut down.
- On the original VM with a display, 7 of 11 shutdowns ended this way, and on the dedicated VM 0 of 18 did (2026-11-03).
- The cause is on UTM's QEMU side and has not been determined.
- The report does not identify the VM, so a QEMU abort of another VM at the same time is also judged a failure.

### Guest run directory is left behind

- `Windows E2E cleanup: guest run directory <path> is still there after ...` means the directory remained although the runner repeated `rmdir` for 1 minute.

The directory remains for one of the following three causes, and the warning alone does not tell which one.

- A guest process is left that keeps a file in the directory open or uses a directory inside it as its current directory.
  - On 2026-11-03, a `hookrec.exe` still running after the tests ended held `src\e2e\scenarios`.
  - End that process with steps 1 and 2 below.
- Windows cannot delete `guest.exe` for several minutes after the guest program ended.
  - On 2026-11-05, when three runners ran at the same time on the same VM, 2 of 9 runs were left this way, and the directories could be removed after about 4 minutes.
  - `rmdir` then printed `guest.exe - Access is denied.` (`액세스가 거부되었습니다.` on a Korean guest), but Restart Manager found no process that had the file open. The XtaCache service, which stores translations of x64 programs on an ARM64 guest, is presumed to hold the file, but this was not confirmed.
  - Wait a few minutes, then continue from step 3 below.
- The QEMU Guest Agent left a file in the directory open.
  - `utmctl file pull` and `file push` send the Guest Agent separate requests to open, read or write, and close the file. If `utmctl` ends between the open and the close, or another request receives the open answer, the file stays open.
  - On 2026-11-05, a `started.txt` left open this way kept a run directory in place, and Restart Manager reported `QEMU Guest Agent` as the process that had the file open. The runner keeps the start marker file next to the run directory and does not stop a `file pull` before 2 minutes pass. The runner cannot prevent another request from receiving the open answer.
  - A file the Guest Agent opened is closed only when the Guest Agent process ends, so `taskkill` in steps 1 and 2 does not make it removable. If steps 1 and 2 found no leftover process and the directory is still there in step 4 a few minutes later, treat it as this case.
  - After the other runs on the same VM end, restart the VM with `make e2e-windows-down` and `make e2e-windows-up`, then continue from step 3.

Check the following before running the procedure below.

- `could not confirm that guest run directory <path> was removed` means the runner could not read a removal result of this run within 1 minute; check whether the directory remains with step 4 below.
- The last line of either warning holds only the command that removes the directory.
- `utmctl exec` does not wait for the guest command to finish, and the output and exit code it returns may belong to another guest command run earlier.
- While other runs or other cleanups use the same VM, `utmctl file pull` may also return, without an error, the contents of a file that another request opened.
- `C:\dw-e2e` in the commands below is the default `-guest-dir`.

The commands in steps 1, 2, and 4 below therefore write their results to a file in the guest, read it with `utmctl file pull`, and follow these rules to read only their own results.

- Run the commands one after another in a single Mac shell (zsh or bash).
- The first line of each step puts a new name in the shell variable `id` and the result file path `C:\dw-e2e\<id>.txt` in `out` every time it runs, and the rest of the step and the command that deletes the result file use these variables.
  - The guest command writes `id` on the first line of the result file and `end <id>` on the last line.
  - When you run a step again, start from its first line so that it uses a new name.
- Read the pulled contents as the step's result only when the first line is `id` and the last line is `end <id>`.
  - If the pull prints `failed to open file` or either line differs, the guest command has not finished yet or the pull returned the contents of another request, so run only the pull again after a few seconds.
  - `utmctl file pull` exits with code 0 even when it cannot open the file, so do not judge by the exit code.

After checking the result of each step, delete the result file with this command:

```sh
/Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "del $out"
```

- If running the pull again after the deletion prints `failed to open file`, the result file has been deleted.
- If the pull returns the contents again, run the delete command again.

1. Check the guest's process list. Look for the `directory-watcher.exe` and `hookrec.exe` that the E2E builds and runs, and for `go.exe` and `*.test.exe`. The output is encoded in the guest's code page.

   ```sh
   id=cleanup-$(openssl rand -hex 8); out='C:\dw-e2e\'$id.txt
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "(echo $id& tasklist & echo end $id) > $out"
   /Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> "$out"
   ```

2. End the remaining processes, then read the result that `taskkill` wrote for each PID. For several PIDs, repeat `/pid <PID>`.

   ```sh
   id=cleanup-$(openssl rand -hex 8); out='C:\dw-e2e\'$id.txt
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "(echo $id& taskkill /f /pid <PID> & echo end $id) > $out 2>&1"
   /Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> "$out"
   ```

- The directories of other tests running on the same VM at the same time are also under `C:\dw-e2e`.
- Remove only the path shown in the warning.

3. Copy the last line of the warning as it is and run it in the Mac shell to remove the directory.
4. If the run ID from the warning is not in the listing of `C:\dw-e2e`, the directory has been removed.

   ```sh
   id=cleanup-$(openssl rand -hex 8); out='C:\dw-e2e\'$id.txt
   /Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "(echo $id& dir /b C:\dw-e2e & echo end $id) > $out"
   /Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> "$out"
   ```

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
- On this VM, checked on 2026-11-05, processes that exited with 0xc0000142 left nothing in the Windows event log, so the event log cannot find this failure.
