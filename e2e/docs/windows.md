# Running Windows E2E and checking CI results

🇺🇸 English | 🇰🇷 [한국어](windows.ko.md)

## Run commands

- To run on a UTM Windows VM from a Mac, see [Running E2E on a UTM Windows VM](windows-utm.md).

Run these from the repository root in PowerShell on a Windows host where Go and the console API are available.

```powershell
go test -count=1 -race ./...
go test -count=3 -v -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$' ./e2e/scenarios
if ($LASTEXITCODE -ne 0) { throw "Windows lifecycle failed" }
```

- What each test checks is in the [`TestGoldenPath`](scenarios/golden-path.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.md), and [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md) documents.

## Graceful shutdown

- If the test process has no console, it allocates one with `AllocConsole` and runs the watcher and each hook in a separate process group.
- A graceful shutdown request sends `CTRL_BREAK_EVENT` only to that group on the same console.
- [Go delivers CTRL+BREAK as `os.Interrupt`](https://github.com/golang/go/blob/go1.27.0/src/os/signal/doc.go#L213-L220), so the existing SIGINT handling of the CLI is used.
- Neither a group-targeted `CTRL_C_EVENT` nor `TASKKILL /F` is used as evidence of graceful shutdown.
- Helper processes inherit the console and group, handle the signal, and wait for their descendants to exit.
- The same graceful shutdown guarantee does not apply to arbitrary external programs that create a new console or ignore the signal.
- The watcher forcibly ends hook processes that are still running 20 seconds after the first CTRL+BREAK through that hook's Job Object, and this termination is not a graceful shutdown ([Overload and shutdown](../../docs/guide/running.md#overload-and-shutdown)).

## Hook cancellation checks

- The dates below are when each check was confirmed on the UTM VM (Windows 11 ARM64).

The Windows E2E runs the watcher without Git Bash and checks that the watcher stops canceled hooks.

- For `run.interrupt: oldest` replacement and for shutdown, a hook's parent, child, and grandchild processes end on the signal: [`TestProcessLifecycle`](scenarios/process-lifecycle.md) and [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md) (2026-11-04).
- On shutdown, the parent, child, and grandchild processes of two hooks that run at the same time with `run.concurrency: 2` all end on the signal before the finish hook runs: [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md) (2026-11-04).
- The next program of a canceled `cmd` command list ends: [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.md) (2026-11-04).
- A `powershell` hook canceled 0–1000 ms after it starts does not run its program to completion: [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md) (2026-11-04).

## CI steps

- `test (windows-latest)` in `.github/workflows/ci.yml` runs `go test -count=1 -race ./...`, then the error notification process verification and the `-count=3` repeat of the three tests above.
- It then checks the intentional failure and timeout of `DW_E2E_FAULT=failure` and `timeout`, the diagnostics files, and cleanup of the three PIDs, and confirms that a separately started Go parent, child, and grandchild tree is still alive.
- The Windows-only steps run even if an earlier step failed, as long as the job was not cancelled.
- The earlier failure remains, so the job still fails in the end, and the diagnostics upload step always runs.
- Apart from the Go build cache, **the test result cache is not reused**.
- The `Verify Windows error notification process trees` step runs `TestEventLossNotificationCLI` and saves the test output to `notification.log`.
- `notification.log` holds the results of the checks for a single notification, the first error, and the elapsed time.
- This step runs with `DW_E2E_KEEP=1` to keep the run folders, so each run's `watcher.log` holds the actual error diagnostics.
- What each mode checks and the liveness record files left in the run folder are in the [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) document.
- The Windows notification runner assigns the parent to a separate Job Object while it is suspended, before it creates any descendants.
- Even after the parent's exit 7 or exit 0, it terminates the descendants in that Job and waits, within the existing cleanup limit, until there are no active processes.
- The conditions that check this cleanup are in [Extra checks for tree modes](scenarios/event-loss-notification-cli.md#extra-checks-for-tree-modes).
- Normal hooks also run each command in its own Job Object, but the descendants that a hook leaves when it finishes without being canceled are not ended.

## Checking CI results

- In GitHub Actions, open `ci` → `test (windows-latest)` for **the commit SHA you are verifying**.
- Check that the four combinations of `TestFastInflowSlowHook`, `TestTargetsRunIndependently`, `TestBoundedOverloadCLI`, `TestQueueWarningCLI`, `TestConcurrentRunsShutdown`, the four subtests of `TestRunInterrupt`, the three separators of `TestCanceledCommandListStopsNextProgram`, and the eleven cancel points of `TestCanceledPowershellHookStops` in the `go test -count=1 -race ./...` step, the six modes in the notification verification step, and the three tests and four option combinations in the repeat step show `PASS` and no `SKIP`, and that the intentional failure verification step succeeded.
- In the same `go test -count=1 -race ./...` step, also check that all four subtests of [`TestBatchingOptions`](scenarios/batching-options.md) show `PASS` without `SKIP`.
- From the `e2e-diagnostics-windows-latest` artifact, download the records from before and after notification cleanup and `notification.log`, `failure.log`, and `timeout.log`.
- A past success on a different SHA, a cross-compilation, or a Linux Docker result is not evidence of a Windows run for that commit.
- If there is no Windows run result for that commit, leave the Windows item as `Not verified`.
- Run the items that the UTM VM can check with [Running E2E on a UTM Windows VM](windows-utm.md), and check the `-race` and Windows x64 items only with this CI result.

## Failure analysis

- When Windows fails, first compare `records.log.processes`, written **before cleanup**, with the last `signal` and `complete` in `records.log`.
- `records.log.processes` has one line per process tree process (`run=`, with generation and depth) and per delayed hook (`delayed=`, with its record), each holding the PID, liveness, first observation time, creation time, process access error, wait result, the program of the handle kept from the first observation (`held-image`), and the program that holds the PID now (`current-image`).
- `cleanup.log` shows the state **after cleanup** and does not replace the graceful shutdown judgment.
- For `GenerateConsoleCtrlEvent` or `AllocConsole` errors, check the console attachment and the process groups on the same console.
- Access-denied and wait API errors are treated as alive and fail the test.
- The handle opened at the first observation is kept until after cleanup to avoid confusion from PID reuse, and a PID that a program other than `hookrec` already held at that observation is judged gone with `open-error=PID reused by another program: <program path>`.
- Failure cleanup terminates only the recorded helper binary handles of that run, and if the watcher is still alive, applies `TASKKILL /T /F` only to that watcher's tree.

To deliberately exercise the cleanup path, run the commands below.

```powershell
$env:DW_E2E_FAULT = 'failure' # run timeout the same way
go test -count=1 -v -timeout=2m -run '^TestProcessLifecycle$' ./e2e/scenarios
Remove-Item Env:DW_E2E_FAULT
```

- For these commands, a nonzero exit is the expected result.
- Check the diagnostics files at the `diagnostics retained:` path and the three `alive=false` entries in `cleanup.log`.
- `DW_E2E_DIAGNOSTICS` sets the parent directory for diagnostics, and CI uses `${{ runner.temp }}/dw-e2e`.
- The list of diagnostics files and the same checks on macOS and Linux are in [E2E failure and timeout diagnostics and isolation](diagnostics.md).
