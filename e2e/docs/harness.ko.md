# E2E 하네스 구조와 시나리오 추가

🇺🇸 [English](harness.md) | 🇰🇷 한국어

아래 표와 절차의 경로는 `e2e/` 디렉터리를 기준으로 합니다.

## 책임별 코드 위치

| 파일 | 책임 |
|---|---|
| `scenarios/main_test.go` | 시나리오 패키지의 `TestMain`. 테스트 실행 전체의 준비·정리를 `harness.Run`에 맡김 |
| `scenarios/*_test.go` | 시나리오 테스트. 대응은 [시나리오와 지원 코드의 대응](#시나리오와-지원-코드의-대응)에 있음 |
| `harness/harness.go` | 테스트 실행 전체의 콘솔 준비, watcher·`hookrec` 빌드와 바이너리 삭제(`Run`, `WatcherBin`, `HookrecBin`). 시나리오별 실행 폴더(`NewScenario`), watcher 실행(`Start`·`Launch`), 보조 프로세스 실행(`StartProcess`), 출력·종료 관찰(`Log`·`Done`·`Exited`·`ExitErr`), 관찰 기반 대기(`Await`), 정상 종료 검사(`Stop`·`StopWithin`), 진단 보존·정리(`cleanup`) |
| `harness/hook_records.go` | `hookrec` 한 줄 기록 해석. 기록 읽기(`Lines`·`ReadLinesLenient`), hook 명령 조립(`Hook`), 기록 대기(`AwaitLine`), 기록 개수(`CountRecord`), 지연 hook의 `BEGIN`·`CANCEL` 기록(`delayedHooks`·`DelayedHookBegun`·`DelayedHookCanceled`), 수신 이벤트의 완료 경계(`AwaitChangesComplete`) |
| `harness/process_records.go` | 프로세스 트리 기록(`ProcessRecord`) 해석. 세대 준비 대기(`AwaitReadyGenerations`), 생존·부재 확인(`Alive`·`AssertRunning`·`ProcessesGone`·`AwaitProcessesGone`·`AwaitDelayedHooksGone`), 정상 완료 판정(`AssertGracefulCompletion`), 단계별 생존 기록(`SnapshotProcesses`), 시나리오 기록에 한정한 강제 정리(`rescueRecordedProcesses`) |
| `harness/signal_unix.go`, `harness/signal_windows.go` | 플랫폼별 프로세스 그룹, 정상 종료 신호, watcher 강제 종료, 시나리오가 소유하는 `processObserver` |
| `harness/hook_records_test.go` | 수신 경계·완료 판정 함수의 단위 검증 |
| `harness/cleanup_windows_test.go` | Windows에서 실행 폴더·바이너리 삭제 실패가 비정상 종료로 드러나는지 검증. 같은 `Run`을 거치는 하네스 패키지의 `TestMain` 포함 |
| `harness/process_observer_windows_test.go` | Windows에서 다른 프로그램이 이미 쓰고 있는 기록된 PID를 사라진 것으로 보는지, 기록된 hook이 실행 중이면 지연 hook 부재 대기가 여전히 실패하는지 검증 |
| `hookrec/` | 한 줄 기록 또는 실제 부모·자식·손자 프로세스를 만드는 보조 프로그램 |
| `linux/` | 현재 소스 사본을 독립 Linux 컨테이너로 전달하고 같은 Go E2E를 실행하는 진입점 |

바이너리 빌드와 콘솔 준비는 테스트 실행 전체가 한 번 수행합니다. 실행 폴더, watcher, 프로세스 관찰과 정리는 시나리오마다 소유합니다. Windows의 `processObserver`는 기록된 PID를 처음 관찰할 때 연 handle을 해당 시나리오의 정리가 끝날 때까지 유지하고, 강제 정리는 그 시나리오의 기록에 있는 `hookrec` handle만 종료합니다. 기록된 프로세스는 모두 `hookrec`이므로, 처음 관찰할 때 이미 이번 실행의 `hookrec`이 아닌 프로그램이 그 PID를 쓰고 있으면 기록된 프로세스는 끝났고 PID가 재사용된 것으로 보고 사라졌다고 판정합니다. 프로그램을 확인하지 못하면 handle을 유지하고 그 handle로 생존을 판정합니다. 따라서 다른 시나리오나 다른 워크트리의 보호 트리는 정리 대상이 아닙니다.

## 시나리오와 지원 코드의 대응

| 시나리오 | 파일 | 검증 내용 | 사용하는 지원 코드 |
|---|---|---|---|
| [`TestGoldenPath`](scenarios/golden-path.ko.md) | `scenarios/golden_path_test.go` | 생성·수정·삭제, hook 순서, 7개 이벤트 변수 | `Start`, `Hook`, `AwaitLine`, `Stop` |
| [`TestProcessLifecycle`](scenarios/process-lifecycle.ko.md) | `scenarios/process_test.go` | 시작 hook의 세 프로세스, 이전 세대 완료 후 재시작, 종료 hook 순서, `DW_E2E_FAULT` | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestStartHookShutdownModes`](scenarios/start-hook-shutdown-modes.ko.md) | `scenarios/process_test.go` | `noWait`·`interruptible` 네 조합의 시작 hook 종료·교체 | `AwaitReadyGenerations`, `AssertRunning`, `AwaitProcessesGone`, `AssertGracefulCompletion` |
| [`TestFastInflowSlowHook`](scenarios/fast-inflow-slow-hook.ko.md) | `scenarios/reliability_test.go` | 느린 hook 중 빠른 유입의 네 조합, 제외·debounce, 종료 후 실행 금지 | `DelayedHookBegun`, `AwaitChangesComplete`, `AwaitLine`, `AwaitDelayedHooksGone` |
| [`TestTargetsRunIndependently`](scenarios/targets-run-independently.ko.md) | `scenarios/reliability_test.go` | 한 대상의 느린 hook이 다른 대상을 막지 않음 | `DelayedHookBegun`, `AwaitLine`, `AwaitDelayedHooksGone` |
| [`TestBoundedOverloadCLI`](scenarios/bounded-overload-cli.ko.md) | `scenarios/reliability_test.go` | 전달 대기열 포화의 비정상 종료와 트리 정리 | `AwaitReadyGenerations`, `Exited`, `AwaitProcessesGone` |
| [`TestEventLossNotificationCLI`](scenarios/event-loss-notification-cli.ko.md) | `scenarios/error_notification_test.go` | 유실 알림 여섯 모드, 알림 부모가 먼저 종료된 뒤 자손 정리, 보호 트리 생존 | `Launch`, `StartProcess`, `DelayedHookBegun`, `AwaitReadyGenerations`, `SnapshotProcesses`, `ProcessesGone`, `Alive`, `AwaitDelayedHooksGone` |
| [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.ko.md) | `scenarios/error_notification_test.go` | 일반 변경·debounce에서 알림 미실행 | `AwaitChangesComplete`, `AwaitLine` |
| [`TestCanceledCommandListStopsNextProgram`](scenarios/canceled-command-list-stops-next-program.ko.md) | `scenarios/windows_cancel_test.go` | Windows에서 취소된 `cmd` 명령 목록(`&`, `&&`, `\|\|`)의 다음 프로그램 정지 | `Launch`, `Hook`, `DelayedHookBegun`, `DelayedHookCanceled`, `Stop`, `AwaitDelayedHooksGone` |
| [`TestCanceledPowershellHookStops`](scenarios/canceled-powershell-hook-stops.ko.md) | `scenarios/windows_cancel_test.go` | Windows에서 시작 후 0–1000ms에 취소한 `powershell` hook의 정지 | `Launch`, `Hook`, `Log`, `StopWithin`, `AwaitDelayedHooksGone` |
| `TestCleanupReportsLockedResources` | `harness/cleanup_windows_test.go` | Windows 삭제 실패의 비정상 종료 | `cleanup` |
| `TestDelayedHooksGoneIgnoresPIDReusedByOtherProgram` | `harness/process_observer_windows_test.go` | Windows에서 다른 프로그램이 쓰는 기록된 PID의 부재 판정 | `NewScenario`, `AwaitDelayedHooksGone` |
| `TestDelayedHooksGoneFailsWhileHookRuns` | `harness/process_observer_windows_test.go` | Windows에서 실행 중인 기록된 hook이 있을 때 지연 hook 부재 대기의 실패 | `NewScenario`, `Await`, `DelayedHookBegun`, `AwaitDelayedHooksGone` |

시나리오 이름의 링크는 확인 내용과 성공 조건, 진행 순서 다이어그램, 하위 테스트, 실행 명령을 설명하는 시나리오 문서로 이어집니다. 여러 시나리오에 걸친 수신 경계 판정과 유실 검증의 한계는 [경합·과부하·유실 검증](reliability.ko.md)에, Windows 시나리오의 실행과 CI 확인은 [Windows E2E 실행과 CI 결과 확인](windows.ko.md)에 있습니다.

## 새 시나리오 작성

1. `scenarios/`에 새 `*_test.go`를 만들고(`package scenarios`, `harness` 패키지 import) 설정·입력 순서·성공 조건을 작성합니다. 특정 OS 전용 build tag는 해당 OS에서 실제로 실행해 원인을 확인한 경우에만 붙입니다.
2. `s := harness.NewScenario(t)`로 실행을 만들고, 설정 YAML의 hook 명령은 `s.Hook("--line=...")`으로 조립합니다. hook 명령은 기본 셸(sh, cmd)이 해석합니다. `Hook`은 템플릿이 없는 인자를 큰따옴표로 감싸고, 템플릿이 든 인자는 값이 인자 하나로 넘어가도록 감싸지 않으므로, 템플릿이 든 인자에는 `|`나 `&`처럼 셸이 해석하는 문자를 넣지 않습니다. 설정에 `exclude: ['*.probe']`를 넣고 `s.Start(config)`를 호출합니다. `Start`는 DEBUG 로그에서 제외된 probe 파일의 실제 이벤트를 관찰해 감시 준비를 확인합니다. 기본 ERROR 로그 수준을 검증하려면 `s.Launch(harness.WatcherBin(), config)`로 실행하고 시작 hook 기록으로 준비를 확인합니다. 고정 sleep은 준비·완료 판정에 쓰지 않습니다.
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
      exclude: ['*.probe']
`, s.Watch, s.Hook("--line=CHANGE:{{.FileName}}")))
	s.Write(filepath.Join(s.Watch, "another.txt"), nil)
	s.AwaitLine("create hook", 10*time.Second, "CHANGE:another.txt")
	s.Stop()
	if got := harness.CountRecord(s.Lines(), "CHANGE:another.txt"); got != 1 {
		t.Fatalf("create hook count = %d, want 1", got)
	}
}
```

테스트를 작성한 뒤 시나리오 문서를 만들고 시나리오 목록을 갱신합니다.

5. 시나리오 문서를 한국어판 `docs/scenarios/<이름>.ko.md`와 영어판 `docs/scenarios/<이름>.md` 한 쌍으로 만듭니다. 파일 이름은 테스트 이름에서 `Test`를 뺀 kebab-case입니다(`TestBoundedOverloadCLI` → `bounded-overload-cli.ko.md`, `bounded-overload-cli.md`). 두 판의 절 구성과 사실은 같게 쓰고, 절 이름은 각 언어판의 기존 문서를 따릅니다. [`TestTargetsRunIndependently`](scenarios/targets-run-independently.ko.md) 문서처럼 제목을 `` `<테스트 이름>`: <확인하는 동작> ``으로 쓰고, 제목 바로 아래에 상대 언어판으로 가는 언어 표시 줄을 둡니다. 한국어판에는 `🇺🇸 [English](<이름>.md) | 🇰🇷 한국어`를, 영어판에는 `🇺🇸 English | 🇰🇷 [한국어](<이름>.ko.md)`를 씁니다. 한 문장 요약 뒤에 `확인 내용과 성공 조건`, `설정`, `진행 순서`(Mermaid `sequenceDiagram`), `실행`, `관련 코드` 절을 둡니다. 하위 테스트가 있으면 [`TestNormalChangesDoNotNotify`](scenarios/normal-changes-do-not-notify.ko.md)처럼 `하위 테스트` 절을 추가합니다. `실행` 절에는 로컬·Linux Docker·UTM Windows VM 명령을, `관련 코드` 절에는 테스트 파일과 사용하는 `harness` 함수를 적습니다.
6. `harness.ko.md`와 `harness.md`의 [시나리오와 지원 코드의 대응](#시나리오와-지원-코드의-대응) 표에 각각 행을 추가하고, 시나리오 이름을 같은 언어판 문서로 연결합니다. `harness.ko.md`에서는 `<이름>.ko.md`로, `harness.md`에서는 `<이름>.md`로 연결합니다. E2E README 두 판의 `세부 문서` 표에 있는 `시나리오별 문서` 행에도 같은 언어판 링크를 추가합니다. [`README.ko.md`](../README.ko.md)에는 `<이름>.ko.md`를, [`README.md`](../README.md)에는 `<이름>.md`를 연결합니다.
7. 시나리오가 아래 조건에 해당하면 해당 위치의 `-run` 정규식과 시나리오 목록에도 추가합니다. 아래 위치 중 문서는 한국어판과 영어판을 모두 고치고, 추가하는 시나리오 문서 링크는 같은 언어판을 가리키게 합니다. 예를 들어 `reliability.ko.md`에는 `<이름>.ko.md`를, `reliability.md`에는 `<이름>.md`를 연결합니다.
   - **경합·과부하 검증**: [경합과 과부하 검증](reliability.ko.md#경합과-과부하-검증)의 두 반복 명령과 그 아래 문서 링크. Windows CI에서 이름으로 따로 실행하지 않는다면 [CI 결과 확인](windows.ko.md#ci-결과-확인)에서 `go test -count=1 -race ./...` 단계의 확인 대상에도 추가합니다.
   - **유실 알림 검증**: [이벤트 유실 알림 검증](reliability.ko.md#이벤트-유실-알림-검증)의 두 명령과 그 아래 문서 링크.
   - **Windows CI의 정상 종료 반복**: 저장소 루트의 `.github/workflows/ci.yml`에 있는 `Repeat Windows graceful lifecycle without test cache` 단계, Windows 문서의 [실행 명령](windows.ko.md#실행-명령)·[CI 단계](windows.ko.md#ci-단계)·[CI 결과 확인](windows.ko.md#ci-결과-확인) 절, UTM 문서의 [테스트 선택·반복·시간 제한](windows-utm.ko.md#테스트-선택반복시간-제한)에 있는 첫 번째 예시 명령.
   - **Windows CI의 알림 프로세스 트리 검증**: 같은 `ci.yml`의 `Verify Windows error notification process trees` 단계, Windows 문서의 [CI 단계](windows.ko.md#ci-단계)·[CI 결과 확인](windows.ko.md#ci-결과-확인) 절.

## `hookrec` 프로세스 트리와 지연 hook

장시간 프로세스는 `s.Hook("--depth=2")`를 시작·변경 hook에 넣습니다. 부모는 자식, 자식은 손자를 실제로 실행합니다. 세대 식별자는 부모 PID이며 각 프로세스는 `세대|PID|깊이|상태`를 기록합니다. 부모의 `ready`는 자식의 `ready` 뒤에 기록되므로 `AwaitReadyGenerations`는 부모의 `ready`로 세대 전체의 준비를 판정합니다. 종료 신호를 받으면 150ms 처리 지연과 자식의 정상 종료를 기다린 뒤 `complete`를 남깁니다. 지연 hook은 `s.Hook("--delay=80ms", "--line=...")`으로 만들며 `BEGIN|PID|기록`과 취소 시 `CANCEL|PID|기록`을 남깁니다. `--cancel-exit-code=<코드>`를 더하면 `CANCEL`을 기록한 뒤 그 코드로 끝나며, 셸의 `||`처럼 앞 프로그램의 실패로 다음 프로그램을 시작하는 명령 목록에 씁니다.
