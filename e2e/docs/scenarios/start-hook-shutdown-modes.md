# `TestStartHookShutdownModes`: a running start hook racing with changes and shutdown

🇺🇸 English | 🇰🇷 [한국어](start-hook-shutdown-modes.ko.md)

- While the start hook runs a real parent, child, and grandchild process tree, a change event and a shutdown signal arrive.
- Checks, for the two `run.interrupt` values `none` and `oldest`, that the start hook is kept or replaced, and that all processes complete gracefully on shutdown.

## What it checks and success criteria

- The start hook's generation becomes ready, and its processes are alive.
- With `run.interrupt: oldest`, the change event replaces the start hook and a second generation becomes ready.
- Right before shutdown, the processes of the last recorded generation, that is the running generation, are alive.
- The watcher exits with exit code 0 within 10 seconds of receiving the shutdown signal. Every process that recorded `ready` records `signal` and then `complete`, and every recorded process is gone within 5 seconds ([liveness check](../harness.md#process-liveness-and-forced-cleanup)).
- With `run.interrupt: none`, there must be no parent (depth 2) `start` record other than the first generation. The start hook does not finish until it receives the shutdown signal, and shutdown discards pending changes, so the change hook must never run.

## Configuration

- The target `onStartWatch` and `onChange` are set to the same `hookrec --depth=2` command. The record format and signal handling are described in [`hookrec` process trees and delayed hooks](../harness.md#hookrec-process-trees-and-delayed-hooks).
- The target options are the per-subtest `run.interrupt`, plus `hookBatch.waitMillisecond: 10`, `hookBatch.deliver: last`, `watch.fileEvent: C`, and `watch.exclude: ['*.probe']`. No finish hook is configured.

## Subtests

| Subtest | Change event handling | Extra waits and checks |
|---|---|---|
| `interrupt=none` | Last changed path after a 10ms wait, start hook kept | No parent `start` other than the first generation |
| `interrupt=oldest` | Last changed path after a 10ms wait, start hook cancelled and replaced | Wait for 2 generations ready |

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant watcherProc as watcher
    participant startTree as start hook tree
    participant watchDir as watch folder
    participant changeTree as change hook tree
    testCode->>watcherProc: Start (confirm watch readiness via DEBUG log)
    watcherProc->>startTree: run onStartWatch (parent, child, grandchild)
    testCode->>startTree: wait for 1 generation ready (up to 10s), check alive
    testCode->>watchDir: create during-start.txt
    watchDir-->>watcherProc: create event
    alt run.interrupt: oldest
        watcherProc->>startTree: cancel signal
        watcherProc->>changeTree: run onChange
        testCode->>changeTree: wait for 2 generations ready (up to 10s)
    else run.interrupt: none
        Note over watcherProc: start hook is running, so the change waits
    end
    testCode->>watcherProc: shutdown signal after checking the last generation is alive
    watcherProc-->>testCode: exit code 0 after cancelling the running hook (wait up to 10s)
    testCode->>testCode: check complete after signal for all processes
    testCode->>testCode: wait for absence of all recorded processes (up to 5s)
    opt run.interrupt: none
        testCode->>testCode: check there is no parent start other than the first generation
    end
```

- `during-start.txt` is created as an empty file.

## Running

Run these commands from the repository root, locally, in Linux Docker, and on the UTM Windows VM, in that order.

```sh
go test -count=1 -v -run '^TestStartHookShutdownModes$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestStartHookShutdownModes$'
make e2e-windows ARGS="-- -run '^TestStartHookShutdownModes$'"
```

- To run a single subtest, specify it as in `-run '^TestStartHookShutdownModes$/^interrupt=none$'`.
- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM with `make e2e-windows-up`.
- The commands that repeat the race scenarios together are in [Race and overload verification](../reliability.md#race-and-overload-verification).

## Related code

- `e2e/scenarios/process_test.go` contains `TestStartHookShutdownModes`.
- `e2e/harness`: `NewScenario`, `Start`, `Hook`, `Write`, `AwaitReadyGenerations`, `AssertRunning`, `ProcessRecords`, `Stop`, `AssertGracefulCompletion`, `AwaitProcessesGone`. Where each function lives and what it does is in [Code locations by responsibility](../harness.md#code-locations-by-responsibility).
