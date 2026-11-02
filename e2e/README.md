# 자동 E2E 실행

실제 현재 소스의 watcher와 Go 보조 프로그램을 빌드하고 파일 이벤트·hook 기록·프로세스 종료를 검사합니다. macOS 로컬과 Linux Docker는 같은 시나리오를 실행합니다. Windows CI에서는 기존 생성·수정·삭제, hook 순서와 이벤트 변수 검증을 유지하며, 강제 종료하므로 종료 hook과 프로세스 트리 시나리오는 제외합니다.

## 준비와 실행

저장소 루트에서 실행합니다. 로컬은 `go.mod`에 지정된 버전 이상의 Go가 필요합니다. Linux 실행에는 Docker CLI와 실행 중인 Linux Docker 엔진도 필요합니다. Java·Gradle은 사용하지 않습니다.

```sh
make e2e
make e2e-linux
```

성공하면 각 테스트에 `PASS`가 표시되고 명령이 0으로 종료합니다. 실패·시간 초과는 0이 아닌 종료 코드로 전달합니다. Linux 기본 이미지는 `golang:1.23-bookworm`이며, 다른 Go 이미지는 `go run ./e2e/linux -image=<이미지>`로 지정합니다.

### 특정 시나리오 선택·반복

```sh
go test -count=3 -v -run '^TestGoldenPath$' ./e2e
go test -count=3 -v -run '^TestProcessLifecycle$' ./e2e
go run ./e2e/linux -- -count=3 -run '^TestGoldenPath$'
go run ./e2e/linux -- -count=3 -run '^TestProcessLifecycle$'
```

모든 실행에 `-count`를 지정해 Go 결과 캐시를 사용하지 않습니다. Linux 실행기는 기본 `-count=1` 뒤에 전달 인자를 붙이므로 위의 반복 횟수가 적용됩니다. 컨테이너 전체 제한 시간은 `go run ./e2e/linux -deadline=10m -- -run TestProcessLifecycle`처럼 지정합니다. Go 테스트 자체의 제한 시간은 `--` 뒤의 `-timeout=2m`으로 지정할 수 있습니다.

## 워크트리 병렬 실행과 격리

서로 다른 터미널에서 각 워크트리 루트로 이동한 뒤 위 명령을 실행합니다. 로컬과 Linux 실행을 동시에 실행해도 됩니다.

- 바이너리·설정·감시 대상·기록은 실행별 임시 디렉터리에 생성합니다.
- Linux 실행기는 Git이 추적하는 현재 파일과 ignore되지 않은 새 파일을 복사합니다. 미커밋 수정·추가·삭제를 반영하며, 무시된 파일과 Git 저장소 자체는 복사하지 않습니다.
- 컨테이너는 매번 새 ID를 사용합니다. 호스트 bind mount·공유 볼륨·고정 컨테이너 이름이 없으며, 빌드·파일 변경·hook 검증은 컨테이너 내부에서 수행합니다.
- 컨테이너의 테스트 종료·실패·실행기 제한 시간 초과 후 로그를 복사하고 컨테이너를 제거합니다. 실행 중 터미널을 강제 종료하거나 실행기를 SIGKILL로 죽이면 정리 코드를 실행할 수 없습니다. 이때 출력된 컨테이너 ID로 `docker rm --force <컨테이너 ID>`를 실행합니다. 다른 실행의 컨테이너를 일괄 삭제하지 마세요.

## 시나리오 추가

| 구성 | 책임 |
|---|---|
| `harness_test.go` | 실행별 데이터, 실제 watcher 빌드·시작, 관찰 기반 대기, 종료 상태 검사, 진단 보존·정리 |
| `golden_path_test.go` | 생성·수정·삭제, hook 순서, 7개 이벤트 변수 |
| `process_test.go` | 시작 hook의 장시간 작업, 종료 처리 후 재시작, 부모·자식·손자 종료 |
| `hookrec/main.go` | 한 줄 기록 또는 실제 하위 프로세스 생성, 세대·PID·시작·준비·신호·종료 완료 기록 |
| `linux/main.go` | 현재 소스 사본을 독립 Linux 컨테이너로 전달, 종료 코드·로그·컨테이너 수명 관리 |

1. 새 `*_test.go`에 설정·행동·기대 결과를 작성합니다. macOS·Linux 전용이면 `//go:build !windows`를 붙입니다.
2. `s := newScenario(t)`로 실행을 만들고 `s.start(config)`를 호출합니다. 설정에는 `excludeSuffix: [.probe]`를 넣습니다. 공통 시작 함수는 제외된 probe 파일을 변경하고 실제 watcher 이벤트 로그를 관찰해 감시 준비를 확인합니다. 시작 hook 기록이나 고정 sleep은 준비 판정에 쓰지 않습니다.
3. `s.write`, `s.waitLines`, `s.await`로 실제 행동과 제한 시간 내 결과를 확인하고 `s.stop()`으로 정상 종료 코드까지 검사합니다. 공통 준비·정리 코드를 복제하지 않습니다.

예를 들어, golden 설정의 기존 함수와 보조 프로그램을 재사용하는 새 시나리오는 다음과 같습니다.

```go
func TestCreateAnotherFile(t *testing.T) {
    s := newScenario(t)
    s.start(buildConfig(hookrecBin, s.records, s.watch))
    s.waitLines(2)
    path := filepath.Join(s.watch, "another.txt")
    f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
    if err != nil { t.Fatal(err) }
    if err := f.Close(); err != nil { t.Fatal(err) }
    s.waitLines(5)
    s.stop()
    if !strings.Contains(strings.Join(readLinesLenient(s.records), "\n"), "TARGET:onChange:"+path+"|") {
        t.Fatal("created file was not reported")
    }
}
```

장시간 프로세스는 `hookrec --file=<기록 경로> --depth=2`를 시작·변경 hook에 넣습니다. 부모는 자식, 자식은 손자를 실제로 실행합니다. 세대 식별자는 부모 PID이며 각 프로세스는 `세대|PID|깊이|상태`를 기록합니다. 종료 신호를 받은 뒤 150ms 처리 지연과 자식의 정상 종료를 기다리고 `complete`를 남깁니다. 새 세대의 `start` 전에 이전 세대 전체의 완료 기록이 있어야 하며, 종료 후 기록된 PID가 살아 있지 않아야 합니다.

## 실패·시간 초과 진단

로컬 실패 시 `diagnostics retained:` 뒤의 경로에 `config.yml`, `records.log`, `watcher.log`, `watcher.pid`, `cleanup.log`를 남깁니다. 성공 시에도 보존하려면 `DW_E2E_KEEP=1`을 설정합니다. `cleanup.log`에서 보조 프로세스의 `alive=false`를 확인하고 watcher PID도 종료되었는지 확인합니다. Windows golden 실행에는 보조 프로세스 트리 기록이 없습니다.

Linux는 `Linux diagnostics:` 뒤의 호스트 경로에 `runner.log`와 컨테이너 `/tmp`의 진단을 복사합니다. 제한 시간 초과 시 실행기 로그의 `deadline`과 watcher 로그의 마지막 이벤트·hook, 기록의 마지막 `signal`·`complete`를 비교합니다. 컨테이너 ID가 더 이상 `docker inspect <ID>`에 나타나지 않아야 합니다.

정리 경로를 일부러 검사하는 아래 명령은 **실패가 기대 결과**입니다. `failure`는 실제 프로세스 트리 준비 직후 실패하고, `timeout`은 준비 후 충족되지 않는 조건의 제한 시간을 초과합니다. 두 경우 모두 진단 경로와 PID 종료, Linux 컨테이너 제거를 확인합니다.

```sh
DW_E2E_FAULT=failure go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e
DW_E2E_FAULT=timeout go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e
DW_E2E_FAULT=failure go run ./e2e/linux -- -run '^TestProcessLifecycle$'
DW_E2E_FAULT=timeout go run ./e2e/linux -- -run '^TestProcessLifecycle$'
```

## 경합과 과부하 검증

```sh
go test -count=3 -v -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle' ./e2e
go run ./e2e/linux -- -count=3 -run 'TestFastInflowSlowHook|TestStartHookShutdownModes|TestTargetsRunIndependently|TestBoundedOverloadCLI|TestProcessLifecycle'
go test -race ./...
```

`TestFastInflowSlowHook`는 noWait·interruptible의 네 조합을 검사합니다. 80ms hook 실행 중 10ms 간격으로 고유 파일 24개 생성, 같은 파일 24회 변경, 제외 파일 25회 변경을 약 0.3초 동안 입력합니다. 유입 종료 뒤 마지막 유효 변경 완료를 기다리고, 비중단 noWait에서는 고유 파일 전체가 완료되었는지 확인합니다. 종료 시 대기 작업을 실행하지 않고 프로세스가 사라지는지도 검사합니다. `hookrec --delay=80ms`의 `BEGIN|PID|기록`, `CANCEL|PID|기록`과 완료 기록으로 실행·취소·잔존 PID를 관찰합니다.

`TestStartHookShutdownModes`는 네 조합의 장시간 시작 hook, 변경·종료 경합과 실제 부모·자식·손자 정리를 검사합니다. `TestTargetsRunIndependently`는 한 대상의 800ms hook이 다른 대상의 처리를 막지 않는지 검사합니다. `TestBoundedOverloadCLI`는 시작 hook 실행 중 유효 생성 이벤트 257개 이상을 입력하여 애플리케이션 전달 대기열 포화, 대상·원인 로그, 비정상 종료와 프로세스 트리 정리를 검사합니다.

제어된 내부 테스트는 OS 알림 개수와 무관하게 noWait의 정확한 순서, debounce의 마지막 유효 이벤트, 제외 이벤트, 취소 완료 전 교체 금지, 종료 후 실행 금지, 첫 치명 오류 보존을 검사합니다. 대상별 대기 상한은 256개이며 실행은 한 세대, debounce 타이머는 하나입니다. 대기열이 가득 차면 버퍼를 무한 확장하거나 무한 대기하지 않고 유실 오류로 종료합니다. 종료는 대기열을 처리 완료하는 동작이 아니라 취소·정리하는 동작입니다.

### 유실 검증의 구분

- 실제 파일시스템 부하 테스트는 OS 알림의 중복·병합을 허용합니다. 파일 조작 횟수와 hook 횟수를 같다고 단정하지 않습니다.
- CLI까지의 오류 전달은 검증 전용 소스 overlay로 fsnotify 유실 오류를 주입해 별도 검사할 수 있습니다. 배포 바이너리에는 주입 옵션이 없습니다. 주입 성공은 실제 OS overflow 관찰 증거가 아닙니다.
- 실제 Linux overflow 실험은 fsnotify 수신을 잠시 멈추고 현재 `max_queued_events`보다 많은 파일을 생성한 뒤 `ErrEventOverflow`를 관찰합니다. 호스트·공유 커널 설정은 변경하지 않습니다. macOS kqueue는 이 sentinel을 보고하지 않으므로 같은 보장을 주장하지 않습니다.
- fsnotify v1.7.0의 [이벤트·오류 계약](https://github.com/fsnotify/fsnotify/blob/v1.7.0/backend_inotify.go)과 [공통 오류](https://github.com/fsnotify/fsnotify/blob/v1.7.0/fsnotify.go)를 기준으로 합니다. 모든 OS 유실의 감지, 무제한 입력의 무손실 처리, 자동 복구는 보장하지 않습니다. 임의의 종료 hook에는 별도 제한 시간이 없으며, 프로세스 트리 검증은 SIGTERM을 처리하고 자손을 기다리는 보조 프로그램을 대상으로 합니다.
