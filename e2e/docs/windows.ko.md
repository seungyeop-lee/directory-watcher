# Windows E2E 실행과 CI 결과 확인

🇺🇸 [English](windows.md) | 🇰🇷 한국어

## 실행 명령

- Mac에서 UTM Windows VM으로 실행하는 방법은 [UTM Windows VM에서 E2E 실행](windows-utm.ko.md)에 있습니다.

Go와 콘솔 API를 사용할 수 있는 Windows 호스트의 PowerShell에서 저장소 루트로 이동한 뒤 실행합니다.

```powershell
go test -count=1 -race ./...
go test -count=3 -v -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$' ./e2e/scenarios
if ($LASTEXITCODE -ne 0) { throw "Windows lifecycle failed" }
```

- 각 테스트가 확인하는 내용은 [`TestGoldenPath`](scenarios/golden-path.ko.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md) 문서에 있습니다.

## 정상 종료 방식

- 테스트 프로세스에 콘솔이 없으면 `AllocConsole`로 할당하고, watcher와 각 hook을 별도 프로세스 그룹으로 실행합니다.
- 정상 종료 요청은 같은 콘솔의 해당 그룹에만 `CTRL_BREAK_EVENT`를 보냅니다.
- [Go는 CTRL+BREAK를 `os.Interrupt`로 전달](https://github.com/golang/go/blob/go1.27.0/src/os/signal/doc.go#L213-L220)하므로 기존 CLI의 SIGINT 수신을 사용합니다.
- `CTRL_C_EVENT`의 그룹 지정이나 `TASKKILL /F`를 정상 종료 증거로 사용하지 않습니다.
- 보조 프로세스는 콘솔과 프로세스 그룹을 상속합니다.
- 새 콘솔을 만들거나 신호를 무시하는 외부 프로그램에는 같은 정상 종료를 보장하지 않습니다([프로세스 트리](harness.ko.md#프로세스-트리)).
- watcher는 첫 CTRL+BREAK 뒤 20초가 지나도 남은 hook 프로세스를 그 hook의 Job Object로 강제로 끝내며, 이 종료는 정상 종료가 아닙니다([과부하와 종료](../../docs/guide/running.ko.md#과부하와-종료)).

## hook 취소 검증

Windows E2E는 Git Bash 없이 watcher를 실행해, watcher가 취소된 hook을 멈추는지 확인합니다.

- `run.interrupt: oldest`의 교체와 종료에서 hook의 부모·자식·손자 프로세스가 신호를 받고 끝납니다: [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md).
- 종료할 때 `run.concurrency: 2`로 동시에 도는 hook 두 개의 부모·자식·손자 프로세스가 모두 신호를 받고 끝난 뒤 종료 hook이 실행됩니다: [`TestConcurrentRunsShutdown`](scenarios/concurrent-runs-shutdown.ko.md).
- 시작 후 0–1000ms에 취소한 `powershell` hook은 프로그램을 끝까지 실행하지 않습니다: [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.ko.md).

아래 동작은 E2E가 아니라 `internal/process/process_windows_test.go`의 단위 테스트가 확인합니다.

| 동작 | 테스트 |
|---|---|
| `cmd`가 첫 CTRL+BREAK 뒤에 시작한 프로그램도 CTRL+BREAK를 받고 끝남(구분자 `&`, `&&`, `\|\|`) | `TestRunCommandSendsBreakAgainToProgramStartedAfterBreak` |
| 첫 CTRL+BREAK 전에 만들어졌지만 그 뒤에 콘솔에 연결된 프로세스도 CTRL+BREAK를 받고 끝남 | `TestRunCommandSendsBreakAgainToProcessAttachedToConsoleAfterBreak` |
| 첫 CTRL+BREAK를 보내기 직전에 시작한 자손은 시작 후 500ms가 지난 뒤 CTRL+BREAK를 받고 끝남 | `TestRunCommandDelaysBreakForDescendantStartedJustBeforeBreak` |
| 첫 CTRL+BREAK 뒤 20초가 지나도 남은 프로세스를 강제로 끝냄(테스트는 20초를 1초로 줄여 확인) | `TestRunCommandEndsProcessTreeAfterGrace` |

- 이 동작들의 계약은 [과부하와 종료](../../docs/guide/running.ko.md#과부하와-종료)에 있습니다.

## CI 단계

- `.github/workflows/ci.yml`의 `test (windows-latest)`는 `go test -count=1 -race ./...` 뒤 오류 알림 프로세스 검증과 [실행 명령](#실행-명령)에 나온 세 테스트의 `-count=3` 반복을 실행합니다.
- 이어 `DW_E2E_FAULT=failure`, `timeout`의 의도적인 실패·시간 초과, 진단 파일과 세 PID 정리를 검사하고 별도로 실행한 Go 부모·자식·손자 트리가 살아 있는지 확인합니다.
- Windows 전용 단계는 앞 단계가 실패해도 job이 취소되지 않았다면 실행합니다.
- 앞 단계의 실패는 그대로 남아 최종 job도 실패하며, 진단 업로드 단계는 항상 실행됩니다.
- Go 빌드 캐시와 별개로 테스트 **결과 캐시는 재사용하지 않습니다**.
- `Verify Windows error notification process trees` 단계는 `TestEventLossNotificationCLI`를 실행하고 테스트 출력을 `notification.log`에 저장합니다.
  - `notification.log`에는 알림 1회·최초 오류·소요 시간의 검사 결과가 남습니다.
  - 이 단계는 `DW_E2E_KEEP=1`로 실행 폴더를 보존하므로 실행별 `watcher.log`에 실제 오류 진단이 남습니다.
  - 모드별 확인 내용과 실행 폴더에 남는 생존 기록 파일은 [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md) 문서에 있고, Windows에서 알림의 자손을 정리하는 방식은 [플랫폼별 자손 정리](scenarios/event-loss-notification-cli.ko.md#플랫폼별-자손-정리)에 있습니다.

## CI 결과 확인

- GitHub Actions에서 **검증할 커밋 SHA**의 `ci` → `test (windows-latest)`를 엽니다.
- `go test -count=1 -race ./...` 단계에서 `TestFastInflowSlowHook`의 네 조합, `TestQueueWarningCLI`, `TestConcurrentRunsShutdown`, `TestRunInterrupt`의 네 하위 테스트, [`TestBatchingOptions`](scenarios/batching-options.ko.md)의 두 하위 테스트, `TestRunCommandSendsBreakAgainToProgramStartedAfterBreak`의 세 구분자, `TestCanceledPowershellHookStops`의 열한 시점에 `PASS`가 있고 `SKIP`이 없는지 확인합니다.
- 알림 검증 단계의 여섯 모드와 반복 단계의 세 테스트와 두 옵션 조합에 `PASS`가 있고 `SKIP`이 없는지 확인합니다.
- 의도적 실패 검증 단계 `Verify Windows failure and timeout diagnostics`가 성공했는지 확인합니다.
- `e2e-diagnostics-windows-latest` artifact에서 알림 정리 전후 기록과 `notification.log`·`failure.log`·`timeout.log`를 받습니다.
- 과거 다른 SHA의 성공, 교차 컴파일, Linux Docker 결과는 해당 커밋의 Windows 실행 증거가 아닙니다.
- 해당 커밋의 Windows 실행 결과가 없으면 Windows 항목은 `Not verified`로 남깁니다.
- `-race`와 Windows x64 동작은 이 CI 결과로만 확인합니다([Windows ARM64와 x64의 차이](windows-utm.ko.md#windows-arm64와-x64의-차이)).

## 실패 분석

- Windows 실패 시 먼저 **정리 전**에 기록한 `records.log.processes`와 `records.log`의 마지막 `signal`·`complete`를 비교합니다.
- `records.log.processes`에는 프로세스 트리의 프로세스(`run=`, 세대·깊이 포함)와 지연 hook(`delayed=`, 기록 포함)마다 한 줄씩, PID·생존·처음 관찰한 시각·생성 시각·프로세스 접근 오류·대기 결과와 함께 처음 관찰할 때 연 handle의 프로그램(`held-image`)과 지금 그 PID를 쓰는 프로그램(`current-image`)이 남습니다.
  - `open-error=PID reused by another program: <프로그램 경로>`는 처음 관찰할 때 다른 프로그램이 그 PID를 쓰고 있어 사라진 것으로 판정했다는 뜻입니다([프로세스 생존 판정과 강제 정리](harness.ko.md#프로세스-생존-판정과-강제-정리)).
- `cleanup.log`는 **정리 후** 상태이며 정상 종료 판정을 대신하지 않습니다.
- `GenerateConsoleCtrlEvent`·`AllocConsole` 오류는 콘솔 연결과 동일 콘솔의 프로세스 그룹을 확인합니다.
- hook이나 watcher가 0xc0000142로 끝났으면, 그 실행 폴더의 `dll-init-failed.log`를 [hook이 0xc0000142로 끝남](windows-utm.ko.md#hook이-0xc0000142로-끝남)에 따라 읽습니다.

정리 경로를 일부러 검사하려면 아래 PowerShell 명령을 실행합니다.

```powershell
$env:DW_E2E_FAULT = 'failure' # timeout도 같은 방법으로 실행
go test -count=1 -v -timeout=2m -run '^TestProcessLifecycle$' ./e2e/scenarios
Remove-Item Env:DW_E2E_FAULT
```

- `failure`·`timeout`의 동작과 기대 결과, 확인할 진단은 [의도적인 실패·시간 초과](diagnostics.ko.md#의도적인-실패시간-초과)에 있습니다.
- Windows에서는 `diagnostics retained:` 경로의 `records.log.processes`도 확인합니다.
