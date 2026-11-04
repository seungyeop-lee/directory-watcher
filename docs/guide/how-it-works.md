# How it works

🇺🇸 English | 🇰🇷 [한국어](how-it-works.ko.md)

- Global `onStartWatch` runs first.

Then each watch target starts on its own and handles its changes independently of the other targets:

```mermaid
flowchart TD
    globalStart["global onStartWatch"]
    subgraph eachTarget["Each watch target, independently"]
        watchDirs["Start watching the target directory"]
        targetStart["target onStartWatch"]
        receiveEvent(("Event received"))
        excludeCheck{"Excluded path?"}
        collectChoice{"Folder created or deleted,<br>or file deleted in a subfolder?"}
        collectFolder["Collect the target's events<br>and absorb those inside the folder"]
        waitChoice{"hookBatch.waitMillisecond<br>is 0?"}
        collectHook["Collect events until hookBatch.waitMillisecond<br>passes with no event or hookBatch.maxWaitMillisecond ends"]
        mergeRuns["Merge them into runs by hookBatch.deliver,<br>keeping types chosen in watch.fileEvent or watch.dirEvent"]
        queueEvent{"Room in the target's queue<br>of queue.size runs?"}
        queueWarning["global onQueueWarning,<br>apart from event handling"]
        interruptChoice{"Does run.interrupt pick<br>running runs to cancel?"}
        cancelPrevious["Cancel them and wait<br>for them to exit"]
        waitSlot["Wait until a slot of run.concurrency is free<br>and target onStartWatch has ended"]
        runChange["global onBeforeChange → onChange<br>→ global onAfterChange"]
    end
    fatalStop(["Fatal error: see the next diagram"])

    globalStart --> watchDirs
    watchDirs --> targetStart
    targetStart --> receiveEvent
    receiveEvent -->|"a new directory that is not excluded<br>joins the watch list"| excludeCheck
    excludeCheck -->|"yes: ignore"| receiveEvent
    excludeCheck -->|"no"| collectChoice
    collectChoice -->|"yes, or already collecting"| collectFolder
    collectChoice -->|"no"| waitChoice
    collectFolder -->|"dirBatch.waitMillisecond with no event,<br>or dirBatch.maxWaitMillisecond"| waitChoice
    waitChoice -->|"no"| collectHook
    waitChoice -->|"yes: each event on its own"| mergeRuns
    collectHook --> mergeRuns
    mergeRuns -->|"no run: ignore"| receiveEvent
    mergeRuns -->|"each run"| queueEvent
    queueEvent -->|"no"| fatalStop
    queueEvent -.->|"pending runs reach 70% or 90%,<br>or fall below 80% or 60%"| queueWarning
    queueEvent -->|"yes"| interruptChoice
    interruptChoice -->|"yes: samePath for the same path,<br>oldest when every slot is taken"| cancelPrevious
    interruptChoice -->|"no"| waitSlot
    cancelPrevious --> waitSlot
    waitSlot --> runChange
    runChange -->|"next event"| receiveEvent
```

- Events keep being received and collected while commands run, so each `hookBatch` wait that ends adds its runs to the queue.
- A run that finds no room stops the watcher at once, including runs made from events collected for folder events. See [Folder events](events.md#folder-events) for what the folder events promise and how long they are collected.
- A run waits in the queue until it starts, so runs made while target `onStartWatch` or `run.concurrency` runs are running wait there. With `run.interrupt: oldest`, the first run also cancels a running target `onStartWatch`.
- The commands of one run execute one after another, and the run stops at the first command that fails. The failure is logged as an `[ERROR]` line; see [`config.yml`](configuration.md#configyml).

A shutdown signal and a fatal error both stop every target before the finish commands run:

```mermaid
flowchart TD
    shutdownSignal(("SIGINT or SIGTERM"))
    fatalError(("Fatal error in a target:<br>queue full, OS overflow,<br>watcher error, missing path"))
    stopTargets["All targets: stop starting new commands<br>and cancel running ones"]
    targetFinish["Each target: discard pending changes,<br>wait for running commands to exit,<br>then run target onFinishWatch if watching started"]
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
