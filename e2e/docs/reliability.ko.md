# 경합·과부하·유실 검증

🇺🇸 [English](reliability.md) | 🇰🇷 한국어

## 경합과 과부하 검증

```sh
go test -count=3 -v -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestBatchingOptions|TestFastInflowSlowHook|TestStartHookShutdownModes|TestProcessLifecycle|TestRunInterrupt|TestQueueWarningCLI|TestConcurrentRunsShutdown'
go test -count=1 -race ./...
```

- 위 명령이 반복하는 시나리오의 확인 내용과 진행 순서는 [`TestBatchingOptions`](scenarios/batching-options.ko.md), [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md), [`TestRunInterrupt`](scenarios/run-interrupt.ko.md), [`TestQueueWarningCLI`](scenarios/queue-warning-cli.ko.md), [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.ko.md) 문서에 있습니다.
- 대기열 상한, 동시 실행 수, 종료 때 대기 중인 변경을 버리는 동작의 계약은 [과부하와 종료](../../docs/guide/running.ko.md#과부하와-종료)에 있습니다.

### 내부 테스트의 검증 범위

OS 알림의 수와 순서에 기대지 않고 정확한 결과를 확인해야 하는 아래 동작은 E2E가 아니라 제어한 이벤트 입력을 쓰는 단위 테스트가 확인합니다.

| 동작 | 테스트 |
|---|---|
| `hookBatch.deliver: each`의 수신 순서와 `deliver: last`의 마지막 유효 경로 | `internal/app/service/runner_watch_target_test.go`의 `TestControlledModesPreservePolicy`, `runner_watch_target_hook_batch_test.go`의 `TestDeliverTurnsOneWaitIntoRuns` |
| hook이 도는 동안 끝난 기다림마다 따로 만드는 실행 | `internal/app/service/runner_watch_target_test.go`의 `TestRunsFromSeparateWaitsAreNotMerged` |
| 이벤트가 계속 들어오는 동안 `hookBatch`와 `dirBatch`의 최대 대기 | `internal/app/service/runner_watch_target_test.go`의 `TestMaxWaitSendsRunWhileEventsKeepArriving` |
| 제외한 경로의 이벤트 | `internal/app/service/runner_watch_target_test.go`의 `TestExcludePatternDropsFileEvents` |
| 취소가 끝나기 전의 교체 금지 | `internal/app/service/hook_runner_test.go`의 `TestReplacementWaitsForCanceledGeneration` |
| 종료 뒤 실행 금지와 대기 중인 변경 버리기 | `internal/app/service/hook_runner_test.go`의 `TestShutdownDuringReplacementPreventsNewGeneration`, `TestFinishCancelsAllRunsBeforeFinishHook`, `runner_watch_target_test.go`의 `TestStopDiscardsPendingEvents` |
| 첫 치명적 오류 보존 | `internal/app/service/runner_test.go`의 `TestRunnerPreservesOnlyFirstFatalError` |
| 대기열이 가득 찼을 때의 유실 오류 | `internal/app/service/runner_watch_target_queue_test.go`의 `TestQueueFullRejectsRun`, `runner_watch_target_test.go`의 `TestBoundedDispatchReportsLoss`, `TestBatchLargerThanQueueOverflows` |
| `Stop` 뒤 감시 디렉터리 해제 | `internal/app/service/runner_watch_target_test.go`의 `TestNativeWatcherStopReleasesDirectory` |
| 존재·디렉터리 판별 뒤 파일과 디렉터리 해제 | `internal/helper/os_test.go`의 `TestFilesystemPredicatesReleaseResources` |

### OS E2E의 수신 경계 판정

- OS E2E는 파일 생성 순서로 마지막 수신 경로를 정하지 않고, watcher의 DEBUG 로그에 실제로 수신된 이벤트로 정합니다.
  - 이 판정 함수는 `e2e/harness/hook_records_test.go`의 `TestReceivedChangesFollowWatcherOrder`와 `TestChangesCompletedUsesReceivedBoundary`가 확인합니다.
- 120ms 안정 구간은 관찰 조건이지 OS가 앞으로 알림을 보내지 않는다는 보장은 아닙니다.
- 완료 대기는 받은 이벤트나 기록이 늘어나는 동안 계속 기다리고, 둘 다 10초 동안 변하지 않으면 실패합니다.
  - 걸리는 시간이 받은 이벤트 수와 hook 하나에 드는 시간에 따라 늘어나므로, 전체 시간에는 고정 제한을 두지 않습니다.
  - 그래서 받은 이벤트의 hook이 기록을 남기지 않으면 마지막 변화 뒤 10초가 지나 테스트가 실패합니다.
  - 진행 기반 대기는 `e2e/harness/harness_test.go`의 `TestAwaitProgressWaitsWhileProgressContinues`, `TestAwaitProgressFailsWhenProgressStops`, `TestAwaitProgressWithoutProgressMatchesFixedWait`가 확인합니다.
- 이 대기는 [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md)과 [`TestBatchingOptions`](scenarios/batching-options.ko.md)가 씁니다.

## 이벤트 유실 알림 검증

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestFastInflowSlowHook|TestInjectedLossNotificationCLI'
```

- 위 명령이 실행하는 시나리오의 확인 내용과 진행 순서는 [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md), [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md), [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.ko.md) 문서에 있습니다.
  - `TestFastInflowSlowHook`은 일반 변경과 `deliver: last`가 합친 이벤트가 알림을 실행하지 않는지 확인합니다.
- macOS·Linux·Windows의 알림 자손 정리 방식은 [플랫폼별 자손 정리](scenarios/event-loss-notification-cli.ko.md#플랫폼별-자손-정리)에 있습니다.
- 알림의 환경 변수와 실행·정리 제한 시간의 계약은 [이벤트 유실 알림](../../docs/guide/running.ko.md#이벤트-유실-알림)에 있습니다.
- 디코딩한 구조화 `onError` 명령의 환경 변수 원문 전달과 `dir` 작업 디렉터리, 기존 설정 호환성은 `internal/config/yaml/config_test.go`의 `TestDecoded`로 시작하는 테스트가 확인합니다.

## 유실 검증의 구분

- 실제 파일시스템 부하 테스트는 OS 알림의 중복·병합을 허용하므로, 파일 조작 횟수와 hook 실행 횟수가 같다고 단정하지 않습니다.
- E2E는 실제 OS overflow를 일으키지 않으며, 유실 알림 시나리오의 오류는 아래 두 가지로 만듭니다.
  - [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md)는 애플리케이션의 전달 대기열을 실제로 채워 `dispatch queue full` 오류를 일으킵니다.
  - [`TestInjectedLossNotificationCLI`](scenarios/injected-loss-notification-cli.ko.md)는 `dw_e2e_inject` build tag로 빌드한 watcher의 watcher 오류 채널에 오류를 주입해, 그 오류가 CLI의 알림과 종료 코드까지 이어지는지 확인합니다.
- 두 시나리오의 통과는 실제 OS overflow를 관찰한 증거가 아닙니다.
- OS overflow의 판정은 fsnotify v1.10.1이 보고하는 `ErrEventOverflow`([공통 오류](https://github.com/fsnotify/fsnotify/blob/v1.10.1/fsnotify.go#L262-L270))를 기준으로 합니다.
  - Linux는 inotify 대기열이 넘치면 이 오류를 보고합니다([Linux overflow 처리](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_inotify.go#L397-L401)).
  - Windows는 알림 버퍼가 넘치면 이 오류를 보고합니다([Windows 백엔드](https://github.com/fsnotify/fsnotify/blob/v1.10.1/backend_windows.go#L580-L584)).
  - macOS kqueue는 이 오류를 보고하지 않으므로, macOS에서는 OS 유실을 감지한다고 주장하지 않습니다.
- 애플리케이션이 보장하지 않는 범위는 [과부하와 종료](../../docs/guide/running.ko.md#과부하와-종료)에 있고, Windows에서 취소한 hook의 종료를 확인하는 테스트는 [hook 취소 검증](windows.ko.md#hook-취소-검증)에 있습니다.
