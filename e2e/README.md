# Automated E2E runs

🇺🇸 English | 🇰🇷 [한국어](README.ko.md)

- These runs build the watcher and the Go helper programs from the actual current source and check file events, hook records, and process termination.
- macOS local, Linux Docker, and Windows runs execute the same Go E2E scenarios in `e2e/scenarios`.
- The Windows-only harness checks `TestCleanupReportsLockedResources`, `TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram`, `TestDelayedHooksGoneFailsWhileHookRuns`, and `TestDLLInitFailureReportFromWatcher` are in the `e2e/harness` package, so they run with `go test ./...`, not with `make e2e-local`.
- With `go test -short`, every test that creates its run with `NewScenario` is skipped, and `harness.Run` does not build the binaries.
- What each scenario checks and where its code is are in [Scenarios and their supporting code](docs/harness.md#scenarios-and-their-supporting-code).

## Setup and run

- Local runs need Go at or above the version in `go.mod`.
- Linux runs also need the Docker CLI and a running Linux Docker engine.
- Java and Gradle are not used.

Run these from the repository root.

```sh
make e2e-local
make e2e-linux
```

To run on a UTM Windows VM from a Mac, finish the prerequisites in [Running E2E on a UTM Windows VM](docs/windows-utm.md), then use the following command.

```sh
DW_E2E_UTM_VM=Windows-E2E make e2e-windows-all
```

- On success, each test shows `PASS` and the command exits with 0.
- Failures and timeouts are reported as a non-zero exit code.
- The default Linux image is `golang:1.27-bookworm`; specify another Go image with `go run ./e2e/linux -image=<image>`.

### Selecting and repeating specific scenarios

```sh
go test -count=3 -v -run '^TestGoldenPath$' ./e2e/scenarios
go test -count=3 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run '^TestGoldenPath$'
go run ./e2e/linux -- -count=3 -run '^TestProcessLifecycle$'
```

- Every run specifies `-count` so that the Go test result cache is not used.
- The Linux runner appends the passed arguments after its default `-count=1`, so the repeat counts above apply.
- Set the time limit for the whole container as in `go run ./e2e/linux -deadline=10m -- -run TestProcessLifecycle`.
- Set the time limit for the Go tests themselves with `-timeout=2m` after `--`.

## Detailed documents

| Document | Contents |
|---|---|
| [Harness structure and adding scenarios](docs/harness.md) | Code locations by responsibility, scenarios and their supporting code, steps and an example for writing a new scenario, `hookrec` options, process trees, and delayed hooks |
| Per-scenario documents: [`TestGoldenPath`](docs/scenarios/golden-path.md), [`TestFolderEvents`](docs/scenarios/folder-events.md), [`TestWatchOptions`](docs/scenarios/watch-options.md), [`TestBatchingOptions`](docs/scenarios/batching-options.md), [`TestProcessLifecycle`](docs/scenarios/process-lifecycle.md), [`TestStartHookShutdownModes`](docs/scenarios/start-hook-shutdown-modes.md), [`TestRunInterrupt`](docs/scenarios/run-interrupt.md), [`TestConcurrentRunsShutdown`](docs/scenarios/concurrent-runs-shutdown.md), [`TestFastInflowSlowHook`](docs/scenarios/fast-inflow-slow-hook.md), [`TestTargetsRunIndependently`](docs/scenarios/targets-run-independently.md), [`TestOverlappingTargets`](docs/scenarios/overlapping-targets.md), [`TestBoundedOverloadCLI`](docs/scenarios/bounded-overload-cli.md), [`TestEventLossNotificationCLI`](docs/scenarios/event-loss-notification-cli.md), [`TestNormalChangesDoNotNotify`](docs/scenarios/normal-changes-do-not-notify.md), [`TestInjectedLossNotificationCLI`](docs/scenarios/injected-loss-notification-cli.md), [`TestQueueWarningCLI`](docs/scenarios/queue-warning-cli.md), [`TestCanceledCommandListStopsNextProgram`](docs/scenarios/canceled-command-list-stops-next-program.md), [`TestCanceledPowershellHookStops`](docs/scenarios/canceled-powershell-hook-stops.md), [`TestHookWorkingDirectory`](docs/scenarios/hook-working-directory.md), [`TestPowershellEventVariables`](docs/scenarios/powershell-event-variables.md), [`TestWindowsShellEmptyArgument`](docs/scenarios/windows-shell-empty-argument.md), [`TestWindowsShellNonASCIIOutput`](docs/scenarios/windows-shell-non-ascii-output.md), [`TestWindowsShellRejectedOnUnix`](docs/scenarios/windows-shell-rejected-on-unix.md) | For each scenario, what it checks and success criteria, configuration, sequence diagram, subtests, run commands, and related code |
| [Running E2E on a UTM Windows VM](docs/windows-utm.md) | UTM Windows VM prerequisites on a Mac and the dedicated E2E VM, `make e2e-windows-up`, `e2e-windows`, `e2e-windows-down`, and `e2e-windows-all`, result locations, shutdown behavior, troubleshooting |
| [Running Windows E2E and checking CI results](docs/windows.md) | Windows run commands, graceful shutdown with CTRL+BREAK, CI steps and checking artifacts, Windows failure analysis |
| [Failure and timeout diagnostics and isolation](docs/diagnostics.md) | Diagnostics file locations, intentional failures and timeouts with `DW_E2E_FAULT`, Linux diagnostics, parallel runs across worktrees and container cleanup |
| [Race, overload, and loss verification](docs/reliability.md) | Repeat commands for the race, overload, and loss notification scenarios, scope of the internal tests, receive boundary checks in OS E2E, kinds of loss verification and the injection smoke test |
