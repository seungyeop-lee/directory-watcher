# Race, overload, and loss verification

🇺🇸 English | 🇰🇷 [한국어](reliability.ko.md)

## Race and overload verification

```sh
go test -count=3 -v -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown'
go test -count=1 -race ./...
```

- What each scenario repeated by these commands checks, and its sequence, is in the [`TestBatchingOptions`](scenarios/batching-options.md), [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.md), [`TestRunInterrupt`](scenarios/run-interrupt.md), [`TestQueueWarningCLI`](scenarios/queue-warning-cli.md), and [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.md) documents.
- The contract for the queue limit, the number of concurrent runs, and dropping pending changes on shutdown is in [Overload and shutdown](../../docs/guide/running.md#overload-and-shutdown).

### Scope of the internal tests

The behaviors below need exact results that do not depend on the number or order of OS notifications, so unit tests with controlled event input check them instead of the E2E.

| Behavior | Test |
|---|---|
| Receive order under `hookBatch.deliver: each` and the last valid path under `deliver: last` | `TestControlledModesPreservePolicy` in `internal/app/service/runner_watch_target_test.go`, `TestDeliverTurnsOneWaitIntoRuns` in `runner_watch_target_hook_batch_test.go` |
| A separate run for each wait that ends while a hook runs | `TestRunsFromSeparateWaitsAreNotMerged` in `internal/app/service/runner_watch_target_test.go` |
| The maximum wait of `hookBatch` and `dirBatch` while events keep arriving | `TestMaxWaitSendsRunWhileEventsKeepArriving` in `internal/app/service/runner_watch_target_test.go` |
| Events of excluded paths | `TestExcludePatternDropsFileEvents` in `internal/app/service/runner_watch_target_test.go` |
| No replacement before cancellation completes | `TestReplacementWaitsForCanceledGeneration` in `internal/app/service/hook_runner_test.go` |
| No runs after shutdown, and pending changes dropped | `TestShutdownDuringReplacementPreventsNewGeneration` and `TestFinishCancelsAllRunsBeforeFinishHook` in `internal/app/service/hook_runner_test.go`, `TestStopDiscardsPendingEvents` in `runner_watch_target_test.go` |
| Preservation of the first fatal error | `TestRunnerPreservesOnlyFirstFatalError` in `internal/app/service/runner_test.go` |
| The loss error when the queue is full | `TestQueueFullRejectsRun` in `internal/app/service/runner_watch_target_queue_test.go`, `TestBoundedDispatchReportsLoss` and `TestBatchLargerThanQueueOverflows` in `runner_watch_target_test.go` |
| Release of the watched directory after `Stop` | `TestNativeWatcherStopReleasesDirectory` in `internal/app/service/runner_watch_target_test.go` |
| Release of files and directories after the existence and directory predicates | `TestFilesystemPredicatesReleaseResources` in `internal/helper/os_test.go` |

### Receive boundary checks in OS E2E

- OS E2E does not decide the last received path from the order in which files were created, but from the events the watcher's DEBUG log shows as actually received.
  - `TestReceivedChangesFollowWatcherOrder` and `TestChangesCompletedUsesReceivedBoundary` in `e2e/harness/hook_records_test.go` check these functions.
- The 120ms quiet period is an observation condition, not a guarantee that the OS will send no further notifications.
- The completion wait keeps waiting while the received events or the records grow, and fails when neither changes for 10 seconds.
  - The time it takes grows with the number of received events and the time one hook takes, so the total time has no fixed limit.
  - A received event whose hook leaves no record therefore fails the test 10 seconds after the last change.
  - `TestAwaitProgressWaitsWhileProgressContinues`, `TestAwaitProgressFailsWhenProgressStops`, and `TestAwaitProgressWithoutProgressMatchesFixedWait` in `e2e/harness/harness_test.go` check the progress-based wait.
- [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md) and [`TestBatchingOptions`](scenarios/batching-options.md) use this wait.

## Event-loss notification verification

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI'
```

- What each scenario run by these commands checks, and its sequence, is in the [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md), [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md), and [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) documents.
  - `TestFastInflowSlowHook` checks that normal changes and the events merged by `deliver: last` do not run the notification.
- How notification descendants are cleaned up on macOS, Linux, and Windows is in [Descendant cleanup by platform](scenarios/event-loss-notification-cli.md#descendant-cleanup-by-platform).
- The contract for the notification's environment variables and its run and cleanup time limits is in [Event-loss notification](../../docs/guide/running.md#event-loss-notification).
- The tests whose names start with `TestDecoded` in `internal/config/yaml/config_test.go` check that a decoded structured `onError` command receives environment variables verbatim, runs in its `dir` working directory, and stays compatible with existing configurations.

## Kinds of loss verification

- Real filesystem load tests allow OS notifications to be duplicated or merged, so they do not assume that the number of file operations equals the number of hook runs.
- The E2E does not cause a real OS overflow; the loss notification scenarios create their errors in the two ways below.
  - [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) actually fills the application's dispatch queue and causes a `dispatch queue full` error.
  - [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.md) injects errors into the watcher error channel of a watcher built with the `dw_e2e_inject` build tag and checks that they reach the CLI's notification and exit code.
- A pass of these two scenarios is not evidence of an observed real OS overflow.
- OS overflow is judged by the `ErrEventOverflow` that fsnotify v1.10.1 reports ([common errors](https://github.com/fsnotify/fsnotify/blob/v1.10.1/fsnotify.go#L262-L270)).
  - Linux reports it when the inotify queue overflows ([Linux overflow handling](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_inotify.go#L397-L401)).
  - Windows reports it when its notification buffer overflows ([Windows backend](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_windows.go#L580-L584)).
  - macOS kqueue does not report it, so OS-level loss detection is not claimed on macOS.
- What the application does not guarantee is in [Overload and shutdown](../../docs/guide/running.md#overload-and-shutdown), and the tests that check how canceled hooks end on Windows are in [Hook cancellation checks](windows.md#hook-cancellation-checks).
