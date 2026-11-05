# `TestPowershellEventVariables`: literal event values through Windows PowerShell

🇺🇸 English | 🇰🇷 [한국어](powershell-event-variables.ko.md)

- The test checks eight event variables and injection prevention through Windows PowerShell using the same file name as `TestGoldenPath`.

## What it checks and success criteria

- The eight variables in global before-change, target change, and global after-change hooks match the complete original values for create, update, and delete events.
- The file name contains spaces, `;`, `$(...)`, a single quote, `&`, and `%OS%`; no `pwned*` path may appear in the program or watched directory.
- The entire lifecycle sequence matches the golden sequence, including target then global finish hooks; shutdown exits with code 0.

## Configuration

- The test reuses `goldenConfig` with `shell: powershell` and prefixes hook commands with the PowerShell call operator `&`.
- The test waits up to 60s for each step: watching readiness, the target start hook, the after-change record of each event, and shutdown.
  - Under load from scenarios running in parallel on the VM, one Windows PowerShell hook took 14s to finish, while a `cmd` hook at the same time finished within 1s.
  - Each event runs the before-change, change, and after-change hooks in turn, each in its own PowerShell process.
- Templates remain outside quotes as specified in [Variables in commands](../../../docs/guide/configuration.md#variables-in-commands).

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant watcherProc as watcher
    participant hookProc as hookrec
    participant recordFile as records.log
    testCode->>watcherProc: Start PowerShell configuration, await readiness (60s)
    watcherProc->>hookProc: start hooks
    hookProc->>recordFile: lifecycle labels
    loop create, update, delete
    testCode->>watcherProc: act on goldenFileName
    watcherProc->>hookProc: before, change, after hooks with eight variables
    hookProc->>recordFile: literal event values
    testCode->>recordFile: await after-change record (60s)
    end
    testCode->>watcherProc: stop
    watcherProc-->>testCode: finish hooks and exit 0 (60s)
    testCode->>recordFile: exact sequence and no injected paths
```

## Running

- Run from the repository root.
- These Windows-only shells use a Windows build tag; this scenario is not selected on macOS or Linux.

```sh
make e2e-windows ARGS="-- -run '^TestPowershellEventVariables$'"
```

- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM.

## Related code

- `e2e/scenarios/shell_windows_test.go` defines `TestPowershellEventVariables`.
- `e2e/scenarios/helpers_test.go`: `goldenConfig` and `goldenFileName` are the configuration and file name shared with `TestGoldenPath`, and `changeValues` and `expectedSequence` build the expected records.
- The test uses `NewScenario`, `StartWithin`, `Write`, `AwaitLine`, `StopWithin`, and `Lines` from `e2e/harness`.
