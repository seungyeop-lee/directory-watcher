# `TestBatchingOptions`: final per-path changes and zero folder wait

🇺🇸 English | 🇰🇷 [한국어](batching-options.ko.md)

- Checks `hookBatch.deliver: lastPerPath` and `dirBatch.waitMillisecond: 0` through the actual watcher binary, OS events, and shell hooks.
- Maximum waits during continued inflow are checked by `TestMaxWaitSendsRunWhileEventsKeepArriving` in `internal/app/service/runner_watch_target_test.go`.

## What it checks and success criteria

- `lastPerPath` produces one final change per path in the order of each path's last received change: a new file remains `C` after updating it, an existing file becomes `U`, a created-then-deleted file disappears, and deleting a folder drops its descendant's `D`.
  - The expected order comes from actual DEBUG receipts, not file-operation order; folder collection is disabled so it cannot hide descendant deletion before hook batching.
  - After each action, the test confirms within 5 seconds that the DEBUG log received that path before moving to the next action.
  - The test waits up to 15 seconds until the records equal the three final changes and neither the log nor the records change for 120ms.
- With `dirBatch.waitMillisecond: 0`, the new folder and both contained files reach the hook, with one record per received `CREATE`, including duplicate receipts.
  - Completion follows [Receive boundary checks in OS E2E](../reliability.md#receive-boundary-checks-in-os-e2e), so a received `CREATE` whose hook leaves no record fails the subtest after 10 seconds.
- All subtests stop the watcher gracefully with exit code 0 and require `PASS` without `SKIP` on macOS, Linux Docker, and UTM Windows.
- Repeated runs on Windows ARM64 sometimes fail `lastPerPath`: after duplicate folder `REMOVE` receipts, the folder `D` has `IsDir` `false` and a descendant `D` remains. The test retains the documented contract rather than accepting this result.
- The contracts are [Hook batching](../../../docs/guide/running.md#waiting-and-merging-with-hookbatch) and [Folder event collection](../../../docs/guide/events.md#collecting-folder-events).

## Configuration

| Subtest | `dirBatch` wait / maximum (ms) | `hookBatch` wait / maximum (ms) | Delivery |
|---|---|---|---|
| `lastPerPath` | 0 / 0 | 1500 / 10000 | `lastPerPath` |
| `dirBatchWaitZero` | 0 / 0 | 0 / 0 | `each` |

- All subtests exclude `**/*.probe`, select file events `CUD`, and use the default folder events and sequential execution.
- Hooks record `CHANGE:<absolute path>`, except `lastPerPath`, which records `CHANGE:<event>,<isDir>,<absolute path>`.

## Subtests

- `lastPerPath` checks final per-path changes, last-receipt order, transient removal, and deleted descendant filtering in one wait.
- `dirBatchWaitZero` checks forwarding of a folder and its contained files without folder collection.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant fs as filesystem
    participant watcher as watcher
    participant hook as shell / hookrec
    testCode->>watcher: Start and observe excluded readiness probe
    alt lastPerPath
        testCode->>fs: create/update files, remove transient and folder tree
        fs-->>watcher: actual received changes
        watcher-->>testCode: DEBUG receive order
        watcher->>hook: final C, U and folder D in last-receipt order
        testCode->>hook: compare exact records; no transient or child D
    else dirBatchWaitZero
        testCode->>fs: create folder, observe nested probe, create contained files
        fs-->>watcher: folder and file CREATE receipts
        watcher->>hook: forward each received CREATE
        testCode->>hook: await received-event completion boundary
    end
    testCode->>watcher: stop
    watcher-->>testCode: exit 0
```

## Running

The commands run from the repository root locally, in Linux Docker, and on the UTM Windows VM, in that order.

```sh
go test -count=1 -v -run '^TestBatchingOptions$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestBatchingOptions$'
make e2e-windows ARGS="-- -run '^TestBatchingOptions$'"
```

- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM with `make e2e-windows-up`.
- To run a single subtest, specify it as in `-run '^TestBatchingOptions$/^lastPerPath$'`.
- The commands that repeat the race scenarios together are in [Race and overload verification](../reliability.md#race-and-overload-verification).

## Related code

- `e2e/scenarios/batching_test.go` contains `TestBatchingOptions`, `batchConfig`, which builds the configuration, and `batchReceipts`, which reads the DEBUG receive order.
- `e2e/scenarios/helpers_test.go` contains `mkdirAll` and `createEmpty`, which create folders and empty files.
- `e2e/harness`: `NewScenario`, `Start`, `Hook`, `Write`, `Await`, `AwaitChangesComplete`, `Lines`, `Log`, `CountRecord`, `Stop`. Where each function lives and what it does is in [Code locations by responsibility](../harness.md#code-locations-by-responsibility).
