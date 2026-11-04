# 동작 방식

🇺🇸 [English](how-it-works.md) | 🇰🇷 한국어

- 전역 `onStartWatch`가 가장 먼저 실행됩니다.

그 뒤 감시 대상마다 따로 시작하고, 각 대상은 다른 대상과 관계없이 자기 변경을 처리합니다.

```mermaid
flowchart TD
    globalStart["전역 onStartWatch"]
    subgraph eachTarget["감시 대상마다 독립적으로"]
        watchDirs["대상 디렉터리 감시 시작"]
        targetStart["대상 onStartWatch"]
        receiveEvent(("이벤트 수신"))
        excludeCheck{"제외된 경로인가?"}
        collectChoice{"폴더 생성·삭제이거나<br>하위 폴더 안의 파일 삭제인가?"}
        collectFolder["대상의 이벤트를 모으고<br>폴더 안의 이벤트는 흡수"]
        waitChoice{"hookBatch.waitMillisecond가<br>0인가?"}
        collectHook["hookBatch.waitMillisecond 동안 이벤트가 없거나<br>hookBatch.maxWaitMillisecond가 지날 때까지 모음"]
        mergeRuns["hookBatch.deliver에 따라 실행으로 합치고<br>watch.fileEvent나 watch.dirEvent에서 고른 종류만 남김"]
        queueEvent{"대상의 queue.size 크기<br>대기열에 자리가 있는가?"}
        queueWarning["전역 onQueueWarning,<br>이벤트 처리와 따로 실행"]
        interruptChoice{"run.interrupt가 취소할<br>실행을 고르는가?"}
        cancelPrevious["그 실행을 취소하고<br>끝나기를 기다림"]
        waitSlot["run.concurrency의 자리가 비고<br>대상 onStartWatch가 끝날 때까지 대기"]
        runChange["전역 onBeforeChange → onChange<br>→ 전역 onAfterChange"]
    end
    fatalStop(["치명적 오류: 다음 다이어그램"])

    globalStart --> watchDirs
    watchDirs --> targetStart
    targetStart --> receiveEvent
    receiveEvent -->|"제외되지 않은 새 디렉터리는<br>감시 목록에 추가"| excludeCheck
    excludeCheck -->|"예: 무시"| receiveEvent
    excludeCheck -->|"아니오"| collectChoice
    collectChoice -->|"예, 또는 이미 모으는 중"| collectFolder
    collectChoice -->|"아니오"| waitChoice
    collectFolder -->|"dirBatch.waitMillisecond 동안 이벤트 없음,<br>또는 dirBatch.maxWaitMillisecond 지남"| waitChoice
    waitChoice -->|"아니오"| collectHook
    waitChoice -->|"예: 이벤트마다 따로"| mergeRuns
    collectHook --> mergeRuns
    mergeRuns -->|"실행 없음: 무시"| receiveEvent
    mergeRuns -->|"실행마다"| queueEvent
    queueEvent -->|"아니오"| fatalStop
    queueEvent -.->|"대기 실행이 70%나 90%에 닿거나<br>80%나 60% 아래로 내려감"| queueWarning
    queueEvent -->|"예"| interruptChoice
    interruptChoice -->|"예: samePath는 같은 경로,<br>oldest는 자리가 다 찼을 때"| cancelPrevious
    interruptChoice -->|"아니오"| waitSlot
    cancelPrevious --> waitSlot
    waitSlot --> runChange
    runChange -->|"다음 이벤트"| receiveEvent
```

- 명령이 실행되는 동안에도 이벤트를 계속 받아 모으므로, `hookBatch`가 기다리는 시간이 끝날 때마다 그때 만든 실행이 대기열에 들어갑니다.
- 대기열에 자리가 없는 실행은 곧바로 watcher를 멈추며, 폴더 이벤트를 위해 모은 이벤트에서 만든 실행도 같습니다. 폴더 이벤트의 약속과 모으는 시간은 [폴더 이벤트](events.ko.md#폴더-이벤트)에 있습니다.
- 실행은 시작할 때까지 대기열에서 기다리므로, 대상 `onStartWatch`나 `run.concurrency`개의 실행이 도는 동안 만든 실행은 대기열에 남습니다. `run.interrupt: oldest`에서는 실행 중인 대상 `onStartWatch`도 첫 실행이 취소합니다.
- 한 번의 실행에 속한 명령은 차례대로 실행되며, 실패한 명령이 나오면 그 실행을 멈춥니다. 이 실패는 `[ERROR]` 로그로 남습니다. 자세한 내용은 [`config.yml`](configuration.ko.md#configyml)을 확인하세요.

종료 신호와 치명적 오류는 모두 전체 감시 대상을 멈춘 뒤 종료 명령을 실행합니다.

```mermaid
flowchart TD
    shutdownSignal(("SIGINT 또는 SIGTERM"))
    fatalError(("대상의 치명적 오류:<br>대기열 포화, OS overflow,<br>watcher 오류, 없는 경로"))
    stopTargets["모든 대상: 새 명령 시작을 멈추고<br>실행 중인 명령 취소"]
    targetFinish["대상마다: 대기 중인 변경을 버리고<br>실행 중인 명령이 끝나기를 기다린 뒤<br>감시를 시작한 대상이면 onFinishWatch 실행"]
    lossCheck{"이벤트 유실 가능성?<br>대기열 포화 또는 OS overflow"}
    errorHook["전역 onError 한 번<br>5초 실행 제한, 이어서 최대 20초 정리"]
    globalFinish["전역 onFinishWatch"]
    exitCheck{"치명적 오류가 있었는가?"}
    exitSuccess(("종료 코드 0"))
    exitFailure(("종료 코드 1"))

    shutdownSignal --> stopTargets
    fatalError --> stopTargets
    stopTargets --> targetFinish
    stopTargets -->|"치명적 오류 뒤"| lossCheck
    lossCheck -->|"예"| errorHook
    lossCheck -->|"아니오"| globalFinish
    targetFinish --> globalFinish
    errorHook --> globalFinish
    globalFinish --> exitCheck
    exitCheck -->|"아니오"| exitSuccess
    exitCheck -->|"예"| exitFailure
```

- 대상 종료 명령은 `onError`가 실행되는 동안에도 실행될 수 있습니다. 전역 `onFinishWatch`는 두 쪽이 모두 끝나기를 기다립니다.
- 종료 명령에는 별도의 실행 제한 시간이 없습니다.
