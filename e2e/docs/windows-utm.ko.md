# UTM Windows VM에서 E2E 실행

🇺🇸 [English](windows-utm.md) | 🇰🇷 한국어

- Mac에서 UTM의 Windows 11 ARM64 VM을 켜고, 현재 작업 트리의 소스로 `./e2e/scenarios` E2E를 게스트 안에서 실행한 뒤 결과를 Mac으로 가져옵니다.
- 실행기는 `e2e/windows`에 있고 macOS에서만 빌드됩니다.

## 사전 준비

- 실행기는 VM 구성을 바꾸지 않습니다.

아래 준비는 한 번만 합니다.

1. UTM을 `/Applications/UTM.app`에 설치합니다. 다른 위치에 설치했다면 실행할 때 `-utmctl=<UTM.app>/Contents/MacOS/utmctl`을 지정합니다. Homebrew의 `/opt/homebrew/bin/utmctl` 같은 심볼릭 링크는 실행기가 거절합니다.
2. UTM에 Windows 11 ARM64 VM을 만들고 Windows 설치와 라이선스 동의를 직접 마칩니다. 이 단계는 자동화하지 않습니다.
3. 게스트에 [UTM Windows Guest Tools](https://docs.getutm.app/guest-support/windows/)를 설치합니다. VM이 켜진 상태에서 `/Applications/UTM.app/Contents/MacOS/utmctl ip-address <VM>`이 IPv4 주소를 출력하면 QEMU Guest Agent가 동작하는 것입니다. Guest Tools 설치 뒤 화면에 `Display output is not active.`가 나오면, 이 프로젝트를 확인한 Mac에서는 VM의 Display 설정에서 DirectX Driver를 DXMT로 바꾸고 VM을 정상 재시작해 해결됐습니다.
4. 게스트에 `go.mod`의 버전 이상인 Go를 MSI로 설치합니다. 실행기는 기본 설치 경로 `C:\Program Files\Go\bin\go.exe`를 사용하며, 다른 경로는 `-guest-go`로 지정합니다. PowerShell 실행 정책은 바꾸지 않아도 됩니다.
5. 아래 [E2E 전용 VM 만들기](#e2e-전용-vm-만들기) 절차로 디스플레이와 사운드 장치가 없는 복제본을 만듭니다.

### E2E 전용 VM 만들기

- **디스플레이 장치가 남은 VM은 `utmctl start --hide`로 켜도 VM 창이 화면에 뜹니다.**
- `--hide`는 UTM 메인 창만 대상으로 하므로, 디스플레이 장치를 제거한 복제본에서 E2E를 실행합니다.
- 원래 VM은 바꾸지 않습니다.

복제와 설정 변경은 두 VM이 모두 꺼진 상태에서 합니다.

1. 원래 VM을 복제합니다. 게스트에 설치한 Guest Tools와 Go도 함께 복제됩니다.

   ```sh
   /Applications/UTM.app/Contents/MacOS/utmctl clone <원래 VM 이름 또는 UUID> --name Windows-E2E
   ```

2. 복제본의 디스플레이 목록을 비웁니다. UTM 설정 화면에서 Display 장치를 제거하거나, 다음 JXA 명령을 실행합니다. 이 명령은 다른 E2E 실행이 `utmctl`을 쓰는 중에도 실행할 수 있습니다([-600 오류](#utm이-실행-중인데-applescript가--600으로-실패함) 참고).

   ```sh
   osascript -l JavaScript -e 'Application("com.utmapp.UTM").virtualMachines.byName("Windows-E2E").updateConfiguration({with: {displays: []}})'
   ```

   - 바꿀 항목만 `with`에 넘깁니다. JXA에서 `configuration()`으로 읽은 구성 전체를 돌려보내면 드라이브와 네트워크 항목의 형식이 맞지 않아 `유형을 변경할 수 없습니다. (-1700)`으로 실패합니다.

3. UTM 설정 화면에서 복제본의 사운드 장치를 제거합니다. 남겨 두면 headless로 켜도 Windows 부팅음이 납니다. UTM 스크립팅에는 사운드 설정이 없어 실행기가 이 항목을 검사하지 못하므로, 아래 4단계의 확인으로 대신합니다.
4. `make e2e-windows-up ARGS=-vm=Windows-E2E`로 켠 뒤 확인합니다. 디스플레이 장치가 남아 있으면 이 명령이 VM을 켜지 않고 이 절차를 안내하며 실패합니다. 켜졌다면 다음 명령의 출력에 `-vga none`이 있고 `-device` 목록에 사운드 장치가 없어야 합니다.

   ```sh
   ps -ww -o args= -p "$(pgrep -f qemu-aarch64-softmmu)" | tr ' ' '\n' | grep -A1 -E '^-(vga|device|audio)$'
   ```

## VM 선택

`DW_E2E_UTM_VM` 환경 변수나 `ARGS="-vm=<VM>"`에 `utmctl list`에 나오는 전체 이름 또는 UUID를 지정합니다.

```sh
export DW_E2E_UTM_VM=Windows-E2E
```

- 지정하지 않으면 모든 명령이 종료 코드 2로 끝납니다.

## 실행 명령

- VM 기동과 종료에는 시간이 걸리므로, 여러 번 테스트할 때는 `up` → `e2e-windows`(반복) → `down` 순서로 실행합니다.
- 한 번만 실행할 때는 `make e2e-windows-all`을 사용합니다.

| 명령 | 동작 |
|---|---|
| `make e2e-windows-up` | UTM이 꺼져 있으면 열고, VM이 꺼져 있으면 켠 뒤 Guest Agent가 응답할 때까지 기다립니다. 이미 켜진 VM은 준비 확인만 합니다. |
| `make e2e-windows` | 켜져 있는 VM에서 소스 전달·테스트·결과 회수를 합니다. UTM이나 VM을 켜거나 끄지 않으며, 준비되지 않았으면 `run make e2e-windows-up first`와 함께 실패합니다. |
| `make e2e-windows-down` | VM에 정상 종료를 요청하고 꺼질 때까지 기다린 뒤, 실행 중인 VM이 하나도 없으면 UTM을 종료합니다. |
| `make e2e-windows-all` | up·test·down을 차례로 실행하고, 이 실행이 켠 VM과 연 UTM만 끕니다. 처음부터 켜져 있던 VM과 UTM은 그대로 둡니다. |

- **`make e2e-windows-down`은 UTM을 누가 열었는지와 관계없이, 실행 중인 VM이 없으면 UTM을 종료합니다.**
- 직접 열어 둔 UTM도 닫힙니다.
- 다른 VM이 실행 중이면 UTM과 그 VM은 유지됩니다.
- 실행기는 UTM을 `open -g`로 열고 UTM 메인 창을 바로 닫으며, VM 창은 디스플레이 장치가 없어 생기지 않습니다.
- 실행 중에 UTM 창을 직접 열어 클릭하지 않습니다([UTM이 크래시함](#utm이-크래시함)).
- 성공하면 go test 출력에 각 테스트의 `PASS`가 표시되고 명령이 0으로 끝납니다.
- 테스트 실패, 시간 초과, 통신 실패, 결과 누락은 모두 0이 아닌 종료 코드로 끝납니다.
- make를 거치면 종료 코드는 2입니다.
- 게스트 실행 디렉터리를 지우지 못한 것은 종료 코드를 바꾸지 않고 경고로만 알립니다([게스트 실행 디렉터리가 남음](#게스트-실행-디렉터리가-남음)).

### 테스트 선택·반복·시간 제한

- `ARGS`에는 실행기 옵션을 먼저 쓰고, `--` 뒤에 go test 인자를 씁니다.
- 게스트에서는 `go test -count=1 -v <전달 인자> ./e2e/scenarios`로 실행하므로, 뒤에 붙인 `-count`가 기본값을 대신합니다.
- `--` 뒤에 패키지 경로를 더하면 그 패키지의 테스트도 `./e2e/scenarios`와 함께 실행합니다.
- 예를 들어 `make e2e-windows-all ARGS="-vm=Windows-E2E -- ./internal/app/service"`는 게스트에서 `go test -count=1 -v ./internal/app/service ./e2e/scenarios`를 실행합니다.
- `./e2e/scenarios`는 빼고 실행할 수 없습니다.

```sh
make e2e-windows ARGS="-deadline=10m -- -count=3 -timeout=5m -run '^(TestGoldenPath|TestProcessLifecycle|TestStartHookShutdownModes)$'"
DW_E2E_FAULT=failure make e2e-windows ARGS="-- -timeout=2m -run '^TestProcessLifecycle$'"
```

- Mac에 설정한 `DW_E2E_FAULT`는 게스트로 전달되며, 위 두 번째 명령은 실패가 기대 결과입니다.
- Windows 경로를 값으로 넘길 때는 `ARGS="-guest-dir='D:\dw-e2e'"`처럼 작은따옴표로 감쌉니다.
- 감싸지 않으면 make가 실행하는 셸이 `\`를 지워 `D:dw-e2e`가 됩니다.

| 옵션 | 기본값 | 의미 |
|---|---|---|
| `-vm` | `$DW_E2E_UTM_VM` | VM 이름 또는 UUID |
| `-utmctl` | `/Applications/UTM.app/Contents/MacOS/utmctl` | UTM.app 안의 utmctl |
| `-guest-dir` | `C:\dw-e2e` | 게스트 내부 디스크에서 실행별 디렉터리를 만들 상위 경로 |
| `-guest-go` | `C:\Program Files\Go\bin\go.exe` | 게스트의 Go |
| `-deadline` | `20m` | 게스트 go test 제한 시간. 넘기면 go와 그 하위 프로세스를 종료하고 실패로 처리합니다. |
| `-ready-timeout` | `5m` | 기동 후 Guest Agent 응답을 기다리는 시간 |
| `-stop-timeout` | `3m` | 종료 요청 후 VM이 꺼지기를 기다리는 시간 |

- `up`과 `down`은 `--` 뒤의 go test 인자를 받지 않습니다.

### 동시 실행

- 같은 Mac에서 같은 VM에 대한 테스트는 동시에 실행할 수 있고, 각자 다른 실행 디렉터리를 사용합니다.
- 테스트가 실행 중인 동안 `up`의 기동과 `down`은 `another Windows E2E run on this Mac is using the VM`으로 거절됩니다.
- 다른 사용자 계정이나 다른 Mac의 실행과는 조정하지 않습니다.

## 결과 확인

- 명령의 처음과 끝에 `Windows diagnostics: <Mac 경로>`가 출력됩니다.
- 이 디렉터리에 함께 남는 `guest.exe`는 게스트에 보낸 실행 파일입니다.

이 디렉터리에는 다음 파일이 UTF-8 텍스트로 남습니다.

| 파일 | 내용 |
|---|---|
| `runner.log` | 실행기 진행 기록 |
| `source.txt` | 커밋, 전달한 소스 tar의 크기와 SHA-256, go test 인자, 미커밋 변경 목록 |
| `request.json`, `result.json` | 게스트에 보낸 요청과 게스트 결과. 요청에는 실행 ID와 go test 인자가 있고, 결과에는 게스트 쪽 오류, go 종료 코드, 시간 초과 여부, 잔존 프로세스, Windows·Go 버전, 실행 디렉터리와 드라이브 종류가 있습니다. |
| `stdout.log`, `stderr.log` | 게스트 go test의 출력 |
| `diagnostics/` | 게스트 E2E 진단 파일. `cleanup.log`·`records.log.processes`의 분석은 [Windows 실패 분석](windows.ko.md#실패-분석)과 같고, `dll-init-failed.log`를 읽는 방법은 [hook이 0xc0000142로 끝남](#hook이-0xc0000142로-끝남)에 있습니다. |

- 실행기는 결과에 게스트 쪽 오류·시간 초과·잔존 프로세스가 없고, go 종료 코드가 0이며, 실행 디렉터리가 내부 디스크(`fixed`)일 때만 성공으로 판정합니다.
- 게스트 결과를 받지 못해 실패한 실행의 `stdout.log`·`stderr.log`·`diagnostics/`는 같은 VM을 쓰는 다른 실행의 내용일 수 있습니다.
- 작업 트리의 미커밋 수정·추가·삭제가 그대로 전달되며, `.gitignore`로 제외한 파일과 `.agents/skills` 같은 심볼릭 링크는 전달하지 않습니다.
- 게스트의 실행 디렉터리 `<guest-dir>\<실행 ID>`와 그 옆의 표시 파일 `<guest-dir>\<실행 ID>.*`는 회수가 끝나면 지워집니다.
- 회수에 실패하면 실행 디렉터리를 남기고 오류 메시지에 경로를 표시합니다.

## 종료 동작

- `down`과 `all`은 `utmctl stop --request`로 Windows에 정상 종료를 요청하고, 꺼지지 않으면 30초마다 다시 요청합니다.
  - Windows가 첫 요청을 무시하고 다시 보낸 요청으로 꺼지는 경우가 있으며, 원인은 확인되지 않았습니다.
- VM 전원을 강제로 끄지 않으며, `-stop-timeout` 안에 꺼지지 않으면 VM을 켜 둔 채 실패합니다.
- 요청으로 꺼진 게스트는 매번 정상 종료 기록을 남겼고 다음 부팅은 콜드 부팅이었습니다.
- 이 게스트는 최대 절전 모드가 꺼져 있어 빠른 시작을 사용할 수 없습니다.

## Windows ARM64와 x64의 차이

- UTM VM은 Windows ARM64이고 게스트 Go는 `windows/arm64`입니다.
- Go의 race detector는 Windows에서 `windows/amd64`만 지원하므로([요구 사항](https://go.dev/doc/articles/race_detector#Requirements)), 이 VM에서는 `-race` 검증을 할 수 없습니다.
- `-race` 검증과 Windows x64 동작은 계속 GitHub Actions의 `test (windows-latest)`로 확인합니다([CI 단계](windows.ko.md#ci-단계)).
- UTM 실행의 통과를 Windows x64 CI 결과로 대신하지 않습니다.

## 문제 해결

### start가 응답하지 않음

- `utmctl start`가 VM을 켜지 않고 2분 뒤 `OSStatus error -1712`로 끝나는 상태가 같은 UTM 프로세스에서 계속될 수 있습니다([UTM #8006](https://github.com/utmapp/UTM/issues/8006)).
- `up`은 등록된 VM이 모두 꺼져 있으면 UTM을 한 번 정상 종료·재실행한 뒤 다시 기동합니다.
- 다른 VM이 실행 중이면 `up`은 UTM을 재시작하지 않고 실패하므로, 그 VM을 끄고 UTM을 종료한 뒤 `up`을 다시 실행합니다.
- 출력에 섞인 `OSStatus error -10004`는 기동 성공 여부와 관계없이 나오므로 무시해도 됩니다.

### UTM이 크래시함

- 대기 중이던 요청이 `OSStatus error -609`로 끝나고 UTM이 종료되었다면, 실행 중에 UTM 창을 클릭해 UTM이 SIGSEGV로 종료된 것일 수 있습니다.
- 크래시 보고서는 `~/Library/Logs/DiagnosticReports/UTM-*.ips`에 있습니다.

### UTM이 실행 중인데 AppleScript가 -600으로 실패함

- `utmctl` 프로세스가 하나라도 실행 중이면, AppleScript의 `tell application "UTM"`은 이벤트를 UTM이 아니라 그 `utmctl` 프로세스로 보내 곧바로 `응용 프로그램이 실행 중이 아닙니다. (-600)`(영어 환경에서는 `Application isn't running. (-600)`)로 실패합니다(macOS 27.2, UTM 5.0.8에서 확인).
  - UTM을 bundle ID나 앱 경로로 가리켜도 결과가 같습니다.
  - 같은 VM을 쓰는 다른 실행의 `file push` 같은 `utmctl` 요청은 몇 초씩 이어지므로, 동시에 실행하면 이 실패가 자주 나타납니다.
- 실행기와 [E2E 전용 VM 만들기](#e2e-전용-vm-만들기) 2단계의 명령은 JXA(`osascript -l JavaScript`)로 UTM에 이벤트를 보내며, JXA의 이벤트는 `utmctl`이 실행 중이어도 UTM에 도착합니다.
- UTM에 AppleScript를 직접 보낼 때는 `pgrep -x utmctl`이 아무것도 출력하지 않을 때 실행합니다.
- `osascript` 앞에 `AEDebugSends=1`을 붙이면 이벤트마다 대상이 출력되며, `target='psn '[utmctl]`은 이벤트가 `utmctl`로 갔다는 뜻입니다.

### QEMU가 종료 중에 비정상 종료함

- 종료 요청 뒤 `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips`가 새로 생기면 `down`과 `all`은 VM이 꺼졌더라도 `QEMU aborted while the VM was stopping`으로 실패합니다.
- 이 비정상 종료는 디스플레이가 있는 VM에서 자주 일어나고 E2E 전용 VM에서는 관찰되지 않았으며, 원인은 UTM의 QEMU 쪽에 있고 확인되지 않았습니다.
- 보고서에는 VM이 표시되지 않으므로, 같은 시간에 다른 VM의 QEMU가 비정상 종료해도 실패로 판정됩니다.

### 게스트 실행 디렉터리가 남음

- 실행기가 회수를 마친 뒤 게스트 실행 디렉터리를 지우지 못하면, `Windows E2E cleanup:`으로 시작하는 경고를 출력합니다.
  - 경고의 마지막 줄에는 그 디렉터리를 지우는 명령만 있습니다.
  - `guest run directory <경로> is still there after ...`는 실행기가 1분 동안 `rmdir`를 되풀이해도 디렉터리가 남았다는 뜻입니다.
  - `could not confirm that guest run directory <경로> was removed`는 실행기가 1분 안에 삭제 결과를 읽지 못했다는 뜻이며, 디렉터리가 실제로 남았는지는 아래 4단계로 확인합니다.
- 디렉터리가 남는 원인은 세 가지이고 경고만으로는 구별할 수 없으므로, 아래 절차로 차례로 확인합니다.
- **같은 VM에서 동시에 실행 중인 다른 테스트의 디렉터리도 `C:\dw-e2e` 아래에 있으므로, 경고에 나온 경로만 지웁니다.**

아래 절차의 게스트 명령은 Mac의 셸 하나(zsh나 bash)에서 다음 세 줄로 실행하고, `<게스트 명령>`에는 각 단계의 명령을 넣습니다. `C:\dw-e2e`는 기본 `-guest-dir`입니다.

```sh
id=cleanup-$(openssl rand -hex 8); out='C:\dw-e2e\'$id.txt
/Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "(echo $id& <게스트 명령> & echo end $id) > $out 2>&1"
/Applications/UTM.app/Contents/MacOS/utmctl file pull <VM> "$out"
```

- 게스트 명령은 결과를 게스트 파일에 쓰고, 첫 줄과 마지막 줄로 자기 결과인지 보입니다.
  - `utmctl exec`는 게스트 명령이 끝나기를 기다리지 않고, 다른 게스트 명령의 출력을 돌려줄 수 있습니다.
  - `utmctl file pull`도 다른 요청이 연 파일의 내용을 오류 없이 돌려줄 수 있습니다.
- pull한 내용의 첫 줄이 `$id`이고 마지막 줄이 `end $id`일 때만 그 명령의 결과로 읽습니다.
  - pull이 `failed to open file`을 출력하거나 두 줄 가운데 하나라도 다르면, 몇 초 뒤에 pull만 다시 실행합니다.
  - `utmctl file pull`은 파일을 열지 못해도 종료 코드 0으로 끝나므로, 종료 코드로는 판단하지 않습니다.
- 출력은 게스트의 코드 페이지로 인코딩됩니다.
- 다른 게스트 명령을 실행할 때는 첫 줄부터 다시 실행해 새 `id`를 씁니다.

결과를 확인한 뒤에는 다음 명령으로 결과 파일을 지우고, pull이 `failed to open file`을 출력할 때까지 지우는 명령과 pull을 되풀이합니다.

```sh
/Applications/UTM.app/Contents/MacOS/utmctl exec <VM> --cmd cmd.exe /d /c "del $out"
```

1. `tasklist`로 게스트의 프로세스 목록을 보고, E2E가 빌드해 실행하는 `directory-watcher.exe`와 `hookrec.exe`, 그리고 `go.exe`와 `*.test.exe`가 남아 있는지 확인합니다. 이런 프로세스가 디렉터리 안의 파일을 열어 두었거나 그 안을 현재 디렉터리로 쓰면 디렉터리를 지울 수 없습니다.
2. 남은 프로세스가 있으면 `taskkill /f /pid <PID>`로 끝내고, `taskkill`이 PID마다 적은 결과를 확인합니다. PID가 여러 개면 `/pid <PID>`를 이어 적습니다.
3. 경고의 마지막 줄을 그대로 복사해 Mac의 셸에서 실행합니다.
4. `dir /b /ad C:\dw-e2e`의 목록에 경고의 실행 ID가 없으면 디렉터리가 지워진 것입니다.
5. 남은 프로세스가 없는데도 디렉터리가 남아 있으면, 게스트 프로그램이 끝난 뒤에도 Windows가 몇 분 동안 `guest.exe`를 지우지 못하는 경우입니다. 몇 분 기다린 뒤 3단계부터 다시 진행합니다.
   - `rmdir /s /q <경로>`를 위 방법으로 실행하면 `guest.exe - 액세스가 거부되었습니다.`(영어 환경에서는 `Access is denied.`)가 출력되지만, Restart Manager로는 이 파일을 연 프로세스를 찾지 못했습니다.
   - ARM64 게스트에서 x64 프로그램의 변환 결과를 저장하는 XtaCache 서비스가 이 파일을 잡고 있는 것으로 추정하지만, 확인하지는 않았습니다.
6. 몇 분 뒤에도 디렉터리가 남아 있으면, QEMU Guest Agent가 `utmctl file pull`이나 `file push`로 연 디렉터리 안의 파일을 닫지 않은 경우입니다. 이 파일은 Guest Agent 프로세스가 끝나야 닫히므로, 같은 VM을 쓰는 다른 실행이 모두 끝난 뒤 `make e2e-windows-down`과 `make e2e-windows-up`으로 VM을 다시 시작하고 3단계부터 다시 진행합니다.

### VM 창이 화면에 뜨거나 부팅음이 남

- 디스플레이 장치가 남은 VM은 `up`이 거절합니다.
- 창 없이 켜졌는데 소리가 난다면 사운드 장치가 남아 있는 것이므로, [E2E 전용 VM 만들기](#e2e-전용-vm-만들기)의 3단계를 진행합니다.

### hook이 0xc0000142로 끝남

- 하네스는 시나리오를 정리할 때, 그 시나리오에서 종료 코드 0xc0000142(STATUS_DLL_INIT_FAILED)로 끝난 프로세스가 있으면 보고를 남깁니다.
  - 보고는 `stdout.log`에 있는 그 테스트의 출력에서 `STATUS_DLL_INIT_FAILED (0xc0000142):` 줄로 시작합니다.
  - 같은 내용이 `diagnostics/` 아래 그 시나리오 실행 디렉터리의 `dll-init-failed.log`에도 남습니다. 실행기가 게스트에 `DW_E2E_KEEP=1`을 넘기므로 테스트가 통과해도 이 파일은 남습니다.
- watcher는 hook을 `cmd.exe`로 실행하므로, 보고에 나오는 hook의 PID와 시각은 `cmd.exe`의 것입니다.
  - `cmd.exe`는 `hookrec.exe`의 종료 코드를 그대로 돌려주고, `hookrec.exe`는 `cmd.exe`의 프로세스 그룹에 속합니다.
- `./e2e/harness`를 함께 실행하면 `TestDLLInitFailureReportFromWatcher`가 일부러 0xc0000142로 끝나는 hook으로 보고를 만듭니다. 이 테스트의 보고는 실제 실패가 아닙니다.
- 이 VM에서는 0xc0000142로 끝난 프로세스가 Windows 이벤트 로그에 남지 않으므로, 이벤트 로그로는 이 실패를 찾을 수 없습니다.

보고의 첫 줄 다음에 오는 줄은 UTC 시각 순서로 적히며, 다음 두 형식 가운데 하나입니다.

| 줄 | 뜻 |
|---|---|
| `<시각> exit pid=<PID> code=<종료 코드> started=<시작 시각> from=<기록한 쪽>` | 프로세스 하나가 `<시각>`에 끝났습니다. 0xc0000142로 끝난 프로세스와, CTRL+BREAK를 받은 PID의 프로세스만 적습니다. |
| `<시각> CTRL+BREAK pid=<대상 PID> from=<보낸 쪽>` | 시나리오를 시작한 뒤 이 테스트 바이너리의 콘솔에서 `<대상 PID>`의 프로세스 그룹으로 CTRL+BREAK를 보냈습니다. 보내지 못했으면 `error=<오류>`가 붙고, 대상이 0xc0000142로 끝난 프로세스의 PID이면 `target=failed-process`가 붙습니다. |

- `from=harness`는 하네스가 시나리오 프로세스를 멈추려고 보낸 신호이거나, 하네스가 기다린 시나리오 프로세스의 종료입니다.
- `from=watcher:<PID>`는 그 watcher가 DEBUG 로그에 남긴 기록입니다. 같은 실행 디렉터리의 `watcher.log`에서는 `sent CTRL+BREAK to process group`과 `exited with code`가 든 줄입니다.

보고는 다음 순서로 읽습니다.

1. 0xc0000142로 끝난 `exit` 줄의 `started`와 종료 시각 사이에 `target=failed-process`인 CTRL+BREAK 줄이 있으면, 그 신호가 시작 중이던 프로세스 그룹에 간 것입니다.
2. CTRL+BREAK 줄보다 앞에 같은 PID의 `exit` 줄이 있으면, 그 신호는 이미 끝난 프로세스의 PID로 간 것입니다. 그 사이에 이 콘솔의 다른 프로세스 그룹이 그 PID를 받았다면 신호는 그 그룹에 갑니다.
3. 두 경우가 모두 없으면, 이 보고에서는 CTRL+BREAK가 실패를 일으켰다는 근거를 찾을 수 없습니다.

다음 신호와 종료는 보고에 담기지 않습니다.

- DEBUG보다 낮은 로그 수준으로 실행한 watcher는 CTRL+BREAK와 종료를 기록하지 않습니다.
  - `s.Launch`에 `--log-level`을 주지 않아 기본 수준인 ERROR로 실행한 시나리오가 여기에 해당합니다.
  - 이때 보고에는 시각 없이 `logged exit status 3221225794 without exit records` 줄만 남습니다.
- `--` 뒤에 더한 다른 패키지의 테스트 바이너리가 보낸 CTRL+BREAK는 담기지 않습니다. 그 바이너리가 같은 콘솔을 쓰는지는 확인하지 않았습니다.
- 병렬로 함께 실행 중인 다른 시나리오의 watcher가 보낸 CTRL+BREAK는 담기지 않습니다. 그 신호는 그 시나리오 실행 디렉터리의 `watcher.log`에 남습니다.
- 시나리오를 시작하기 전과 시나리오를 정리한 뒤에 보낸 CTRL+BREAK는 담기지 않습니다.
