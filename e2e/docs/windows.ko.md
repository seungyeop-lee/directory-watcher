# Windows E2E 실행과 CI 결과 확인

🇺🇸 [English](windows.md) | 🇰🇷 한국어

## 실행 명령

Go와 콘솔 API를 사용할 수 있는 Windows 호스트의 PowerShell에서 저장소 루트로 이동한 뒤 실행합니다. Mac에서 UTM Windows VM으로 실행하는 방법은 [UTM Windows VM에서 E2E 실행](windows-utm.ko.md)에 있습니다. 임시 경로에 공백이 있으면 공백 없는 `TEMP`·`TMP` 경로를 지정합니다.

```powershell
go test -count=1 -race ./...
go test -count=3 -v -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$' ./e2e/scenarios
if ($LASTEXITCODE -ne 0) { throw "Windows lifecycle failed" }
```

각 테스트가 확인하는 내용은 [`TestGoldenPath`](scenarios/golden-path.ko.md), [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md), [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md) 문서에 있습니다.

## 정상 종료 방식

테스트 프로세스에 콘솔이 없으면 `AllocConsole`로 할당하고, watcher와 각 hook을 별도 프로세스 그룹으로 실행합니다. 정상 종료 요청은 같은 콘솔의 해당 그룹에만 `CTRL_BREAK_EVENT`를 보냅니다. [Go는 CTRL+BREAK를 `os.Interrupt`로 전달](https://github.com/golang/go/blob/go1.23.0/src/os/signal/doc.go)하므로 기존 CLI의 SIGINT 수신을 사용합니다. `CTRL_C_EVENT`의 그룹 지정이나 `TASKKILL /F`를 정상 종료 증거로 사용하지 않습니다. 보조 프로세스는 콘솔·그룹을 상속하고 신호를 처리하며 자손 종료를 기다립니다. 새 콘솔을 만들거나 신호를 무시하는 임의의 외부 프로그램에 같은 정상 종료 보장을 적용하지 않습니다.

## CI 단계

`.github/workflows/ci.yml`의 `test (windows-latest)`는 `go test -count=1 -race ./...` 뒤 오류 알림 프로세스 검증과 위 세 테스트의 `-count=3` 반복을 실행합니다. 이어 `DW_E2E_FAULT=failure`, `timeout`의 의도적인 실패·시간 초과, 진단 파일과 세 PID 정리를 검사하고 별도로 실행한 Go 부모·자식·손자 트리가 살아 있는지 확인합니다. Windows 전용 단계는 앞 단계가 실패해도 job이 취소되지 않았다면 실행합니다. 앞 단계의 실패는 그대로 남아 최종 job도 실패하며, 진단 업로드 단계는 항상 실행됩니다. Go 빌드 캐시와 별개로 테스트 **결과 캐시는 재사용하지 않습니다**.

`Verify Windows error notification process trees` 단계는 `TestEventLossNotificationCLI`를 실행하고 테스트 출력을 `notification.log`에 저장합니다. `notification.log`에는 알림 1회·최초 오류·소요 시간의 검사 결과가 남고, 이 단계는 `DW_E2E_KEEP=1`로 실행 폴더를 보존하므로 실행별 `watcher.log`에 실제 오류 진단이 남습니다. 모드별 확인 내용과 실행 폴더에 남는 생존 기록 파일은 [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md) 문서에 있습니다.

Windows 알림 실행기는 부모가 자손을 생성하기 전에 일시 중단 상태로 별도 Job Object에 배정합니다. 부모의 exit 7·exit 0 뒤에도 해당 Job의 자손을 종료하고 활성 프로세스가 0이 될 때까지 기존 정리 한도 안에서 기다립니다. 이 정리를 검사하는 조건은 [트리 모드의 추가 확인](scenarios/event-loss-notification-cli.ko.md#트리-모드의-추가-확인)에 있습니다.

## CI 결과 확인

GitHub Actions에서 **검증할 커밋 SHA**의 `ci` → `test (windows-latest)`를 엽니다. `go test -count=1 -race ./...` 단계의 `TestFastInflowSlowHook` 네 조합·`TestTargetsRunIndependently`·`TestBoundedOverloadCLI`, 알림 검증 단계의 여섯 모드, 반복 단계의 세 테스트와 네 옵션 조합에 `PASS`가 있고 `SKIP`이 없는지, 의도적 실패 검증 단계는 성공했는지 확인합니다. `e2e-diagnostics-windows-latest` artifact에서 알림 정리 전후 기록과 `notification.log`·`failure.log`·`timeout.log`를 받습니다. 과거 다른 SHA의 성공, 교차 컴파일, Linux Docker 결과는 해당 커밋의 Windows 실행 증거가 아닙니다. 해당 커밋의 Windows 실행 결과가 없으면 Windows 항목은 `Not verified`로 남깁니다. UTM VM으로 확인할 수 있는 항목은 [UTM Windows VM에서 E2E 실행](windows-utm.ko.md)으로 실행하고, `-race`와 Windows x64 항목은 이 CI 결과로만 확인합니다.

## 실패 분석

Windows 실패 시 먼저 `records.log.processes`의 **정리 전** 세대·깊이·PID·생존·생성 시각·프로세스 접근 오류와 `records.log`의 마지막 `signal`·`complete`를 비교합니다. `cleanup.log`는 **정리 후** 상태이며 정상 종료 판정을 대신하지 않습니다. `GenerateConsoleCtrlEvent`·`AllocConsole` 오류는 콘솔 연결과 동일 콘솔의 프로세스 그룹을 확인합니다. 접근 거부·대기 API 오류는 생존으로 취급하여 실패시키며, PID 재사용 혼동을 막기 위해 준비 시 연 프로세스 handle을 정리 후까지 유지합니다. 실패 정리는 기록된 해당 실행의 보조 바이너리 handle만 종료하며, watcher가 아직 살아 있으면 해당 watcher의 트리에만 `TASKKILL /T /F`를 적용합니다.

정리 경로를 일부러 검사하려면 아래 명령을 실행합니다.

```powershell
$env:DW_E2E_FAULT = 'failure' # timeout도 같은 방법으로 실행
go test -count=1 -v -timeout=2m -run '^TestProcessLifecycle$' ./e2e/scenarios
Remove-Item Env:DW_E2E_FAULT
```

이 명령은 0이 아닌 종료가 기대 결과입니다. `diagnostics retained:` 경로의 진단 파일과 `cleanup.log`의 세 `alive=false`를 확인합니다. `DW_E2E_DIAGNOSTICS`로 진단 상위 디렉터리를 지정할 수 있으며 CI는 `${{ runner.temp }}/dw-e2e`를 사용합니다. 진단 파일 목록과 macOS·Linux의 같은 검사는 [E2E 실패·시간 초과 진단과 격리](diagnostics.ko.md)에 있습니다.
