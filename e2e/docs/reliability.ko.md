# 경합·과부하·유실 검증

🇺🇸 [English](reliability.md) | 🇰🇷 한국어

## 경합과 과부하 검증

```sh
go test -count=3 -v -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle'
go test -race ./...
```

- 위 명령이 반복하는 시나리오의 확인 내용과 진행 순서는 [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md), [`TestTargetsRunIndependently`](scenarios/targets-run-independently.ko.md), [`TestBoundedOverloadCLI`](scenarios/bounded-overload-cli.ko.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md) 문서에 있습니다.

### 내부 테스트의 검증 범위

- 제어된 내부 테스트는 OS 알림 개수와 무관하게 `hookBatch.deliver: each`의 정확한 순서, `deliver: last`의 마지막 유효 경로, hook이 도는 동안 끝난 기다림마다 따로 만든 실행, 제외 이벤트, 취소 완료 전 교체 금지, 종료 후 실행 금지, 첫 치명 오류 보존을 검사합니다.
- 대상별 대기 실행 상한은 `queue.size`(기본 2048)개이며, 실행은 한 세대이고 `hookBatch` 기다림은 한 번에 하나입니다. 대기열을 채우는 테스트는 기본값에 기대지 않고 작은 `queue.size`를 지정합니다.
- 대기열이 가득 차면 버퍼를 늘리거나 자리가 나기를 기다리지 않고 유실 오류로 종료하며, 폴더 이벤트용으로 모은 이벤트에서 만든 실행도 같습니다.
- 종료는 대기열을 처리 완료하는 동작이 아니라 취소·정리하는 동작입니다.

- 제어 입력 runner 테스트는 실제 fsnotify watcher와 감시 등록을 유지하고 수신 채널만 바꿉니다.
- 관련 테스트는 `Stop` 완료 후 임시 디렉터리를 즉시 삭제해 자원 반납을 검사합니다.
- `TestNativeWatcherStopReleasesDirectory`는 원래 채널로 실제 생성 이벤트를 처리한 뒤 종료 hook과 디렉터리 삭제를 검사합니다.
- `TestFilesystemPredicatesReleaseResources`는 존재·디렉터리 판별의 기존 bool 결과와 호출 직후 파일·디렉터리 삭제를 함께 검사합니다.

### OS E2E의 수신 경계 판정

- OS E2E는 파일 생성 순서로 마지막 수신 경로를 정하지 않습니다.
- `e2e/harness/hook_records_test.go`는 `last.txt` 뒤에 `normal-19.txt`를 수신한 경계, 빈 기록·미완료 hook의 거부, `deliver: each`의 중복 수신별 완료를 검사합니다.
- 120ms 안정 구간은 관찰 조건이지 OS가 앞으로 알림을 보내지 않는다는 보장은 아닙니다.
- 늦은 유효 이벤트가 고립된 변경을 대체하면 테스트는 성공 처리하지 않고, 배치 관찰과 고립된 변경이 공유하는 완료 대기 제한 시간 안에 실패합니다.
- 제한 시간은 [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md)이 30초, [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.ko.md)가 10초입니다.

## 이벤트 유실 알림 검증

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify'
```

- 위 명령이 실행하는 시나리오의 확인 내용과 진행 순서는 [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md)와 [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.ko.md) 문서에 있습니다.
- macOS·Linux·Windows의 알림 자손 정리 방식은 [플랫폼별 자손 정리](scenarios/event-loss-notification-cli.ko.md#플랫폼별-자손-정리)에 있습니다.

- 내부 YAML 설정 테스트 `go test -count=1 -run '^TestDecoded' ./internal/config/yaml`은 디코딩된 구조화 명령을 실제로 실행하여 환경 변수의 원문 전달과 `dir` 작업 디렉토리, 기존 설정 호환성을 검사합니다.
- 알림의 환경 변수 및 고정 5초 실행 제한·최대 20초 취소 정리 계약은 [이벤트 유실 알림](../../docs/guide/running.ko.md#이벤트-유실-알림)을 따릅니다.

## 유실 검증의 구분

- 실제 파일시스템 부하 테스트는 OS 알림의 중복·병합을 허용합니다. 파일 조작 횟수와 hook 횟수를 같다고 단정하지 않습니다.
- CLI까지의 오류 전달은 검증 전용 소스 overlay로 fsnotify 유실 오류를 주입해 별도 검사할 수 있습니다. `hookBatch.deliver`·`run.interrupt` 네 조합에서 최초 치명적 오류 보존, 알림 한 번 실행, 다른 대상의 새 일반 hook 차단을 확인합니다. overlay 실행 명령과 결과는 이슈 보고서에 남기며 배포 바이너리에는 주입 옵션이 없습니다. 주입 성공은 실제 OS overflow 관찰 증거가 아닙니다.
- 실제 Linux overflow 실험은 fsnotify 수신을 잠시 멈추고 현재 `max_queued_events`보다 많은 파일을 생성한 뒤 `ErrEventOverflow`를 관찰합니다. 호스트·공유 커널 설정은 변경하지 않습니다. macOS kqueue는 이 sentinel을 보고하지 않으므로 같은 보장을 주장하지 않습니다.
- fsnotify v1.7.0의 [이벤트·오류 계약](https://github.com/fsnotify/fsnotify/blob/v1.7.0/backend_inotify.go)과 [공통 오류](https://github.com/fsnotify/fsnotify/blob/v1.7.0/fsnotify.go)를 기준으로 합니다. 모든 OS 유실의 감지, 무제한 입력의 무손실 처리, 자동 복구는 보장하지 않습니다. 임의의 종료 hook에는 별도 제한 시간이 없으며, 프로세스 트리 검증은 SIGTERM 또는 Windows CTRL+BREAK를 처리하고 자손을 기다리는 보조 프로그램을 대상으로 합니다. Windows에서는 [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.ko.md)이 `cmd`가 첫 CTRL+BREAK 뒤에 시작한 프로그램을, [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.ko.md)가 `powershell` hook을 함께 확인합니다. 첫 CTRL+BREAK 뒤 20초가 지나도 남은 프로세스를 강제로 끝내는 동작은 `internal/process`의 단위 테스트 `TestRunCommandEndsProcessTreeAfterGrace`가 20초를 1초로 줄여서만 확인합니다.

### 주입 스모크 재현

- 이슈 보고서의 주입 스모크는 아래 명령으로 재현합니다.

`<이슈 보고서 디렉토리>`는 `smoke.sh`와 overlay 테스트 파일이 있는 경로로 바꿉니다.

```sh
sh <이슈 보고서 디렉토리>/smoke.sh macos
sh <이슈 보고서 디렉토리>/smoke.sh linux
```

- `TestControlledLossNotificationSmoke`는 네 옵션 조합과 유실이 아닌 치명적 오류의 알림 제외를 검사합니다.
- 스크립트는 Go overlay와 임시 생성자 주입을 사용하며 저장소 소스를 수정하지 않습니다.
