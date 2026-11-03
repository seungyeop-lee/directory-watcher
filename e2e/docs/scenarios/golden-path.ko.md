# `TestGoldenPath`: 생성·수정·삭제와 hook 순서

파일 하나를 생성·수정·삭제하고, 전역·대상 hook이 정해진 순서로 한 번씩 실행되며 7개 이벤트 변수가 올바른 값으로 전달되는지 확인합니다.

## 확인 내용과 성공 조건

watcher가 정상 종료한 뒤 `records.log`의 전체 줄이 아래 순서와 정확히 같아야 합니다. 줄이 빠지거나, 더 있거나, 순서가 다르면 실패합니다.

1. `GLOBAL:onStartWatch`
2. `TARGET:onStartWatch`
3. 생성(`C`), 수정(`U`), 삭제(`D`) 이벤트마다 `GLOBAL:onBeforeChange:<변수>`, `TARGET:onChange:<변수>`, `GLOBAL:onAfterChange:<변수>`
4. `TARGET:onFinishWatch`
5. `GLOBAL:onFinishWatch`

`<변수>`는 `{{.Path}}|{{.AbsPath}}|{{.FileName}}|{{.ExtName}}|{{.DirPath}}|{{.DirAbsPath}}|{{.Event}}`입니다. 설정의 감시 경로가 절대 경로이므로 `{{.Path}}`와 `{{.AbsPath}}`는 모두 `foo.txt`의 절대 경로이고, `{{.DirPath}}`와 `{{.DirAbsPath}}`는 모두 감시 폴더의 절대 경로입니다. `{{.FileName}}`은 `foo.txt`, `{{.ExtName}}`은 `.txt`, `{{.Event}}`는 `C`·`U`·`D`입니다.

watcher는 종료 신호를 받은 뒤 10초 안에 종료 코드 0으로 끝나야 합니다.

## 설정

- 전역 hook `onStartWatch`·`onBeforeChange`·`onAfterChange`·`onFinishWatch`와 대상 hook `onStartWatch`·`onChange`·`onFinishWatch`를 모두 설정합니다.
- 각 hook은 `hookrec --line=<라벨>`로 `GLOBAL:onBeforeChange`처럼 자기 라벨을 기록합니다. 변경 hook은 라벨 뒤에 `:<변수>`를 붙여 기록합니다.
- 대상 옵션은 `waitMillisecond: 100`, `watchEvent: CDU`, `excludeSuffix: [.probe]`입니다.

## 진행 순서

```mermaid
sequenceDiagram
    autonumber
    participant testCode as 테스트
    participant watcherProc as watcher
    participant hookRec as hookrec
    participant recordFile as records.log
    participant watchDir as 감시 폴더
    testCode->>watcherProc: Start (DEBUG 로그로 감시 준비 확인)
    watcherProc->>hookRec: 전역·대상 onStartWatch
    hookRec->>recordFile: GLOBAL·TARGET onStartWatch 기록
    testCode->>recordFile: TARGET:onStartWatch 대기 (최대 10초)
    loop 생성 C, 수정 U, 삭제 D 순서로 한 번씩
        testCode->>watchDir: foo.txt 생성·수정·삭제
        watchDir-->>watcherProc: 파일 이벤트
        watcherProc->>hookRec: onBeforeChange, onChange, onAfterChange
        hookRec->>recordFile: 라벨과 7개 변수 세 줄
        testCode->>recordFile: GLOBAL:onAfterChange 대기 (최대 10초)
    end
    testCode->>watcherProc: 종료 신호
    watcherProc->>hookRec: 대상·전역 onFinishWatch
    hookRec->>recordFile: TARGET·GLOBAL onFinishWatch 기록
    watcherProc-->>testCode: 종료 코드 0 (최대 10초 대기)
    testCode->>recordFile: 전체 줄을 기대 순서와 비교
```

- 생성은 `foo.txt`를 내용 없이 `O_CREATE|O_EXCL|O_RDONLY`로 열었다 닫는 방식이고, 수정은 `hello`를 쓰는 방식입니다.
- 각 단계는 그 이벤트의 `GLOBAL:onAfterChange:<변수>` 기록이 나타나야 다음 단계로 넘어갑니다.
- 종료 신호는 Unix에서 SIGTERM, Windows에서 CTRL+BREAK입니다.

## 실행

저장소 루트에서 실행합니다. 차례로 로컬, Linux Docker, UTM Windows VM에서 실행하는 명령입니다.

```sh
go test -count=1 -v -run '^TestGoldenPath$' ./e2e/scenarios
go run ./e2e/linux -- -run '^TestGoldenPath$'
make e2e-windows ARGS="-- -run '^TestGoldenPath$'"
```

UTM 명령은 [UTM Windows VM에서 E2E 실행](../windows-utm.ko.md)의 사전 준비를 마치고 `make e2e-windows-up`으로 VM을 켠 뒤 실행합니다. Windows 호스트에서 직접 실행하는 방법은 [Windows 실행 명령](../windows.ko.md#실행-명령)에 있습니다.

## 관련 코드

- `e2e/scenarios/golden_path_test.go`: `TestGoldenPath`, 설정을 만드는 `goldenConfig`, 기대 기록을 만드는 `changeValues`·`expectedSequence`
- `e2e/harness`: `Start`, `Hook`, `AwaitLine`, `Stop`, `Lines`. 각 함수의 위치와 책임은 [책임별 코드 위치](../harness.ko.md#책임별-코드-위치)에 있습니다.
