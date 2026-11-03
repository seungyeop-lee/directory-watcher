# UTM Windows VM에서 E2E 실행

Mac에서 UTM의 Windows 11 ARM64 VM을 켜고, 현재 작업 트리의 소스로 `./e2e/scenarios` E2E를 게스트 안에서 실행한 뒤 결과를 Mac으로 가져옵니다. 실행기는 `e2e/windows`에 있고 macOS에서만 빌드됩니다.

## 사전 준비

아래 준비는 한 번만 합니다. 실행기는 VM 구성을 바꾸지 않습니다.

1. UTM을 `/Applications/UTM.app`에 설치합니다. 다른 위치에 설치했다면 실행할 때 `-utmctl=<UTM.app>/Contents/MacOS/utmctl`을 지정합니다. Homebrew의 `/opt/homebrew/bin/utmctl` 같은 심볼릭 링크는 실행기가 거절합니다.
2. UTM에 Windows 11 ARM64 VM을 만들고 Windows 설치와 라이선스 동의를 직접 마칩니다. 이 단계는 자동화하지 않습니다.
3. 게스트에 [UTM Windows Guest Tools](https://docs.getutm.app/guest-support/windows/)를 설치합니다. VM이 켜진 상태에서 `/Applications/UTM.app/Contents/MacOS/utmctl ip-address <VM>`이 IPv4 주소를 출력하면 QEMU Guest Agent가 동작하는 것입니다. Guest Tools 설치 뒤 화면에 `Display output is not active.`가 나오면, 이 프로젝트를 확인한 Mac에서는 VM의 Display 설정에서 DirectX Driver를 DXMT로 바꾸고 VM을 정상 재시작해 해결됐습니다.
4. 게스트에 `go.mod`의 버전 이상인 Go를 MSI로 설치합니다. 실행기는 기본 설치 경로 `C:\Program Files\Go\bin\go.exe`를 사용하며, 다른 경로는 `-guest-go`로 지정합니다. PowerShell 실행 정책은 바꾸지 않아도 됩니다.
5. 아래 [E2E 전용 VM 만들기](#e2e-전용-vm-만들기) 절차로 디스플레이와 사운드 장치가 없는 복제본을 만듭니다.

### E2E 전용 VM 만들기

**디스플레이 장치가 남은 VM은 `utmctl start --hide`로 켜도 VM 창이 화면에 뜹니다.** `--hide`는 UTM 메인 창만 대상으로 하므로, 디스플레이 장치를 제거한 복제본에서 E2E를 실행합니다. 원래 VM은 바꾸지 않습니다. 복제와 설정 변경은 두 VM이 모두 꺼진 상태에서 합니다.

1. 원래 VM을 복제합니다. 게스트에 설치한 Guest Tools와 Go도 함께 복제됩니다.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl clone <원래 VM 이름 또는 UUID> --name Windows-E2E
   ```

2. 복제본의 디스플레이 목록을 비웁니다. UTM 설정 화면에서 Display 장치를 제거하거나, 다음 AppleScript를 실행합니다.

   ```sh
   osascript -e 'tell application "UTM"
   set vm to virtual machine named "Windows-E2E"
   set config to configuration of vm
   set displays of config to {}
   update configuration of vm with config
   end tell'
   ```

3. UTM 설정 화면에서 복제본의 사운드 장치를 제거합니다. 남겨 두면 headless로 켜도 Windows 부팅음이 납니다. UTM 스크립팅에는 사운드 설정이 없어 실행기가 이 항목을 검사하지 못하므로, 아래 4단계의 확인으로 대신합니다.
4. `make e2e-windows-up ARGS=-vm=Windows-E2E`로 켠 뒤 확인합니다. 디스플레이 장치가 남아 있으면 이 명령이 VM을 켜지 않고 이 절차를 안내하며 실패합니다. 켜졌다면 다음 명령의 출력에 `-vga none`이 있고 `-device` 목록에 사운드 장치가 없어야 합니다.

   ```sh
   ps -ww -o args= -p "$(pgrep -f qemu-aarch64-softmmu)" | tr ' ' '\n' | grep -A1 -E '^-(vga|device|audio)$'
   ```

## VM 선택

`DW_E2E_UTM_VM` 환경 변수나 `ARGS="-vm=<VM>"`에 `utmctl list`에 나오는 전체 이름 또는 UUID를 지정합니다. 지정하지 않으면 모든 명령이 종료 코드 2로 끝납니다.

```sh
export DW_E2E_UTM_VM=Windows-E2E
```

## 실행 명령

VM 기동과 종료에는 시간이 걸리므로, 여러 번 테스트할 때는 `up` → `e2e-windows`(반복) → `down` 순서로 실행합니다. 한 번만 실행할 때는 `make e2e-windows-all`을 사용합니다.

| 명령 | 동작 |
|---|---|
| `make e2e-windows-up` | UTM이 꺼져 있으면 열고, VM이 꺼져 있으면 켠 뒤 Guest Agent가 응답할 때까지 기다립니다. 이미 켜진 VM은 준비 확인만 합니다. |
| `make e2e-windows` | 켜져 있는 VM에서 소스 전달·테스트·결과 회수를 합니다. UTM이나 VM을 켜거나 끄지 않으며, 준비되지 않았으면 `run make e2e-windows-up first`와 함께 실패합니다. |
| `make e2e-windows-down` | VM에 정상 종료를 요청하고 꺼질 때까지 기다린 뒤, 실행 중인 VM이 하나도 없으면 UTM을 종료합니다. |
| `make e2e-windows-all` | up·test·down을 차례로 실행하고, 이 실행이 켠 VM과 연 UTM만 끕니다. 처음부터 켜져 있던 VM과 UTM은 그대로 둡니다. |

**`make e2e-windows-down`은 UTM을 누가 열었는지와 관계없이, 실행 중인 VM이 없으면 UTM을 종료합니다.** 직접 열어 둔 UTM도 닫힙니다. 다른 VM이 실행 중이면 UTM과 그 VM은 유지됩니다.

실행기는 UTM을 `open -g`로 열고 UTM 메인 창을 바로 닫으며, VM 창은 디스플레이 장치가 없어 생기지 않습니다. 실행 중에 UTM 창을 직접 열어 클릭하지 않습니다([UTM이 크래시함](#utm이-크래시함)).

성공하면 go test 출력에 각 테스트의 `PASS`가 표시되고 명령이 0으로 끝납니다. 테스트 실패, 시간 초과, 통신 실패, 결과 누락은 모두 0이 아닌 종료 코드로 끝납니다. make를 거치면 종료 코드는 2입니다.

### 테스트 선택·반복·시간 제한

`ARGS`에는 실행기 옵션을 먼저 쓰고, `--` 뒤에 go test 인자를 씁니다. 게스트에서는 `go test -count=1 -v <전달 인자> ./e2e/scenarios`로 실행하므로, 뒤에 붙인 `-count`가 기본값을 대신합니다.

```sh
make e2e-windows ARGS="-deadline=10m -- -count=3 -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$'"
DW_E2E_FAULT=failure make e2e-windows ARGS="-- -timeout=2m -run '^TestProcessLifecycle$'"
```

Mac에 설정한 `DW_E2E_FAULT`는 게스트로 전달되며, 위 두 번째 명령은 실패가 기대 결과입니다. Windows 경로를 값으로 넘길 때는 `ARGS="-guest-dir='D:\dw-e2e'"`처럼 작은따옴표로 감쌉니다. 감싸지 않으면 make가 실행하는 셸이 `\`를 지워 `D:dw-e2e`가 됩니다.

| 옵션 | 기본값 | 의미 |
|---|---|---|
| `-vm` | `$DW_E2E_UTM_VM` | VM 이름 또는 UUID |
| `-utmctl` | `/Applications/UTM.app/Contents/MacOS/utmctl` | UTM.app 안의 utmctl |
| `-guest-dir` | `C:\dw-e2e` | 게스트 내부 디스크에서 실행별 디렉터리를 만들 상위 경로 |
| `-guest-go` | `C:\Program Files\Go\bin\go.exe` | 게스트의 Go |
| `-deadline` | `20m` | 게스트 go test 제한 시간. 넘기면 go와 그 하위 프로세스를 종료하고 실패로 처리합니다. |
| `-ready-timeout` | `5m` | 기동 후 Guest Agent 응답을 기다리는 시간 |
| `-stop-timeout` | `3m` | 종료 요청 후 VM이 꺼지기를 기다리는 시간 |

`up`과 `down`은 `--` 뒤의 go test 인자를 받지 않습니다.

## 결과 확인

명령의 처음과 끝에 `Windows diagnostics: <Mac 경로>`가 출력됩니다. 이 디렉터리에는 다음 파일이 UTF-8 텍스트로 남습니다. 함께 남는 `guest.exe`는 게스트에 보낸 실행 파일입니다.

| 파일 | 내용 |
|---|---|
| `runner.log` | 실행기 진행 기록 |
| `source.txt` | 커밋, 전달한 소스 tar의 크기와 SHA-256, go test 인자, 미커밋 변경 목록 |
| `request.json`, `result.json` | 게스트에 보낸 요청과 게스트 결과. 결과에는 실행 ID, go 종료 코드, 시간 초과 여부, 잔존 프로세스, Windows·Go 버전, 실행 디렉터리와 드라이브 종류가 있습니다. |
| `stdout.log`, `stderr.log` | 게스트 go test의 출력 |
| `diagnostics/` | 게스트 E2E 진단 파일. `cleanup.log`·`records.log.processes`의 분석은 [Windows 실패 분석](windows.ko.md#실패-분석)과 같습니다. |

실행기는 결과의 실행 ID가 이번 실행과 같고, 오류·시간 초과·잔존 프로세스가 없으며, go 종료 코드가 0이고, 실행 디렉터리가 내부 디스크(`fixed`)일 때만 성공으로 판정합니다. 작업 트리의 미커밋 수정·추가·삭제가 그대로 전달되며, `.gitignore`로 제외한 파일은 전달하지 않습니다.

게스트의 실행별 디렉터리 `<guest-dir>\<실행 ID>`는 회수가 끝나면 삭제됩니다. 회수에 실패하면 남겨 두고 오류 메시지에 경로를 표시합니다.

같은 Mac에서 같은 VM에 대한 테스트는 동시에 실행할 수 있고, 각자 다른 실행 디렉터리를 사용합니다. 테스트가 실행 중인 동안 `up`의 기동과 `down`은 `another Windows E2E run on this Mac is using the VM`으로 거절됩니다. 다른 사용자 계정이나 다른 Mac의 실행과는 조정하지 않습니다.

## 종료 동작

`down`과 `all`은 `utmctl stop --request`로 Windows에 정상 종료를 요청하고, 꺼지지 않으면 30초마다 다시 요청합니다. VM 전원을 강제로 끄지 않으며, `-stop-timeout` 안에 꺼지지 않으면 VM을 켜 둔 채 실패합니다.

2026-11-03에 확인한 요청은 모두 실제 종료였습니다. 게스트에는 winlogon의 전원 끄기(이벤트 1074, 이유 0x500ff)와 이벤트 로그 정상 중지(6006)가 남았고, 다음 부팅은 콜드 부팅(Kernel-Boot 27, 부팅 유형 0x0)이었습니다. 이 게스트는 최대 절전 모드가 꺼져 있어 빠른 시작을 사용할 수 없습니다. 첫 요청이 무시되고 두 번째 요청으로 꺼진 경우가 있었으며, 원인은 확인하지 못했습니다.

## Windows ARM64와 x64의 차이

UTM VM은 Windows ARM64이고 게스트 Go는 `windows/arm64`입니다. Go의 race detector는 Windows에서 `windows/amd64`만 지원하므로([요구 사항](https://go.dev/doc/articles/race_detector#Requirements)), 이 VM에서는 `-race` 검증을 할 수 없습니다. `-race` 검증과 Windows x64 동작은 계속 GitHub Actions의 `test (windows-latest)`로 확인합니다([CI 단계](windows.ko.md#ci-단계)). UTM 실행의 통과를 Windows x64 CI 결과로 대신하지 않습니다.

## 문제 해결

### start가 응답하지 않음

`utmctl start`가 VM을 켜지 않고 2분 뒤 `OSStatus error -1712`로 끝나는 상태가 같은 UTM 프로세스에서 계속될 수 있습니다([UTM #8006](https://github.com/utmapp/UTM/issues/8006)). `up`은 등록된 VM이 모두 꺼져 있으면 UTM을 한 번 정상 종료·재실행한 뒤 다시 기동합니다. 다른 VM이 실행 중이면 재시작하지 않고 실패하므로, 그 VM을 끄고 UTM을 종료한 뒤 `up`을 다시 실행합니다. 출력에 섞인 `OSStatus error -10004`는 기동 성공 여부와 관계없이 나오므로 무시해도 됩니다.

### UTM이 크래시함

실행 중에 UTM 창을 클릭하면 UTM이 SIGSEGV로 종료된 사례가 있었고, 이때 대기 중이던 요청은 `OSStatus error -609`로 끝났습니다. 실행 중에는 UTM 창을 열거나 클릭하지 않습니다. 크래시 보고서는 `~/Library/Logs/DiagnosticReports/UTM-*.ips`에 있습니다.

### QEMU가 종료 중에 비정상 종료함

종료 요청 뒤 `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips`가 새로 생기면 `down`과 `all`은 VM이 꺼졌더라도 `QEMU aborted while the VM was stopping`으로 실패합니다. 디스플레이가 있는 원래 VM에서는 종료 11회 중 7회가 이렇게 끝났고, 전용 VM에서는 18회 중 0회였습니다(2026-11-03). 원인은 UTM의 QEMU 쪽이며 확인되지 않았습니다. 보고서에는 VM이 표시되지 않으므로, 같은 시간에 다른 VM의 QEMU가 비정상 종료해도 실패로 판정됩니다.

### VM 창이 화면에 뜨거나 부팅음이 남

디스플레이 장치가 남은 VM은 `up`이 거절합니다. 창 없이 켜졌는데 소리가 난다면 사운드 장치가 남아 있는 것이므로, [E2E 전용 VM 만들기](#e2e-전용-vm-만들기)의 3단계를 진행합니다.
