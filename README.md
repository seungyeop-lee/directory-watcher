# Directory Watcher

디렉토리 내 변경 (생성, 수정, 삭제)이 발생 시 정해진 커맨드를 실행하게 하는 프로그램

## 기능 및 특징

- 여러개의 디렉토리 감시 가능
- 각 디렉토리 별 변경 발생 시 실행되는 커맨드 개별 설정 가능
- 공통적으로 실행되어야 하는 커맨드 설정, 감시 시작 및 감시 종료 시 실행되어야 하는 커맨드 설정 가능
- 감시 대상 디렉토리 내 감시 제외 디렉토리 추가 가능
- 감시 대상 디렉토리 내 감시 제외 접미사 추가 가능

## 설치

### Homebrew

```console
$ brew install seungyeop-lee/tap/directory-watcher
```

### `go install`

```console
$ go install github.com/seungyeop-lee/directory-watcher/v2@latest
```

### Releases

[releases page](https://github.com/seungyeop-lee/directory-watcher/releases/latest)에서 실행파일을 다운로드

## 사용법

```shell
$ directory-watcher -h

Usage:
  directory-watcher [flags]

Flags:
  -c, --config-path string   set config path (default "config.yml")
  -h, --help                 help for directory-watcher-macos-amd64
  -l, --log-level string     set log level (default "ERROR")
```

## config.yml

실제 파일은 `config.example.yml` 예제 참조

```yaml
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
```

### cmd 실행 위치

| dir | global onStartWatch, global onFinishWatch | 그외 hook    |
|-----|-------------------------------------------|------------|
| O   | dir 설정 위치                                 | dir 설정 위치  |
| X   | 프로그램 실행 위치                                | path 설정 위치 |

## 동작 다이어그램

![directory-watcher-life-cycle.png](static/directory-watcher-life-cycle.png)
