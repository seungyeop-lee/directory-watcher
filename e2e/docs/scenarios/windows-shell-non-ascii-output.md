# `TestWindowsShellNonASCIIOutput`: non-ASCII output of Windows shell hooks and console output code page restore

🇺🇸 English | 🇰🇷 [한국어](windows-shell-non-ascii-output.ko.md)

- The test checks that non-ASCII characters printed by `cmd` and Windows PowerShell hooks reach the watcher's output unchanged as UTF-8, even when the console output code page is not UTF-8.

## What it checks and success criteria

- Before launching the watcher, the test sets the output code page of the console shared by the test process and the watcher to 437.
  - Code page 437 has no Korean characters and no `✓`, so a hook shell that prints in this code page turns them into `?`.
  - Each subtest restores the original code page when it ends.
- The global `onStartWatch` hook prints one line, `한글-줄 é ✓`.
- The whole stdout and stderr that the watcher writes to the pipe is valid UTF-8 and contains the line `한글-줄 é ✓` unchanged.
- After the watcher receives the stop signal and exits with code 0, the console output code page is back to 437.

## Configuration

- Only the global `onStartWatch` runs a hook; the watch target has no hooks.
- The watcher runs with `--silent` and `--log-level=INFO`.
  - `--silent` removes the command announcement lines, so `한글-줄 é ✓` in the output is the line printed by the hook.
  - Watch targets start only after the global `onStartWatch` finishes, so the `watch start:` log marks the end of the hook.
    - The test waits up to 2 minutes for `watch start:`. When other runs share the VM and load is high, Windows PowerShell once took 46 seconds to print the hook line, while a `cmd` hook at the same time finished within 1 second.

## Subtests

- `cmd` prints with `echo`.
- `powershell` prints with `Write-Output`.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant consoleCP as console
    participant watcherProc as watcher
    participant shellProc as hook shell
    testCode->>consoleCP: output code page 437
    testCode->>watcherProc: Launch --silent --log-level=INFO
    watcherProc->>consoleCP: output code page UTF-8
    watcherProc->>shellProc: global onStartWatch
    shellProc-->>watcherProc: 한글-줄 é ✓ (UTF-8)
    watcherProc-->>testCode: hook line, then watch start:
    testCode->>watcherProc: stop
    watcherProc->>consoleCP: restore output code page 437
    watcherProc-->>testCode: exit 0
    testCode->>testCode: whole output is UTF-8 with the hook line
    testCode->>consoleCP: output code page is 437
```

## Running

- Run from the repository root.
- These Windows-only shells use a Windows build tag; this scenario is not selected on macOS or Linux.

```sh
make e2e-windows ARGS="-- -run '^TestWindowsShellNonASCIIOutput$'"
```

- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM.

## Related code

- `e2e/scenarios/shell_windows_test.go` defines `TestWindowsShellNonASCIIOutput`.
- The test uses `NewScenario`, `Launch`, `WatcherBin`, `Await`, `Log`, and `Stop` from `e2e/harness`.
- The code that changes and restores the watcher's console output code page is in `cmd/console_windows.go`.
