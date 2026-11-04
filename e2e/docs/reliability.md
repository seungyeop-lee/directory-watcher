# Race, overload, and loss verification

🇺🇸 English | 🇰🇷 [한국어](reliability.ko.md)

## Race and overload verification

```sh
go test -count=3 -v -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle'
go test -race ./...
```

- What each scenario repeated by these commands checks, and its sequence, is in the [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.md), [`TestTargetsRunIndependently`](scenarios/targets-run-independently.md), [`TestBoundedOverloadCLI`](scenarios/bounded-overload-cli.md), and [`TestProcessLifecycle`](scenarios/process-lifecycle.md) documents.

### Scope of the internal tests

- Controlled internal tests check, independently of the number of OS notifications, the exact order under `hookBatch.deliver: each`, the last valid path under `deliver: last`, separate runs for waits that end while a hook runs, excluded events, no replacement before cancellation completes, no runs after shutdown, and preservation of the first fatal error.
- The per-target pending limit is `queue.size` runs (2048 by default), with one running generation and one `hookBatch` wait at a time. The tests that fill the queue set a small `queue.size` instead of relying on the default.
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
- If a late valid event replaces the isolated change, the test does not treat it as success and fails within the completion wait time limit shared by the batch observation and the isolated change.
- The time limit is 30 seconds for [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.md) and 10 seconds for [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.md).

## Event-loss notification verification

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify'
```

- What each scenario run by these commands checks, and its sequence, is in the [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.md) and [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.md) documents.
- How notification descendants are cleaned up on macOS, Linux, and Windows is in [Descendant cleanup by platform](scenarios/event-loss-notification-cli.md#descendant-cleanup-by-platform).

- The internal YAML configuration test `go test -count=1 -run '^TestDecoded' ./internal/config/yaml` actually runs decoded structured commands and checks that environment variables are passed verbatim, the `dir` working directory, and compatibility with existing configurations.
- The notification's environment variables and its contract of a fixed 5-second run limit and at most 20 seconds of cancellation cleanup follow [Event-loss notification](../../USAGE.md#event-loss-notification).

## Kinds of loss verification

- Real filesystem load tests allow OS notifications to be duplicated or merged. They do not assume that the number of file operations equals the number of hook runs.
- Error propagation up to the CLI can be checked separately by injecting an fsnotify loss error with a verification-only source overlay. It confirms, in the four `hookBatch.deliver`·`run.interrupt` combinations, that the first fatal error is preserved, the notification runs once, and new normal hooks of other targets are blocked. The overlay run commands and results are kept in the issue report, and release binaries have no injection option. A successful injection is not evidence of an observed real OS overflow.
- The real Linux overflow experiment pauses fsnotify receiving, creates more files than the current `max_queued_events`, and then observes `ErrEventOverflow`. It does not change host or shared kernel settings. macOS kqueue does not report this sentinel, so the same guarantee is not claimed there.
- The basis is fsnotify v1.7.0's [event and error contract](https://github.com/fsnotify/fsnotify/blob/v1.7.0/backend_inotify.go) and [common errors](https://github.com/fsnotify/fsnotify/blob/v1.7.0/fsnotify.go). Detection of every OS-level loss, lossless handling of unbounded input, and automatic recovery are not guaranteed. Arbitrary finish hooks have no separate time limit, and the process tree verification targets helper programs that handle SIGTERM or Windows CTRL+BREAK and wait for their descendants. On Windows, [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.md) also checks a program that `cmd` starts after the first CTRL+BREAK, and [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.md) checks `powershell` hooks. The forced termination of processes still running 20 seconds after the first CTRL+BREAK is checked only by the unit test `TestRunCommandEndsProcessTreeAfterGrace` in `internal/helper`, with the 20 seconds shortened to 1 second.

### Reproducing the injection smoke test

- Reproduce the injection smoke test from the issue report with the commands below.

Replace `<issue report directory>` with the path that holds `smoke.sh` and the overlay test file.

```sh
sh <issue report directory>/smoke.sh macos
sh <issue report directory>/smoke.sh linux
```

- `TestControlledLossNotificationSmoke` checks the four option combinations and that fatal errors other than loss do not trigger the notification.
- The script uses a Go overlay and temporary constructor injection and does not modify the repository source.
