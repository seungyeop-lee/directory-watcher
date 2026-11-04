# E2E harness structure and adding scenarios

🇺🇸 English | 🇰🇷 [한국어](harness.ko.md)

- Paths in the tables and steps below are relative to the `e2e/` directory.

## Code locations by responsibility

| File | Responsibility |
|---|---|
| `scenarios/main_test.go` | `TestMain` for the scenario package. Hands setup and teardown for the whole test run to `harness.Run` |
| `scenarios/*_test.go` | Scenario tests. See [Scenarios and their supporting code](#scenarios-and-their-supporting-code) for the mapping |
| `harness/harness.go` | Console setup for the whole test run, building the watcher and `hookrec` and deleting the binaries (`Run`, `WatcherBin`, `HookrecBin`). Per-scenario run folder (`NewScenario`), watcher launch (`Start`·`Launch`), helper process launch (`StartProcess`), output and exit observation (`Log`·`Done`·`Exited`·`ExitErr`), observation-based waiting (`Await`), graceful shutdown check (`Stop`·`StopWithin`), keeping diagnostics and cleanup (`cleanup`) |
| `harness/hook_records.go` | Parsing of `hookrec` one-line records. Reading records (`Lines`·`ReadLinesLenient`), building hook commands (`Hook`), waiting for a record (`AwaitLine`), counting records (`CountRecord`), `BEGIN` and `CANCEL` records of delayed hooks (`delayedHooks`·`DelayedHookBegun`·`DelayedHookCanceled`), the completion boundary of received events (`AwaitChangesComplete`) |
| `harness/process_records.go` | Parsing of process tree records (`ProcessRecord`). Waiting for generations to be ready (`AwaitReadyGenerations`), checking that processes are alive or gone (`Alive`·`AssertRunning`·`ProcessesGone`·`AwaitProcessesGone`·`AwaitDelayedHooksGone`), judging graceful completion (`AssertGracefulCompletion`), per-step liveness records (`SnapshotProcesses`), forced cleanup limited to the scenario's records (`rescueRecordedProcesses`) |
| `harness/signal_unix.go`, `harness/signal_windows.go` | Per-platform process groups, graceful shutdown signal, forced watcher termination, and the scenario-owned `processObserver` |
| `harness/hook_records_test.go` | Unit tests for the receive boundary and completion functions |
| `harness/cleanup_windows_test.go` | Verifies on Windows that failing to delete the run folder or binaries surfaces as an abnormal exit. Includes the harness package's `TestMain`, which goes through the same `Run` |
| `harness/process_observer_windows_test.go` | Verifies on Windows that a recorded PID already held by another program counts as gone, and that the wait for delayed hooks still fails while a recorded hook runs |
| `hookrec/` | Helper program that writes one-line records or creates real parent, child, and grandchild processes |
| `linux/` | Entry point that passes a copy of the current source to an isolated Linux container and runs the same Go E2E |

- Building the binaries and setting up the console happen once for the whole test run.
- Each scenario owns its run folder, watcher, process observation, and cleanup.
- On Windows, `processObserver` keeps the handle it opened when it first observed a recorded PID until that scenario's cleanup finishes, and forced cleanup terminates only the `hookrec` handles in that scenario's records.
- Every recorded process is `hookrec`, so if a program other than this run's `hookrec` already holds the PID at that first observation, the recorded process has exited and its PID was reused; it is judged gone.
- When the program cannot be identified, the handle is kept and liveness is judged from it.
- Protected trees of other scenarios or other worktrees are therefore not cleaned up.

## Scenarios and their supporting code

| Scenario | File | What it verifies | Supporting code used |
|---|---|---|---|
| [`TestGoldenPath`](scenarios/golden-path.md) | `scenarios/golden_path_test.go` | Create, update, delete, hook order, 8 event variables | `Start`, `Hook`, `AwaitLine`, `Stop` |
| [`TestFolderEvents`](scenarios/folder-events.md) | `scenarios/folder_events_test.go` | Events the hook receives for each folder and file action, one event for the top folder | `Start`, `Hook`, `Await`, `Lines`, `Stop` |
| [`TestProcessLifecycle`](scenarios/process-lifecycle.md) | `scenarios/process_test.go` | Three processes of the start hook, restart after the previous generation completes, finish hook order, `DW_E2E_FAULT` | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md) | `scenarios/process_test.go` | Start hook shutdown and replacement in the four `hookBatch.deliver`·`run.interrupt` combinations | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md) | `scenarios/process_test.go` | Shutdown of two process tree hooks that run at the same time with `run.concurrency: 2`, finish hook after both complete | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md) | `scenarios/reliability_test.go` | Four combinations of fast inflow during a slow hook, exclusion and `deliver: last` merging, no runs after shutdown | `DelayedHookBegun`, `AwaitChangesComplete`, `AwaitLine`, `AwaitDelayedHooksGone` |
| [`TestTargetsRunIndependently`](scenarios/targets-run-independently.md) | `scenarios/reliability_test.go` | A slow hook on one target does not block another target | `DelayedHookBegun`, `AwaitLine`, `AwaitDelayedHooksGone` |
| [`TestBoundedOverloadCLI`](scenarios/bounded-overload-cli.md) | `scenarios/reliability_test.go` | Abnormal exit and tree cleanup when the dispatch queue is full | `AwaitReadyGenerations`, `Exited`, `AwaitProcessesGone` |
| [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) | `scenarios/error_notification_test.go` | Six loss notification modes, descendant cleanup after the notification parent exits first, protected tree survival | `Launch`, `StartProcess`, `DelayedHookBegun`, `AwaitReadyGenerations`, `SnapshotProcesses`, `ProcessesGone`, `Alive`, `AwaitDelayedHooksGone` |
| [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.md) | `scenarios/error_notification_test.go` | No notification on normal changes and `deliver: last` merging | `AwaitChangesComplete`, `AwaitLine` |
| [`TestQueueWarningCLI`](scenarios/queue-warning-cli.md) | `scenarios/queue_warning_test.go` | `onQueueWarning` at 70% and 90% of `queue.size` and on the way down, with its `DW_QUEUE_*` variables | `AwaitReadyGenerations`, `Await`, `AwaitLine` |
| [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.md) | `scenarios/windows_cancel_test.go` | On Windows, stopping the next program of a canceled `cmd` command list (`&`, `&&`, `\|\|`) | `Launch`, `Hook`, `DelayedHookBegun`, `DelayedHookCanceled`, `Stop`, `AwaitDelayedHooksGone` |
| [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md) | `scenarios/windows_cancel_test.go` | On Windows, stopping a `powershell` hook canceled 0–1000ms after it starts | `Launch`, `Hook`, `Log`, `StopWithin`, `AwaitDelayedHooksGone` |
| `TestCleanupReportsLockedResources` | `harness/cleanup_windows_test.go` | Abnormal exit on Windows delete failures | `cleanup` |
| `TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram` | `harness/process_observer_windows_test.go` | A recorded PID held by another program counts as gone on Windows | `NewScenario`, `AwaitDelayedHooksGone` |
| `TestDelayedHooksGoneFailsWhileHookRuns` | `harness/process_observer_windows_test.go` | The delayed hook wait fails while a recorded hook runs on Windows | `NewScenario`, `Await`, `DelayedHookBegun`, `AwaitDelayedHooksGone` |

- Each scenario name links to a scenario document that describes what it checks and its success criteria, a sequence diagram, subtests, and run commands.
- The receive boundary checks and the limits of loss verification that span several scenarios are in [Race, overload, and loss verification](reliability.md), and running the Windows scenarios and checking CI is in [Running Windows E2E and checking CI results](windows.md).

## Writing a new scenario

1. Create a new `*_test.go` in `scenarios/` (`package scenarios`, importing the `harness` package) and write the configuration, the input order, and the success criteria. Add an OS-specific build tag only after you have run the test on that OS and confirmed the cause.
2. Create the run with `s := harness.NewScenario(t)`, and build the hook commands in the configuration YAML with `s.Hook("--line=...")`. The default shell (sh or cmd) interprets hook commands. `Hook` wraps each argument without a template in double quotes and leaves an argument with a template unquoted so that its value is passed as one argument, so do not put characters that shells interpret, such as `|` or `&`, in an argument with a template. Put `exclude: ['*.probe']` under `option.watch` in the configuration and call `s.Start(config)`. `Start` confirms that watching is ready by observing the actual event of the excluded probe file in the DEBUG log. To verify the default ERROR log level, launch with `s.Launch(harness.WatcherBin(), config)` and confirm readiness from a start hook record. Do not use a fixed sleep to judge readiness or completion.
3. Act with `s.Write`, and instead of a record count, specify the hook, path, or generation to wait for as the condition of `s.AwaitLine`, `s.AwaitReadyGenerations`, or `s.Await`. Check exact counts and order directly in the test after waiting.
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

5. Create the scenario document as a pair: a Korean version at `docs/scenarios/<name>.ko.md` and an English version at `docs/scenarios/<name>.md`. The file name is the test name without `Test`, in kebab-case (`TestBoundedOverloadCLI` → `bounded-overload-cli.ko.md` and `bounded-overload-cli.md`). Give both versions the same sections and facts, and take each version's section names from the existing documents in that language. As in the [`TestTargetsRunIndependently`](scenarios/targets-run-independently.md) document, write the title as `` `<test name>`: <behavior it checks> ``, and put a language line linking to the other version directly below the title: `🇺🇸 English | 🇰🇷 [한국어](<name>.ko.md)` in the English version and `🇺🇸 [English](<name>.md) | 🇰🇷 한국어` in the Korean version. After a one-sentence summary, add the sections `What it checks and success criteria`, `Configuration`, `Sequence` (a Mermaid `sequenceDiagram`), `Running`, and `Related code`. If the test has subtests, add a `Subtests` section as in [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.md). In the `Running` section, list the local, Linux Docker, and UTM Windows VM commands; in the `Related code` section, list the test file and the `harness` functions it uses.
6. Add a row to the [Scenarios and their supporting code](#scenarios-and-their-supporting-code) table in both `harness.md` and `harness.ko.md`, and link the scenario name to the document in the same language: `<name>.md` from `harness.md` and `<name>.ko.md` from `harness.ko.md`. Also add a link in the same language to the `Per-scenario documents` row of the `Detailed documents` table in both versions of the E2E README: `<name>.md` from [`README.md`](../README.md) and `<name>.ko.md` from [`README.ko.md`](../README.ko.md).
7. If the scenario meets one of the conditions below, also add it to the `-run` regular expressions and scenario lists in the corresponding places. Update both the Korean and English versions of each document below, and point each scenario document link you add to the version in the same language: for example, link `<name>.md` from `reliability.md` and `<name>.ko.md` from `reliability.ko.md`.
   - **Race and overload verification**: the two repeat commands in [Race and overload verification](reliability.md#race-and-overload-verification) and the document links below them. If Windows CI does not run it separately by name, also add it to the items checked in the `go test -count=1 -race ./...` step in [Checking CI results](windows.md#checking-ci-results).
   - **Loss notification verification**: the two commands in [Event-loss notification verification](reliability.md#event-loss-notification-verification) and the document links below them.
   - **Windows CI graceful lifecycle repeat**: the `Repeat Windows graceful lifecycle without test cache` step in `.github/workflows/ci.yml` at the repository root, the [Run commands](windows.md#run-commands), [CI steps](windows.md#ci-steps), and [Checking CI results](windows.md#checking-ci-results) sections of the Windows document, and the first example command in [Test selection, repetition, and time limits](windows-utm.md#test-selection-repetition-and-time-limits) of the UTM document.
   - **Windows CI notification process tree verification**: the `Verify Windows error notification process trees` step in the same `ci.yml`, and the [CI steps](windows.md#ci-steps) and [Checking CI results](windows.md#checking-ci-results) sections of the Windows document.

## `hookrec` process trees and delayed hooks

- For a long-running process, put `s.Hook("--depth=2")` in a start or change hook.
- The parent actually runs a child, and the child runs a grandchild.
- The generation identifier is the parent PID, and each process records `generation|PID|depth|state`.
- The parent's `ready` is recorded after the child's `ready`, so `AwaitReadyGenerations` judges the whole generation ready by the parent's `ready`.
- On a shutdown signal, a process waits for a 150ms handling delay and for its child's graceful exit, then writes `complete`.
- A delayed hook is made with `s.Hook("--delay=80ms", "--line=...")` and writes `BEGIN|PID|record`, plus `CANCEL|PID|record` when cancelled.
- With `--cancel-exit-code=<code>`, a delayed hook exits with that code after writing `CANCEL`; use it in a command list that starts the next program when the previous one fails, as with the shell `||`.
