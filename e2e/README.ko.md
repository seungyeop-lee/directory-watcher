# 자동 E2E 실행

🇺🇸 [English](README.md) | 🇰🇷 한국어

- 실제 현재 소스의 watcher와 Go 보조 프로그램을 빌드하고 파일 이벤트·hook 기록·프로세스 종료를 검사합니다.
- macOS 로컬·Linux Docker·Windows는 `e2e/scenarios`의 같은 Go E2E 시나리오를 실행합니다.
- Windows에서만 실행하는 하네스 검증 `TestCleanupReportsLockedResources`, `TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram`, `TestDelayedHooksGoneFailsWhileHookRuns`, `TestDLLInitFailureReportFromWatcher`는 `e2e/harness` 패키지에 있어 `make e2e-local`이 아닌 `go test ./...`로 실행됩니다.
- 시나리오별 검증 내용과 코드 위치는 [시나리오와 지원 코드의 대응](docs/harness.ko.md#시나리오와-지원-코드의-대응)에 있습니다.

## 준비와 실행

- 로컬은 `go.mod`에 지정된 버전 이상의 Go가 필요합니다.
- Linux 실행에는 Docker CLI와 실행 중인 Linux Docker 엔진도 필요합니다.
- Java·Gradle은 사용하지 않습니다.

저장소 루트에서 실행합니다.

```sh
make e2e-local
make e2e-linux
```

Mac에서 UTM Windows VM으로 실행하려면 [UTM Windows VM에서 E2E 실행](docs/windows-utm.ko.md)의 사전 준비를 마친 뒤 다음 명령을 사용합니다.

```sh
DW_E2E_UTM_VM=Windows-E2E make e2e-windows-all
```

- 성공하면 각 테스트에 `PASS`가 표시되고 명령이 0으로 종료합니다.
- 실패·시간 초과는 0이 아닌 종료 코드로 전달합니다.
- Linux 기본 이미지는 `golang:1.27-bookworm`이며, 다른 Go 이미지는 `go run ./e2e/linux -image=<이미지>`로 지정합니다.

### 특정 시나리오 선택·반복

```sh
go test -count=3 -v -run '^TestGoldenPath$' ./e2e/scenarios
go test -count=3 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run '^TestGoldenPath$'
go run ./e2e/linux -- -count=3 -run '^TestProcessLifecycle$'
```

- 모든 실행에 `-count`를 지정해 Go 결과 캐시를 사용하지 않습니다.
- Linux 실행기는 기본 `-count=1` 뒤에 전달 인자를 붙이므로 위의 반복 횟수가 적용됩니다.
- 컨테이너 전체 제한 시간은 `go run ./e2e/linux -deadline=10m -- -run TestProcessLifecycle`처럼 지정합니다.
- Go 테스트 자체의 제한 시간은 `--` 뒤의 `-timeout=2m`으로 지정할 수 있습니다.

## 세부 문서

| 문서 | 내용 |
|---|---|
| [하네스 구조와 시나리오 추가](docs/harness.ko.md) | 책임별 코드 위치, 시나리오와 지원 코드의 대응, 새 시나리오 작성 절차와 예시, `hookrec` 옵션·프로세스 트리·지연 hook |
| 시나리오별 문서: [`TestGoldenPath`](docs/scenarios/golden-path.ko.md), [`TestFolderEvents`](docs/scenarios/folder-events.ko.md), [`TestWatchOptions`](docs/scenarios/watch-options.ko.md), [`TestBatchingOptions`](docs/scenarios/batching-options.ko.md), [`TestProcessLifecycle`](docs/scenarios/process-lifecycle.ko.md), [`TestStartHookShutdownModes`](docs/scenarios/start-hook-shutdown-modes.ko.md), [`TestRunInterrupt`](docs/scenarios/run-interrupt.ko.md), [`TestConcurrentRunsShutdown`](docs/scenarios/concurrent-runs-shutdown.ko.md), [`TestFastInflowSlowHook`](docs/scenarios/fast-inflow-slow-hook.ko.md), [`TestTargetsRunIndependently`](docs/scenarios/targets-run-independently.ko.md), [`TestOverlappingTargets`](docs/scenarios/overlapping-targets.ko.md), [`TestBoundedOverloadCLI`](docs/scenarios/bounded-overload-cli.ko.md), [`TestEventLossNotificationCLI`](docs/scenarios/event-loss-notification-cli.ko.md), [`TestNormalChangesDoNotNotify`](docs/scenarios/normal-changes-do-not-notify.ko.md), [`TestInjectedLossNotificationCLI`](docs/scenarios/injected-loss-notification-cli.ko.md), [`TestQueueWarningCLI`](docs/scenarios/queue-warning-cli.ko.md), [`TestCanceledCommandListStopsNextProgram`](docs/scenarios/canceled-command-list-stops-next-program.ko.md), [`TestCanceledPowershellHookStops`](docs/scenarios/canceled-powershell-hook-stops.ko.md), [`TestHookWorkingDirectory`](docs/scenarios/hook-working-directory.ko.md), [`TestPowershellEventVariables`](docs/scenarios/powershell-event-variables.ko.md), [`TestWindowsShellEmptyArgument`](docs/scenarios/windows-shell-empty-argument.ko.md), [`TestWindowsShellNonASCIIOutput`](docs/scenarios/windows-shell-non-ascii-output.ko.md), [`TestWindowsShellRejectedOnUnix`](docs/scenarios/windows-shell-rejected-on-unix.ko.md) | 시나리오마다 확인 내용과 성공 조건, 설정, 진행 순서 다이어그램, 하위 테스트, 실행 명령, 관련 코드 |
| [UTM Windows VM에서 E2E 실행](docs/windows-utm.ko.md) | Mac의 UTM Windows VM 사전 준비와 E2E 전용 VM, `make e2e-windows-up`·`e2e-windows`·`e2e-windows-down`·`e2e-windows-all`, 결과 위치, 종료 동작, 문제 해결 |
| [Windows 실행과 CI 결과 확인](docs/windows.ko.md) | Windows 실행 명령, CTRL+BREAK 정상 종료 방식, CI 단계와 artifact 확인, Windows 실패 분석 |
| [실패·시간 초과 진단과 격리](docs/diagnostics.ko.md) | 진단 파일 위치, `DW_E2E_FAULT` 의도적 실패·시간 초과, Linux 진단, 워크트리 병렬 실행과 컨테이너 정리 |
| [경합·과부하·유실 검증](docs/reliability.ko.md) | 경합·과부하·유실 알림 시나리오의 반복 실행 명령, 내부 테스트의 검증 범위, OS E2E의 수신 경계 판정, 유실 검증의 구분과 주입 스모크 |
