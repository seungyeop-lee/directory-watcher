# Directory Watcher 사용 안내

🇺🇸 [English](USAGE.md) | 🇰🇷 한국어

이 안내는 [README](README.ko.md#설치)의 방법으로 Directory Watcher를 설치했다고 가정합니다.

## 명령줄 옵션

```shell
Usage:
  directory-watcher [flags]

Flags:
  -c, --config-path string   set config path (default "config.yml")
  -h, --help                 help for directory-watcher
  -l, --log-level string     set log level (default "ERROR")
  -v, --version              version for directory-watcher
```

## Docker로 실행하기

게시된 이미지에 `config.yml`과 감시할 폴더를 마운트하고, 이미지 이름 뒤에 설정 파일 경로를 넘깁니다.

```shell
docker run --rm \
  -v "$PWD/config.yml:/config.yml" \
  -v "$PWD/src:/src" \
  ghcr.io/seungyeop-lee/directory-watcher:latest -c /config.yml
```

- 이미지 이름 뒤의 인자는 `directory-watcher`에 [명령줄 옵션](#명령줄-옵션)으로 전달됩니다. 인자를 넘기지 않으면 도움말만 출력하고 종료합니다.
- 컨테이너의 작업 폴더가 `/`이므로 `config.yml`의 상대 경로는 `/`를 기준으로 풀립니다. 예시의 `path: src`는 마운트한 `/src`를 가리킵니다. `-w`로 작업 폴더를 바꾸면 `./directory-watcher: not found` 오류를 내고 컨테이너가 종료되므로, 작업 폴더는 `/`로 둡니다.
- hook은 컨테이너 안에서 실행됩니다. hook의 `dir`이 가리키는 폴더는 `-v "$PWD/<dir>:/<dir>"`처럼 같은 경로에 마운트합니다. 마운트하지 않으면 그 hook의 명령이 실패합니다. 컨테이너 안에서 `dir: .`은 `/`입니다.
- hook 명령은 이미지에 있는 도구만 쓸 수 있습니다. 이미지는 Alpine Linux 기반이고 `bash`, `curl`, `git`, `jq`, `make`, `openssh-client`를 포함합니다.

**알려진 이슈: bind mount된 디렉터리에서 삭제 이벤트 누락.** 호스트에서 컨테이너에 bind mount된 디렉터리 안의 파일을 삭제하면, 삭제 이벤트가 항상 감지되지는 않습니다. 이는 파일 시스템 알림 라이브러리(fsnotify)의 알려진 문제입니다. 자세한 내용은 [fsnotify/fsnotify#592](https://github.com/fsnotify/fsnotify/issues/592)를 참조하세요.

## config.yml

`config.yml` 파일에는 감시할 디렉터리와 각 디렉터리에서 변경이 생겼을 때 실행할 명령을 정의합니다. 감시를 시작하거나 끝낼 때 실행할 명령도 정의할 수 있습니다. 실제 예시는 [`config.example.yml`](config.example.yml)에 있고, 같은 형식을 JSON 스키마로 기술한 파일은 [`directory-watcher.schema.json`](directory-watcher.schema.json)입니다. 저장소 루트에서 `make run`을 실행하면 이 예시가 감시하는 폴더를 먼저 만든 뒤 이 예시로 watcher를 실행합니다.

```yaml
shell: [ hook 명령을 실행할 셸 (sh, cmd, powershell), default는 Unix에서 sh, Windows에서 cmd ]
global:
  lifeCycle:
    onStartWatch: # global onStartWatch hook
      - '실행 커맨드'
      - dir: '실행 커맨드가 실행 될 디렉토리경로'
        cmd: '실행 커맨드'
      - dir: '실행 커맨드가 실행 될 디렉토리경로'
        cmd:
          - '실행 커맨드 1'
          - '실행 커맨드 2'
    onBeforeChange: # global onBeforeChange hook
      [ global onStartWatch hook의 사양과 동일 ]
    onAfterChange: # global onAfterChange hook
      [ global onStartWatch hook의 사양과 동일 ]
    onFinishWatch: # global onFinishWatch hook
      [ global onStartWatch hook의 사양과 동일 ]
    onError: # 선택적인 전역 이벤트 유실 알림
      [ global onStartWatch hook의 사양과 동일 ]
watchTargets:
  - path: [ 감시대상 폴더 path ]
    lifeCycle:
      onStartWatch: # onStartWatch hook
        [ global onStartWatch hook의 사양과 동일 ]
      onChange: # onChange hook
        [ global onStartWatch hook의 사양과 동일 ]
      onFinishWatch: # onFinishWatch hook
        [ global onStartWatch hook의 사양과 동일 ]
    option:
      excludeDir:
        - [ 감시 제외대상 폴더 path ]
      excludeSuffix:
        - [ 감시 제외대상 파일 접미사 ]
      waitMillisecond: [ 이벤트 발생 후, hook을 실행하는 사이 대기시간, default는 100 ]
      watchSubDir: [ 하위 디렉토리 감시 여부, default는 true ]
      watchEvent: [ 감시 이벤트 (C: 생성, U: 수정, D: 삭제), default는 "CUD" ]
      noWait: [ 이벤트 발생 후 대기시간 없이 바로 hook을 실행할지 여부, default는 false ]
      interruptible: [ 실행 중인 커맨드 중지 후 새 커맨드 실행 여부, default는 false ]
```

변경이 생길 때마다 전역 `onBeforeChange`, 대상의 `onChange`, 전역 `onAfterChange`가 이 순서로 실행됩니다. hook 안의 명령은 차례대로 실행됩니다. 명령 하나가 실패하면 같은 변경에서 뒤에 남은 hook을 포함해 그 실행의 나머지 명령을 건너뜁니다. 이 실패는 `[ERROR]` 로그로 남으므로 기본 로그 수준에서도 다음 형태로 출력됩니다.

```text
<대상> hook command failed, skipping remaining commands: command <명령>: <오류>
```

`<대상>`은 감시 대상 `path`의 마지막 경로 요소입니다. 새 변경이 실행을 취소했거나(`interruptible: true`) 감시가 종료되어 멈춘 명령은 실패가 아니므로 로그에 남지 않습니다.

전역 `onStartWatch`나 전역 `onFinishWatch`의 명령이 실패하면 그 hook의 나머지 명령을 건너뜁니다. 이 실패는 대상이나 hook 이름 없이 다음 형태의 `[ERROR]` 로그로 남습니다.

```text
command <명령>: <오류>
```

전역 `onStartWatch`가 실패해도 감시는 시작됩니다. 전역 `onFinishWatch`가 실패해도 종료 코드는 바뀌지 않으므로, 종료 신호로 끝나면 0이고 치명적 오류로 끝나면 1입니다.

### hook 명령을 실행하는 셸

모든 hook 명령은 `shell`로 고른 셸이 실행합니다. 전역 hook, 대상 hook, `onError`, 구조화된 `cmd`, 명령 목록의 각 항목이 모두 해당합니다. 따옴표, 변수 확장, 명령 치환, `&&`, 리다이렉션은 그 셸의 규칙대로 해석되고, 명령이 0이 아닌 종료 코드로 끝나면 실패로 처리합니다.

| `shell` | 실행 방식 | 쓸 수 있는 OS | `{{.Path}}`가 바뀌는 참조 |
|---|---|---|---|
| `sh` | `sh -c <명령>` | 모든 OS. Windows에서는 PATH에 `sh`가 있어야 합니다. | `"$DW_PATH"` |
| `cmd` | `cmd.exe /d /v:on /s /c "<명령>"` | Windows | `!DW_PATH_ARG!` |
| `powershell` | `powershell.exe -NoProfile -NonInteractive -Command <명령>` (Windows PowerShell) | Windows | `${env:DW_PATH_ARG}` |

`shell`을 생략하면 Unix에서는 `sh`, Windows에서는 `cmd`를 씁니다. 목록에 없는 값을 쓰거나 Unix에서 `cmd`나 `powershell`을 고르면, watcher는 hook을 하나도 실행하지 않고 `Error: unknown shell "bash": must be one of sh, cmd, powershell`이나 `Error: shell "cmd" runs only on Windows`를 출력한 뒤 종료 코드 1로 끝납니다.

```yaml
shell: powershell
watchTargets:
  - path: ./src
    lifeCycle:
      onChange:
        - Write-Output "changed: {{.Path}}"
```

설정 파일 하나는 OS 하나에서 쓴다고 보고, 그 OS 셸의 문법으로 hook을 씁니다. 여러 OS에서 쓰려면 OS마다 설정 파일을 따로 두거나 OS별 스크립트를 hook에서 호출합니다. hook 하나만 다른 셸로 실행하려면 `bash -c "..."`처럼 명령 안에서 직접 감쌉니다.

- `cmd`는 지연 확장(`/v:on`)을 켜고 실행하므로, 명령에 쓴 `!`는 `!이름!` 형태의 변수 참조로 해석되고 짝이 없는 `!`는 사라집니다.
- Windows PowerShell 5.1에는 `&&`가 없습니다. 앞 명령이 성공했을 때만 다음 명령을 실행하려면 `명령1; if ($?) { 명령2 }`처럼 씁니다.

실패 로그의 `<명령>`에는 셸 실행 방식과, 템플릿 변수를 참조로 바꾼 명령이 함께 나옵니다(예: `/bin/sh -c cat "$DW_PATH"`).

### 명령 실행 위치

| dir | global onStartWatch, global onFinishWatch, global onError | 그외 hook |
|-----|-------------------------------------------|------------|
| O   | dir 설정 위치                                 | dir 설정 위치  |
| X   | 프로그램 실행 위치                                | path 설정 위치 |

명령을 실행하기 직전에 작업 폴더를 확인합니다. 작업 폴더가 없거나 폴더가 아니면 그 명령을 실행하지 않고, 다른 명령이 실패했을 때처럼 그 hook의 나머지 명령을 건너뜁니다. 이때 실패 로그의 `<오류>` 자리에는 작업 폴더 경로가 다음 형태로 들어갑니다. 전역 `onError`의 실패 로그에서는 `onError notification failed: ` 뒤에 `command <명령>: `과 이 문구가 이어집니다.

```text
working directory <작업 폴더>: <원인>
```

### 실행 명령에서 쓸 수 있는 변수

| 변수              | 환경 변수             | 설명                           | 예시 (`test/dir2`을 감시 대상 폴더로 설정했을 경우)              |
|-----------------|-------------------|------------------------------|--------------------------------------------------|
| {{.Path}}       | `DW_PATH`         | 이벤트가 일어난 파일의 상대 경로           | test/dir2/b.txt                                  |
| {{.AbsPath}}    | `DW_ABS_PATH`     | 이벤트가 일어난 파일의 절대 경로           | /Users/example/directory-watcher/test/dir2/b.txt |
| {{.FileName}}   | `DW_FILE_NAME`    | 이벤트가 일어난 파일 이름               | b.txt                                            |
| {{.ExtName}}    | `DW_EXT_NAME`     | 이벤트가 일어난 파일의 확장자             | .txt                                             |
| {{.DirPath}}    | `DW_DIR_PATH`     | 이벤트가 일어난 파일이 속한 디렉토리 상대 경로   | test/dir2                                        |
| {{.DirAbsPath}} | `DW_DIR_ABS_PATH` | 이벤트가 일어난 파일이 속한 디렉토리 절대 경로   | /Users/example/directory-watcher/test/dir2       |
| {{.Event}}      | `DW_EVENT`        | 이벤트 종류 (C: 생성, U: 수정, D: 삭제) | C                                                |

watcher는 변수의 값을 표의 환경 변수에 넣고, 명령의 변수 자리를 그 환경 변수를 읽는 셸 참조로 바꿉니다. 참조 형태는 [hook 명령을 실행하는 셸](#hook-명령을-실행하는-셸)의 표에 있습니다. 파일 이름이 명령 문자열에 들어가지 않으므로, 이름에 `;`, `&`, `$(...)`가 있어도 명령으로 실행되지 않습니다.

변수는 따옴표 밖에 씁니다. 따옴표 밖에 쓴 변수는 값에 공백이 있어도 인자 하나로 전달됩니다. 따옴표 안에 쓰면 다음과 같이 동작합니다.

- `sh`에서 `'{{.Path}}'`처럼 작은따옴표 안에 쓰면 값 대신 `"$DW_PATH"`라는 글자가 그대로 전달됩니다.
- `sh`와 `cmd`에서 `"{{.Path}}"`처럼 큰따옴표 안에 쓰면 참조가 따옴표 밖에 놓이므로, 값이 공백에서 여러 인자로 나뉠 수 있습니다.
- `powershell`은 `"changed: {{.Path}}"`처럼 큰따옴표 안에서도 값을 펼치고, 작은따옴표 안에서는 `${env:DW_PATH_ARG}`라는 글자를 그대로 둡니다.

`cmd`와 `powershell`의 참조는 `DW_PATH_ARG`처럼 이름 뒤에 `_ARG`를 붙인 환경 변수를 읽습니다. 이 변수에는 프로그램이 명령줄에서 원래 값을 그대로 읽도록 바꾼 값이 들어가고, 원래 값은 `DW_PATH` 같은 환경 변수에 그대로 있습니다. `C:\`처럼 `\`로 끝나는 값을 큰따옴표로 감싸면 프로그램이 닫는 따옴표를 글자로 읽으므로, 끝의 `\`를 두 번 씁니다.

- `cmd`는 값을 큰따옴표로 감싸고 끝의 `\`를 두 번 씁니다. `echo` 같은 `cmd` 내부 명령에는 이 따옴표와 `\`가 그대로 보입니다.
- `powershell`은 값에 공백이 있어 Windows PowerShell 5.1이 인자를 큰따옴표로 감쌀 때만 끝의 `\`를 두 번 씁니다. 공백이 있고 `\`로 끝나는 값을 cmdlet이나 문자열에 쓰면 끝에 `\`가 하나 더 보입니다.
- Windows PowerShell 5.1은 빈 문자열 인자를 프로그램에 넘기지 않습니다. `powershell`에서 확장자가 없는 파일의 `{{.ExtName}}`처럼 빈 값을 따옴표 밖에 쓰면 프로그램이 받는 인자가 하나 줄어듭니다.

`path`가 폴더를 가리키는 링크이면 링크가 가리키는 폴더를 감시합니다. 이때 위 변수에는 `path` 대신 링크를 모두 푼 경로를 기준으로 한 경로가 들어갑니다. `excludeDir`를 링크 경로 아래로 적으면 링크가 가리키는 폴더의 같은 위치를 제외합니다.

## waitMillisecond와 noWait

- `watchTargets.path` 내부에서 이벤트가 발생하게 되면 waitMillisecond 만큼 대기 후 hook을 실행합니다.
- 대기 중 `watchTargets.path` 내부에서 다른 이벤트가 발생하게 되면 그 전 이벤트는 무시됩니다. 즉, 새로 생긴 이벤트 기준으로 waitMillisecond 만큼 다시 대기 후 hook을 실행합니다.
- `watchTargets.path` 내부의 이벤트가 무시되지 않고, 모든 이벤트로 인해 hook이 즉시 실행되길 원한다면 noWait를 true로 설정합니다. (이벤트에 대한 hook은 순차적으로 실행됩니다.)

## 이벤트 유실 알림

`global.lifeCycle.onError`는 선택 항목이며 다른 hook과 같은 명령 목록 및 구조화된 `dir`/`cmd` 형식을 사용합니다. runner별 최대 한 번, 최초 치명적 오류가 `errors.Is(err, fsnotify.ErrEventOverflow)`를 만족할 때만 실행합니다. 전달 대기열 포화와 OS가 보고한 overflow가 여기에 해당합니다. 다른 치명적 오류, 정상 종료, debounce에 의한 대체는 알림을 실행하지 않습니다.

오류 정보는 명령 템플릿에 삽입하지 않고 환경 변수로 전달합니다.

| 환경 변수 | 값 |
|-----------|----|
| `DW_ERROR_TARGET` | 치명적 오류가 발생한 감시 대상 |
| `DW_ERROR_KIND` | `event_loss` |
| `DW_ERROR_CAUSE` | 원래 원인 문자열 |
| `DW_ERROR_MESSAGE` | `FatalError.Error()`의 전체 메시지 |
| `DW_ERROR_POSSIBLE_LOSS` | `true` (유실 가능성이며 유실 개수가 아님) |

알림 명령도 설정한 셸이 실행하므로 환경 변수를 셸 문법으로 읽습니다. 예를 들어 `sh`에서는 `printf '%s\n' "$DW_ERROR_MESSAGE"`, `cmd`에서는 `echo !DW_ERROR_MESSAGE!`, `powershell`에서는 `Write-Output $env:DW_ERROR_MESSAGE`처럼 씁니다. `cmd`에서 `%DW_ERROR_MESSAGE%`로 읽으면 값에 든 `&`가 명령을 나누므로 `!DW_ERROR_MESSAGE!`를 씁니다. 이 hook에는 파일 이벤트 템플릿 변수를 제공하지 않습니다. 구조화된 `dir`이 있으면 해당 디렉토리에서, 없으면 프로그램 실행 디렉토리에서 실행합니다.

최초 치명적 오류가 발생하면 모든 대상에서 새로운 일반 hook 실행 승인을 즉시 중단합니다. 알림은 일반 hook 취소와 독립적으로 고정 5초 실행 제한을 적용하고, 취소 정리에 최대 20초를 더 사용합니다(총 최대 25초). 알림 실패나 시간 초과는 로그에 남기며 원래 치명적 오류를 대체하지 않습니다. CLI는 원래 오류로 비정상 종료합니다. 취소 후에도 정상 종료 hook은 기존 규칙대로 실행합니다.

각 오류 알림 명령이 완료·실패·시간 초과되면 같은 정리 시간 한도 안에서 남은 자손을 종료합니다. Windows에서는 알림 명령마다 별도 Job Object를 만들고, 일시 중단된 부모를 Job에 배정한 뒤 실행하므로 부모 PID의 생존 여부에 의존하지 않습니다. Job을 설정하지 못하면 알림 명령을 실행하지 않습니다. 일반 hook과 다른 실행의 프로세스는 이 알림 Job에 배정하지 않습니다.

## 과부하와 종료

- 대상별 대기 이벤트는 최대 256개, 실행 중인 hook 세대는 최대 1개입니다. `noWait: true`는 수신 순서를 유지하며 과부하에서 최신 이벤트만 남기는 방식으로 바뀌지 않습니다. debounce는 마지막으로 수신한 유효 이벤트를 보존하고, 제외 suffix는 실행하거나 타이머를 다시 설정하지 않습니다. 여러 파일의 생성 호출 순서가 OS 이벤트 수신 순서를 보장하지 않으므로 마지막 생성 파일이 debounce 결과라고 가정하면 안 됩니다.
- 전달 대기열 포화 또는 watcher 오류가 발생하면 처리를 중단합니다. 기본 로그에 감시 대상과 원인을 남기고 모든 대상을 정리한 뒤 CLI가 비정상 종료합니다. 애플리케이션 포화는 `dispatch queue full`로 표시하여 OS overflow와 구분합니다. 자동 재시작·영속 큐는 제공하지 않습니다.
- 대상의 감시를 시작하지 못해도 치명적 오류로 처리합니다. `path`가 없거나, 폴더 링크를 풀지 못하거나, 폴더를 감시 목록에 넣지 못한 경우가 여기에 해당합니다. 이 대상은 `onStartWatch`를 실행하지 않았으므로 `onFinishWatch`도 실행하지 않습니다. 감시를 시작한 다른 대상의 `onFinishWatch`와 전역 `onFinishWatch`는 그대로 실행하고, CLI는 종료 코드 1로 끝납니다.
- 중단 모드는 이전 세대를 취소하고 완료를 기다린 뒤 새 실행을 시작합니다. 종료 신호는 실행 중인 시작·변경 명령을 취소하고, 대기 변경을 버리고, 이벤트 처리기를 기다린 뒤 종료 hook을 실행합니다. 종료 hook에는 기존 명령 실행 규칙을 유지하며 별도 제한 시간을 추가하지 않습니다. 감시 대상끼리는 독립 실행합니다.
- Windows에서 watcher는 실행 중인 명령을 멈출 때 그 명령의 프로세스 그룹에 CTRL+BREAK를 보냅니다. Windows E2E(UTM VM의 Windows 11 ARM64, 2026-11-03 실행)는 Git Bash 없이 watcher를 실행해, `interruptible: true`의 교체와 종료에서 hook의 부모·자식·손자 프로세스가 그 그룹에 남아 신호를 받고 끝나는 경우 모두 종료되는 것을 확인합니다([`TestProcessLifecycle`](e2e/docs/scenarios/process-lifecycle.ko.md), [`TestStartHookShutdownModes`](e2e/docs/scenarios/start-hook-shutdown-modes.ko.md)). 자기만의 콘솔이나 프로세스 그룹을 만들거나 신호를 무시하는 자손 프로세스는 계속 실행될 수 있습니다.
- 경로 판별 중 연 파일은 디렉터리 정보 조회 실패를 포함해 함수가 반환하기 전에 닫습니다. 이 handle의 반납은 가비지 컬렉션에 의존하지 않습니다.
- 파일 조작과 OS 알림은 일대일이 아닙니다. fsnotify v1.7.0의 inotify는 `ErrEventOverflow`를 보고하지만 macOS kqueue는 이 오류를 보고하지 않습니다. 애플리케이션 상한이 모든 OS 유실의 감지를 보장하지는 않습니다. [E2E 시나리오와 한계](e2e/docs/reliability.ko.md#경합과-과부하-검증)를 확인하세요.

이 단계들의 순서는 [README의 동작 다이어그램](README.ko.md#동작-다이어그램)에 있습니다.

## 기본 제외 규칙

- 이름이 `.`이나 `_`로 시작하는 파일과 폴더는 감시하지 않습니다. 예를 들어 `.env`를 만들거나 `.git`, `.idea` 안에서 일어난 변경으로는 hook이 실행되지 않습니다. `path`로 지정한 폴더는 이름과 관계없이 감시합니다.
- 파일은 만들 때만 제외합니다. 이미 있던 `.env`를 수정하거나 삭제하면 hook이 실행됩니다.
- 감시 대상 폴더 안에 있는 폴더 링크는 따라가지 않습니다. 감시 대상 폴더 안의 링크가 감시 대상 밖 폴더를 가리켜도, 그 폴더에서 일어난 변경으로는 hook이 실행되지 않습니다.

## 폴더 이벤트 감지에 대한 제약사항

- 기본동작은 감시 대상 폴더 내의 **파일**의 이벤트에 따라 hook을 실행합니다.
- 즉, 폴더의 생성 및 이름 수정으로는 hook이 실행되지 않습니다.
- 그러나, 폴더의 삭제 (빈 폴더 포함)에 대해서는 삭제 대상이 폴더임을 인식 할 수 없어 hook이 실행됩니다.
