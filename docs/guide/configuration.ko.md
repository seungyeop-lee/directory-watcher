# 설정 파일

🇺🇸 [English](configuration.md) | 🇰🇷 한국어

## config.yml

- `config.yml` 파일에는 감시할 디렉터리와 각 디렉터리에서 변경이 생겼을 때 실행할 명령을 정의합니다.
- 감시를 시작하거나 끝낼 때 실행할 명령도 정의할 수 있습니다.
- 실제 예시는 [`config.example.yml`](../../config.example.yml)에 있습니다.
- 같은 형식을 JSON 스키마로 기술한 파일은 [`directory-watcher.schema.json`](../../directory-watcher.schema.json)입니다.
- 저장소 루트에서 `make run`을 실행하면 이 예시가 감시하는 폴더를 먼저 만든 뒤 이 예시로 watcher를 실행합니다.
- 아래 블록의 첫 줄 `# yaml-language-server: $schema=...`은 파일을 위의 JSON 스키마에 연결합니다. 직접 만드는 `config.yml`의 맨 위에도 이 줄을 넣습니다.
  - 그러면 VS Code의 Red Hat YAML 확장처럼 yaml-language-server를 쓰는 에디터가 키를 자동 완성하고, 스키마가 허용하지 않는 키와 값을 표시합니다.
  - URL은 `v3`처럼 major version 이름을 딴 브랜치를 가리키며, 이 브랜치에는 그 major 릴리스의 스키마가 있습니다.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/seungyeop-lee/directory-watcher/v3/directory-watcher.schema.json
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

- `path`와 `dir`에 쓴 상대 경로는 모두 watcher를 실행한 폴더(실행 위치)를 기준으로 풉니다.
  - `dir`도 `path`가 아니라 실행 위치를 기준으로 풉니다.
- 명령에 `dir`을 적으면 hook 종류와 관계없이 `dir` 폴더에서 명령을 실행합니다.
- `dir`을 적지 않으면 hook 종류에 따라 다음 폴더에서 명령을 실행합니다.

| hook | `dir`이 없을 때 실행 폴더 |
|------|------------------------|
| 감시 대상의 `onStartWatch`, `onChange`, `onFinishWatch` | 그 감시 대상의 `path` |
| 전역 `onBeforeChange`, `onAfterChange` | 변경이 생긴 감시 대상의 `path` |
| 전역 `onStartWatch`, `onFinishWatch`, `onError`, `onQueueWarning` | 실행 위치 |

아래 설정에서 각 명령은 주석에 적은 폴더에서 실행되며, `<실행 위치>`는 watcher를 실행한 폴더입니다.

```yaml
watchTargets:
  - path: server
    lifeCycle:
      onChange:
        - go run .          # <실행 위치>/server
  - path: web/src
    lifeCycle:
      onChange:
        - dir: web          # <실행 위치>/web (<실행 위치>/web/src/web이 아님)
          cmd: npm run build
```

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
| {{.IsDir}}         | `DW_IS_DIR`          | [폴더 이벤트](events.ko.md#폴더-이벤트)이면 `true`, 파일 이벤트이면 `false`  | false                                            |

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

- 명령에는 `{{.Path}}`나 `{{$.Path}}`처럼 표의 변수를 그대로 넣는 자리만 쓸 수 있습니다.
- watcher는 변수 자리를 셸 참조로 바꾼 명령을 셸에 넘기므로, 템플릿 안에 쓴 조건이나 함수는 실제 값이 아니라 참조 문자열을 보고 평가됩니다.

명령에 `{{if eq .Event "C"}}` 같은 조건(`if`, `with`, `range`), `{{len .Path}}` 같은 함수, `{{.Path | printf "%q"}}` 같은 pipeline이 있으면 watcher는 hook을 하나도 실행하지 않고 다음과 같은 오류를 출력한 뒤 종료 코드 1로 끝납니다.

```text
Error: watchTargets[0].lifeCycle.onChange[0]: unsupported template action {{len .Path}}; only template variables such as {{.Path}} are supported
```

- 값에 따라 명령을 다르게 실행하려면 조건을 셸 명령에 씁니다. `sh`에서는 `if [ {{.Event}} = C ]; then echo created; fi`처럼 씁니다.

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
