# Race, overload, and loss verification

🇺🇸 English | 🇰🇷 [한국어](reliability.ko.md)

## Race and overload verification

```sh
go test -count=3 -v -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown'
go test -race ./...
```

- What each scenario repeated by these commands checks, and its sequence, is in the [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.md), and [`TestRunInterrupt`](scenarios/run-interrupt.md) documents.
- The repeated scenarios also include [`TestQueueWarningCLI`](scenarios/queue-warning-cli.md) and [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md).
- [`TestBatchingOptions`](scenarios/batching-options.md) also repeats final-change ordering and maximum-wait flushes during continued inflow.

### Scope of the internal tests

- Controlled internal tests check, independently of the number of OS notifications, the exact order under `hookBatch.deliver: each`, the last valid path under `deliver: last`, separate runs for waits that end while a hook runs, excluded events, no replacement before cancellation completes, no runs after shutdown, and preservation of the first fatal error.
- The per-target pending limit is `queue.size` runs (2048 by default), with at most `run.concurrency` runs at a time (1 by default) and one `hookBatch` wait at a time. The tests that fill the queue set a small `queue.size` instead of relying on the default.
- When the queue is full, the watcher exits with a loss error instead of growing the buffer or waiting for room, including for runs made from events collected for folder events.
- Shutdown cancels and cleans up the queue rather than draining it.

- The controlled-input runner tests keep the real fsnotify watcher and its watch registration and replace only the receive channel.
- The related tests delete the temporary directory right after `Stop` completes to check that resources are released.
- `TestNativeWatcherStopReleasesDirectory` handles a real create event through the original channel, then checks the finish hook and directory deletion.
- `TestFilesystemPredicatesReleaseResources` checks both the existing bool results of the existence and directory predicates and deletion of the file and directory right after the calls.

### Receive boundary checks in OS E2E

- OS E2E does not decide the last received path from the order in which files were created.
- `e2e/harness/hook_records_test.go` checks a boundary where `normal-19.txt` was received after `last.txt`, rejection of empty records and unfinished hooks, and completion per duplicate receipt under `deliver: each`.
- The 120ms quiet period is an observation condition, not a guarantee that the OS will send no further notifications.
- The completion wait keeps waiting while the received events or the records grow, and fails when neither changes for 10 seconds.
  - The time it takes grows with the number of received events and the time one hook takes, so the total time has no fixed limit.
  - A received event whose hook leaves no record therefore fails the test 10 seconds after the last change.
  - `e2e/harness/harness_test.go` checks that the wait continues past the limit while progress continues and fails once progress stops.
- [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md) and [`TestBatchingOptions`](scenarios/batching-options.md) use this wait.
- If a late valid event replaces the isolated change, the test does not treat it as success and fails when the isolated change's record does not appear within 10 seconds.

## Event-loss notification verification

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI'
```

- What each scenario run by these commands checks, and its sequence, is in the [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md), [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md), and [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) documents.
  - `TestFastInflowSlowHook` checks that normal changes and the events merged by `deliver: last` do not run the notification.
- How notification descendants are cleaned up on macOS, Linux, and Windows is in [Descendant cleanup by platform](scenarios/event-loss-notification-cli.md#descendant-cleanup-by-platform).

- The internal YAML configuration test `go test -count=1 -run '^TestDecoded' ./internal/config/yaml` actually runs decoded structured commands and checks that environment variables are passed verbatim, the `dir` working directory, and compatibility with existing configurations.
- The notification's environment variables and its contract of a fixed 5-second run limit and at most 20 seconds of cancellation cleanup follow [Event-loss notification](../../docs/guide/running.md#event-loss-notification).

## Kinds of loss verification

- Real filesystem load tests allow OS notifications to be duplicated or merged. They do not assume that the number of file operations equals the number of hook runs.
- [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) checks how an error that arrives on the watcher error channel reaches the CLI, using a watcher built with the `dw_e2e_inject` build tag. A successful injection is not evidence of an observed real OS overflow.
  - In the four combinations of `hookBatch.deliver` `last`·`each` and `run.interrupt` `none`·`oldest`, an injected loss error must preserve the first fatal error, run the notification once, exit with code 1, and start no new normal hook after the notification.
  - An injected fatal error other than loss must exit with code 1 without running the notification.
  - Release binaries and the make build targets are built without this tag, so they have no injection point.
- The real Linux overflow experiment pauses fsnotify receiving, creates more files than the current `max_queued_events`, and then observes `ErrEventOverflow`. It does not change host or shared kernel settings.
- Windows also reports `ErrEventOverflow` when its notification buffer overflows, as shown in the [Windows backend](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_windows.go#L580-L584); a real Windows overflow has not been observed in this verification.
- macOS kqueue does not report this sentinel, so the same guarantee is not claimed there.
- The basis is fsnotify v1.10.1's [Linux overflow handling](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_inotify.go#L397-L401) and [common errors](https://github.com/fsnotify/fsnotify/blob/v1.10.1/fsnotify.go#L262-L270). Detection of every OS-level loss, lossless handling of unbounded input, and automatic recovery are not guaranteed. Arbitrary finish hooks have no separate time limit, and the process tree verification targets helper programs that handle SIGTERM or Windows CTRL+BREAK and wait for their descendants. On Windows, [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.md) also checks a program that `cmd` starts after the first CTRL+BREAK, and [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md) checks `powershell` hooks. The forced termination of processes still running 20 seconds after the first CTRL+BREAK is checked only by the unit test `TestRunCommandEndsProcessTreeAfterGrace` in `internal/process`, with the 20 seconds shortened to 1 second.

### Running the injection verification

Run the commands below from the repository root, locally, in Linux Docker, and on the UTM Windows VM, in that order.

```sh
go test -count=1 -v -run '^TestInjectedLossNotificationCLI$' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run '^TestInjectedLossNotificationCLI$'
make e2e-windows ARGS="-- -run '^TestInjectedLossNotificationCLI$'"
```

- The harness builds the watcher with the injection tag itself, so the commands need no separate build.
- The subtests and the success criteria of each are in the [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) document.
