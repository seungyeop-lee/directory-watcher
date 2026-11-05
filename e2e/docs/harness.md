# E2E harness structure and adding scenarios

🇺🇸 English | 🇰🇷 [한국어](harness.ko.md)

- Paths in the tables and steps below are relative to the `e2e/` directory.

## Code locations by responsibility

| File | Responsibility |
|---|---|
| `scenarios/main_test.go` | `TestMain` for the scenario package. Hands setup and teardown for the whole test run to `harness.Run` |
| `scenarios/*_test.go` | Scenario tests. See [Scenarios and their supporting code](#scenarios-and-their-supporting-code) for the mapping |
| `harness/harness.go` | Console setup for the whole test run, building the watcher, the watcher with the error injection build tag, and `hookrec`, running each once right after the build, and deleting the binaries (`Run`, `WatcherBin`, `InjectWatcherBin`, `HookrecBin`). Per-scenario run folder (`NewScenario`), watcher launch (`Start`·`Launch`), helper process launch (`StartProcess`), output and exit observation (`Log`·`Done`·`Exited`·`ExitErr`), observation-based waiting (`Await`·`AwaitProgress`), graceful shutdown check (`Stop`·`StopWithin`), keeping diagnostics and cleanup (`cleanup`) |
| `harness/hook_records.go` | Parsing of `hookrec` one-line records. Reading records (`Lines`·`ReadLinesLenient`), building hook commands (`Hook`), waiting for a record (`AwaitLine`), counting records (`CountRecord`), `BEGIN` records of delayed hooks (`delayedHooks`·`DelayedHookBegun`), the completion boundary of received events (`AwaitChangesComplete`) |
| `harness/process_records.go` | Parsing of process tree records (`ProcessRecord`). Waiting for generations to be ready (`AwaitReadyGenerations`), generations by label (`GenerationsLabeled`), checking that processes are alive or gone (`Alive`·`AssertRunning`·`ProcessesGone`·`AwaitProcessesGone`·`AwaitDelayedHooksGone`), judging graceful completion (`AssertGracefulCompletion`), per-step liveness records (`SnapshotProcesses`), forced cleanup limited to the scenario's records (`rescueRecordedProcesses`) |
| `harness/signal_unix.go`, `harness/signal_windows.go` | Per-platform process groups, graceful shutdown signal, forced watcher termination, the start and exit times of an exited scenario process (`processExit`), and the scenario-owned `processObserver` |
| `harness/dll_init_failure.go` | Report of the CTRL+BREAK and exits around a process that exited with 0xc0000142 on Windows. Recording the CTRL+BREAK the harness sends (`recordHarnessSignal`), reading the watcher's DEBUG lines (`watcherEvents`), building the report that `cleanup` writes to `dll-init-failed.log` and the test output (`dllInitFailureReport`·`formatDLLInitFailureReport`) |
| `harness/harness_test.go` | Unit tests for the progress-based wait (`awaitProgress`) |
| `harness/hook_records_test.go` | Unit tests for the receive boundary and completion functions |
| `harness/dll_init_failure_test.go` | Unit tests for the report built from watcher log lines and harness signals |
| `harness/cleanup_windows_test.go` | Verifies on Windows that failing to delete the run folder or binaries surfaces as an abnormal exit. Includes the harness package's `TestMain`, which goes through the same `Run` |
| `harness/process_observer_windows_test.go` | Verifies on Windows that a recorded PID already held by another program counts as gone, and that the wait for delayed hooks still fails while a recorded hook runs |
| `harness/dll_init_failure_windows_test.go` | Verifies on Windows that when a real watcher's hook exits with 0xc0000142, the report holds that exit, the CTRL+BREAK sent by the harness and the watcher, and the exit of the canceled hook |
| `hookrec/` | Helper program that writes one-line records or creates real parent, child, and grandchild processes. Its options are in [`hookrec` process trees and delayed hooks](#hookrec-process-trees-and-delayed-hooks) |
| `hookrec/main_test.go` | Verifies that `--error-context` and `--queue-context` record every environment variable whose name starts with `DW_ERROR_` or `DW_QUEUE_`, including names the code does not list, and no other variable, as JSON before the `--line` record; that `--error-context` also records the working directory; that `--workdir` records the working directory before the line; that `--exit-code` sets the exit status; and that `--label` records the label of a process tree before the `start` record of its generation |
| `linux/` | Entry point that passes a copy of the current source to an isolated Linux container and runs the same Go E2E |
| `worktree/` | Packs the current working tree into a tar stream with entries under `src/` for the Linux and Windows runners (`Root`, `Files`, `Archive`). The archive holds tracked files and untracked files that are not ignored, with uncommitted modifications, additions, and deletions |
| `worktree/worktree_test.go` | Verifies that `Archive` writes the working tree contents of modified and untracked files and skips listed files that are gone from the working tree |
| `windows/` | Entry point that runs the same Go E2E on a UTM Windows VM from a Mac (`up`, `test`, `down`, `all`). Controls UTM and the VM (`utm.go`), locks the VM against other runs on the same Mac (`lock.go`), and sends the source and the guest program to the guest and collects the results (`test.go`). The package and its tests build only on macOS |
| `windows/utm_test.go` | Verifies reading `utmctl` and JXA answers with stand-in scripts: finding a VM by UUID or complete name, counting VMs that are not stopped, judging success by stderr rather than by the exit code, failing a `start` that leaves the VM stopped, requiring an IPv4 address for Guest Agent readiness, and reading the display count |
| `windows/main_test.go` | Verifies that `up` refuses a VM with a display before requesting its start |
| `windows/lock_test.go` | Verifies that the VM lock is shared by test runs and exclusive for starting and stopping, and that it applies per VM |
| `windows/test_test.go` | Verifies that creating the guest run directory and starting the guest program are judged by files read back from the guest rather than by the `exec` answer, that removing the guest run directory is retried and a remaining directory is reported with a removal command, that this cleanup failure stays out of the test result, that waiting for the result ignores a result of another run or an unreadable one, that collected files are read again until they match the SHA-256 in the result, and that the source archive leaves out symbolic links |
| `windows/guest/` | Program that runs one E2E run inside the Windows guest. Checks the source checksum, runs `go test`, terminates leftover processes started from the run directory, archives the diagnostics, and writes the result with the SHA-256 of the files the runner collects. Builds only on Windows |
| `windows/guestrun/` | Files the Mac runner and the guest program exchange in the guest run directory (`Request`, `Result`), how the runner accepts a result of its run (`Decode`) and judges it (`Check`), and the tar format for moving directories between the two machines (`ArchiveDir`, `Extract`) |
| `windows/guestrun/guestrun_test.go` | Verifies that `Decode` accepts only a result of the same run and `Check` only a passed result, that `ArchiveDir` and `Extract` round trip, and that `Extract` rejects paths leaving the target and symbolic links |

- Building the binaries and setting up the console happen once for the whole test run.
  - `Run` runs each binary once right after building it, before any scenario.
    - Windows Defender pauses process creation to scan a newly built exe the first time it runs, and on a loaded VM this scan can take longer than a scenario's wait limit.
    - Running the same file again does not wait for this scan.
  - If this run fails, `Run` prints an error starting with `warm up:` and returns exit code 1 without running the tests.
- Each scenario owns its run folder, watcher, process observation, and cleanup.
- On Windows, `processObserver` keeps the handle it opened when it first observed a recorded PID until that scenario's cleanup finishes, and forced cleanup terminates only the `hookrec` handles in that scenario's records.
- Every recorded process is `hookrec`, so if a program other than this run's `hookrec` already holds the PID at that first observation, the recorded process has exited and its PID was reused; it is judged gone.
- When the program cannot be identified, the handle is kept and liveness is judged from it.
- Protected trees of other scenarios or other worktrees are therefore not cleaned up.

## Scenarios and their supporting code

| Scenario | File | What it verifies | Supporting code used |
|---|---|---|---|
| [`TestGoldenPath`](scenarios/golden-path.md) | `scenarios/golden_path_test.go` | Create, update, delete, hook order, 8 event variables | `Start`, `Hook`, `AwaitLine`, `Stop`, `scenarios/helpers_test.go` |
| [`TestHookWorkingDirectory`](scenarios/hook-working-directory.md) | `scenarios/shell_test.go` | Global and target hook execution locations with and without `dir` | `Start`, `Hook`, `AwaitLine`, `Stop`, `Lines` |
| [`TestWindowsShellNonASCIIOutput`](scenarios/windows-shell-non-ascii-output.md) | `scenarios/shell_windows_test.go` | Non-ASCII `cmd` and Windows PowerShell hook output reaching the watcher's output as UTF-8, and the console output code page restored after exit | `Launch`, `WatcherBin`, `Await`, `Log`, `Stop` |
| [`TestWindowsShellRejectedOnUnix`](scenarios/windows-shell-rejected-on-unix.md) | `scenarios/shell_unix_test.go` | No hook, exact error, and exit 1 when Unix selects `shell: cmd` | `Launch`, `Await`, `Exited`, `ExitErr`, `Log`, `Lines` |
| [`TestFolderEvents`](scenarios/folder-events.md) | `scenarios/folder_events_test.go` | Events the hook receives for each folder and file action, one event for the top folder, a file created later in a folder created or moved in during watching, and no hook for a folder moved out | `Start`, `Hook`, `Await`, `AwaitProgress`, `Lines`, `Stop`, `scenarios/helpers_test.go` |
| [`TestWatchOptions`](scenarios/watch-options.md) | `scenarios/watch_options_test.go` | exclude patterns and exceptions, `fileEvent: none`, and all eight resolved event variables for a linked target | `Start`, `Hook`, `Await`, `AwaitProgress`, `AwaitLine`, `Lines`, `Stop`, `scenarios/helpers_test.go` |
| [`TestProcessLifecycle`](scenarios/process-lifecycle.md) | `scenarios/process_test.go` | Three processes of the start hook, restart after the previous generation completes, finish hook order, `DW_E2E_FAULT` | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md) | `scenarios/process_test.go` | Start hook shutdown and replacement for the two `run.interrupt` values | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md) | `scenarios/process_test.go` | Shutdown of two process tree hooks that run at the same time with `run.concurrency: 2`, finish hook after both complete | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestRunInterrupt`](scenarios/run-interrupt.md) | `scenarios/process_test.go` | With `run.concurrency: 2`, the runs that `run.interrupt: samePath` and `oldest` cancel among running change hooks and a running start hook, the replacement after the canceled tree completes | `GenerationsLabeled`, `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestBatchingOptions`](scenarios/batching-options.md) | `scenarios/batching_test.go` | `lastPerPath` final changes and order, deleted descendant filtering, zero folder wait | `Start`, `Hook`, `Await`, `AwaitChangesComplete`, `Stop`, `scenarios/helpers_test.go` |
| [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md) | `scenarios/reliability_test.go` | Four combinations of fast inflow during a slow hook, exclusion and `deliver: last` merging, no `onError` notification, no runs after shutdown | `DelayedHookBegun`, `AwaitChangesComplete`, `AwaitLine`, `AwaitDelayedHooksGone` |
| [`TestOverlappingTargets`](scenarios/overlapping-targets.md) | `scenarios/overlapping_targets_test.go` | Targets with the same or nested paths: start without errors, one change runs each target's `onChange` once and global `onBeforeChange`·`onAfterChange` once per target, no waiting for the other target, a hook writing into another target, and the parent's `exclude` of the child folder | `Start`, `Hook`, `Await`, `AwaitLine`, `DelayedHookBegun`, `Stop`, `AwaitDelayedHooksGone`, `scenarios/helpers_test.go` |
| [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) | `scenarios/error_notification_test.go` | Six loss notification modes, descendant cleanup after the notification parent exits first, protected tree survival | `Launch`, `StartProcess`, `DelayedHookBegun`, `AwaitReadyGenerations`, `SnapshotProcesses`, `ProcessesGone`, `Alive`, `AwaitDelayedHooksGone` |
| [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) | `scenarios/error_notification_test.go` | Notification and exit code for errors injected into the watcher error channel, in four `hookBatch.deliver`·`run.interrupt` combinations and for a non-loss fatal error | `InjectWatcherBin`, `Launch`, `AwaitReadyGenerations`, `Exited`, `AwaitProcessesGone` |
| [`TestQueueWarningCLI`](scenarios/queue-warning-cli.md) | `scenarios/queue_warning_test.go` | `onQueueWarning` at 70% and 90% of `queue.size` and on the way down, with its `DW_QUEUE_*` variables | `AwaitReadyGenerations`, `Await`, `AwaitLine` |
| [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md) | `scenarios/windows_cancel_test.go` | On Windows, stopping a `powershell` hook canceled 0–1000ms after it starts | `Launch`, `Hook`, `Log`, `StopWithin`, `AwaitDelayedHooksGone` |
| `TestCleanupReportsLockedResources` | `harness/cleanup_windows_test.go` | Abnormal exit on Windows delete failures | `cleanup` |
| `TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram` | `harness/process_observer_windows_test.go` | A recorded PID held by another program counts as gone on Windows | `NewScenario`, `AwaitDelayedHooksGone` |
| `TestDelayedHooksGoneFailsWhileHookRuns` | `harness/process_observer_windows_test.go` | The delayed hook wait fails while a recorded hook runs on Windows | `NewScenario`, `Await`, `DelayedHookBegun`, `AwaitDelayedHooksGone` |
| `TestDLLInitFailureReportFromWatcher` | `harness/dll_init_failure_windows_test.go` | The 0xc0000142 report on Windows holds the failed exit and the CTRL+BREAK around it | `NewScenario`, `Start`, `Hook`, `Await`, `DelayedHookBegun`, `Stop`, `dllInitFailureReport` |

- Each scenario name links to a scenario document that describes what it checks and its success criteria, a sequence diagram, subtests, and run commands.
- The receive boundary checks and the limits of loss verification that span several scenarios are in [Race, overload, and loss verification](reliability.md), and running the Windows scenarios and checking CI is in [Running Windows E2E and checking CI results](windows.md).

## Writing a new scenario

1. Create a new `*_test.go` in `scenarios/` (`package scenarios`, importing the `harness` package) and write the configuration, the input order, and the success criteria. Add an OS-specific build tag only after you have run the test on that OS and confirmed the cause.
2. Create the run with `s := harness.NewScenario(t)`, and build the hook commands in the configuration YAML with `s.Hook("--line=...")`. The default shell (sh or cmd) interprets hook commands. `Hook` wraps each argument without a template in double quotes and leaves an argument with a template unquoted so that its value is passed as one argument, so do not put characters that shells interpret, such as `|` or `&`, in an argument with a template. Put `exclude: ['*.probe']` under `option.watch` in the configuration and call `s.Start(config)`. `Start` confirms that watching is ready by observing the actual event of the excluded probe file in the DEBUG log. To verify the default ERROR log level, launch with `s.Launch(harness.WatcherBin(), config)` and confirm readiness from a start hook record. Do not use a fixed sleep to judge readiness or completion.
3. Act with `s.Write`, and instead of a record count, specify the hook, path, or generation to wait for as the condition of `s.AwaitLine`, `s.AwaitReadyGenerations`, or `s.Await`. Check exact counts and order directly in the test after waiting. If the time a wait needs grows with the number of hooks that run, use `s.AwaitProgress` and return as progress a value that grows as hooks finish, such as the number of record lines, so that the limit counts only time without progress.
4. Check graceful shutdown and exit code 0 with `s.Stop()`, then judge the process records. The forced termination in scenario cleanup runs after the test ends and is not evidence of graceful shutdown.

```go
func TestCreateAnotherFile(t *testing.T) {
	s := harness.NewScenario(t)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
`, s.Watch, s.Hook("--line=CHANGE:{{.Name}}")))
	s.Write(filepath.Join(s.Watch, "another.txt"), nil)
	s.AwaitLine("create hook", 10*time.Second, "CHANGE:another.txt")
	s.Stop()
	if got := harness.CountRecord(s.Lines(), "CHANGE:another.txt"); got != 1 {
		t.Fatalf("create hook count = %d, want 1", got)
	}
}
```

After writing the test, create the scenario documents and update the scenario lists.

5. Create the scenario document as a pair: a Korean version at `docs/scenarios/<name>.ko.md` and an English version at `docs/scenarios/<name>.md`. The file name is the test name without `Test`, in kebab-case (`TestQueueWarningCLI` → `queue-warning-cli.ko.md` and `queue-warning-cli.md`). Give both versions the same sections and facts, and take each version's section names from the existing documents in that language. As in the [`TestGoldenPath`](scenarios/golden-path.md) document, write the title as `` `<test name>`: <behavior it checks> ``, and put a language line linking to the other version directly below the title: `🇺🇸 English | 🇰🇷 [한국어](<name>.ko.md)` in the English version and `🇺🇸 [English](<name>.md) | 🇰🇷 한국어` in the Korean version. After a one-sentence summary, add the sections `What it checks and success criteria`, `Configuration`, `Sequence` (a Mermaid `sequenceDiagram`), `Running`, and `Related code`. If the test has subtests, add a `Subtests` section as in [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md). Write each subtest name in backticks, either in the first cell of a table whose first column header is `Subtest` (or `Mode`) or, in a `Subtests` section without such a table, at the start of a bullet. The `TestE2EDocsSubtestListsMatchCode` test at the repository root fails when these names differ from the `t.Run` names. In the `Running` section, list the local, Linux Docker, and UTM Windows VM commands; in the `Related code` section, list the test file and the `harness` functions it uses.
6. Add a row to the [Scenarios and their supporting code](#scenarios-and-their-supporting-code) table in both `harness.md` and `harness.ko.md`, and link the scenario name to the document in the same language: `<name>.md` from `harness.md` and `<name>.ko.md` from `harness.ko.md`. Also add a link in the same language to the `Per-scenario documents` row of the `Detailed documents` table in both versions of the E2E README: `<name>.md` from [`README.md`](../README.md) and `<name>.ko.md` from [`README.ko.md`](../README.ko.md).
7. If the scenario meets one of the conditions below, also add it to the `-run` regular expressions and scenario lists in the corresponding places. Update both the Korean and English versions of each document below, and point each scenario document link you add to the version in the same language: for example, link `<name>.md` from `reliability.md` and `<name>.ko.md` from `reliability.ko.md`.
   - **Race and overload verification**: the two repeat commands in [Race and overload verification](reliability.md#race-and-overload-verification) and the document links below them. If Windows CI does not run it separately by name, also add it to the items checked in the `go test -count=1 -race ./...` step in [Checking CI results](windows.md#checking-ci-results).
   - **Loss notification verification**: the two commands in [Event-loss notification verification](reliability.md#event-loss-notification-verification) and the document links below them.
   - **Windows CI graceful lifecycle repeat**: the `Repeat Windows graceful lifecycle without test cache` step in `.github/workflows/ci.yml` at the repository root, the [Run commands](windows.md#run-commands), [CI steps](windows.md#ci-steps), and [Checking CI results](windows.md#checking-ci-results) sections of the Windows document, and the first example command in [Test selection, repetition, and time limits](windows-utm.md#test-selection-repetition-and-time-limits) of the UTM document.
   - **Windows CI notification process tree verification**: the `Verify Windows error notification process trees` step in the same `ci.yml`, and the [CI steps](windows.md#ci-steps) and [Checking CI results](windows.md#checking-ci-results) sections of the Windows document.

## `hookrec` process trees and delayed hooks

- `hookrec` appends one-line records to the file given with `--file=<record file>`, and exits with code 2 without recording when `--file` is missing.
  - `s.Hook` adds `--file` with the scenario's record file, so pass `--file` yourself only when starting `hookrec` directly, as with `exec.Command(harness.HookrecBin(), "--file="+s.Records, ...)`.
- Without `--depth`, `hookrec` writes the `--line=<record>` value as one record and exits.
- With `--workdir`, `hookrec` writes `WORKDIR|<working directory>` before the line or process tree records.
  - `TestHookWorkingDirectory` uses this record before each hook label to check the execution location independently of error notification variables.

### Process trees

- For a long-running process, put `s.Hook("--depth=2")` in a start or change hook.
- The parent actually runs a child, and the child runs a grandchild.
- The generation identifier is the parent PID, and each process records `generation|PID|depth|state`.
  - The parent passes the generation to its child and grandchild with `--run=<generation>`, so scenarios do not set `--run`.
- The parent's `ready` is recorded after the child's `ready`, so `AwaitReadyGenerations` judges the whole generation ready by the parent's `ready`.
- On a shutdown signal, a process waits for a 150ms handling delay and for its child's graceful exit, then writes `complete`.
- With `--release-file=<path>`, the parent exits after `ready` once that file appears, recording `parent-exit` and leaving its descendants running.
  - If a shutdown signal arrives before the file appears, the parent shuts down the usual way and writes `complete`.
  - With `--depth=0`, only the parent runs, so the hook keeps running until the test creates the file. `TestQueueWarningCLI` uses `s.Hook("--depth=0", "--release-file="+release)` as `onStartWatch` so that the target's change hooks cannot run and stay in the queue.
- With `--label=<text>`, the parent writes `LABEL|<generation>|<text>` before its `start` record, and `GenerationsLabeled` returns the generations with that label in the order they started.
  - `TestRunInterrupt` uses `s.Hook("--depth=2", "--label={{.Name}}")` as `onChange` to tell which path each generation runs for.

### Delayed hooks

- A delayed hook is made with `s.Hook("--delay=80ms", "--line=...")` and writes `BEGIN|PID|record`, plus `CANCEL|PID|record` when cancelled.

### Error notification and queue warning context

- `--error-context` records every environment variable whose name starts with `DW_ERROR_`, plus the working directory under the key `workdir`, as `ERROR|<JSON>`.
- `--queue-context` records every environment variable whose name starts with `DW_QUEUE_` as `QUEUE|<JSON>`.
- The JSON is an object that maps each variable name to its value.
- These records come before the `--line` record or the process tree records, so `hookrec --error-context --line=NOTIFY` writes `ERROR|<JSON>` and then `NOTIFY`.

### Exit status

- `--exit-code=<code>` sets the exit status after recording.
  - A one-line record or a delayed hook exits with that code after writing its `--line` record.
  - A process tree parent exits with that code at `parent-exit` when `--release-file` is set, and otherwise right after `ready` without waiting for a shutdown signal, leaving its descendants running.
- `TestEventLossNotificationCLI` uses `--exit-code=7` to make the notification command fail.
