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
- Helper processes inherit the console and the process group.
- The same graceful shutdown is not guaranteed for external programs that create a new console or ignore the signal ([Process trees](harness.md#process-trees)).
- The watcher forcibly ends hook processes that are still running 20 seconds after the first CTRL+BREAK through that hook's Job Object, and this termination is not a graceful shutdown ([Overload and shutdown](../../docs/guide/running.md#overload-and-shutdown)).

## Hook cancellation checks

The Windows E2E runs the watcher without Git Bash and checks that the watcher stops canceled hooks.

- For `run.interrupt: oldest` replacement and for shutdown, a hook's parent, child, and grandchild processes end on the signal: [`TestProcessLifecycle`](scenarios/process-lifecycle.md) and [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md).
- On shutdown, the parent, child, and grandchild processes of two hooks that run at the same time with `run.concurrency: 2` all end on the signal before the finish hook runs: [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md).
- A `powershell` hook canceled 0–1000 ms after it starts does not run its program to completion: [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md).

The unit tests in `internal/process/process_windows_test.go`, not the E2E, check the behaviors below.

| Behavior | Test |
|---|---|
| A program that `cmd` starts after the first CTRL+BREAK also receives CTRL+BREAK and ends (separators `&`, `&&`, `\|\|`) | `TestRunCommandSendsBreakAgainToProgramStartedAfterBreak` |
| A process created before the first CTRL+BREAK but attached to the console after it also receives CTRL+BREAK and ends | `TestRunCommandSendsBreakAgainToProcessAttachedToConsoleAfterBreak` |
| A descendant started just before the first CTRL+BREAK is sent receives CTRL+BREAK after it has run for 500ms, and ends | `TestRunCommandDelaysBreakForDescendantStartedJustBeforeBreak` |
| Processes still running 20 seconds after the first CTRL+BREAK are forcibly ended (the test shortens the 20 seconds to 1 second) | `TestRunCommandEndsProcessTreeAfterGrace` |

- The contract for these behaviors is in [Overload and shutdown](../../docs/guide/running.md#overload-and-shutdown).

## CI steps

- `test (windows-latest)` in `.github/workflows/ci.yml` runs `go test -count=1 -race ./...`, then the error notification process verification and the `-count=3` repeat of the three tests in [Run commands](#run-commands).
- It then checks the intentional failure and timeout of `DW_E2E_FAULT=failure` and `timeout`, the diagnostics files, and cleanup of the three PIDs, and confirms that a separately started Go parent, child, and grandchild tree is still alive.
- The Windows-only steps run even if an earlier step failed, as long as the job was not cancelled.
- The earlier failure remains, so the job still fails in the end, and the diagnostics upload step always runs.
- Apart from the Go build cache, **the test result cache is not reused**.
- The `Verify Windows error notification process trees` step runs `TestEventLossNotificationCLI` and saves the test output to `notification.log`.
  - `notification.log` holds the results of the checks for a single notification, the first error, and the elapsed time.
  - This step runs with `DW_E2E_KEEP=1` to keep the run folders, so each run's `watcher.log` holds the actual error diagnostics.
  - What each mode checks and the liveness record files left in the run folder are in the [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) document, and how the notification's descendants are cleaned up on Windows is in [Descendant cleanup by platform](scenarios/event-loss-notification-cli.md#descendant-cleanup-by-platform).

## Checking CI results

- In GitHub Actions, open `ci` → `test (windows-latest)` for **the commit SHA you are verifying**.
- Check that the four combinations of `TestFastInflowSlowHook`, `TestQueueWarningCLI`, `TestConcurrentRunsShutdown`, the four subtests of `TestRunInterrupt`, the two subtests of [`TestBatchingOptions`](scenarios/batching-options.md), the three separators of `TestRunCommandSendsBreakAgainToProgramStartedAfterBreak`, and the eleven cancel points of `TestCanceledPowershellHookStops` in the `go test -count=1 -race ./...` step show `PASS` and no `SKIP`.
- Check that the six modes in the notification verification step and the three tests and two option combinations in the repeat step show `PASS` and no `SKIP`.
- Check that the intentional failure verification step `Verify Windows failure and timeout diagnostics` succeeded.
- From the `e2e-diagnostics-windows-latest` artifact, download the records from before and after notification cleanup and `notification.log`, `failure.log`, and `timeout.log`.
- A past success on a different SHA, a cross-compilation, or a Linux Docker result is not evidence of a Windows run for that commit.
- If there is no Windows run result for that commit, leave the Windows item as `Not verified`.
- Check `-race` and the Windows x64 behavior only with this CI result ([Differences between Windows ARM64 and x64](windows-utm.md#differences-between-windows-arm64-and-x64)).

## Failure analysis

- When Windows fails, first compare `records.log.processes`, written **before cleanup**, with the last `signal` and `complete` in `records.log`.
- `records.log.processes` has one line per process tree process (`run=`, with generation and depth) and per delayed hook (`delayed=`, with its record), each holding the PID, liveness, first observation time, creation time, process access error, wait result, the program of the handle kept from the first observation (`held-image`), and the program that holds the PID now (`current-image`).
  - `open-error=PID reused by another program: <program path>` means that another program held the PID at the first observation, so the process was judged gone ([Process liveness and forced cleanup](harness.md#process-liveness-and-forced-cleanup)).
- `cleanup.log` shows the state **after cleanup** and does not replace the graceful shutdown judgment.
- For `GenerateConsoleCtrlEvent` or `AllocConsole` errors, check the console attachment and the process groups on the same console.
- If a hook or the watcher exited with 0xc0000142, read `dll-init-failed.log` in that run folder as in [A hook exits with 0xc0000142](windows-utm.md#a-hook-exits-with-0xc0000142).

To deliberately exercise the cleanup path, run the PowerShell commands below.

```powershell
$env:DW_E2E_FAULT = 'failure' # run timeout the same way
go test -count=1 -v -timeout=2m -run '^TestProcessLifecycle$' ./e2e/scenarios
Remove-Item Env:DW_E2E_FAULT
```

- What `failure` and `timeout` do, the expected result, and the diagnostics to check are in [Intentional failures and timeouts](diagnostics.md#intentional-failures-and-timeouts).
- On Windows, also check `records.log.processes` at the `diagnostics retained:` path.
