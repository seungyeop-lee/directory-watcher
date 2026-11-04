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

- 2.x 버전을 `brew install seungyeop-lee/tap/directory-watcher`로 설치했다면 Homebrew formula로 설치된 것이고, formula는 더 이상 새 버전을 받지 않습니다.

formula를 지운 뒤 cask로 설치합니다.

```shell
brew uninstall --formula directory-watcher
brew install --cask seungyeop-lee/tap/directory-watcher
```

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

### 직접 만드는 이미지에 넣기

Dockerfile에서 게시된 이미지의 실행 파일을 복사합니다.

```dockerfile
COPY --from=ghcr.io/seungyeop-lee/directory-watcher:latest /directory-watcher /usr/local/bin/
```

## Windows에서 실행하기

- PowerShell이나 명령 프롬프트에서는 출력이 제대로 보이지 않을 수 있습니다. 이때는 Git Bash에서 실행하세요.
- watcher가 hook을 멈출 때, CTRL+BREAK로 끝나지 않는 hook 프로세스는 20초가 지나서야 강제로 끝냅니다. 이런 hook은 일부 `powershell` hook을 포함해 교체와 종료가 최대 20초 늦어질 수 있습니다. 자세한 내용은 [과부하와 종료](running.ko.md#과부하와-종료)에 있습니다.
