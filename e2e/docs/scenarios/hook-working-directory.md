# `TestHookWorkingDirectory`: hook execution locations with and without dir

🇺🇸 English | 🇰🇷 [한국어](hook-working-directory.ko.md)

- The test checks the actual working directories of global and target lifecycle hooks against the [execution location table](../../../docs/guide/configuration.md#command-execution-location).

## What it checks and success criteria

- Without `dir`, global start and finish hooks run in the program execution directory, and target start, change, and finish hooks run in the watched `path`.
- With `dir`, all five hooks run in the specified directory, whose name contains a space.
- Each hook records `WORKDIR|<directory>` immediately before its label; the complete ten-record sequence must match exactly.
- The target change hook completes within 10 seconds, and shutdown exits with code 0 within 10 seconds.

## Configuration

- Each lifecycle command uses structured `cmd` with `hookrec --workdir --line=<label>`.
- The target uses `watch.fileEvent: C` and `watch.exclude: ['*.probe']`; hooks use the OS default shell.
- The run directory is resolved before configuring paths so that OS aliases such as macOS `/var` and `/private/var` cannot change the spelling of an otherwise identical directory between lifecycle phases.

## Subtests

- `dir=false` checks the default locations.
- `dir=true` sets the same explicit directory for all five commands.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant watcherProc as watcher
    participant hookProc as hookrec
    participant recordFile as records.log
    testCode->>watcherProc: Start (confirm watch readiness via the readiness.probe event)
    watcherProc->>hookProc: global and target start hooks
    hookProc->>recordFile: record the working directory, then the label
    testCode->>recordFile: wait for TARGET:start (up to 10s)
    testCode->>watcherProc: create created.txt
    watcherProc->>hookProc: target change hook
    hookProc->>recordFile: record the working directory, then TARGET:change
    testCode->>recordFile: wait for TARGET:change (up to 10s)
    testCode->>watcherProc: shutdown signal
    watcherProc->>hookProc: target, then global finish hooks
    hookProc->>recordFile: record the working directories and labels
    watcherProc-->>testCode: exit code 0 (wait up to 10s)
    testCode->>recordFile: compare all ten records
```

## Running

Run these commands from the repository root, locally, in Linux Docker, and on the UTM Windows VM, in that order.

```sh
go test -count=1 -v -run '^TestHookWorkingDirectory$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestHookWorkingDirectory$'
make e2e-windows ARGS="-- -run '^TestHookWorkingDirectory$'"
```

- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM with `make e2e-windows-up`.

## Related code

- `e2e/scenarios/shell_test.go` contains `TestHookWorkingDirectory`.
- `e2e/harness`: `NewScenario`, `Hook`, `Start`, `Write`, `AwaitLine`, `Stop`, `Lines`. Where each function lives and what it does is in [Code locations by responsibility](../harness.md#code-locations-by-responsibility).
- `e2e/hookrec/main.go` implements the `--workdir` option, and `TestWorkingDirectoryBeforeLine` in `e2e/hookrec/main_test.go` checks it.
