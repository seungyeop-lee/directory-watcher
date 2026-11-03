# Directory Watcher

🇺🇸 [English](README.md) | 🇰🇷 한국어

Directory Watcher는 감시하는 디렉터리에서 파일이 생성·수정·삭제되면 미리 정해 둔 명령을 실행하는 프로그램입니다. 코드를 고칠 때마다 빌드나 테스트를 다시 실행하거나, 소스가 바뀌면 개발 서버를 재시작하거나, 파일이 바뀔 때 백업을 만드는 작업을 자동화할 수 있습니다.

## 기능

### 파일이 바뀌면 명령 실행하기

- 여러 디렉터리를 한 번에 감시합니다. 감시 대상마다 `onChange` 명령을 따로 정하고, 전역 `onBeforeChange`와 `onAfterChange` 명령은 모든 대상의 `onChange` 앞뒤에 실행됩니다.
- 반응할 이벤트(생성, 수정, 삭제)를 고르고, 특정 디렉터리나 파일 접미사, 하위 디렉터리를 감시에서 뺄 수 있습니다.
- 바뀐 파일의 경로, 이름, 확장자, 디렉터리, 이벤트 종류를 [템플릿 변수](USAGE.ko.md#실행-명령에서-쓸-수-있는-변수)로 명령에 넘깁니다.

### 몰려드는 변경 다루기

- 기본 동작에서는 이벤트가 몰려도 마지막 이벤트 뒤 `waitMillisecond`(기본 100ms)가 지나면 hook을 한 번만 실행합니다.
- `noWait: true`로 설정하면 이벤트마다 hook을 실행하며, 이벤트를 받은 순서를 지킵니다.
- 대상마다 대기 이벤트를 최대 256개까지 보관합니다. 대기열이 가득 차면 대기열을 늘리지 않고 watcher를 멈춥니다.

### 오래 실행되는 프로세스 재시작하기

- `interruptible: true`로 설정하면 새 변경이 생겼을 때 실행 중인 명령을 취소하고, 그 명령이 끝나기를 기다린 뒤 변경 명령을 다시 실행합니다. 스스로 끝나지 않는 서버 같은 프로세스에 사용합니다.
- watcher를 멈추면 실행 중인 명령을 취소한 뒤 종료 명령을 실행합니다.

### 시작·종료 명령 실행하기

- `onStartWatch`와 `onFinishWatch`는 감시를 시작하고 끝낼 때 실행되며, 전역과 감시 대상별로 각각 정할 수 있습니다.

### 이벤트 유실 가능성 알림 받기

- 대상의 대기열이 가득 차거나 OS가 이벤트 overflow를 보고하면, 선택 항목인 전역 `onError` 명령이 한 번 실행됩니다. 오류 정보는 `DW_ERROR_*` 환경 변수로 전달됩니다. 자세한 내용은 [이벤트 유실 알림](USAGE.ko.md#이벤트-유실-알림)을 확인하세요.

## 요구 사항

- macOS 또는 Linux. 이 두 환경에서 정상 작동하는 것을 확인했습니다.
- Windows. 아래 경고의 제약이 있습니다.
- Go 1.23 이상. `go install`로 설치할 때만 필요합니다.

> [!WARNING]
> Windows에서는 다음을 주의하세요.
> - PowerShell이나 명령 프롬프트에서는 출력이 제대로 보이지 않을 수 있습니다. 이때는 Git Bash에서 실행하세요.
> - watcher가 hook을 멈출 때, hook이 별도 콘솔이나 프로세스 그룹으로 띄운 프로세스나 CTRL+BREAK를 무시하는 프로세스는 계속 실행될 수 있습니다. 자세한 내용은 [과부하와 종료](USAGE.ko.md#과부하와-종료)에 있습니다.

## 설치

### Homebrew

```shell
brew install seungyeop-lee/tap/directory-watcher
```

### `go install`

```shell
go install github.com/seungyeop-lee/directory-watcher/v2@latest
```

### Docker로 실행

게시된 이미지에 `config.yml`과 감시할 폴더를 마운트해 실행합니다.

```shell
docker run --rm \
  -v "$PWD/config.yml:/config.yml" \
  -v "$PWD/src:/src" \
  ghcr.io/seungyeop-lee/directory-watcher:latest -c /config.yml
```

컨테이너의 작업 폴더가 `/`이므로 `config.yml`의 `path: src`는 마운트한 `/src`를 가리킵니다. hook은 컨테이너 안에서 실행되므로, hook이 쓰는 파일과 도구가 컨테이너에 있어야 합니다. bind mount의 알려진 이슈는 [Docker로 실행하기](USAGE.ko.md#docker로-실행하기)에 있습니다.

### Dockerfile에서 설치

```dockerfile
COPY --from=ghcr.io/seungyeop-lee/directory-watcher:latest /directory-watcher /usr/local/bin/
```

### 릴리스 파일

[releases page](https://github.com/seungyeop-lee/directory-watcher/releases/latest)에서 실행 파일을 내려받습니다.

## 시작하기

1. watcher를 실행할 디렉터리에 `config.yml`을 만듭니다. 다음 예시는 `src` 아래의 파일이 바뀔 때마다 그 디렉터리에서 `make test`를 실행합니다.

   ```yaml
   watchTargets:
     - path: src
       lifeCycle:
         onChange:
           - dir: .
             cmd: make test
   ```

2. 같은 디렉터리에서 watcher를 시작합니다.

   ```shell
   directory-watcher
   ```

   다른 위치의 설정 파일을 쓰려면 `-c <설정 파일 경로>`로 경로를 지정합니다.

3. `src` 아래의 파일을 고칩니다. 마지막 변경 뒤 100ms가 지나면 `make test`가 실행됩니다.

- **멈추기:** Ctrl+C를 누르거나 SIGTERM을 보냅니다. watcher는 실행 중인 명령을 취소하고 종료 명령을 실행한 뒤 끝납니다.
- **더 알아보기:** [사용 안내](USAGE.ko.md)에서 명령줄 옵션, Docker로 실행하기, `config.yml` 전체 사양, 명령 실행 위치, 템플릿 변수, 이벤트 유실 알림, 과부하·종료·폴더 이벤트의 제약을 다룹니다. 더 많은 설정을 담은 예시는 [`config.example.yml`](config.example.yml)에 있습니다.

## 동작 다이어그램

전역 `onStartWatch`가 가장 먼저 실행됩니다. 그 뒤 감시 대상마다 따로 시작하고, 각 대상은 다른 대상과 관계없이 자기 변경을 처리합니다.

```mermaid
flowchart TD
    globalStart["전역 onStartWatch"]
    subgraph eachTarget["감시 대상마다 독립적으로"]
        watchDirs["대상 디렉터리 감시 시작"]
        targetStart["대상 onStartWatch"]
        receiveEvent(("이벤트 수신"))
        filterEvent{"감시하는 이벤트 종류이고<br>제외 접미사가 아닌가?"}
        queueEvent{"대상의 256개 대기열에<br>자리가 있는가?"}
        modeChoice{"noWait?"}
        debounceWait["마지막 이벤트 뒤 waitMillisecond 대기<br>그 이벤트만 남김"]
        orderedRun["받은 순서대로 이벤트 처리"]
        interruptChoice{"interruptible?"}
        cancelPrevious["실행 중인 명령을 취소하고<br>끝나기를 기다림"]
        runChange["전역 onBeforeChange → onChange<br>→ 전역 onAfterChange"]
    end
    fatalStop(["치명적 오류: 다음 다이어그램"])

    globalStart --> watchDirs
    watchDirs --> targetStart
    targetStart --> receiveEvent
    receiveEvent -->|"새 디렉터리는 감시 목록에 추가"| filterEvent
    filterEvent -->|"아니오: 무시"| receiveEvent
    filterEvent -->|"예"| queueEvent
    queueEvent -->|"아니오"| fatalStop
    queueEvent -->|"예"| modeChoice
    modeChoice -->|"false"| debounceWait
    modeChoice -->|"true"| orderedRun
    debounceWait --> interruptChoice
    orderedRun --> interruptChoice
    interruptChoice -->|"true"| cancelPrevious
    interruptChoice -->|"false: 실행 중인 명령이 끝날 때까지 대기"| runChange
    cancelPrevious --> runChange
    runChange -->|"다음 이벤트"| receiveEvent
```

- `interruptible: false`에서는 대상 `onStartWatch`나 변경 명령이 실행되는 동안 들어온 이벤트가 그 명령이 끝날 때까지 대기열에서 기다립니다. `interruptible: true`에서는 실행 중인 대상 `onStartWatch`도 다음 변경이 취소합니다.
- 한 번의 실행에 속한 명령은 차례대로 실행되며, 실패한 명령이 나오면 그 실행을 멈춥니다. 이 실패는 `[ERROR]` 로그로 남습니다. 자세한 내용은 사용 안내의 [`config.yml`](USAGE.ko.md#configyml)을 확인하세요.

종료 신호와 치명적 오류는 모두 전체 감시 대상을 멈춘 뒤 종료 명령을 실행합니다.

```mermaid
flowchart TD
    shutdownSignal(("SIGINT 또는 SIGTERM"))
    fatalError(("대상의 치명적 오류:<br>대기열 포화, OS overflow,<br>watcher 오류, 없는 경로"))
    stopTargets["모든 대상: 새 명령 시작을 멈추고<br>실행 중인 명령 취소"]
    targetFinish["대상마다: 대기 중인 변경을 버리고<br>실행 중인 명령이 끝나기를 기다린 뒤<br>감시를 시작한 대상이면 onFinishWatch 실행"]
    lossCheck{"이벤트 유실 가능성?<br>대기열 포화 또는 OS overflow"}
    errorHook["전역 onError 한 번<br>5초 실행 제한, 이어서 최대 20초 정리"]
    globalFinish["전역 onFinishWatch"]
    exitCheck{"치명적 오류가 있었는가?"}
    exitSuccess(("종료 코드 0"))
    exitFailure(("종료 코드 1"))

    shutdownSignal --> stopTargets
    fatalError --> stopTargets
    stopTargets --> targetFinish
    stopTargets -->|"치명적 오류 뒤"| lossCheck
    lossCheck -->|"예"| errorHook
    lossCheck -->|"아니오"| globalFinish
    targetFinish --> globalFinish
    errorHook --> globalFinish
    globalFinish --> exitCheck
    exitCheck -->|"아니오"| exitSuccess
    exitCheck -->|"예"| exitFailure
```

- 대상 종료 명령은 `onError`가 실행되는 동안에도 실행될 수 있습니다. 전역 `onFinishWatch`는 두 쪽이 모두 끝나기를 기다립니다.
- 종료 명령에는 별도의 실행 제한 시간이 없습니다.

## 개발 검증

현재 워크트리 소스를 사용하는 macOS·Linux·Windows 자동 E2E 실행과 시나리오 추가 방법은 [E2E 가이드](e2e/README.ko.md)를 확인하세요.

## 라이선스

이 프로젝트는 [GPL-3.0 라이선스](LICENSE)에 따라 제공됩니다. 자세한 내용은 라이선스 파일을 확인하세요.
