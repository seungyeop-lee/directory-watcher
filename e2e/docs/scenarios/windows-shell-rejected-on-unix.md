# `TestWindowsShellRejectedOnUnix`: cmd rejection before any hook runs on Unix

🇺🇸 English | 🇰🇷 [한국어](windows-shell-rejected-on-unix.ko.md)

- The test checks that selecting `shell: cmd` on Unix fails before any hook runs.

## What it checks and success criteria

- The process exits within 10 seconds with exit code 1.
- The combined stdout and stderr, after trimming surrounding whitespace, is exactly two lines: `config: <path>` and then `Error: shell "cmd" runs only on Windows`.
  - `<path>` is the path of the `config.yml` that `Launch` passes with `-c`.
- The record file remains empty, so neither global nor target lifecycle commands ran.

## Configuration

- `shell: cmd` is set with global start, finish, error, and queue-warning hooks and target start, change, and finish hooks, all pointing to `hookrec`.
- The test uses `Launch` rather than `Start`, because rejection must happen before watch readiness; see [Shell that runs hook commands](../../../docs/guide/configuration.md#shell-that-runs-hook-commands).

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant watcherProc as watcher
    participant recordFile as records.log
    testCode->>watcherProc: Launch shell: cmd
    watcherProc->>watcherProc: validate shell before hooks
    watcherProc-->>testCode: exact error and exit 1
    testCode->>recordFile: assert no records
```

## Running

- Run from the repository root.
- This Unix-only shell rejection test is not selected on Windows.

```sh
go test -count=1 -v -run '^TestWindowsShellRejectedOnUnix$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestWindowsShellRejectedOnUnix$'
```

## Related code

- `e2e/scenarios/shell_unix_test.go` defines `TestWindowsShellRejectedOnUnix`.
- The test uses `NewScenario`, `Hook`, `Launch`, `Await`, `Exited`, `ExitErr`, `Log`, and `Lines` from `e2e/harness`.
