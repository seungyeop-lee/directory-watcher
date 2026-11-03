# Directory Watcher

🇺🇸 English | 🇰🇷 [한국어](README.ko.md)

Directory Watcher runs the commands you define when files are created, modified, or deleted in the directories you watch. Use it to rebuild or test code as you edit it, restart a development server when its sources change, or back up files when they change.

## Features

### Run commands when files change

- Watch several directories at once. Each watch target has its own `onChange` commands, and the global `onBeforeChange` and `onAfterChange` commands run before and after every target's `onChange`.
- Choose which events to react to (create, update, delete), and exclude directories, file suffixes, or subdirectories from watching.
- Pass the changed file's path, name, extension, directory, and event type to a command with [template variables](USAGE.md#variables-in-commands).

### Handle bursts of changes

- By default, a burst of events runs the hook once, `waitMillisecond` (100 ms by default) after the last event.
- With `noWait: true`, every event runs the hook, in the order the events were received.
- Each target holds at most 256 pending events. Instead of growing without bound, the watcher stops when a target's queue is full.

### Restart long-running processes

- With `interruptible: true`, a new change cancels the running commands, waits for them to exit, and then runs the change commands again. Use it for servers and other processes that do not exit on their own.
- When you stop the watcher, it cancels running commands and then runs the finish commands.

### Run setup and cleanup commands

- `onStartWatch` and `onFinishWatch` run when watching starts and stops, both globally and for each target.

### Get notified when events may be lost

- When a target's queue fills up or the OS reports an event overflow, the optional global `onError` command runs once, with the details in `DW_ERROR_*` environment variables. See [Event-loss notification](USAGE.md#event-loss-notification).

## Requirements

- macOS or Linux. The program has been verified to work correctly on these systems.
- Windows, with the limits below.
- Go 1.23 or later, only if you install with `go install`.

> [!WARNING]
> On Windows, standard output may have issues in PowerShell or Command Prompt.
>
> On Windows, the watcher stops a running command by sending CTRL+BREAK to that command's process group. The Windows E2E (Windows 11 ARM64 on a UTM VM, run on 2026-11-03) runs the watcher without Git Bash and confirms, for `interruptible: true` replacement and for shutdown, that a hook's parent, child, and grandchild processes terminate when they stay in that group and exit on the signal ([`TestProcessLifecycle`](e2e/docs/scenarios/process-lifecycle.md), [`TestStartHookShutdownModes`](e2e/docs/scenarios/start-hook-shutdown-modes.md)). A descendant that creates its own console or process group, or ignores the signal, may keep running.

## Installation

### Homebrew

```shell
brew install seungyeop-lee/tap/directory-watcher
```

### `go install`

```shell
go install github.com/seungyeop-lee/directory-watcher/v2@latest
```

### In Dockerfile

```dockerfile
COPY --from=ghcr.io/seungyeop-lee/directory-watcher:latest /directory-watcher /usr/local/bin/
```

### Releases

Download the executable from the [releases page](https://github.com/seungyeop-lee/directory-watcher/releases/latest).

## Getting started

1. Create `config.yml` in the directory where you will run the watcher. This example runs `make test` in that directory whenever a file under `src` changes:

   ```yaml
   watchTargets:
     - path: src
       lifeCycle:
         onChange:
           - dir: .
             cmd: make test
   ```

2. Start the watcher in the same directory:

   ```shell
   directory-watcher
   ```

   To use a config file elsewhere, pass its path with `-c <config path>`.

3. Change a file under `src`. `make test` runs 100 ms after the last change.

- **Stop:** press Ctrl+C or send SIGTERM. The watcher cancels running commands, runs the finish commands, and exits.
- **More:** the [usage guide](USAGE.md) covers command-line options, running with Docker, the full `config.yml` format, where commands run, template variables, event-loss notification, and the limits of overload, shutdown, and folder events. For a fuller example, see [`config.example.yml`](config.example.yml).

## How it works

Global `onStartWatch` runs first. Then each watch target starts on its own and handles its changes independently of the other targets:

```mermaid
flowchart TD
    globalStart["global onStartWatch"]
    subgraph eachTarget["Each watch target, independently"]
        watchDirs["Start watching the target directory"]
        targetStart["target onStartWatch"]
        receiveEvent(("Event received"))
        filterEvent{"Watched event type and<br>not an excluded suffix?"}
        queueEvent{"Room in the target's<br>256-event queue?"}
        modeChoice{"noWait?"}
        debounceWait["Wait waitMillisecond after the last event<br>and keep only that event"]
        orderedRun["Take events in received order"]
        interruptChoice{"interruptible?"}
        cancelPrevious["Cancel the running commands<br>and wait for them to exit"]
        runChange["global onBeforeChange → onChange<br>→ global onAfterChange"]
    end
    fatalStop(["Fatal error: see the next diagram"])

    globalStart --> watchDirs
    watchDirs --> targetStart
    targetStart --> receiveEvent
    receiveEvent -->|"a new directory joins the watch list"| filterEvent
    filterEvent -->|"no: ignore"| receiveEvent
    filterEvent -->|"yes"| queueEvent
    queueEvent -->|"no"| fatalStop
    queueEvent -->|"yes"| modeChoice
    modeChoice -->|"false"| debounceWait
    modeChoice -->|"true"| orderedRun
    debounceWait --> interruptChoice
    orderedRun --> interruptChoice
    interruptChoice -->|"true"| cancelPrevious
    interruptChoice -->|"false: wait until the running commands finish"| runChange
    cancelPrevious --> runChange
    runChange -->|"next event"| receiveEvent
```

- With `interruptible: false`, events that arrive while target `onStartWatch` or a change is running wait in the queue until it finishes. With `interruptible: true`, the next change cancels a running target `onStartWatch` too.
- The commands of one run execute one after another, and the run stops at the first command that fails. The failure is logged as an `[ERROR]` line; see [`config.yml`](USAGE.md#configyml) in the usage guide.

A shutdown signal and a fatal error both stop every target before the finish commands run:

```mermaid
flowchart TD
    shutdownSignal(("SIGINT or SIGTERM"))
    fatalError(("Fatal error in a target:<br>queue full, OS overflow,<br>watcher error, missing path"))
    stopTargets["All targets: stop starting new commands<br>and cancel running ones"]
    targetFinish["Each target: discard pending changes,<br>wait for running commands to exit,<br>then run target onFinishWatch"]
    lossCheck{"Possible event loss?<br>queue full or OS overflow"}
    errorHook["global onError, once<br>5 s limit, then up to 20 s of cleanup"]
    globalFinish["global onFinishWatch"]
    exitCheck{"Fatal error?"}
    exitSuccess(("Exit code 0"))
    exitFailure(("Exit code 1"))

    shutdownSignal --> stopTargets
    fatalError --> stopTargets
    stopTargets --> targetFinish
    stopTargets -->|"after a fatal error"| lossCheck
    lossCheck -->|"yes"| errorHook
    lossCheck -->|"no"| globalFinish
    targetFinish --> globalFinish
    errorHook --> globalFinish
    globalFinish --> exitCheck
    exitCheck -->|"no"| exitSuccess
    exitCheck -->|"yes"| exitFailure
```

- Target finish commands may run while `onError` is still running. Global `onFinishWatch` waits for both.
- Finish commands have no time limit of their own.

## Development checks

See the [E2E guide (Korean)](e2e/README.md) for automated macOS, Linux, and Windows checks against the current worktree and instructions for adding scenarios.

## License

This project is licensed under the [GPL-3.0 License](LICENSE). See the license file for details.
