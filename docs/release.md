# 릴리스

version은 git tag `v<X.Y.Z>`로만 정합니다. 저장소에는 version을 적는 파일이 없습니다. `v`로 시작하는 tag를 push하면 `.github/workflows/goreleaser.yml`이 그 tag의 커밋을 배포하고 GitHub Release를 만듭니다.

## 브랜치

- `feature/working`: 평소 작업하는 브랜치입니다. 릴리스가 끝나면 릴리스 커밋을 가리킵니다.
- `master`: 최신 릴리스 커밋을 가리킵니다. 릴리스할 때 `feature/working`으로 fast-forward합니다.
- `v<major>`(`v1`, `v2`, `v3` …): 그 major의 마지막 릴리스 커밋을 가리킵니다. major의 첫 릴리스에서 만들고, 같은 major를 릴리스할 때마다 새 릴리스 커밋으로 fast-forward합니다. 이전 major의 브랜치는 옮기지 않습니다.

브랜치는 모두 fast-forward로만 옮깁니다. 브랜치 `v<major>`와 tag `v<X.Y.Z>`는 이름이 비슷하므로, push할 때는 `refs/heads/v<major>`와 `refs/tags/v<X.Y.Z>`처럼 전체 이름으로 적습니다. 같은 이름의 브랜치와 tag는 만들지 않습니다.

## 릴리스 절차

아래 절차는 프로젝트 skill `/versionup [patch|minor|major]`(Codex에서는 `$versionup`)로 실행할 수 있습니다. skill은 `feature/working`이나 `master`에서 실행하며, 2단계(`master`에서 실행하면 건너뜀), 6단계, 7단계 앞에서 사용자에게 확인을 받습니다. 확인을 거절하면 skill은 그 자리에서 멈추고, 남은 단계의 명령을 알려 줍니다.

1. 시작하기 전에 아래를 확인합니다. 하나라도 맞지 않으면 아무것도 바꾸지 않습니다.
   - 작업 트리가 깨끗합니다.
   - `git fetch --tags origin` 뒤에 로컬 `master`가 `origin/master`와 같습니다.
   - `master`, `feature/working`, `origin/feature/working`, 그리고 있다면 `v<major>`와 `origin/v<major>`가 모두 릴리스할 커밋의 조상입니다.
   - `go vet ./...`와 `go test ./...`가 통과합니다.
2. `master`를 `feature/working`으로 옮깁니다.

   ```sh
   git switch master
   git merge --ff-only feature/working
   ```

3. 저장소 루트 `CHANGELOG.md`에 `## [<version>] - <YYYY-MM-DD>` 절을 쓰고, 바뀐 내용을 영어로 적습니다. `## [Unreleased]` 절이 있으면 그 절을 새 version 절로 바꿉니다. 새 절의 본문(다음 `## ` 제목 앞까지)이 GitHub Release의 노트가 됩니다. `go run ./scripts/releasenotes <version>`이 성공하고 그 본문을 출력하면 tag workflow도 같은 노트를 뽑습니다.
4. 이 변경을 `master`에 커밋하고, 그 커밋에 annotated tag를 답니다.

   ```sh
   git tag -a v<version> -m "v<version>"
   ```

5. 로컬 `v<major>`를 릴리스 커밋으로 옮깁니다. 이 명령은 `v<major>`가 없으면 새로 만들고, 있으면 fast-forward만 합니다.

   ```sh
   git fetch . refs/heads/master:refs/heads/v<major>
   ```

6. `master`, `v<major>`, tag를 함께 push합니다. tag가 push되면 배포가 시작됩니다. `--atomic`이라서 하나라도 거절되면 원격은 바뀌지 않습니다.

   ```sh
   git push --atomic origin refs/heads/master refs/heads/v<major> refs/tags/v<version>
   ```

7. `feature/working`을 릴리스 커밋으로 옮기고 push합니다.

   ```sh
   git switch feature/working
   git merge --ff-only master
   git push origin refs/heads/feature/working
   ```

push하기 전에 릴리스 커밋을 고치면 tag와 `v<major>`는 고치기 전 커밋에 남습니다. `master`에서 `git tag -d v<version>`으로 tag를 지우고 `git branch -f v<major> master`로 `v<major>`를 고친 커밋으로 옮긴 뒤, 4단계의 명령으로 tag를 다시 답니다.

### major를 올릴 때

Go는 major 2 이상인 version의 모듈을 `/v<major>` 접미사가 붙은 경로로 찾습니다. major를 올리기 전에 `go.mod`의 모듈 경로와 그 경로를 쓰는 import, `.goreleaser.yaml`의 ldflags, README의 `go install` 명령을 새 major로 바꿔 먼저 커밋합니다. versionup skill은 모듈 경로가 새 version의 major와 맞지 않으면 아무것도 바꾸지 않고 멈춥니다. 새 major의 첫 릴리스에서는 5단계가 `v<major>` 브랜치를 새로 만듭니다.

## 배포 흐름

tag를 push하면 `goreleaser.yml`이 아래 순서로 실행합니다. 검사나 노트 추출이 실패하면 아무것도 게시하지 않습니다.

1. `go vet ./...`와 `go test ./...`로 검사합니다.
2. `go run ./scripts/releasenotes <version>`으로 `CHANGELOG.md`에서 tag의 version 절 본문을 뽑습니다. 절이 없거나, 본문이 비었거나, 제목이 날짜로 끝나지 않으면 여기서 실패합니다.
3. goreleaser가 뽑은 본문을 노트로 GitHub Release를 만들고, `.goreleaser.yaml` 설정대로 배포물을 게시합니다.

검사나 노트 추출에서 실패했으면 아래 순서로 다시 배포합니다.

1. `master`에서 문제를 고쳐 커밋합니다.
2. 로컬 tag를 고친 커밋으로 옮기고, `v<major>`를 고친 커밋으로 fast-forward합니다.

   ```sh
   git tag -d v<version>
   git tag -a v<version> -m "v<version>"
   git fetch . refs/heads/master:refs/heads/v<major>
   ```

3. 원격 tag를 지운 뒤, 릴리스 절차 6단계와 7단계를 다시 실행합니다.

   ```sh
   git push --delete origin refs/tags/v<version>
   ```
