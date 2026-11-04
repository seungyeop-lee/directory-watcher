# `TestBatchingOptions`: final changes and bounded batching

🇺🇸 English | 🇰🇷 [한국어](batching-options.ko.md)

- Checks four batching options through the actual watcher binary, OS events, and shell hooks.

## What it checks and success criteria

- `lastPerPath` produces one final change per path in the order of each path's last received change: a new file remains `C` after updating it, an existing file becomes `U`, a created-then-deleted file disappears, and deleting a folder drops its descendant's `D`.
  - The expected order comes from actual DEBUG receipts, not file-operation order; folder collection is disabled so it cannot hide descendant deletion before hook batching.
- With `dirBatch.waitMillisecond: 0`, the new folder and both contained files reach the hook, with one record per received `CREATE`, including duplicate receipts.
- Each `maxWaitMillisecond` subtest observes a flush while new files continue arriving with received gaps below 500ms; the first flush must occur 1700–3300ms after the initial operation with a configured maximum of 1800ms.
  - The -100ms/+1500ms allowance covers observation and shell scheduling; a gap reaching the 500ms quiet wait fails instead of being accepted as a maximum-wait flush.
  - Hook batching must produce a record during inflow; folder batching must send the initial folder `C` during inflow and forward a subsequently created contained file independently.
- Completion uses the harness's actual-receipt boundary and 120ms observed quiet period, not a fixed sleep or an assumed correspondence between file operations and OS notifications.
- All subtests stop the watcher gracefully with exit code 0 and require `PASS` without `SKIP` on macOS, Linux Docker, and UTM Windows.
- On 2026-11-04, Windows ARM64 repeated `lastPerPath` fails: duplicate folder `REMOVE` receipts leave a folder `D` with `IsDir: false` and a descendant `D`; the other three subtests pass. The test retains the documented contract rather than accepting this result.
- The contracts are [Hook batching](../../../docs/guide/running.md#waiting-and-merging-with-hookbatch) and [Folder event collection](../../../docs/guide/events.md#collecting-folder-events).

## Configuration

| Subtest | `dirBatch` wait / maximum (ms) | `hookBatch` wait / maximum (ms) | Delivery |
|---|---|---|---|
| `lastPerPath` | 0 / 0 | 1500 / 10000 | `lastPerPath` |
| `dirBatchWaitZero` | 0 / 0 | 0 / 0 | `each` |
| `hookBatchMaxWait` | 0 / 0 | 500 / 1800 | `last` |
| `dirBatchMaxWait` | 500 / 1800 | 0 / 0 | `each` |

- All subtests exclude `**/*.probe`, select file events `CUD`, and use the default folder events and sequential execution.
- Hooks record `CHANGE:<absolute path>`, except `lastPerPath`, which records `CHANGE:<event>,<isDir>,<absolute path>`.

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
    else hookBatchMaxWait or dirBatchMaxWait
        loop until first hook observed, with received gaps below 500ms
            testCode->>fs: create another file
            fs-->>watcher: CREATE receipt
        end
        watcher->>hook: flush at maximum wait during continued inflow
        testCode->>fs: create post-flush tail file
        watcher->>hook: forward tail file independently
        testCode->>hook: await received-event completion boundary
    end
    testCode->>watcher: stop
    watcher-->>testCode: exit 0
```

## Subtests

- `lastPerPath` checks final per-path changes, last-receipt order, transient removal, and deleted descendant filtering in one wait.
- `dirBatchWaitZero` checks forwarding of a folder and its contained files without folder collection.
- `hookBatchMaxWait` checks bounded hook waiting under continuous short-gap inflow.
- `dirBatchMaxWait` checks bounded folder collection and independent forwarding after its flush.

## Running

- Run from the repository root after preparing Linux Docker and the UTM VM as described in [UTM Windows E2E](../windows-utm.md).

```sh
go test -count=3 -v -run '^TestBatchingOptions$' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run '^TestBatchingOptions$'
make e2e-windows ARGS="-vm=Windows-E2E -- -count=3 -run '^TestBatchingOptions$'"
```

- This timing-sensitive scenario is included in the [race and overload repeat list](../reliability.md#race-and-overload-verification), not in loss notification or Windows graceful-lifecycle lists.

## Related code

- `e2e/scenarios/batching_test.go` contains `TestBatchingOptions`, its configuration, and DEBUG receipt parsing.
- `e2e/scenarios/helpers_test.go`: `mkdirAll` and `createEmpty` create folders and empty files.
- `e2e/harness` supplies `NewScenario`, `Start`, `Hook`, `Write`, `Await`, `AwaitLine`, `AwaitChangesComplete`, `Lines`, `Log`, `CountRecord`, and `Stop`; their responsibilities are in [Harness structure](../harness.md#code-locations-by-responsibility).
