# E2E 실패·시간 초과 진단과 격리

🇺🇸 [English](diagnostics.md) | 🇰🇷 한국어

## 실패·시간 초과 진단

- 로컬 실패 시 `diagnostics retained:` 뒤의 경로에 `config.yml`, `records.log`, `watcher.log`, `watcher.pid`, `cleanup.log`를 남깁니다.
- 성공 시에도 보존하려면 `DW_E2E_KEEP=1`을 설정합니다.
- `cleanup.log`에서 보조 프로세스의 `alive=false`를 확인하고 watcher PID도 종료되었는지 확인합니다.
- Windows에는 정리 전 `records.log.processes`도 남기며, 분석 방법은 [Windows 실패 분석](windows.ko.md#실패-분석)에 있습니다.
- Windows에서 그 시나리오에 0xc0000142로 끝난 프로세스가 있었으면 `dll-init-failed.log`도 남깁니다. 이 파일에는 시나리오 동안 테스트 바이너리의 콘솔에서 보낸 CTRL+BREAK와 관련 종료가 시각 순서로 담기며, 같은 보고가 테스트 출력에도 나옵니다. 읽는 방법은 [hook이 0xc0000142로 끝남](windows-utm.ko.md#hook이-0xc0000142로-끝남)에 있습니다.

- 보존하지 않는 실행 폴더의 삭제 오류는 해당 테스트를 실패시키고, 테스트 바이너리 폴더의 삭제 오류는 최종 종료 코드를 1로 만듭니다.
- `TestCleanupReportsLockedResources`는 Windows에서 실제 삭제 방지 handle을 열어 두 오류가 성공으로 숨겨지지 않는지 검사합니다.
- 실패 또는 `DW_E2E_KEEP=1`로 실행 폴더를 보존한 경우에는 그 폴더의 삭제를 검증한 것이 아닙니다.

- Linux는 `Linux diagnostics:` 뒤의 호스트 경로에 `runner.log`와 컨테이너 `/tmp`의 진단을 복사합니다.
- 제한 시간 초과 시 실행기 로그의 `deadline`과 watcher 로그의 마지막 이벤트·hook, 기록의 마지막 `signal`·`complete`를 비교합니다.
- 컨테이너 ID가 더 이상 `docker inspect <ID>`에 나타나지 않아야 합니다.

### 의도적인 실패·시간 초과

- 정리 경로를 일부러 검사하는 아래 명령은 **실패가 기대 결과**입니다.
- `failure`는 실제 프로세스 트리 준비 직후 실패하고, `timeout`은 준비 후 충족되지 않는 조건의 제한 시간을 초과합니다.
- 두 경우 모두 진단 경로와 PID 종료, Linux 컨테이너 제거를 확인합니다.
- Windows PowerShell 명령은 [Windows 실패 분석](windows.ko.md#실패-분석)에 있습니다.

```sh
DW_E2E_FAULT=failure go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=timeout go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=failure go run ./e2e/linux -- -run '^TestProcessLifecycle$'
DW_E2E_FAULT=timeout go run ./e2e/linux -- -run '^TestProcessLifecycle$'
```

## 워크트리 병렬 실행과 격리

- 서로 다른 터미널에서 각 워크트리 루트로 이동한 뒤 [준비와 실행](../README.ko.md#준비와-실행)의 명령을 실행합니다.
- 로컬과 Linux 실행을 동시에 실행해도 됩니다.

- 바이너리·설정·감시 대상·기록은 실행별 임시 디렉터리에 생성합니다.
- Linux 실행기는 Git이 추적하는 현재 파일과 ignore되지 않은 새 파일을 복사합니다. 미커밋 수정·추가·삭제를 반영하며, 무시된 파일과 Git 저장소 자체는 복사하지 않습니다.
- 컨테이너는 매번 새 ID를 사용합니다. 호스트 bind mount·공유 볼륨·고정 컨테이너 이름이 없으며, 빌드·파일 변경·hook 검증은 컨테이너 내부에서 수행합니다.
- 컨테이너의 테스트 종료·실패·실행기 제한 시간 초과 후 로그를 복사하고 컨테이너를 제거합니다. 실행 중 터미널을 강제 종료하거나 실행기를 SIGKILL로 죽이면 정리 코드를 실행할 수 없습니다. 이때 출력된 컨테이너 ID로 `docker rm --force <컨테이너 ID>`를 실행합니다. 다른 실행의 컨테이너를 일괄 삭제하지 마세요.
