# Directory Watcher 사용 안내

🇺🇸 [English](USAGE.md) | 🇰🇷 한국어

- 이 안내는 [README](README.ko.md#설치)의 방법으로 Directory Watcher를 설치했다고 가정합니다.

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

- **알려진 이슈: bind mount된 디렉터리에서 삭제 이벤트 누락.** 호스트에서 컨테이너에 bind mount된 디렉터리 안의 파일을 삭제하면, 삭제 이벤트가 항상 감지되지는 않습니다.
  - 이는 파일 시스템 알림 라이브러리(fsnotify)의 알려진 문제입니다.
  - 자세한 내용은 [fsnotify/fsnotify#592](https://github.com/fsnotify/fsnotify/issues/592)를 참조하세요.

## config.yml

- `config.yml` 파일에는 감시할 디렉터리와 각 디렉터리에서 변경이 생겼을 때 실행할 명령을 정의합니다.
- 감시를 시작하거나 끝낼 때 실행할 명령도 정의할 수 있습니다.
- 실제 예시는 [`config.example.yml`](config.example.yml)에 있습니다.
- 같은 형식을 JSON 스키마로 기술한 파일은 [`directory-watcher.schema.json`](directory-watcher.schema.json)입니다.
- 저장소 루트에서 `make run`을 실행하면 이 예시가 감시하는 폴더를 먼저 만든 뒤 이 예시로 watcher를 실행합니다.

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
    onQueueWarning: # 선택적인 전역 대기열 경고
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
      watch: # 무엇을 감시할지
        subDir: [ 하위 디렉토리 감시 여부, default는 true ]
        exclude:
          - [ 감시하지 않을 파일과 폴더의 경로 패턴, path 기준 ]
        fileEvent: [ hook을 실행할 파일 이벤트 (C: 생성 또는 옮겨 옴, U: 수정, D: 삭제 또는 옮겨 나감) 또는 none, default는 "CUD" ]
        dirEvent: [ hook을 실행할 폴더 이벤트 (C: 생성 또는 옮겨 옴, D: 삭제 또는 옮겨 나감) 또는 none, default는 "CD" ]
      dirBatch: # 폴더 이벤트를 만들려고 이벤트를 모으는 방식
        waitMillisecond: [ 모은 폴더 이벤트를 보내기 전에 마지막 이벤트 뒤로 기다리는 시간, 0이면 모으지 않음, default는 100 ]
        maxWaitMillisecond: [ 폴더 이벤트를 모으는 가장 긴 시간, waitMillisecond보다 작을 수 없음, default는 1000이고 waitMillisecond가 더 크면 그 값 ]
      hookBatch: # 이벤트를 기다려 모은 뒤 hook 실행으로 합치는 방식
        waitMillisecond: [ 모은 이벤트로 실행을 만들기 전에 마지막 이벤트 뒤로 기다리는 시간, 0이면 기다리지 않음, default는 100 ]
        maxWaitMillisecond: [ 이벤트를 모으는 가장 긴 시간, waitMillisecond보다 작을 수 없음, default는 1000이고 waitMillisecond가 더 크면 그 값 ]
        deliver: [ 모은 이벤트로 만드는 실행 (last, lastPerPath, each), default는 last ]
      queue: # hook 실행을 기다리는 대기열
        size: [ 기다릴 수 있는 실행의 최대 수, 1 이상, default는 2048 ]
      run: # hook을 실행하는 방식
        concurrency: [ 동시에 돌리는 hook 실행의 최대 수, 1 이상, default는 1 ]
        interrupt: [ 새 실행이 취소하는 실행 (none, samePath, oldest), default는 none ]
```

- 변경이 생길 때마다 전역 `onBeforeChange`, 대상의 `onChange`, 전역 `onAfterChange`가 이 순서로 실행됩니다.
- hook 안의 명령은 차례대로 실행됩니다.
- 명령 하나가 실패하면 같은 변경에서 뒤에 남은 hook을 포함해 그 실행의 나머지 명령을 건너뜁니다.

이 실패는 `[ERROR]` 로그로 남으므로 기본 로그 수준에서도 다음 형태로 출력됩니다.

```text
<대상> hook command failed, skipping remaining commands: command <명령>: <오류>
```

- `<대상>`은 감시 대상 `path`의 마지막 경로 요소입니다.
- `<명령>`의 형태는 [hook 명령을 실행하는 셸](#hook-명령을-실행하는-셸)에 있습니다.
- 새 실행이 취소했거나(`run.interrupt`) 감시가 종료되어 멈춘 명령은 실패가 아니므로 로그에 남지 않습니다.

- 전역 `onStartWatch`나 전역 `onFinishWatch`의 명령이 실패하면 그 hook의 나머지 명령을 건너뜁니다.

이 실패는 대상이나 hook 이름 없이 다음 형태의 `[ERROR]` 로그로 남습니다.

```text
command <명령>: <오류>
```

- 전역 `onStartWatch`가 실패해도 감시는 시작됩니다.
- 전역 `onFinishWatch`가 실패해도 종료 코드는 바뀌지 않으므로, 종료 신호로 끝나면 0이고 치명적 오류로 끝나면 1입니다.

### hook 명령을 실행하는 셸

- 모든 hook 명령은 `shell`로 고른 셸이 실행합니다.
  - 전역 hook, 대상 hook, `onError`, `onQueueWarning`, 구조화된 `cmd`, 명령 목록의 각 항목이 모두 해당합니다.
- 따옴표, 변수 확장, 명령 치환, `&&`, 리다이렉션은 그 셸의 규칙대로 해석되고, 명령이 0이 아닌 종료 코드로 끝나면 실패로 처리합니다.

| `shell` | 실행 방식 | 쓸 수 있는 OS | `{{.Path}}`가 바뀌는 참조 |
|---|---|---|---|
| `sh` | `sh -c <명령>` | 모든 OS. Windows에서는 PATH에 `sh`가 있어야 합니다. | `"$DW_PATH"` |
| `cmd` | `cmd.exe /d /v:on /s /c "<명령>"` | Windows | `!DW_PATH_ARG!` |
| `powershell` | `powershell.exe -NoProfile -NonInteractive -Command <명령>` (Windows PowerShell) | Windows | `${env:DW_PATH_ARG}` |

- `shell`을 생략하면 Unix에서는 `sh`, Windows에서는 `cmd`를 씁니다.
- 목록에 없는 값을 쓰거나 Unix에서 `cmd`나 `powershell`을 고르면, watcher는 hook을 하나도 실행하지 않고 `Error: unknown shell "bash": must be one of sh, cmd, powershell`이나 `Error: shell "cmd" runs only on Windows`를 출력한 뒤 종료 코드 1로 끝납니다.

```yaml
shell: powershell
watchTargets:
  - path: ./src
    lifeCycle:
      onChange:
        - Write-Output "changed: {{.Path}}"
```

- 설정 파일 하나는 OS 하나에서 쓴다고 보고, 그 OS 셸의 문법으로 hook을 씁니다.
- 여러 OS에서 쓰려면 OS마다 설정 파일을 따로 두거나 OS별 스크립트를 hook에서 호출합니다.
- hook 하나만 다른 셸로 실행하려면 `bash -c "..."`처럼 명령 안에서 직접 감쌉니다.

- `cmd`는 지연 확장(`/v:on`)을 켜고 실행하므로, 명령에 쓴 `!`는 `!이름!` 형태의 변수 참조로 해석되고 짝이 없는 `!`는 사라집니다.
- Windows PowerShell 5.1에는 `&&`가 없습니다. 앞 명령이 성공했을 때만 다음 명령을 실행하려면 `명령1; if ($?) { 명령2 }`처럼 씁니다.

- 실패 로그의 `<명령>`에는 셸 실행 방식과, 템플릿 변수를 참조로 바꾼 명령이 함께 나옵니다.
  - 템플릿 변수를 쓴 명령이면 그 뒤의 `(with values: ...)`에 변수 자리에 값을 그대로 넣은 명령이 붙어, 어느 파일에서 실패했는지 알 수 있습니다.
  - 이 명령은 로그에만 쓰고 실행하지 않습니다.

```text
command /bin/sh -c cat "$DW_PATH" (with values: cat test/dir2/b.txt): exit status 1
```

### 명령 실행 위치

| dir | global onStartWatch, global onFinishWatch, global onError, global onQueueWarning | 그외 hook |
|-----|-------------------------------------------|------------|
| O   | dir 설정 위치                                 | dir 설정 위치  |
| X   | 프로그램 실행 위치                                | path 설정 위치 |

- 명령을 실행하기 직전에 작업 폴더를 확인합니다.
- 작업 폴더가 없거나 폴더가 아니면 그 명령을 실행하지 않고, 다른 명령이 실패했을 때처럼 그 hook의 나머지 명령을 건너뜁니다.
- 이때 실패 로그의 `<오류>` 자리에는 작업 폴더 경로가 다음 형태로 들어갑니다.
  - 전역 `onError`와 전역 `onQueueWarning`의 실패 로그에서는 `onError notification failed: `나 `onQueueWarning notification failed: ` 뒤에 `command <명령>: `과 이 문구가 이어집니다.

```text
working directory <작업 폴더>: <원인>
```

### 실행 명령에서 쓸 수 있는 변수

| 변수                 | 환경 변수                | 설명                                       | 예시 (`test/dir2`을 감시 대상 폴더로 설정했을 경우)              |
|--------------------|----------------------|------------------------------------------|--------------------------------------------------|
| {{.Path}}          | `DW_PATH`            | 이벤트가 일어난 파일이나 폴더의 상대 경로                  | test/dir2/b.txt                                  |
| {{.AbsPath}}       | `DW_ABS_PATH`        | 이벤트가 일어난 파일이나 폴더의 절대 경로                  | /Users/example/directory-watcher/test/dir2/b.txt |
| {{.Name}}          | `DW_NAME`            | 이벤트가 일어난 파일이나 폴더의 이름                     | b.txt                                            |
| {{.ExtName}}       | `DW_EXT_NAME`        | 이벤트가 일어난 파일의 확장자. 폴더이면 빈 값               | .txt                                             |
| {{.ParentPath}}    | `DW_PARENT_PATH`     | 이벤트가 일어난 파일이나 폴더가 들어 있는 폴더의 상대 경로        | test/dir2                                        |
| {{.ParentAbsPath}} | `DW_PARENT_ABS_PATH` | 이벤트가 일어난 파일이나 폴더가 들어 있는 폴더의 절대 경로        | /Users/example/directory-watcher/test/dir2       |
| {{.Event}}         | `DW_EVENT`           | 이벤트 종류 (C: 생성 또는 옮겨 옴, U: 수정, D: 삭제 또는 옮겨 나감) | C                                                |
| {{.IsDir}}         | `DW_IS_DIR`          | [폴더 이벤트](#폴더-이벤트)이면 `true`, 파일 이벤트이면 `false`  | false                                            |

- 이 변수는 변경이 생겼을 때 실행하는 hook, 곧 감시 대상의 `onChange`와 전역 `onBeforeChange`, `onAfterChange`에서만 씁니다.
- 다른 hook은 명령을 적힌 그대로 실행하므로, 그 명령에 쓴 `{{.Path}}`는 글자 그대로 남습니다.

- watcher는 감시를 시작하기 전에 변경이 생겼을 때 실행하는 hook의 명령을 검사합니다.

명령에 표에 없는 변수가 있으면 watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].lifeCycle.onChange[1]: unknown template variable "FilePath"; available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event, IsDir
```

- 쌍점 앞의 위치는 설정에서 문제가 된 명령을 가리킵니다.
  - `onChange[1]`은 `onChange`의 두 번째 항목이고, `onChange[0].cmd[1]`은 첫 번째 항목의 `cmd` 목록에 있는 두 번째 명령입니다.
  - 전역 hook은 `global.lifeCycle.onBeforeChange[0]`처럼 `global.lifeCycle.`로 시작합니다.
- `{{.Path.Missing}}`처럼 어떤 이벤트에서도 템플릿을 채우지 못하는 명령이 있어도 watcher는 끝나고, 그 오류에는 위치 뒤에 `can't execute command:`가 이어집니다.
  - 변수의 값은 모두 문자열이므로, `.Missing`처럼 변수에서 필드를 읽으면 항상 실패합니다.

- watcher는 변수의 값을 표의 환경 변수에 넣고, 명령의 변수 자리를 그 환경 변수를 읽는 셸 참조로 바꿉니다.
- 참조 형태는 [hook 명령을 실행하는 셸](#hook-명령을-실행하는-셸)의 표에 있습니다.
- 파일 이름이 명령 문자열에 들어가지 않으므로, 이름에 `;`, `&`, `$(...)`가 있어도 명령으로 실행되지 않습니다.

- 변수는 따옴표 밖에 씁니다.
- 따옴표 밖에 쓴 변수는 값에 공백이 있어도 인자 하나로 전달됩니다.

따옴표 안에 쓰면 다음과 같이 동작합니다.

- `sh`에서 `'{{.Path}}'`처럼 작은따옴표 안에 쓰면 값 대신 `"$DW_PATH"`라는 글자가 그대로 전달됩니다.
- `sh`와 `cmd`에서 `"{{.Path}}"`처럼 큰따옴표 안에 쓰면 참조가 따옴표 밖에 놓이므로, 값이 공백에서 여러 인자로 나뉠 수 있습니다.
- `powershell`은 `"changed: {{.Path}}"`처럼 큰따옴표 안에서도 값을 펼치고, 작은따옴표 안에서는 `${env:DW_PATH_ARG}`라는 글자를 그대로 둡니다.

- `cmd`와 `powershell`의 참조는 `DW_PATH_ARG`처럼 이름 뒤에 `_ARG`를 붙인 환경 변수를 읽습니다.
  - 이 변수에는 프로그램이 명령줄에서 원래 값을 그대로 읽도록 바꾼 값이 들어가고, 원래 값은 `DW_PATH` 같은 환경 변수에 그대로 있습니다.
- `C:\`처럼 `\`로 끝나는 값을 큰따옴표로 감싸면 프로그램이 닫는 따옴표를 글자로 읽으므로, 끝의 `\`를 두 번 씁니다.

- `cmd`는 값을 큰따옴표로 감싸고 끝의 `\`를 두 번 씁니다. `echo` 같은 `cmd` 내부 명령에는 이 따옴표와 `\`가 그대로 보입니다.
- `powershell`은 값에 공백이 있어 Windows PowerShell 5.1이 인자를 큰따옴표로 감쌀 때만 끝의 `\`를 두 번 씁니다. 공백이 있고 `\`로 끝나는 값을 cmdlet이나 문자열에 쓰면 끝에 `\`가 하나 더 보입니다.
- Windows PowerShell 5.1은 빈 문자열 인자를 프로그램에 넘기지 않습니다. `powershell`에서 확장자가 없는 파일의 `{{.ExtName}}`처럼 빈 값을 따옴표 밖에 쓰면 프로그램이 받는 인자가 하나 줄어듭니다.

- `path`가 폴더를 가리키는 링크이면 링크가 가리키는 폴더를 감시합니다.
  - 이때 위 변수에는 `path` 대신 링크를 모두 푼 경로를 기준으로 한 경로가 들어갑니다.
  - `exclude` 패턴도 링크가 가리키는 폴더의 같은 위치를 제외합니다.

### FileName, DirPath, DirAbsPath에서 옮기기

- 버전 3에서 템플릿 변수 세 개와 그 환경 변수의 이름을 바꿨습니다.
  - 이름을 바꾼 변수의 값은 그대로입니다.
- 예전 템플릿 변수를 쓴 hook이 있으면 watcher는 시작할 때 [실행 명령에서 쓸 수 있는 변수](#실행-명령에서-쓸-수-있는-변수)에 적은 `unknown template variable` 오류를 내고 끝납니다.
- watcher는 예전 환경 변수를 더 이상 설정하지 않고, 명령이 그 변수를 읽는지도 검사하지 않습니다.
  - `DW_FILE_NAME`, `DW_DIR_PATH`, `DW_DIR_ABS_PATH`를 읽는 명령이나 스크립트는 값 없이 실행되므로, hook 명령과 그 명령이 실행하는 스크립트에서 이 이름을 찾아 바꿉니다.

각 변수는 다음과 같이 바꿉니다.

| 옛 이름 | 바뀐 이름 |
|---|---|
| `{{.FileName}}`, `DW_FILE_NAME` | `{{.Name}}`, `DW_NAME` |
| `{{.DirPath}}`, `DW_DIR_PATH` | `{{.ParentPath}}`, `DW_PARENT_PATH` |
| `{{.DirAbsPath}}`, `DW_DIR_ABS_PATH` | `{{.ParentAbsPath}}`, `DW_PARENT_ABS_PATH` |

### 버전 2의 옵션에서 옮기기

- 버전 3에서는 감시 대상 옵션을 `option` 아래의 `watch`, `dirBatch`, `hookBatch`, `queue`, `run` 그룹으로 나눴습니다.

감시 대상의 `option` 바로 아래에 `watchSubDir`, `waitMillisecond`, `noWait`, `interruptible`이 남아 있으면 watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.watchSubDir is no longer supported; use option.watch.subDir (see USAGE "Moving from version 2 options")
```

- 오류에 적힌 "Moving from version 2 options"는 영어판에서 이 절의 제목입니다.

버전 2의 옵션은 다음과 같이 옮깁니다.

| 버전 2 | 버전 3 |
|---|---|
| `watchSubDir: false` | `watch: {subDir: false}` |
| `waitMillisecond: 500` | `hookBatch: {waitMillisecond: 500}` |
| `waitMillisecond: 0` | `waitMillisecond`를 적지 않습니다. 버전 3에서 `0`은 기본값 100이 아니라 기다리지 않음을 뜻합니다 |
| `waitMillisecond: 3000`처럼 1000보다 큰 `waitMillisecond` | `hookBatch: {waitMillisecond: 3000}`. `maxWaitMillisecond`를 적지 않으면 가장 긴 대기 시간이 1000이 아니라 `waitMillisecond`와 같아집니다 |
| `noWait: true` | `hookBatch: {waitMillisecond: 0, deliver: each}` |
| `noWait: false` | `noWait`를 적지 않습니다 |
| `excludeDir`, `excludeSuffix` | `watch.exclude`, [excludeDir와 excludeSuffix에서 옮기기](#excludedir와-excludesuffix에서-옮기기)를 따릅니다 |
| `watchEvent` | `watch.fileEvent`와 `watch.dirEvent`, [watchEvent에서 옮기기](#watchevent에서-옮기기)를 따릅니다 |
| `interruptible: true` | `run: {interrupt: oldest}` |
| `interruptible: false` | `interruptible`을 적지 않습니다. `run.interrupt`의 기본값이 `none`입니다 |

- 버전 2의 `noWait: false`는 기다리는 동안 받은 마지막 이벤트 하나로 hook을 실행했습니다. 버전 3의 기본값 `deliver: last`는 가장 나중에 바뀐 경로의 최종 변화로 hook을 실행합니다. 자세한 내용은 [hookBatch로 기다리고 합치기](#hookbatch로-기다리고-합치기)에 있습니다.
  - 한 번 기다리는 동안 만들었다가 지운 파일은 버전 2에서 `D`로 hook을 실행했고, 버전 3에서는 hook을 실행하지 않습니다.
  - watcher가 생성 이벤트를 놓친 파일은 `U`가 아니라 `C`로 hook에 갑니다.
- 버전 2는 이벤트가 계속 오는 동안 계속 기다렸습니다. 버전 3은 첫 이벤트부터 늦어도 `hookBatch.maxWaitMillisecond`(기본 1000, `hookBatch.waitMillisecond`가 더 크면 그 값)가 지나면 hook을 실행합니다.
- 버전 2는 hook이 실행되는 동안 들어온 이벤트를 OS에 남겨 두었습니다. 버전 3은 그동안에도 이벤트를 읽고, 대상마다 hook을 기다리는 실행을 최대 `queue.size`(기본 2048)개까지 둡니다. 들어갈 자리가 없는 실행이 생기면 watcher를 멈춥니다. 자세한 내용은 [대기열 용량과 경고](#대기열-용량과-경고)에 있습니다.

## 이름 바꾸기와 옮기기

- watcher는 파일의 이름을 바꾸거나 파일을 옮기면 옛 경로의 삭제(`D`)와 감시 대상 안의 새 경로의 생성(`C`)으로 알립니다.

동작마다 hook이 받는 이벤트는 다음과 같습니다.

| 동작 | hook이 받는 이벤트 |
|---|---|
| 같은 폴더 안에서 이름 바꾸기 | 옛 이름의 `D`, 그다음 새 이름의 `C` |
| 감시 대상 안의 다른 폴더로 옮기기 | 옛 경로의 `D`, 그다음 새 경로의 `C` |
| 감시 대상 밖으로 옮기기 | 옛 경로의 `D` |
| 감시 대상 안으로 옮겨 오기 | 새 경로의 `C` |

- 기본값 `hookBatch.deliver: last`에서는 한 번 기다리는 동안 가장 나중에 바뀐 경로만 hook을 실행하므로, 이름 바꾸기와 감시 대상 안의 옮기기는 새 경로의 `C`로 hook을 한 번 실행합니다.
- 옛 경로가 `exclude`나 [기본 제외 규칙](#기본-제외-규칙)에 맞으면 hook은 그 `D`를 받지 않습니다.
- macOS는 새 경로를 옛 경로보다 먼저 알리기도 합니다. 그러면 watcher는 옛 경로의 `D`가 올 때까지 `C`와 그 뒤의 이벤트를 최대 1초 동안 미뤄 둡니다.
- 폴더의 이름 바꾸기와 옮기기는 [폴더 이벤트](#폴더-이벤트)에 있습니다.

### 버전 2의 이름 바꾸기 이벤트에서 옮기기

- 버전 3 전에는 macOS와 Linux에서 감시 대상 안에서 이름을 바꾸거나 옮긴 파일은 hook이 새 경로의 `C`만 받았고, 감시 대상 밖으로 옮긴 파일은 아무 이벤트도 받지 않았습니다.
  - Windows에서는 다른 폴더나 감시 대상 밖으로 옮기면 이미 옛 경로의 `D`가 왔지만, 같은 폴더 안에서 이름을 바꾸면 오지 않았습니다.
- `D`로 실행되는 hook은 이제 파일의 이름을 바꾸거나 파일을 옮겨 나갈 때도 실행되며, 그 경우와 삭제를 구별하지 못합니다.

## hookBatch로 기다리고 합치기

- watcher는 감시 대상의 이벤트를 모으다가, `hookBatch.waitMillisecond`(기본 100) 동안 새 이벤트가 없거나 첫 이벤트부터 `hookBatch.maxWaitMillisecond`(기본 1000, `hookBatch.waitMillisecond`가 더 크면 그 값)가 지나면 모은 이벤트로 hook 실행을 만듭니다.
- hook이 도는 동안에도 이벤트를 계속 모읍니다. 기다리는 시간이 끝날 때마다 그때 만든 실행을 감시 대상의 대기열에 넣고, 다른 때 만든 실행끼리는 합치지 않습니다.
- `hookBatch.waitMillisecond: 0`이면 기다리지 않고 이벤트 하나마다 따로 실행을 만들며, `maxWaitMillisecond`는 쓰이지 않습니다.

직접 적은 `maxWaitMillisecond`가 `waitMillisecond`보다 작거나 `deliver`가 아래 값이 아니면, watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.hookBatch: maxWaitMillisecond 1000 is less than waitMillisecond 3000
```

`hookBatch.deliver`는 한 번 기다리는 동안 모은 이벤트로 어떤 실행을 만들지 정합니다.

| `deliver` | 만드는 실행 |
|---|---|
| `last` (기본값) | 최종 변화가 있는 경로 가운데 가장 나중에 바뀐 경로 하나로 실행 하나를 만듭니다. 그런 경로가 없으면 실행을 만들지 않습니다. |
| `lastPerPath` | 최종 변화가 있는 경로마다 실행 하나를 만들고, 경로가 마지막으로 바뀐 순서대로 둡니다. |
| `each` | 이벤트마다 실행 하나를 만들고, 이벤트가 생긴 순서대로 둡니다. 최종 변화는 계산하지 않습니다. |

예를 들어 한 번 기다리는 동안 `a.txt` 생성, `a.txt` 수정, `b.txt` 수정, `x.swp` 생성, `x.swp` 삭제가 모이면 다음과 같이 실행합니다.

- `last`는 `b.txt`의 `U`로 hook을 한 번 실행합니다.
- `lastPerPath`는 `a.txt`의 `C`로, 그다음 `b.txt`의 `U`로 hook을 실행합니다.
- `each`는 hook을 다섯 번 실행합니다.

`last`와 `lastPerPath`는 기다리기 전에 watcher가 기록해 둔 감시 대상의 파일 목록과 실행을 만들기 직전의 디스크를 비교해 경로마다 최종 변화를 정합니다.

| 기다리기 전 | 지금 | 최종 변화 |
|---|---|---|
| 없었음 | 있음 | `C` |
| 있었음 | 있음 | `U`, 폴더면 실행하지 않음 |
| 있었음 | 없음 | `D` |
| 없었음 | 없음 | 실행하지 않음 |

- watcher가 생성 이벤트를 놓친 파일은 기록에 없으므로, 그 파일의 첫 변경은 `C`로 hook에 갑니다.
- `watch.fileEvent`와 `watch.dirEvent`는 최종 변화 가운데서 고르고, `each`에서는 이벤트 가운데서 고릅니다.
- watcher가 디스크를 확인한 뒤 hook이 돌기 전에 경로가 또 바뀔 수 있습니다. 그 변화는 새 이벤트로 따라오므로, hook은 실행될 때 실제 파일을 읽어야 합니다.
- `lastPerPath`는 경로가 마지막으로 바뀐 순서대로 실행합니다. 파일 사이의 앞뒤 관계가 중요하면 `each`를 씁니다.

## 이벤트 유실 알림

- `global.lifeCycle.onError`는 선택 항목이며 다른 hook과 같은 명령 목록 및 구조화된 `dir`/`cmd` 형식을 사용합니다.
- watcher를 한 번 실행하는 동안 최대 한 번, 최초 치명적 오류가 이벤트 유실일 때만 실행합니다.
  - 대상의 전달 대기열이 가득 찬 경우(`dispatch queue full`)와 OS가 이벤트 overflow를 보고한 경우가 여기에 해당합니다.
- 다른 치명적 오류, 정상 종료, `hookBatch`가 합쳐서 넘기지 않은 이벤트는 알림을 실행하지 않습니다.

오류 정보는 명령 템플릿에 삽입하지 않고 환경 변수로 전달합니다.

| 환경 변수 | 값 |
|-----------|----|
| `DW_ERROR_TARGET` | 치명적 오류가 발생한 감시 대상 |
| `DW_ERROR_KIND` | `event_loss` |
| `DW_ERROR_CAUSE` | 원래 원인 문자열 |
| `DW_ERROR_MESSAGE` | `watch target "<대상>": <원인>` 형태의 치명적 오류 전체 메시지 |
| `DW_ERROR_POSSIBLE_LOSS` | `true` (유실 가능성이며 유실 개수가 아님) |

- 알림 명령도 설정한 셸이 실행하므로 환경 변수를 셸 문법으로 읽습니다.
  - 예를 들어 `sh`에서는 `printf '%s\n' "$DW_ERROR_MESSAGE"`, `cmd`에서는 `echo !DW_ERROR_MESSAGE!`, `powershell`에서는 `Write-Output $env:DW_ERROR_MESSAGE`처럼 씁니다.
  - `cmd`에서 `%DW_ERROR_MESSAGE%`로 읽으면 값에 든 `&`가 명령을 나누므로 `!DW_ERROR_MESSAGE!`를 씁니다.
- 이 hook에는 파일 이벤트 템플릿 변수를 제공하지 않습니다.
- 구조화된 `dir`이 있으면 해당 디렉토리에서, 없으면 프로그램 실행 디렉토리에서 실행합니다.

- 최초 치명적 오류가 발생하면 어느 대상에서도 일반 hook의 새 실행을 시작하지 않습니다.
- 알림은 일반 hook 취소와 독립적으로 고정 5초 실행 제한을 적용하고, 취소 정리에 최대 20초를 더 사용합니다(총 최대 25초).
- 알림 실패나 시간 초과는 로그에 남기며 원래 치명적 오류를 대체하지 않습니다.
  - CLI는 원래 오류로 비정상 종료합니다.
- 취소 후에도 정상 종료 hook은 기존 규칙대로 실행합니다.

- 각 알림 명령이 완료·실패·시간 초과되면, 그 명령이 시작해 아직 실행 중인 프로세스를 같은 정리 시간 한도 안에서 끝냅니다.
  - Windows에서 이 프로세스들을 끝낼 수 있게 알림 명령을 준비하지 못하면, watcher는 그 명령을 실행하지 않고 실패를 로그에 남깁니다.
- 일반 hook이 시작한 프로세스는 hook이 취소될 때만 끝냅니다([과부하와 종료](#과부하와-종료)).

## 대기열 용량과 경고

- 대상의 실행은 [run으로 hook 실행하기](#run으로-hook-실행하기)에 적은 대로 시작할 때까지 대상의 대기열에서 기다립니다. `option.queue.size`(기본 2048)는 기다릴 수 있는 실행의 최대 수이며, 실행 중인 실행은 세지 않습니다.
- 들어갈 자리가 없는 실행이 생기면 `dispatch queue full (capacity <size>)`로 처리를 중단하고, [이벤트 유실 알림](#이벤트-유실-알림)처럼 전역 `onError`를 한 번 실행합니다.
  - watcher는 대기열에 자리가 나기를 기다리지 않습니다. 한꺼번에 많이 들어올 수 있는 [폴더 이벤트](#폴더-이벤트)용으로 모은 이벤트에서 만든 실행도 같습니다.

`queue.size`가 1보다 작으면 watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.queue: size 0 is less than 1
```

`global.lifeCycle.onQueueWarning`은 선택 항목이며 다른 hook과 같은 명령 목록 및 구조화된 `dir`/`cmd` 형식을 사용합니다. 대상의 대기열에서 기다리는 실행의 수가 세 상태 사이를 오갈 때 실행합니다.

| 상태 변화 | 실행하는 때 | `DW_QUEUE_LEVEL` | `DW_QUEUE_DIRECTION` |
|---|---|---|---|
| 정상에서 경고로 | 기다리는 실행이 `queue.size`의 70%에 닿을 때 | `70` | `up` |
| 경고에서 위험으로 | 기다리는 실행이 `queue.size`의 90%에 닿을 때 | `90` | `up` |
| 위험에서 경고로 | 기다리는 실행이 `queue.size`의 80% 아래로 내려갈 때 | `90` | `down` |
| 경고에서 정상으로 | 기다리는 실행이 `queue.size`의 60% 아래로 내려갈 때 | `70` | `down` |

- 상태는 들어선 지점보다 10%p 낮아져야 벗어나므로, 기다리는 수가 70%나 90% 근처에서 오르내려도 같은 경고를 되풀이하지 않습니다.
- `queue.size`가 아주 작아서 실행 하나로 두 지점을 넘으면 두 경고를 차례로 실행합니다.
- 경고 hook은 이벤트 처리와 따로 실행하므로, 경고 hook이 실행되는 동안에도 이벤트를 받고 hook을 실행합니다.
- 같은 대상의 경고 hook이 아직 실행 중일 때 생긴 경고는 가장 최근 것 하나만 남겼다가, 지금 실행이 끝나면 실행합니다. 대상마다 가장 최근 경고를 따로 남기므로, 서로 다른 대상의 경고 hook은 동시에 실행될 수 있습니다.
- 경고 hook은 한 번 실행할 때마다 `onError`와 같은 제한을 받습니다. 5초 실행 제한이 있고, 취소 정리에 최대 20초를 더 씁니다.
- 실패나 시간 초과는 `onQueueWarning notification failed: <원인>`으로 로그에 남기며 감시를 멈추지 않습니다.
- 종료할 때는 남겨 둔 경고를 버리고, 실행 중인 경고 hook이 끝날 때까지 기다립니다.

경고 정보는 환경 변수로 전달합니다.

| 환경 변수 | 값 |
|-----------|----|
| `DW_QUEUE_TARGET` | 대기열의 상태가 바뀐 감시 대상 |
| `DW_QUEUE_LEVEL` | `70` 또는 `90` |
| `DW_QUEUE_DIRECTION` | `up` 또는 `down` |
| `DW_QUEUE_LENGTH` | 상태가 바뀐 때 기다리는 실행의 수 |
| `DW_QUEUE_CAPACITY` | 대상의 `queue.size` |
| `DW_QUEUE_TOP_PATHS` | 기다리는 실행이 많은 경로부터 최대 5개를 한 줄에 하나씩 `<개수> <경로>` 형태로 담습니다. 개수가 같으면 대기열에 먼저 들어간 경로가 앞섭니다. 경로는 `{{.Path}}`와 같은 형태입니다. |

- `DW_QUEUE_TOP_PATHS`는 hook이 만든 파일 때문에 hook이 계속 다시 실행되는 경우를 찾는 데 씁니다. 해결 방법은 [hook이 자기 결과로 다시 실행되지 않게 하기](#hook이-자기-결과로-다시-실행되지-않게-하기)에 있습니다.
- 환경 변수는 `onError`처럼 셸 문법으로 읽습니다. 이 hook에는 파일 이벤트 템플릿 변수를 제공하지 않습니다.
- 구조화된 `dir`이 있으면 해당 디렉토리에서, 없으면 프로그램 실행 디렉토리에서 실행합니다.

## run으로 hook 실행하기

- `option.run.concurrency`(기본 1)는 한 대상에서 동시에 돌 수 있는 실행의 최대 수입니다. 기본값이면 대상은 실행을 한 번에 하나씩 돌립니다.
- 실행은 대기열에서 나온 순서, 곧 이벤트가 생긴 순서대로 시작합니다. 동시에 도는 실행이 끝나는 순서는 보장하지 않습니다.
- 같은 경로의 실행이나, 폴더와 그 안 경로의 실행도 동시에 돌 수 있습니다. 그래서 한 파일의 `D` 실행이 먼저 시작한 같은 파일의 `C` 실행보다 먼저 끝날 수 있습니다. hook이 실행이 끝나는 순서에 기대면 `concurrency: 1`을 유지합니다.
- 실행마다 자기 `onChange` 앞뒤로 전역 `onBeforeChange`와 `onAfterChange`를 따로 돌립니다. 이 hook이 출력 파일 하나처럼 공유하는 자원을 쓰면 동시에 도는 실행끼리 부딪칠 수 있습니다.
- `concurrency`와 관계없이 실행은 대상 `onStartWatch`가 끝날 때까지 기다립니다. `interrupt: oldest`일 때만 첫 실행이 실행 중인 대상 `onStartWatch`를 취소합니다.

`option.run.interrupt`(기본 `none`)는 새 실행이 돌고 있는 실행 가운데 무엇을 취소할지 정합니다.

| `interrupt` | 새 실행이 취소하는 실행 |
|---|---|
| `none` | 취소하지 않습니다. 새 실행은 자리가 날 때까지 기다립니다. |
| `samePath` | 같은 경로를 처리하는 실행을 자리가 남아 있어도 취소합니다. 같은 경로의 실행이 없으면 취소하지 않고 자리가 날 때까지 기다립니다. |
| `oldest` | 자리가 다 찼을 때 가장 먼저 시작한 실행을 취소합니다. `concurrency: 1`이면 실행 중인 실행입니다. |

- 새 실행은 자기가 취소한 실행이 끝난 뒤에 시작합니다.
- `samePath`에서 서버처럼 스스로 끝나지 않는 hook이 자리를 모두 차지하면, 다른 경로의 실행은 감시를 멈출 때까지 기다립니다. 그런 hook은 `concurrency: 1`과 `interrupt: oldest`로 실행해, 새 실행이 돌고 있는 실행을 늘 대신하게 합니다.

`concurrency`가 1보다 작거나 `interrupt`가 이 값들이 아니면 watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.run: concurrency 0 is less than 1
Error: watchTargets[0].option.run.interrupt: unknown value "true"; use none, samePath, or oldest
```

## 과부하와 종료

- 대상마다 대기 실행은 [대기열 용량과 경고](#대기열-용량과-경고)에 적은 대로 최대 `queue.size`개까지 두고, hook 실행은 한 번에 최대 `run.concurrency`개까지 진행합니다. `hookBatch.deliver: each`는 수신 순서를 유지하며 과부하에서 최신 이벤트만 남기는 방식으로 바뀌지 않습니다. 제외된 경로는 실행을 만들지 않고 기다리는 시간도 늘리지 않습니다. 여러 파일의 생성 호출 순서가 OS 이벤트 수신 순서를 보장하지 않으므로, 마지막으로 만든 파일이 `deliver: last`가 실행하는 경로라고 가정하면 안 됩니다.
- 전달 대기열 포화 또는 watcher 오류가 발생하면 처리를 중단합니다. 기본 로그에 감시 대상과 원인을 남기고 모든 대상을 정리한 뒤 CLI가 비정상 종료합니다. 애플리케이션 포화는 `dispatch queue full`로 표시하여 OS overflow와 구분합니다. 자동 재시작·영속 큐는 제공하지 않습니다.
- 대상의 감시를 시작하지 못해도 치명적 오류로 처리합니다. `path`가 없거나, 폴더 링크를 풀지 못하거나, 폴더를 감시 목록에 넣지 못한 경우가 여기에 해당합니다. 이 대상은 `onStartWatch`를 실행하지 않았으므로 `onFinishWatch`도 실행하지 않습니다. 감시를 시작한 다른 대상의 `onFinishWatch`와 전역 `onFinishWatch`는 그대로 실행하고, CLI는 종료 코드 1로 끝납니다.
- 종료 신호는 실행 중인 시작·변경 실행을 모두 취소하고, 대기 변경을 버리고, 이벤트 처리와 취소한 실행이 모두 멈출 때까지 기다린 뒤 종료 hook을 실행합니다. 종료 hook에는 기존 명령 실행 규칙을 유지하며 별도 제한 시간을 추가하지 않습니다. 감시 대상끼리는 독립 실행합니다.
- Windows에서 취소된 hook은 CTRL+BREAK를 받고, `cmd`에서 `a & b`의 다음 프로그램처럼 hook이 취소 뒤에 시작한 프로그램도 CTRL+BREAK를 받습니다. 첫 CTRL+BREAK 뒤 20초가 지나도 실행 중인 hook의 프로세스는 함께 끝내며, 신호를 무시하거나 자기만의 콘솔이나 프로세스 그룹을 쓰는 프로세스도 여기서 끝납니다. 교체와 종료는 hook의 프로세스가 모두 끝날 때까지 기다리므로, 일부 `powershell` hook처럼 CTRL+BREAK로 끝나지 않는 hook은 교체와 종료를 최대 20초 늦춥니다.
- Windows에서 시작한 지 100ms가 지나기 전에 취소된 hook은 시작 후 100ms가 지났을 때 CTRL+BREAK를 받으므로, hook이 시작된 직후의 교체와 종료는 최대 100ms 늦어집니다.
- Windows에서 취소되지 않고 끝난 hook이 시작한 프로세스는 계속 실행됩니다.
- Windows E2E가 취소된 hook의 종료에 대해 확인하는 내용은 [Windows E2E](e2e/docs/windows.ko.md#hook-취소-검증)에 있습니다.
- 파일 조작과 OS 알림은 일대일이 아닙니다. Linux는 OS 이벤트 overflow를 보고하며, 이때도 전달 대기열이 가득 찼을 때처럼 처리를 중단합니다. macOS는 overflow를 보고하지 않습니다. 애플리케이션 상한이 모든 OS 유실의 감지를 보장하지는 않습니다. [E2E 시나리오와 한계](e2e/docs/reliability.ko.md#경합과-과부하-검증)를 확인하세요.

- 이 단계들의 순서는 [README의 동작 다이어그램](README.ko.md#동작-다이어그램)에 있습니다.

## 기본 제외 규칙

- 이름이 `.`이나 `_`로 시작하는 파일과 폴더는 감시하지 않습니다. 예를 들어 `.env`를 만들거나 수정하거나 삭제해도, `.git`, `.idea` 안에서 변경이 일어나도 hook이 실행되지 않습니다. `path`로 지정한 폴더는 이름과 관계없이 감시합니다.
- 감시 대상 폴더 안에 있는 폴더 링크는 따라가지 않습니다. 감시 대상 폴더 안의 링크가 감시 대상 밖 폴더를 가리켜도, 그 폴더에서 일어난 변경으로는 hook이 실행되지 않습니다.

## exclude로 경로 제외하기

- `option.watch.exclude`에는 경로 패턴을 적습니다.
- 패턴에 맞는 파일과 폴더는 감시하지 않습니다.
  - 패턴에 맞는 폴더는 그 아래 전체와 함께 감시 목록에 넣지 않습니다. 감시를 시작할 때 있던 폴더든 감시 중에 생긴 폴더든 같습니다. 그래서 `node_modules`처럼 큰 폴더를 빼면 그 안의 항목은 하나도 감시 목록에 들어가지 않습니다.
  - 패턴에 맞는 파일은 만들거나 수정하거나 삭제해도 hook이 실행되지 않고, 패턴에 맞는 폴더는 만들거나 지워도 [폴더 이벤트](#폴더-이벤트)가 오지 않습니다.

```yaml
watchTargets:
  - path: site
    option:
      watch:
        exclude:
          - '**/node_modules/**'
          - '**/*.log'
          - '!important.log'
```

- 패턴은 감시 대상 `path`를 기준으로 한 상대 경로에 맞춥니다. 폴더 구분자는 OS와 관계없이 `/`로 씁니다. `**`가 없는 패턴은 `path` 바로 아래부터 맞추므로, `build.log`는 `site/build.log`에는 맞지만 `site/logs/build.log`에는 맞지 않습니다.
- `*`는 이름의 일부, `?`는 글자 하나, `**`는 폴더 여러 단계에 맞습니다. `dist/**`는 `dist` 폴더와 그 아래 전체에 맞고, `**/*.log`는 어느 깊이에 있든 `.log` 파일에 맞습니다. `[abc]` 같은 문자 집합과 `{js,css}` 같은 선택지도 쓸 수 있습니다.
- 패턴은 위에서부터 차례로 맞춰 보고, 마지막으로 맞은 패턴이 결과를 정합니다. `!`로 시작하는 패턴에 맞으면 앞선 패턴이 뺀 경로를 다시 감시합니다. 위 예시에서는 `site` 바로 아래의 `important.log`만 hook을 실행하고, 다른 `.log` 파일은 hook을 실행하지 않습니다.
- 뺀 폴더는 감시하지 않으므로, 그 안의 경로는 `!`로 다시 감시하게 할 수 없습니다. 폴더 안의 일부만 남기려면 폴더 대신 폴더 안의 항목을 빼는 패턴을 씁니다. `dist/*`와 `!dist/keep.txt`를 쓰면 `dist` 안에서 `keep.txt`만 hook을 실행합니다. `dist/**`와 `!dist/keep.txt`를 쓰면 `dist`를 감시하지 않으므로 `keep.txt`도 hook을 실행하지 않습니다.
- `dist/**`는 `dist` 폴더 자신에도 맞으므로, `dist`를 만들거나 지워도 폴더 이벤트가 오지 않습니다. `dist/*`는 `dist` 안의 항목에만 맞으므로, hook은 `dist`의 폴더 이벤트를 그대로 받습니다.
- [기본 제외 규칙](#기본-제외-규칙)으로 빠지는 이름도 `!`로 다시 감시하게 할 수 없습니다.
- 패턴은 따옴표로 감쌉니다. 따옴표가 없으면 YAML은 `*`나 `!`로 시작하는 값을 YAML 문법으로 읽습니다.

- 패턴 문법이 잘못되면 watcher는 hook을 하나도 실행하지 않고 `Error: watchTargets[0].option.watch.exclude[1]: invalid pattern "[a"` 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

### hook이 자기 결과로 다시 실행되지 않게 하기

- hook이 감시 대상 `path` 안에 파일을 쓰면 그 쓰기도 변경이므로, hook이 다시 실행되고 또 파일을 쓰면서 끝없이 반복됩니다.
- hook이 쓰는 경로를 `watch.exclude`에 적어 이 반복을 막습니다.

다음 예시에서 `onChange`는 `site/build.log`에 쓰고, `watch` 아래의 `exclude: ['build.log']`가 그 쓰기로 hook이 실행되는 것을 막습니다.

```yaml
watchTargets:
  - path: site
    lifeCycle:
      onChange:
        - dir: .
          cmd: make build > site/build.log
    option:
      watch:
        exclude:
          - 'build.log'
```

- 기본값인 `run.interrupt: none`에서는 hook이 실행되는 동안 바뀐 다른 파일도 hook이 끝난 뒤 hook을 실행합니다.

### excludeDir와 excludeSuffix에서 옮기기

- 버전 3에서 `excludeDir`와 `excludeSuffix`를 없앴습니다.

감시 대상의 `option`에 둘 중 하나가 남아 있으면 watcher는 hook을 하나도 실행하지 않고 다음 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.excludeDir is no longer supported; use option.watch.exclude (see USAGE "Moving from excludeDir and excludeSuffix")
```

- 오류에 적힌 "Moving from excludeDir and excludeSuffix"는 영어판에서 이 절의 제목입니다.

각 값은 다음 차이를 확인하며 `watch.exclude`로 옮깁니다.

- `excludeDir` 값은 watcher를 실행한 위치를 기준으로 한 폴더 경로였습니다. `exclude` 패턴은 감시 대상 `path`를 기준으로 합니다.
- `excludeSuffix` 값은 절대 경로의 끝부분과 비교했으므로, `out.txt`를 적으면 `layout.txt`도 빠졌습니다. `exclude`에서는 파일 이름 앞에 `**/`를 붙여 어느 깊이에 있든 그 이름에만 맞게 합니다.
- `excludeSuffix`에서 옮긴 패턴이 폴더 이름에 맞으면 그 폴더 안 전체도 빠집니다. 예를 들어 `**/*~`는 `backup~` 폴더 안의 파일도 뺍니다.

감시 대상이 `path: 'site'`일 때는 다음과 같이 옮깁니다.

| 옛 설정 | 옮긴 설정 |
|---|---|
| `excludeDir: ['site/dist']` | `watch: {exclude: ['dist/**']}` |
| `excludeSuffix: ['_test.go']` | `watch: {exclude: ['**/*_test.go']}` |
| `excludeSuffix: ['~']` | `watch: {exclude: ['**/*~']}` |
| `excludeSuffix: ['/out.txt']` | `watch: {exclude: ['**/out.txt']}` |

## 감시 대상의 경로가 겹칠 때

- 두 감시 대상의 `path`가 같거나 한 대상의 `path`가 다른 대상의 상위 폴더이면, 겹치는 폴더를 두 대상이 각각 감시합니다.
- watcher는 이런 설정도 오류나 경고 없이 실행합니다.

- 겹치는 폴더 안에서 변경이 하나 생기면, 그 폴더를 감시하는 대상마다 hook이 실행됩니다.
- 예를 들어 `src`와 `src/api`를 함께 감시하면, `src/api/a.txt`를 만들었을 때 두 대상의 `onChange`가 모두 실행됩니다.
- 전역 `onBeforeChange`와 `onAfterChange`도 대상마다 실행되므로, 위 예시의 변경 하나에 각각 두 번 실행됩니다.
- 두 대상의 hook은 상대 대상의 hook이 끝나기를 기다리지 않습니다.
- 대기열, `hookBatch.waitMillisecond` 대기, `run`의 동시 실행과 취소는 대상마다 따로 적용됩니다.
- 겹치는 폴더는 대상마다 감시 목록에 들어가므로, Linux의 inotify watch나 macOS의 열린 파일 같은 OS 감시 자원도 대상 수만큼 씁니다.
- 한 대상의 hook이 다른 대상의 `path` 안에 파일을 쓰면, 그 쓰기로 다른 대상의 hook이 실행됩니다.

- 상위 대상이 하위 대상의 폴더를 감시하지 않게 하려면, 상위 대상의 `watch.exclude`에 그 폴더를 적습니다.

다음 설정에서는 `src/api` 안의 변경으로 `src/api` 대상의 hook만 실행되고, `src` 대상은 `src/api`를 감시 목록에 넣지 않습니다.

```yaml
watchTargets:
  - path: src
    option:
      watch:
        exclude:
          - 'api/**'
  - path: src/api
```

- `exclude` 패턴은 상위 대상의 `path`를 기준으로 하므로 `src/api/**`가 아니라 `api/**`로 적습니다.

## 폴더 이벤트

- hook은 파일뿐 아니라 폴더의 이벤트도 받습니다. `{{.IsDir}}`와 `DW_IS_DIR`는 폴더 이벤트이면 `true`, 파일 이벤트이면 `false`입니다.
- 폴더 이벤트는 `C`와 `D`뿐입니다. 폴더에는 `U`가 없으며, 폴더 안의 변경은 그 안에 있는 파일의 이벤트로 hook에 갑니다.
- `option.watch.fileEvent`로 hook을 실행할 파일 이벤트를, `option.watch.dirEvent`로 폴더 이벤트를 고릅니다. 기본값은 `CUD`와 `CD`이고, `none`을 적으면 그쪽 이벤트를 받지 않습니다.

동작마다 hook이 받는 이벤트는 다음과 같습니다. 이벤트, `IsDir`, 경로 순서로 적습니다.

| 동작 | hook이 받는 이벤트 |
|---|---|
| `mkdir src/pkg` | `C true src/pkg` |
| `mkdir -p src/a/b/c` | `C true src/a` 하나 |
| 파일이 든 폴더를 감시 대상 밖에서 `src/x`로 옮겨 오기 | `C true src/x` 하나 |
| `mkdir src/n && echo hi > src/n/a.txt` | `C true src/n` 하나 |
| 안에 파일 300개가 있는 `src/pkg`를 `rm -rf`로 지우기 | `D true src/pkg` 하나 |
| `src/pkg`를 감시 대상 밖으로 옮기기 | `D true src/pkg` 하나 |
| 폴더 이름 바꾸기, 감시 대상 안의 다른 폴더 아래로 옮기기 | 옛 경로의 `D true`, 그다음 새 경로의 `C true` |
| 파일 생성, 수정, 삭제, 이름 바꾸기 | [파일 이벤트](#이름-바꾸기와-옮기기) 그대로, `IsDir`는 `false` |

hook은 다음 약속에 기댈 수 있습니다.

- 폴더 `C`는 그 폴더가 안에 든 것과 함께 생겼다는 뜻입니다. 폴더 안의 내용은 hook이 직접 읽습니다. 폴더를 옮겨 와도 watcher는 그 폴더와 함께 들어온 파일의 이벤트를 만들지 않습니다.
- 폴더 `D`는 그 폴더와 그 안의 모든 것이 사라졌다는 뜻입니다.
- 폴더 이벤트를 모으는 동안 온, 그 폴더 안의 파일과 하위 폴더의 이벤트는 폴더 이벤트에 흡수합니다. `watch.dirEvent: none`이어도 똑같이 흡수하므로, 이때 폴더 단위의 변경은 hook을 실행하지 않습니다.
- `hookBatch.deliver: lastPerPath`는 한 번 기다리는 동안 모은 이벤트에서, 최종 변화가 `D`인 폴더 아래 경로의 `D`를 넘기지 않습니다. 그래서 폴더 삭제가 폴더 이벤트 묶음 여러 개로 나뉘어도, 한 번 기다리는 동안 모이면 폴더 `D` 하나로 hook에 갑니다.
  - `watch.dirEvent: none`이어도 똑같이 빼므로, 이때는 그 폴더 삭제로 hook이 실행되지 않습니다.
  - 폴더 `C` 아래 경로의 `C`는 빼지 않습니다. 폴더 `C`를 받은 hook이 폴더를 읽은 뒤에 생긴 항목일 수 있기 때문입니다.
  - `last`는 마지막 실행 하나만, `each`는 나뉜 삭제를 모두 그대로 넘깁니다.
- `hookBatch.deliver: each`에서는 한 감시 대상의 이벤트를 생긴 순서대로 hook에 넘깁니다. 폴더 이벤트를 모으는 동안에는 그 대상의 파일 이벤트도 함께 기다립니다.
- hook은 같은 변경을 두 번 받을 수 있습니다. 예를 들어 폴더 `C`를 받고 안을 읽은 뒤, 그 폴더 안 파일의 `C`를 또 받을 수 있습니다.

### 폴더 이벤트를 모으는 시간

- 폴더가 생기거나 사라지면 watcher는 그 감시 대상의 이벤트를 모으고, `dirBatch.waitMillisecond`(기본 100) 동안 새 이벤트가 없으면 모은 이벤트를 보냅니다.
- 이벤트가 계속 오면 첫 이벤트부터 `dirBatch.maxWaitMillisecond`(기본 1000, `dirBatch.waitMillisecond`가 더 크면 그 값)가 지났을 때 그때까지 모은 이벤트를 보냅니다. 그 뒤에 그 폴더 안에서 생긴 이벤트는 흡수하지 않고 따로 hook에 갑니다. 다만 `hookBatch.deliver: lastPerPath`는 한 번 기다리는 동안 모인 폴더 `D` 아래의 `D`를 넘기지 않습니다.
- 감시 대상의 하위 폴더 안에서 파일을 지워도 이벤트를 모으기 시작합니다. 폴더를 통째로 지우면 폴더 안의 파일을 지운 이벤트가 폴더의 이벤트보다 먼저 오기 때문입니다. 그래서 `hookBatch.waitMillisecond: 0`에서도 이런 삭제와 그 뒤의 이벤트는 최대 `dirBatch.waitMillisecond`만큼 늦게 hook에 갑니다.
- `dirBatch.waitMillisecond: 0`이면 이벤트를 모으지 않으므로, hook은 생기거나 사라진 폴더 안의 파일과 하위 폴더의 이벤트도 받습니다.
- 직접 적은 `dirBatch.maxWaitMillisecond`가 `dirBatch.waitMillisecond`보다 작으면, watcher는 hook을 하나도 실행하지 않고 `Error: watchTargets[0].option.dirBatch: maxWaitMillisecond 200 is less than waitMillisecond 500` 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

`watch.fileEvent`나 `watch.dirEvent`가 빈 값이거나 받지 않는 글자를 담았거나 둘 다 `none`이면, watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.watch.dirEvent: U is not a folder event; use C, D, or none
```

### 폴더 이벤트 감지에 대한 제약사항

- 느린 디스크에서나 `dirBatch.waitMillisecond`가 작을 때처럼 트리를 지우는 도중에 `dirBatch.waitMillisecond`보다 오래 이벤트가 끊기면, 하위 폴더가 위 폴더의 `D`보다 먼저 따로 `D`로 hook에 갈 수 있습니다. 그런 디스크에서는 `dirBatch.waitMillisecond`를 늘립니다.
- 폴더 링크는 따라가지 않으므로, 링크를 만들거나 지우면 `IsDir`가 `false`인 파일 이벤트로 갑니다.
- watcher가 폴더의 생성을 처리하기 전에 그 폴더가 지워지면, watcher는 그것이 폴더였는지 알 수 없어 파일의 `C`와 `D`로 hook에 넘깁니다.
- 감시 대상 밖으로 옮긴 폴더 안의 변경은 더 이상 hook을 실행하지 않습니다.
  - Windows에서는 그렇게 옮기고 약 100ms 뒤에 watcher가 감시 대상의 감시를 다시 만들며, 그때 감시 대상 안에서 생긴 변경은 hook에 두 번 갈 수 있습니다.
- Windows에서는 감시 중인 하위 폴더가 있는 폴더의 이름을 바꾸거나 그 폴더를 옮기면, 감시하는 동안 `Access is denied`로 실패합니다.

### watchEvent에서 옮기기

- 버전 3에서 `watchEvent`를 `watch.fileEvent`와 `watch.dirEvent`로 바꿨습니다.

감시 대상의 `option`에 `watchEvent`가 남아 있으면 watcher는 hook을 하나도 실행하지 않고 다음 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].option.watchEvent is no longer supported; use option.watch.fileEvent and option.watch.dirEvent (see USAGE "Moving from watchEvent")
```

- 오류에 적힌 "Moving from watchEvent"는 영어판에서 이 절의 제목입니다.

값은 다음과 같이 옮깁니다.

| 옛 설정 | 옮긴 설정 |
|---|---|
| `watchEvent: CU` | `watch: {fileEvent: CU}` |
| `watchEvent: ''` | `watch: {fileEvent: CUD}`, 또는 `fileEvent`를 적지 않음 |

- 폴더 이벤트는 기본으로 켜집니다. `cat {{.Path}}`처럼 파일 경로를 기대하는 hook은 `{{.IsDir}}`를 확인하거나, 그 감시 대상에 `watch.dirEvent: none`을 적어야 합니다.
- 버전 3 전에는 폴더에 대해 hook이 다음 이벤트를 받았습니다.
  - 폴더를 만들거나 옮겨 와도 hook이 실행되지 않았습니다. 폴더를 만든 직후에 쓴 파일은 hook을 실행하기도 했지만, 이제는 폴더 `C`에 흡수됩니다.
  - 폴더를 지우면 hook이 폴더 경로의 `D`를 파일 이벤트와 구별 없이 받았고, Linux와 macOS에서는 안의 파일마다 `D`도 받았습니다. 이제는 폴더 `D` 하나만 받습니다.
  - 이렇게 흡수되는 파일 이벤트는 `watch.dirEvent: none`이어도 오지 않습니다.
