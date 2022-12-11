# Directory Watcher

디렉토리 내 변경 (생성, 수정, 삭제)이 발생 시 정해진 커맨드를 실행하게 하는 프로그램

## 기능 및 특징

- 여러개의 디렉토리 감시 가능
- 각 디렉토리 별 변경 발생 시 실행되는 커맨드 개별 설정 가능
- 공통적으로 실행되어야 하는 커맨드 설정, 감시 시작 및 감시 종료 시 실행되어야 하는 커맨드 설정 가능
- 감시 대상 디렉토리 내 감시 제외 디렉토리 추가 가능
- 감시 대상 디렉토리 내 감시 제외 접미사 추가 가능

## 빌드

```shell
make build
```

## 실행

빌드 후 `bin` 폴더에 생성된 실행파일을 실행 

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
