# hook 실행

🇺🇸 [English](running.md) | 🇰🇷 한국어

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

## 대기열 용량과 경고

- 대상의 실행은 [run으로 hook 실행하기](#run으로-hook-실행하기)에 적은 대로 시작할 때까지 대상의 대기열에서 기다립니다. `option.queue.size`(기본 2048)는 기다릴 수 있는 실행의 최대 수이며, 실행 중인 실행은 세지 않습니다.
- 들어갈 자리가 없는 실행이 생기면 `dispatch queue full (capacity <size>)`로 처리를 중단하고, [이벤트 유실 알림](#이벤트-유실-알림)처럼 전역 `onError`를 한 번 실행합니다.
  - watcher는 대기열에 자리가 나기를 기다리지 않습니다. 한꺼번에 많이 들어올 수 있는 [폴더 이벤트](events.ko.md#폴더-이벤트)용으로 모은 이벤트에서 만든 실행도 같습니다.

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

- `DW_QUEUE_TOP_PATHS`는 hook이 만든 파일 때문에 hook이 계속 다시 실행되는 경우를 찾는 데 씁니다. 해결 방법은 [hook이 자기 결과로 다시 실행되지 않게 하기](events.ko.md#hook이-자기-결과로-다시-실행되지-않게-하기)에 있습니다.
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

## 과부하와 종료

- 대상마다 대기 실행은 [대기열 용량과 경고](#대기열-용량과-경고)에 적은 대로 최대 `queue.size`개까지 두고, hook 실행은 한 번에 최대 `run.concurrency`개까지 진행합니다. `hookBatch.deliver: each`는 수신 순서를 유지하며 과부하에서 최신 이벤트만 남기는 방식으로 바뀌지 않습니다. 제외된 경로는 실행을 만들지 않고 기다리는 시간도 늘리지 않습니다. 여러 파일의 생성 호출 순서가 OS 이벤트 수신 순서를 보장하지 않으므로, 마지막으로 만든 파일이 `deliver: last`가 실행하는 경로라고 가정하면 안 됩니다.
- 전달 대기열 포화 또는 watcher 오류가 발생하면 처리를 중단합니다. 기본 로그에 감시 대상과 원인을 남기고 모든 대상을 정리한 뒤 CLI가 비정상 종료합니다. 애플리케이션 포화는 `dispatch queue full`로 표시하여 OS overflow와 구분합니다. 자동 재시작·영속 큐는 제공하지 않습니다.
- 대상의 감시를 시작하지 못해도 치명적 오류로 처리합니다. `path`가 없거나, 폴더 링크를 풀지 못하거나, 폴더를 감시 목록에 넣지 못한 경우가 여기에 해당합니다. 이 대상은 `onStartWatch`를 실행하지 않았으므로 `onFinishWatch`도 실행하지 않습니다. 감시를 시작한 다른 대상의 `onFinishWatch`와 전역 `onFinishWatch`는 그대로 실행하고, CLI는 종료 코드 1로 끝납니다.
- 종료 신호는 실행 중인 시작·변경 실행을 모두 취소하고, 대기 변경을 버리고, 이벤트 처리와 취소한 실행이 모두 멈출 때까지 기다린 뒤 종료 hook을 실행합니다. 종료 hook에는 기존 명령 실행 규칙을 유지하며 별도 제한 시간을 추가하지 않습니다. 감시 대상끼리는 독립 실행합니다.
- Windows에서 취소된 hook은 CTRL+BREAK를 받고, `cmd`에서 `a & b`의 다음 프로그램처럼 hook이 취소 뒤에 시작한 프로그램도 CTRL+BREAK를 받습니다. 첫 CTRL+BREAK 뒤 20초가 지나도 실행 중인 hook의 프로세스는 함께 끝내며, 신호를 무시하거나 자기만의 콘솔이나 프로세스 그룹을 쓰는 프로세스도 여기서 끝납니다. 교체와 종료는 hook의 프로세스가 모두 끝날 때까지 기다리므로, 일부 `powershell` hook처럼 CTRL+BREAK로 끝나지 않는 hook은 교체와 종료를 최대 20초 늦춥니다.
- Windows에서 시작한 지 100ms가 지나기 전에 취소된 hook은 시작 후 100ms가 지났을 때 CTRL+BREAK를 받으므로, hook이 시작된 직후의 교체와 종료는 최대 100ms 늦어집니다.
- Windows에서 취소되지 않고 끝난 hook이 시작한 프로세스는 계속 실행됩니다.
- Windows E2E가 취소된 hook의 종료에 대해 확인하는 내용은 [Windows E2E](../../e2e/docs/windows.ko.md#hook-취소-검증)에 있습니다.
- 파일 조작과 OS 알림은 일대일이 아닙니다. Linux는 OS 이벤트 overflow를 보고하며, 이때도 전달 대기열이 가득 찼을 때처럼 처리를 중단합니다. macOS는 overflow를 보고하지 않습니다. 애플리케이션 상한이 모든 OS 유실의 감지를 보장하지는 않습니다. [E2E 시나리오와 한계](../../e2e/docs/reliability.ko.md#경합과-과부하-검증)를 확인하세요.

- 이 단계들의 순서는 [동작 방식](how-it-works.ko.md)에 그려 두었습니다.
