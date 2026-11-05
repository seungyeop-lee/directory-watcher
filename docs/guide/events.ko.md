# 이벤트와 감시 경로

🇺🇸 [English](events.md) | 🇰🇷 한국어

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

## 폴더 이벤트

- hook은 파일뿐 아니라 폴더의 이벤트도 받습니다. `{{.IsDir}}`와 `DW_IS_DIR`는 폴더 이벤트이면 `true`, 파일 이벤트이면 `false`입니다.
- 폴더 이벤트는 `C`와 `D`뿐입니다. 폴더에는 `U`가 없으며, 폴더 안의 변경은 그 안에 있는 파일의 이벤트로 hook에 갑니다.
- `option.watch.fileEvent`로 hook을 실행할 파일 이벤트를, `option.watch.dirEvent`로 폴더 이벤트를 고릅니다. 기본값은 `CUD`와 `CD`이고, `none`을 적으면 그쪽 이벤트를 받지 않습니다.
- `cat {{.Path}}`처럼 파일 경로를 기대하는 hook은 `if [ {{.IsDir}} = false ]; then cat {{.Path}}; fi`처럼 셸에서 `{{.IsDir}}`를 확인하거나, 그 감시 대상에 `watch.dirEvent: none`을 적어야 합니다.

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
  - 이미 폴더로 확인한 경로를 지운 뒤 2초 동안, 그 경로의 중복 삭제 알림과 경로가 없는 동안의 후행 수정 알림은 버립니다. 폴더 이벤트를 모으지 않거나 묶음이 나뉘어도 같으며, 같은 경로에 새로 생긴 파일이나 폴더의 변경은 별도로 처리합니다.
    - Windows는 이런 알림을 보내고, macOS는 가끔 중복 삭제 알림을 보냅니다. 이 알림은 보통 삭제 직후에 옵니다. Linux는 둘 다 보내지 않습니다.
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
