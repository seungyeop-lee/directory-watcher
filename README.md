# Directory Watcher

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
