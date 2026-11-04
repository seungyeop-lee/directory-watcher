# `TestFolderEvents`: events for folder and file actions

🇺🇸 English | 🇰🇷 [한국어](folder-events.ko.md)

- Runs folder and file actions one after another in the watch folder and checks that the hook receives, for each action, exactly the events in the [Folder events](../../../docs/guide/events.md#folder-events) table of the usage guide, including one event for the top folder when a folder appears or disappears with its contents.

## What it checks and success criteria

- Each record is `<Event>,<IsDir>,<path relative to the watch folder>`.
- After each action, the records up to the sentinels of that action must be exactly the list below, in that order.
- A missing record, an extra record, or a different order fails the test.

| Action | Expected records |
|---|---|
| Create `src/pkg` | `C,true,src/pkg` |
| Create `src/a/b/c` at once | `C,true,src/a` |
| Move `x` with 3 files in from outside the watch folder as `src/x` | `C,true,src/x` |
| Create `src/n` and write `src/n/a.txt` right after | `C,true,src/n` |
| Move `big` with 300 files in from outside as `src/big` | `C,true,src/big` |
| Delete `src/big` with everything in it | `D,true,src/big` |
| Move `src/x` out of the watch folder | `D,true,src/x` |
| Rename `src/n` to `src/m` | `D,true,src/n`, `C,true,src/m` |
| Move `src/m` to `dst/m` | `D,true,src/m`, `C,true,dst/m` |
| Create the empty file `src/f.txt` | `C,false,src/f.txt` |
| Rename `src/f.txt` to `src/g.txt` | `D,false,src/f.txt`, `C,false,src/g.txt` |
| Delete `src/g.txt` | `D,false,src/g.txt` |

- The folders that are moved or renamed hold only files, because Windows refuses to rename or move a folder that holds a watched subfolder.
- The watcher must exit with exit code 0 within 10 seconds of receiving the shutdown signal.

## Configuration

- The target `onChange` hook records `CHANGE:{{.Event}},{{.IsDir}},{{.Path}}` with `hookrec`.
- The target options are `hookBatch.waitMillisecond: 0`, `hookBatch.deliver: each`, and `watch.exclude: ['*.probe']`. `watch.fileEvent` and `watch.dirEvent` keep their defaults, `CUD` and `CD`.
- Before watching starts, the test creates `src` and `dst` in the watch folder, and `outside/x` with 3 files and `outside/big` with 300 files in the run folder.

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant testCode as test
    participant watcherProc as watcher
    participant hookRec as hookrec
    participant recordFile as records.log
    participant watchDir as watch folder
    testCode->>watcherProc: Start (confirm watch readiness via DEBUG log)
    loop once for each action in the table
        testCode->>watchDir: run the action
        testCode->>watchDir: create a sentinel file in the watch folder, src, and dst
        watchDir-->>watcherProc: folder and file events
        watcherProc->>hookRec: onChange for the folder event and the other events in order
        hookRec->>recordFile: one line per event
        testCode->>recordFile: wait for the three sentinel records (up to 10s)
        testCode->>testCode: compare the records before the sentinels with the expected records
    end
    testCode->>watcherProc: shutdown signal
    watcherProc-->>testCode: exit code 0 (wait up to 10s)
```

- On Windows, each watched folder reports its events separately, so the test waits for a sentinel in every folder where the action makes events.
- Sentinel files are created empty with `O_CREATE|O_EXCL|O_RDONLY`, so they make only a create event on every OS.

## Running

- Run from the repository root.

The commands run locally, in Linux Docker, and on the UTM Windows VM, in that order.

```sh
go test -count=1 -v -run '^TestFolderEvents$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestFolderEvents$'
make e2e-windows ARGS="-- -run '^TestFolderEvents$'"
```

- Run the UTM command after completing the prerequisites in [Running E2E on a UTM Windows VM](../windows-utm.md) and starting the VM with `make e2e-windows-up`.
- To run directly on a Windows host, see [Windows run commands](../windows.md#run-commands).

## Related code

- `e2e/scenarios/folder_events_test.go`: `TestFolderEvents`, and `relativeChange`, which turns a record into a path relative to the watch folder
- `e2e/harness`: `Start`, `Hook`, `Await`, `Lines`, `Stop`. Where each function lives and what it does is in [Code locations by responsibility](../harness.md#code-locations-by-responsibility).
