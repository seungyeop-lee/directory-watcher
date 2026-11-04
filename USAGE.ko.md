# Directory Watcher 사용 안내

🇺🇸 [English](USAGE.md) | 🇰🇷 한국어

- 이 안내는 [README](README.ko.md#설치)의 방법으로 Directory Watcher를 설치했다고 가정합니다.
- 레시피를 쓰려면 설정을 `config.yml`로 저장하고, 그 파일이 있는 디렉터리에서 `directory-watcher`를 실행합니다.

## 레시피

### 저장할 때마다 개발 서버 재시작하기

```yaml
watchTargets:
  - path: server
    lifeCycle:
      onStartWatch:
        - go run .
      onChange:
        - go run .
    option:
      run:
        interrupt: oldest
```

- 감시를 시작할 때 서버가 시작됩니다.
- `interrupt: oldest`이면 저장할 때마다 실행 중인 서버와 서버가 시작한 프로세스를 멈추고, 그 프로세스가 끝나기를 기다린 뒤 서버를 다시 시작합니다. 자세한 내용은 [run으로 hook 실행하기](docs/guide/running.ko.md#run으로-hook-실행하기)를 확인하세요.
- `dir`이 없는 hook은 감시 대상의 `path`에서 실행되므로, `go run .`은 `server`에서 실행됩니다. 자세한 내용은 [명령 실행 위치](docs/guide/configuration.ko.md#명령-실행-위치)를 확인하세요.
- Ctrl+C를 누르면 서버가 멈춘 뒤 watcher가 끝납니다.

### 저장한 패키지의 테스트 실행하기

```yaml
watchTargets:
  - path: .
    lifeCycle:
      onChange:
        - if [ {{.ExtName}} = .go ]; then go test ./{{.ParentPath}}; fi
    option:
      watch:
        dirEvent: none
```

- `{{.ExtName}}`와 `{{.ParentPath}}`는 바뀐 파일의 확장자와 폴더입니다. 그래서 `.go` 파일을 저장하면 그 파일이 든 패키지에서 `go test`가 실행되고, 다른 파일을 저장하면 아무것도 실행되지 않습니다. 자세한 내용은 [실행 명령에서 쓸 수 있는 변수](docs/guide/configuration.ko.md#실행-명령에서-쓸-수-있는-변수)를 확인하세요.
- `dirEvent: none`이 있으므로 폴더를 만들거나 지워도 hook이 실행되지 않습니다. 자세한 내용은 [폴더 이벤트](docs/guide/events.ko.md#폴더-이벤트)를 확인하세요.
- 여러 파일이 한꺼번에 바뀌면 가장 나중에 바뀐 파일로 hook이 한 번 실행됩니다. 바뀐 패키지를 모두 테스트하려면 `hookBatch.deliver: lastPerPath`를 적어, 바뀐 파일마다 hook을 한 번씩 실행합니다. 자세한 내용은 [hookBatch로 기다리고 합치기](docs/guide/running.ko.md#hookbatch로-기다리고-합치기)를 확인하세요.
- 테스트가 실패하면 `[ERROR]` 로그가 남습니다.

### 바뀐 내용을 다른 폴더에 복사하기

이 레시피에는 `rsync`가 필요합니다.

```yaml
watchTargets:
  - path: src
    lifeCycle:
      onStartWatch:
        - dir: .
          cmd: rsync -a --delete src/ backup/
      onChange:
        - dir: .
          cmd: rsync -a --delete src/ backup/
```

- `onStartWatch`가 감시를 시작할 때 `src`를 `backup`에 복사하고, `onChange`가 변경이 생길 때마다 다시 복사하므로 `backup`에는 `src`와 같은 파일이 남습니다.
- `--delete`는 `src`에서 사라진 파일을 `backup`에서도 지웁니다.
- `dir: .`이 있으므로 `rsync`는 watcher를 실행한 디렉터리에서 실행됩니다. 자세한 내용은 [명령 실행 위치](docs/guide/configuration.ko.md#명령-실행-위치)를 확인하세요.
- 복사할 곳은 `path` 밖에 둡니다. `path` 안에 복사한 파일도 변경이므로 hook이 끝없이 다시 실행됩니다. 자세한 내용은 [hook이 자기 결과로 다시 실행되지 않게 하기](docs/guide/events.ko.md#hook이-자기-결과로-다시-실행되지-않게-하기)를 확인하세요.
- 이름이 `.`이나 `_`로 시작하는 파일도 복사되지만, 그런 파일만 바꾸면 hook이 실행되지 않습니다. 자세한 내용은 [기본 제외 규칙](docs/guide/events.ko.md#기본-제외-규칙)을 확인하세요.

### Docker에서 watcher 실행하기

프로젝트 디렉터리를 `/app`에 마운트하고, 그 안의 `config.yml` 경로를 넘깁니다.

```shell
docker run --rm \
  -v "$PWD:/app" \
  ghcr.io/seungyeop-lee/directory-watcher:latest -c /app/config.yml
```

`config.yml`에는 경로를 `app` 아래로 적습니다.

```yaml
watchTargets:
  - path: app/src
    lifeCycle:
      onChange:
        - dir: app
          cmd: make build
```

- 컨테이너의 작업 폴더가 `/`이므로 `config.yml`의 상대 경로는 `/`를 기준으로 풀리고, `app/src`는 프로젝트의 `src` 폴더를 가리킵니다.
- `make build`는 컨테이너 안에서 실행되므로 `make`, `git`, `curl`처럼 이미지에 있는 도구만 쓸 수 있습니다. 직접 쓰는 빌드 도구가 필요하면 [직접 만드는 이미지에 넣기](docs/guide/installation.ko.md#직접-만드는-이미지에-넣기)처럼 그 도구가 있는 이미지에 watcher를 넣습니다.
- 이미지에 있는 도구와 bind mount된 디렉터리에서 삭제 이벤트가 누락되는 알려진 이슈는 [Docker로 실행하기](docs/guide/installation.ko.md#docker로-실행하기)에 있습니다.

## 주제별 문서

- [설치와 실행](docs/guide/installation.ko.md)은 Homebrew로 설치하기, 명령줄 옵션, Docker로 실행하기, Windows에서 실행하기를 다룹니다.
- [설정 파일](docs/guide/configuration.ko.md)은 `config.yml` 전체 사양, hook 명령을 실행하는 셸, 명령 실행 위치, 명령에서 쓸 수 있는 변수를 다룹니다.
- [이벤트와 감시 경로](docs/guide/events.ko.md)는 파일의 이름 바꾸기와 옮기기, 폴더 이벤트, 기본 제외 규칙, `exclude`로 경로 제외하기, 감시 대상의 경로가 겹칠 때를 다룹니다.
- [hook 실행](docs/guide/running.ko.md)은 `hookBatch`로 이벤트를 기다리고 합치기, 대기열 용량과 경고, `run`으로 hook 실행하기, 이벤트 유실 알림, 과부하와 종료를 다룹니다.
- [동작 방식](docs/guide/how-it-works.ko.md)은 watcher가 변경을 처리하는 순서와 멈추는 순서를 그림으로 보여 줍니다.
