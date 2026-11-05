# 설치와 실행

🇺🇸 [English](installation.md) | 🇰🇷 한국어

## Homebrew로 설치하기

- cask는 macOS와 Linux에서 설치합니다.
- cask를 설치하려면 Homebrew 5.1.14 이상이 필요합니다.
- 그보다 낮은 버전에서는 `Unexpected method 'postflight_steps'` 오류로 설치가 실패합니다.

`brew --version`으로 버전을 확인하고, 5.1.14보다 낮으면 먼저 Homebrew를 업데이트합니다.

```shell
brew update
```

그다음 cask를 설치합니다.

```shell
brew install --cask seungyeop-lee/tap/directory-watcher
```

## 명령줄 옵션

```shell
Usage:
  directory-watcher [flags]

Flags:
  -c, --config-path string   set config path (default "config.yml")
  -n, --dry-run              print hook commands without running them
  -h, --help                 help for directory-watcher
  -s, --silent               don't print hook commands before running them
  -v, --version              version for directory-watcher

Maintenance Flags:
  -l, --log-level string   set log level (default "ERROR")
```

- watcher는 hook 명령을 실행하기 전에 그 명령을 출력합니다. `-s`와 `-n`은 [hook 명령 출력](running.ko.md#hook-명령-출력)을 참고하세요.

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

### 직접 만드는 이미지에 넣기

Dockerfile에서 게시된 이미지의 실행 파일을 복사합니다.

```dockerfile
COPY --from=ghcr.io/seungyeop-lee/directory-watcher:latest /directory-watcher /usr/local/bin/
```

## Windows에서 실행하기

- watcher는 실행되는 동안 콘솔의 출력 code page를 UTF-8(65001)로 바꿉니다. 그래서 `cmd`와 `powershell` hook이 출력한 한글 같은 비ASCII 글자가 hook을 직접 실행했을 때와 같은 글자로 watcher 출력에 나타납니다.
  - watcher가 끝날 때까지는 사용자의 셸을 포함해 같은 콘솔을 쓰는 다른 프로그램도 UTF-8 출력 code page를 씁니다.
  - watcher는 CTRL+C로 멈추거나 오류로 끝날 때 출력 code page를 원래 값으로 되돌립니다.
- watcher가 hook을 멈출 때, CTRL+BREAK로 끝나지 않는 hook 프로세스는 20초가 지나서야 강제로 끝냅니다. 이런 hook은 일부 `powershell` hook을 포함해 교체와 종료가 최대 20초 늦어질 수 있습니다. 자세한 내용은 [과부하와 종료](running.ko.md#과부하와-종료)에 있습니다.
