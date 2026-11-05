# `TestWindowsShellNonASCIIOutput`: Windows 셸 hook의 비ASCII 출력과 콘솔 출력 code page 복원

🇺🇸 [English](windows-shell-non-ascii-output.md) | 🇰🇷 한국어

- 콘솔 출력 code page가 UTF-8이 아니어도 `cmd`와 Windows PowerShell hook이 출력한 비ASCII 글자가 watcher의 출력에 UTF-8로 그대로 나오는지 확인합니다.

## 확인 내용과 성공 조건

- 테스트는 watcher를 띄우기 전에, 테스트 프로세스와 watcher가 함께 쓰는 콘솔의 출력 code page를 437로 바꿉니다.
  - 437에는 한글과 `✓`가 없으므로, hook 셸이 이 code page로 출력하면 글자가 `?`로 바뀝니다.
  - 하위 테스트가 끝나면 원래 code page로 되돌립니다.
- 전역 `onStartWatch` hook이 `한글-줄 é ✓` 한 줄을 출력합니다.
- watcher가 pipe로 내보낸 stdout과 stderr 전체가 올바른 UTF-8이고, 그 안에 `한글-줄 é ✓` 줄이 그대로 있어야 합니다.
- watcher가 종료 신호를 받고 종료 코드 0으로 끝난 뒤, 콘솔 출력 code page가 437로 돌아와 있어야 합니다.

## 설정

- 전역 `onStartWatch`만 hook을 실행하고, 감시 대상에는 hook이 없습니다.
- watcher는 `--silent`와 `--log-level=INFO`로 실행합니다.
  - `--silent`로 명령 안내 줄을 빼므로, 출력에 있는 `한글-줄 é ✓`는 hook이 출력한 줄입니다.
  - 전역 `onStartWatch`가 끝나야 감시 대상이 시작하므로, `watch start:` 로그가 hook이 끝났다는 표시입니다.

## 하위 테스트

- `cmd`는 `echo`로 출력합니다.
- `powershell`은 `Write-Output`으로 출력합니다.

## 진행 순서

```mermaid
sequenceDiagram
    autonumber
    participant testCode as 테스트
    participant consoleCP as 콘솔
    participant watcherProc as watcher
    participant shellProc as hook 셸
    testCode->>consoleCP: output code page 437
    testCode->>watcherProc: Launch --silent --log-level=INFO
    watcherProc->>consoleCP: output code page UTF-8
    watcherProc->>shellProc: global onStartWatch
    shellProc-->>watcherProc: 한글-줄 é ✓ (UTF-8)
    watcherProc-->>testCode: hook line, then watch start:
    testCode->>watcherProc: stop
    watcherProc->>consoleCP: restore output code page 437
    watcherProc-->>testCode: exit 0
    testCode->>testCode: whole output is UTF-8 with the hook line
    testCode->>consoleCP: output code page is 437
```

## 실행

- 저장소 루트에서 실행합니다.
- Windows에서만 지원하는 셸이므로 Windows 전용 build tag를 쓰며, macOS와 Linux에서는 실행 대상이 아닙니다.

```sh
make e2e-windows ARGS="-- -run '^TestWindowsShellNonASCIIOutput$'"
```

- UTM 명령은 [UTM Windows VM에서 E2E 실행](../windows-utm.ko.md)의 사전 준비를 마치고 VM을 켠 뒤 실행합니다.

## 관련 코드

- `e2e/scenarios/shell_windows_test.go`에 `TestWindowsShellNonASCIIOutput`를 정의합니다.
- 테스트는 `e2e/harness`의 `NewScenario`, `Launch`, `WatcherBin`, `Await`, `Log`, `Stop`을 사용합니다.
- watcher가 콘솔 출력 code page를 바꾸고 되돌리는 코드는 `cmd/console_windows.go`에 있습니다.
