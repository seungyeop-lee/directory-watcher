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

## usage

```shell
./bin/directory-watcher-macos-amd64

directory-watcher run
Usage of ./bin/directory-watcher-macos-amd64:
  -c string
        config path
  -d    debug
  -v    verbose
```

## 동작 다이어그램

![directory-watcher-life-cycle.png](static/directory-watcher-life-cycle.png)

## cmd 사양

```yaml
# initCmd, endCmd, beforeCmd, afterCmd, cmd 에 동일 사양 적용
# dir 설정 하는 곳이 없는 경우 dir은 commandSet의 path를 따른다.
## commandSet의 Path => root: 프로그램 실행 위치의 path, sets: path로 설정한 폴더
# dir 설정하는 곳의 기준 path는 프로그램 실행 위치의 path이다. 
---
cmd: '실행 커맨드'
---
cmd:
  dir: '실행 커맨드가 실행 될 디렉토리경로'
  cmd: '실행 커맨드'
---
cmd:
  dir: '실행 커맨드가 실행 될 디렉토리경로'
  cmd:
    - '실행 커맨드 1'
    - '실행 커맨드 2'
---
cmd:
  - '실행 커맨드 1'
  - '실행 커맨드 2'
  - dir: '실행 커맨드가 실행 될 디렉토리경로'
    cmd: '실행 커맨드 3'
  - - '실행 커맨드 4-1'
    - '실행 커맨드 4-2'
```
