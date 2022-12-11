# Directory Watcher

디렉토리 내 변경 (생성, 수정, 삭제)이 발생 시 정해진 커맨드를 실행하게 하는 프로그램

## 기능 및 특징

- 여러개의 디렉토리 감시 가능
- 각 디렉토리 별 변경 발생 시 실행되는 커맨드 개별 설정 가능
- 공통적으로 실행되어야 하는 커맨드 설정, 감시 시작 및 감시 종료 시 실행되어야 하는 커맨드 설정 가능
- 감시 대상 디렉토리 내 감시 제외 디렉토리 추가 가능
- 감시 대상 디렉토리 내 감시 제외 접미사 추가 가능

## 실행

`bin` 폴더에 생성된 실행파일을 실행 

## 사용법

```shell
./bin/directory-watcher-macos-amd64 -h

Usage:
  directory-watcher-macos-amd64 [flags]

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
      - [cmdInfo]
    onBeforeChange: # global onBeforeChange hook
      - [cmdInfo]
    onAfterChange: # global onAfterChange hook
      - [cmdInfo]
    onFinishWatch: # global onFinishWatch hook
      - [cmdInfo]
watchTargets:
  - path: [감시대상 폴더 path]
    lifeCycle:
      onStartWatch: # onStartWatch hook
        - [cmdInfo]
      onChange: # onChange hook
        - [cmdInfo]
      onFinishWatch: # onFinishWatch hook
        - [cmdInfo]
    option:
      excludeDir:
        - [감시 제외대상 폴더 path]
      excludeSuffix:
        - [감시 제외대상 파일 접미사]
      waitMillisecond: [이벤트 발생 후, hook을 실행하는 사이 대기시간, default는 100]
```

[cmdInfo]는 [cmd 사양](#cmd-사양) 참조

## 동작 다이어그램

![directory-watcher-life-cycle.png](static/directory-watcher-life-cycle.png)

## cmd 사양

### yaml

```yaml
# lifeCycle하부 callback에 동일 사양 적용
cmdInfo:
  - '실행 커맨드'
  - dir: '실행 커맨드가 실행 될 디렉토리경로'
    cmd: '실행 커맨드'
  - dir: '실행 커맨드가 실행 될 디렉토리경로'
    cmd:
      - '실행 커맨드 1'
      - '실행 커맨드 2'
```

### cmd 실행 위치 적용 룰

- global
  - dir O => dir 값 적용
  - dir X => 프로그램 실행 위치 적용
- watchTargets
  - dir O => dir 값 적용
  - dir X => path 값 적용

## 빌드

직접 빌드하고 싶다면 아래의 명령어로 빌드가 가능하다. 빌드된 파일은 `bin` 폴더에 생성된다.

```shell
make build
```
