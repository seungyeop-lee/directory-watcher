# 릴리스

version은 git tag `v<X.Y.Z>`로만 정합니다. 저장소에는 version을 적는 파일이 없습니다. `v`로 시작하는 tag를 push하면 `.github/workflows/goreleaser.yml`이 그 tag의 커밋을 배포합니다.

## 릴리스 커밋과 tag 만들기

1. 저장소 루트 `CHANGELOG.md`에 `## [<version>] - <YYYY-MM-DD>` 절을 쓰고, 바뀐 내용을 영어로 적습니다. `## [Unreleased]` 절이 있으면 그 절을 새 version 절로 바꿉니다. 새 절의 본문(다음 `## ` 제목 앞까지)이 GitHub Release의 노트가 됩니다.
2. 이 변경을 `master`에 커밋하고, 그 커밋에 tag `v<version>`을 답니다.
3. `git push origin master v<version>`으로 커밋과 tag를 push합니다.

1–2단계는 프로젝트 skill `/versionup [patch|minor|major]`(Codex에서는 `$versionup`)로 실행할 수 있습니다. skill은 push하지 않으므로, 커밋과 tag를 검토한 뒤 3단계는 직접 실행합니다.

push하기 전에 커밋을 고치면 tag는 고치기 전 커밋에 남습니다. `git tag -d v<version>`으로 지우고 고친 커밋에 다시 답니다.

### major를 올릴 때

Go는 major 2 이상인 version의 모듈을 `/v<major>` 접미사가 붙은 경로로 찾습니다. major를 올리기 전에 `go.mod`의 모듈 경로와 그 경로를 쓰는 import, `.goreleaser.yaml`의 ldflags, README의 `go install` 명령을 새 major로 바꿔 먼저 커밋합니다. versionup skill은 모듈 경로가 새 version의 major와 맞지 않으면 아무것도 바꾸지 않고 멈춥니다.

## 배포 흐름

tag를 push하면 `goreleaser.yml`이 아래 순서로 실행합니다. 검사나 노트 추출이 실패하면 아무것도 게시하지 않습니다.

1. `go vet ./...`와 `go test ./...`로 검사합니다.
2. `go run ./scripts/releasenotes <version>`으로 `CHANGELOG.md`에서 tag의 version 절 본문을 뽑습니다. 절이 없거나, 본문이 비었거나, 제목이 날짜로 끝나지 않으면 여기서 실패합니다.
3. goreleaser가 뽑은 본문을 노트로 GitHub Release를 만들고, `.goreleaser.yaml` 설정대로 배포물을 게시합니다.

검사나 노트 추출에서 실패했으면, 고친 커밋을 `master`에 push한 뒤 원격 tag를 `git push --delete origin v<version>`으로 지우고, 고친 커밋에 tag를 다시 달아 push합니다.
