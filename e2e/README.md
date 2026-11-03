# 자동 E2E 실행

실제 현재 소스의 watcher와 Go 보조 프로그램을 빌드하고 파일 이벤트·hook 기록·프로세스 종료를 검사합니다. macOS 로컬·Linux Docker·Windows는 `e2e/scenarios`의 같은 Go E2E 시나리오를 실행합니다. Windows에서만 실행하는 하네스 검증 `TestCleanupReportsLockedResources`는 `e2e/harness` 패키지에 있어 `make e2e`가 아닌 `go test ./...`로 실행됩니다. 시나리오별 검증 내용과 코드 위치는 [시나리오와 지원 코드의 대응](#시나리오와-지원-코드의-대응)에 있습니다.

## 준비와 실행

저장소 루트에서 실행합니다. 로컬은 `go.mod`에 지정된 버전 이상의 Go가 필요합니다. Linux 실행에는 Docker CLI와 실행 중인 Linux Docker 엔진도 필요합니다. Java·Gradle은 사용하지 않습니다.

```sh
make e2e
make e2e-linux
```

성공하면 각 테스트에 `PASS`가 표시되고 명령이 0으로 종료합니다. 실패·시간 초과는 0이 아닌 종료 코드로 전달합니다. Linux 기본 이미지는 `golang:1.23-bookworm`이며, 다른 Go 이미지는 `go run ./e2e/linux -image=<이미지>`로 지정합니다.

### 특정 시나리오 선택·반복

```sh
go test -count=3 -v -run '^TestGoldenPath$' ./e2e/scenarios
go test -count=3 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run '^TestGoldenPath$'
go run ./e2e/linux -- -count=3 -run '^TestProcessLifecycle$'
```

모든 실행에 `-count`를 지정해 Go 결과 캐시를 사용하지 않습니다. Linux 실행기는 기본 `-count=1` 뒤에 전달 인자를 붙이므로 위의 반복 횟수가 적용됩니다. 컨테이너 전체 제한 시간은 `go run ./e2e/linux -deadline=10m -- -run TestProcessLifecycle`처럼 지정합니다. Go 테스트 자체의 제한 시간은 `--` 뒤의 `-timeout=2m`으로 지정할 수 있습니다.

### Windows 실행과 CI 결과 확인

Go와 콘솔 API를 사용할 수 있는 Windows 호스트의 PowerShell에서 저장소 루트로 이동한 뒤 실행합니다. 임시 경로에 공백이 있으면 공백 없는 `TEMP`·`TMP` 경로를 지정합니다.

```powershell
go test -count=1 -race ./...
go test -count=3 -v -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$' ./e2e/scenarios
if ($LASTEXITCODE -ne 0) { throw "Windows lifecycle failed" }
```

`TestGoldenPath`는 대상·전역 `onFinishWatch`의 순서와 watcher 종료 코드 0을 검사합니다. `TestProcessLifecycle`는 실제 세 프로세스의 준비·생존을 확인한 뒤 파일을 생성하고, 이전 세대 전체의 `signal`·`complete`가 새 세대의 `start`보다 앞서는지 검사합니다. watcher 종료 때 현재 세대도 정상 완료해야 하며, 두 종료 hook은 마지막에 기록되어야 합니다. 이전 세대와 전체 세대의 PID 부재는 강제 정리 전에 검사합니다. `TestStartHookShutdownModes`는 `noWait`·`interruptible` 네 조합을 같은 보조 프로세스로 검사합니다.

테스트 프로세스에 콘솔이 없으면 `AllocConsole`로 할당하고, watcher와 각 hook을 별도 프로세스 그룹으로 실행합니다. 정상 종료 요청은 같은 콘솔의 해당 그룹에만 `CTRL_BREAK_EVENT`를 보냅니다. [Go는 CTRL+BREAK를 `os.Interrupt`로 전달](https://github.com/golang/go/blob/go1.23.0/src/os/signal/doc.go)하므로 기존 CLI의 SIGINT 수신을 사용합니다. `CTRL_C_EVENT`의 그룹 지정이나 `TASKKILL /F`를 정상 종료 증거로 사용하지 않습니다. 보조 프로세스는 콘솔·그룹을 상속하고 신호를 처리하며 자손 종료를 기다립니다. 새 콘솔을 만들거나 신호를 무시하는 임의의 외부 프로그램에 같은 정상 종료 보장을 적용하지 않습니다.

`.github/workflows/ci.yml`의 `test (windows-latest)`는 `go test -count=1 -race ./...` 뒤 오류 알림 프로세스 검증과 위 세 테스트의 `-count=3` 반복을 실행합니다. 이어 `DW_E2E_FAULT=failure`, `timeout`의 의도적인 실패·시간 초과, 진단 파일과 세 PID 정리를 검사하고 별도로 실행한 Go 부모·자식·손자 트리가 살아 있는지 확인합니다. Windows 전용 단계는 앞 단계가 실패해도 job이 취소되지 않았다면 실행합니다. 앞 단계의 실패는 그대로 남아 최종 job도 실패하며, 진단 업로드 단계는 항상 실행됩니다. Go 빌드 캐시와 별개로 테스트 **결과 캐시는 재사용하지 않습니다**.

`Verify Windows error notification process trees`는 `TestEventLossNotificationCLI`를 실행합니다. `failure`는 준비된 부모를 exit 7로, `success-tree`는 exit 0으로 먼저 종료시키고 자식·손자를 남깁니다. 두 모드는 테스트가 세 PID의 생존과 handle을 확인한 뒤 부모 종료를 허용합니다. `timeout`은 준비된 트리에 5초 알림 제한을 적용합니다. `notification-before.log`와 `notification-after.log`에는 시간·PID·생존·handle 조회 결과가, 별도 실행의 `protected-before.log`·`protected-after.log`에는 보호 트리의 생존이 기록됩니다. `notification.log`는 알림 1회·최초 오류·소요 시간의 검사 결과를 담고, 실행별 `watcher.log`는 실제 오류 진단을 보존합니다. 문자열 명령 항목, 구조화 `dir`/문자열 `cmd`, 구조화 `dir`/목록 `cmd`를 같은 알림 경로로 실행합니다. 최상위 `onError` 자체는 명령 목록이어야 합니다.

Windows 알림 실행기는 부모가 자손을 생성하기 전에 일시 중단 상태로 별도 Job Object에 배정합니다. 부모의 exit 7·exit 0 뒤에도 해당 Job의 자손을 종료하고 활성 프로세스가 0이 될 때까지 기존 정리 한도 안에서 기다립니다. 검증은 CLI 종료 직후 자손 부재를 검사하며 테스트 자체의 rescue 정리 결과를 제품의 정리 성공으로 세지 않습니다.

GitHub Actions에서 **검증할 커밋 SHA**의 `ci` → `test (windows-latest)`를 엽니다. `go test -count=1 -race ./...` 단계의 `TestFastInflowSlowHook` 네 조합·`TestTargetsRunIndependently`·`TestBoundedOverloadCLI`, 알림 검증 단계의 여섯 모드, 반복 단계의 세 테스트와 네 옵션 조합에 `PASS`가 있고 `SKIP`이 없는지, 의도적 실패 검증 단계는 성공했는지 확인합니다. `e2e-diagnostics-windows-latest` artifact에서 알림 정리 전후 기록과 `notification.log`·`failure.log`·`timeout.log`를 받습니다. 과거 다른 SHA의 성공, 교차 컴파일, Linux Docker 결과는 해당 커밋의 Windows 실행 증거가 아닙니다. 실행 결과가 없으면 Windows 항목은 `Not verified`로 남깁니다.

Windows 실패 시 먼저 `records.log.processes`의 **정리 전** 세대·깊이·PID·생존·생성 시각·프로세스 접근 오류와 `records.log`의 마지막 `signal`·`complete`를 비교합니다. `cleanup.log`는 **정리 후** 상태이며 정상 종료 판정을 대신하지 않습니다. `GenerateConsoleCtrlEvent`·`AllocConsole` 오류는 콘솔 연결과 동일 콘솔의 프로세스 그룹을 확인합니다. 접근 거부·대기 API 오류는 생존으로 취급하여 실패시키며, PID 재사용 혼동을 막기 위해 준비 시 연 프로세스 handle을 정리 후까지 유지합니다. 실패 정리는 기록된 해당 실행의 보조 바이너리 handle만 종료하며, watcher가 아직 살아 있으면 해당 watcher의 트리에만 `TASKKILL /T /F`를 적용합니다.

```powershell
$env:DW_E2E_FAULT = 'failure' # timeout도 같은 방법으로 실행
go test -count=1 -v -timeout=2m -run '^TestProcessLifecycle$' ./e2e/scenarios
Remove-Item Env:DW_E2E_FAULT
```

이 명령은 0이 아닌 종료가 기대 결과입니다. `diagnostics retained:` 경로의 진단 파일과 `cleanup.log`의 세 `alive=false`를 확인합니다. `DW_E2E_DIAGNOSTICS`로 진단 상위 디렉터리를 지정할 수 있으며 CI는 `${{ runner.temp }}/dw-e2e`를 사용합니다.

## 워크트리 병렬 실행과 격리

서로 다른 터미널에서 각 워크트리 루트로 이동한 뒤 위 명령을 실행합니다. 로컬과 Linux 실행을 동시에 실행해도 됩니다.

- 바이너리·설정·감시 대상·기록은 실행별 임시 디렉터리에 생성합니다.
- Linux 실행기는 Git이 추적하는 현재 파일과 ignore되지 않은 새 파일을 복사합니다. 미커밋 수정·추가·삭제를 반영하며, 무시된 파일과 Git 저장소 자체는 복사하지 않습니다.
- 컨테이너는 매번 새 ID를 사용합니다. 호스트 bind mount·공유 볼륨·고정 컨테이너 이름이 없으며, 빌드·파일 변경·hook 검증은 컨테이너 내부에서 수행합니다.
- 컨테이너의 테스트 종료·실패·실행기 제한 시간 초과 후 로그를 복사하고 컨테이너를 제거합니다. 실행 중 터미널을 강제 종료하거나 실행기를 SIGKILL로 죽이면 정리 코드를 실행할 수 없습니다. 이때 출력된 컨테이너 ID로 `docker rm --force <컨테이너 ID>`를 실행합니다. 다른 실행의 컨테이너를 일괄 삭제하지 마세요.

## 코드 구성과 시나리오 추가

### 책임별 코드 위치

| 파일 | 책임 |
|---|---|
| `scenarios/main_test.go` | 시나리오 패키지의 `TestMain`. 테스트 실행 전체의 준비·정리를 `harness.Run`에 맡김 |
| `scenarios/*_test.go` | 시나리오 테스트. 대응은 [시나리오와 지원 코드의 대응](#시나리오와-지원-코드의-대응)에 있음 |
| `harness/harness.go` | 테스트 실행 전체의 콘솔 준비, watcher·`hookrec` 빌드와 바이너리 삭제(`Run`, `WatcherBin`, `HookrecBin`). 시나리오별 실행 폴더(`NewScenario`), watcher 실행(`Start`·`Launch`), 보조 프로세스 실행(`StartProcess`), 출력·종료 관찰(`Log`·`Done`·`Exited`·`ExitErr`), 관찰 기반 대기(`Await`), 정상 종료 검사(`Stop`), 진단 보존·정리(`cleanup`) |
| `harness/hook_records.go` | `hookrec` 한 줄 기록 해석. 기록 읽기(`Lines`·`ReadLinesLenient`), hook 명령 조립(`Hook`), 기록 대기(`AwaitLine`), 기록 개수(`CountRecord`), 지연 hook의 `BEGIN` 기록(`delayedHooks`·`DelayedHookBegun`), 수신 이벤트의 완료 경계(`AwaitChangesComplete`) |
| `harness/process_records.go` | 프로세스 트리 기록(`ProcessRecord`) 해석. 세대 준비 대기(`AwaitReadyGenerations`), 생존·부재 확인(`Alive`·`AssertRunning`·`ProcessesGone`·`AwaitProcessesGone`·`AwaitDelayedHooksGone`), 정상 완료 판정(`AssertGracefulCompletion`), 단계별 생존 기록(`SnapshotProcesses`), 시나리오 기록에 한정한 강제 정리(`rescueRecordedProcesses`) |
| `harness/signal_unix.go`, `harness/signal_windows.go` | 플랫폼별 프로세스 그룹, 정상 종료 신호, watcher 강제 종료, 시나리오가 소유하는 `processObserver` |
| `harness/hook_records_test.go` | 수신 경계·완료 판정 함수의 단위 검증 |
| `harness/cleanup_windows_test.go` | Windows에서 실행 폴더·바이너리 삭제 실패가 비정상 종료로 드러나는지 검증. 같은 `Run`을 거치는 하네스 패키지의 `TestMain` 포함 |
| `hookrec/` | 한 줄 기록 또는 실제 부모·자식·손자 프로세스를 만드는 보조 프로그램 |
| `linux/` | 현재 소스 사본을 독립 Linux 컨테이너로 전달하고 같은 Go E2E를 실행하는 진입점 |

바이너리 빌드와 콘솔 준비는 테스트 실행 전체가 한 번 수행합니다. 실행 폴더, watcher, 프로세스 관찰과 정리는 시나리오마다 소유합니다. Windows의 `processObserver`는 기록된 PID를 처음 관찰할 때 연 handle을 해당 시나리오의 정리가 끝날 때까지 유지하고, 강제 정리는 그 시나리오의 기록에 있는 `hookrec` handle만 종료합니다. 따라서 다른 시나리오나 다른 워크트리의 보호 트리는 정리 대상이 아닙니다.

### 시나리오와 지원 코드의 대응

| 시나리오 | 파일 | 검증 내용 | 사용하는 지원 코드 |
|---|---|---|---|
| `TestGoldenPath` | `scenarios/golden_path_test.go` | 생성·수정·삭제, hook 순서, 7개 이벤트 변수 | `Start`, `Hook`, `AwaitLine`, `Stop` |
| `TestProcessLifecycle` | `scenarios/process_test.go` | 시작 hook의 세 프로세스, 이전 세대 완료 후 재시작, 종료 hook 순서, `DW_E2E_FAULT` | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| `TestStartHookShutdownModes` | `scenarios/process_test.go` | `noWait`·`interruptible` 네 조합의 시작 hook 종료·교체 | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| `TestFastInflowSlowHook` | `scenarios/reliability_test.go` | 느린 hook 중 빠른 유입의 네 조합, 제외·debounce, 종료 후 실행 금지 | `DelayedHookBegun`, `AwaitChangesComplete`, `AwaitLine`, `AwaitDelayedHooksGone` |
| `TestTargetsRunIndependently` | `scenarios/reliability_test.go` | 한 대상의 느린 hook이 다른 대상을 막지 않음 | `DelayedHookBegun`, `AwaitLine`, `AwaitDelayedHooksGone` |
| `TestBoundedOverloadCLI` | `scenarios/reliability_test.go` | 전달 대기열 포화의 비정상 종료와 트리 정리 | `AwaitReadyGenerations`, `Exited`, `AwaitProcessesGone` |
| `TestEventLossNotificationCLI` | `scenarios/error_notification_test.go` | 유실 알림 여섯 모드, 알림 부모가 먼저 종료된 뒤 자손 정리, 보호 트리 생존 | `Launch`, `StartProcess`, `DelayedHookBegun`, `AwaitReadyGenerations`, `SnapshotProcesses`, `ProcessesGone`, `Alive`, `AwaitDelayedHooksGone` |
| `TestNormalChangesDoNotNotify` | `scenarios/error_notification_test.go` | 일반 변경·debounce에서 알림 미실행 | `AwaitChangesComplete`, `AwaitLine` |
| `TestCleanupReportsLockedResources` | `harness/cleanup_windows_test.go` | Windows 삭제 실패의 비정상 종료 | `cleanup` |

### 새 시나리오 작성

1. `scenarios/`에 새 `*_test.go`를 만들고(`package scenarios`, `harness` 패키지 import) 설정·입력 순서·성공 조건을 작성합니다. 특정 OS 전용 build tag는 해당 OS에서 실제로 실행해 원인을 확인한 경우에만 붙입니다.
2. `s := harness.NewScenario(t)`로 실행을 만들고, 설정 YAML의 hook 명령은 `s.Hook("--line=...")`으로 조립합니다. 설정에 `excludeSuffix: [.probe]`를 넣고 `s.Start(config)`를 호출합니다. `Start`는 DEBUG 로그에서 제외된 probe 파일의 실제 이벤트를 관찰해 감시 준비를 확인합니다. 기본 ERROR 로그 수준을 검증하려면 `s.Launch(harness.WatcherBin(), config)`로 실행하고 시작 hook 기록으로 준비를 확인합니다. 고정 sleep은 준비·완료 판정에 쓰지 않습니다.
3. `s.Write`로 행동하고, 기록 개수 대신 기다리는 hook·경로·세대를 `s.AwaitLine`, `s.AwaitReadyGenerations`, `s.Await`의 조건으로 지정합니다. 정확한 개수·순서는 대기 뒤 테스트에서 직접 검사합니다.
4. `s.Stop()`으로 정상 종료와 종료 코드 0을 검사한 뒤 프로세스 기록을 판정합니다. 시나리오 정리의 강제 종료는 테스트가 끝난 뒤 실행되며 정상 종료 증거가 아닙니다.

```go
func TestCreateAnotherFile(t *testing.T) {
	s := harness.NewScenario(t)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watchEvent: C
      excludeSuffix: [.probe]
`, s.Watch, s.Hook("--line=CHANGE:{{.FileName}}")))
	s.Write(filepath.Join(s.Watch, "another.txt"), nil)
	s.AwaitLine("create hook", 10*time.Second, "CHANGE:another.txt")
	s.Stop()
	if got := harness.CountRecord(s.Lines(), "CHANGE:another.txt"); got != 1 {
		t.Fatalf("create hook count = %d, want 1", got)
	}
}
```

장시간 프로세스는 `s.Hook("--depth=2")`를 시작·변경 hook에 넣습니다. 부모는 자식, 자식은 손자를 실제로 실행합니다. 세대 식별자는 부모 PID이며 각 프로세스는 `세대|PID|깊이|상태`를 기록합니다. 부모의 `ready`는 자식의 `ready` 뒤에 기록되므로 `AwaitReadyGenerations`는 부모의 `ready`로 세대 전체의 준비를 판정합니다. 종료 신호를 받으면 150ms 처리 지연과 자식의 정상 종료를 기다린 뒤 `complete`를 남깁니다. 지연 hook은 `s.Hook("--delay=80ms", "--line=...")`으로 만들며 `BEGIN|PID|기록`과 취소 시 `CANCEL|PID|기록`을 남깁니다.

## 실패·시간 초과 진단

로컬 실패 시 `diagnostics retained:` 뒤의 경로에 `config.yml`, `records.log`, `watcher.log`, `watcher.pid`, `cleanup.log`를 남깁니다. 성공 시에도 보존하려면 `DW_E2E_KEEP=1`을 설정합니다. `cleanup.log`에서 보조 프로세스의 `alive=false`를 확인하고 watcher PID도 종료되었는지 확인합니다. Windows에는 정리 전 `records.log.processes`도 남깁니다.

보존하지 않는 실행 폴더의 삭제 오류는 해당 테스트를 실패시키고, 테스트 바이너리 폴더의 삭제 오류는 최종 종료 코드를 1로 만듭니다. `TestCleanupReportsLockedResources`는 Windows에서 실제 삭제 방지 handle을 열어 두 오류가 성공으로 숨겨지지 않는지 검사합니다. 실패 또는 `DW_E2E_KEEP=1`로 실행 폴더를 보존한 경우에는 그 폴더의 삭제를 검증한 것이 아닙니다.

Linux는 `Linux diagnostics:` 뒤의 호스트 경로에 `runner.log`와 컨테이너 `/tmp`의 진단을 복사합니다. 제한 시간 초과 시 실행기 로그의 `deadline`과 watcher 로그의 마지막 이벤트·hook, 기록의 마지막 `signal`·`complete`를 비교합니다. 컨테이너 ID가 더 이상 `docker inspect <ID>`에 나타나지 않아야 합니다.

정리 경로를 일부러 검사하는 아래 명령은 **실패가 기대 결과**입니다. `failure`는 실제 프로세스 트리 준비 직후 실패하고, `timeout`은 준비 후 충족되지 않는 조건의 제한 시간을 초과합니다. 두 경우 모두 진단 경로와 PID 종료, Linux 컨테이너 제거를 확인합니다.

```sh
DW_E2E_FAULT=failure go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=timeout go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=failure go run ./e2e/linux -- -run '^TestProcessLifecycle$'
DW_E2E_FAULT=timeout go run ./e2e/linux -- -run '^TestProcessLifecycle$'
```

## 경합과 과부하 검증

```sh
go test -count=3 -v -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle' ./e2e/scenarios
go run ./e2e/linux -- -count=3 -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle'
go test -race ./...
```

`TestFastInflowSlowHook`는 noWait·interruptible의 네 조합을 검사합니다. 80ms hook 실행 중 10ms 간격으로 고유 파일 24개 생성, 같은 파일 24회 변경, 제외 파일 24회 변경을 약 0.3초 동안 입력합니다. 유입 배치의 실제 마지막 수신 유효 경로와 완료 기록을 대조하고, 수신·기록이 120ms 동안 변하지 않으며 시작된 hook이 모두 완료 또는 취소된 뒤 별도 `last.txt`를 생성합니다. 그 뒤 제외 파일을 한 번 더 변경하고 고립된 유효 변경의 완료를 기다립니다. 비중단 noWait에서는 생성 호출 횟수가 아니라 관찰된 유효 수신 이벤트 수만큼의 완료와 고유 파일 전체의 완료를 모두 검사합니다. 종료 시 대기 작업을 실행하지 않고 프로세스가 사라지는지도 검사합니다. `hookrec --delay=80ms`의 `BEGIN|PID|기록`, `CANCEL|PID|기록`과 완료 기록으로 실행·취소·잔존 PID를 관찰합니다.

`TestStartHookShutdownModes`는 세 OS에서 네 조합의 장시간 시작 hook, 변경·종료 경합과 실제 부모·자식·손자 정리를 검사합니다. `TestTargetsRunIndependently`는 한 대상의 800ms hook이 다른 대상의 처리를 막지 않는지 검사합니다. `TestBoundedOverloadCLI`는 시작 hook 실행 중 유효 생성 이벤트 257개 이상을 입력하여 애플리케이션 전달 대기열 포화, 대상·원인 로그, 비정상 종료와 프로세스 트리 정리를 검사합니다.

제어된 내부 테스트는 OS 알림 개수와 무관하게 noWait의 정확한 순서, debounce의 마지막 유효 이벤트, 제외 이벤트, 취소 완료 전 교체 금지, 종료 후 실행 금지, 첫 치명 오류 보존을 검사합니다. 대상별 대기 상한은 256개이며 실행은 한 세대, debounce 타이머는 하나입니다. 대기열이 가득 차면 버퍼를 무한 확장하거나 무한 대기하지 않고 유실 오류로 종료합니다. 종료는 대기열을 처리 완료하는 동작이 아니라 취소·정리하는 동작입니다.

제어 입력 runner 테스트는 실제 fsnotify watcher와 감시 등록을 유지하고 수신 채널만 바꿉니다. 관련 테스트는 `Stop` 완료 후 임시 디렉터리를 즉시 삭제해 자원 반납을 검사합니다. `TestNativeWatcherStopReleasesDirectory`는 원래 채널로 실제 생성 이벤트를 처리한 뒤 종료 hook과 디렉터리 삭제를 검사합니다. `TestFilesystemPredicatesReleaseResources`는 존재·디렉터리 판별의 기존 bool 결과와 호출 직후 파일·디렉터리 삭제를 함께 검사합니다.

OS E2E는 파일 생성 순서로 마지막 수신 경로를 정하지 않습니다. `harness/hook_records_test.go`는 `last.txt` 뒤에 `normal-19.txt`를 수신한 경계, 빈 기록·미완료 hook의 거부, noWait의 중복 수신별 완료를 검사합니다. 120ms 안정 구간은 관찰 조건이지 OS가 앞으로 알림을 보내지 않는다는 보장은 아닙니다. 늦은 유효 이벤트가 고립된 변경을 대체하면 테스트는 성공 처리하지 않고 기존 제한 시간 안에 실패합니다. 배치 관찰과 고립된 변경은 기존 10초·30초 완료 대기 예산을 공유합니다.

### 이벤트 유실 알림 시나리오

```sh
go test -count=1 -v -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify' ./e2e/scenarios
go run ./e2e/linux -- -count=1 -run 'TestEventLossNotificationCLI|TestNormalChangesDoNotNotify'
```

`TestEventLossNotificationCLI`는 실제 전달 대기열 포화로 CLI의 `global.lifeCycle.onError`를 실행합니다. 환경 변수·한 번 실행·hook 생략·실패·시간 초과에서도 원래 치명적 오류와 비정상 종료가 보존되는지 검사합니다. `TestNormalChangesDoNotNotify`는 일반 noWait 변경과 debounce에서 배치의 실제 수신·완료와 안정 구간을 관찰한 뒤 고립된 변경을 보내고, 두 단계에서 알림을 실행하지 않는지 검사합니다. 이 테스트의 포화는 애플리케이션 대기열 유실이며 OS overflow 관찰 증거가 아닙니다.

내부 YAML 설정 테스트 `go test -count=1 -run '^TestDecoded' ./internal/config/yaml`은 디코딩된 구조화 명령을 실제로 실행하여 환경 변수의 원문 전달과 `dir` 작업 디렉토리, 기존 설정 호환성을 검사합니다. 알림의 환경 변수 및 고정 5초 실행 제한·최대 20초 취소 정리 계약은 [이벤트 유실 알림](../README.ko.md#이벤트-유실-알림)을 따릅니다.

macOS·Linux 알림 실패·시간 초과 시나리오는 부모가 먼저 실패한 뒤 남은 자손까지 정리되는지 검사합니다. 알림의 프로세스 그룹에는 TERM 후 필요 시 KILL을 적용하며, 5초 실행·20초 정리 상한을 공유합니다. Windows의 같은 시나리오는 Job Object로 자손을 정리하며, 확인 방법은 [Windows 실행과 CI 결과 확인](#windows-실행과-ci-결과-확인)에 있습니다.

### 유실 검증의 구분

- 실제 파일시스템 부하 테스트는 OS 알림의 중복·병합을 허용합니다. 파일 조작 횟수와 hook 횟수를 같다고 단정하지 않습니다.
- CLI까지의 오류 전달은 검증 전용 소스 overlay로 fsnotify 유실 오류를 주입해 별도 검사할 수 있습니다. noWait·interruptible 네 조합에서 최초 치명적 오류 보존, 알림 한 번 실행, 다른 대상의 새 일반 hook 차단을 확인합니다. overlay 실행 명령과 결과는 이슈 보고서에 남기며 배포 바이너리에는 주입 옵션이 없습니다. 주입 성공은 실제 OS overflow 관찰 증거가 아닙니다.
- 실제 Linux overflow 실험은 fsnotify 수신을 잠시 멈추고 현재 `max_queued_events`보다 많은 파일을 생성한 뒤 `ErrEventOverflow`를 관찰합니다. 호스트·공유 커널 설정은 변경하지 않습니다. macOS kqueue는 이 sentinel을 보고하지 않으므로 같은 보장을 주장하지 않습니다.
- fsnotify v1.7.0의 [이벤트·오류 계약](https://github.com/fsnotify/fsnotify/blob/v1.7.0/backend_inotify.go)과 [공통 오류](https://github.com/fsnotify/fsnotify/blob/v1.7.0/fsnotify.go)를 기준으로 합니다. 모든 OS 유실의 감지, 무제한 입력의 무손실 처리, 자동 복구는 보장하지 않습니다. 임의의 종료 hook에는 별도 제한 시간이 없으며, 프로세스 트리 검증은 SIGTERM 또는 Windows CTRL+BREAK를 처리하고 자손을 기다리는 보조 프로그램을 대상으로 합니다.

이슈 보고서의 주입 스모크는 아래 명령으로 재현합니다. `<이슈 보고서 디렉토리>`는 `smoke.sh`와 overlay 테스트 파일이 있는 경로로 바꿉니다.

```sh
sh <이슈 보고서 디렉토리>/smoke.sh macos
sh <이슈 보고서 디렉토리>/smoke.sh linux
```

`TestControlledLossNotificationSmoke`는 네 옵션 조합과 유실이 아닌 치명적 오류의 알림 제외를 검사합니다. 스크립트는 Go overlay와 임시 생성자 주입을 사용하며 저장소 소스를 수정하지 않습니다.

