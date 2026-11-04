# 릴리스

- version은 git tag `v<X.Y.Z>`로만 정합니다.
- 저장소에는 version을 적는 파일이 없습니다.
- `v<major>` 브랜치에 push하면 `.github/workflows/ci.yml`이 3-OS 테스트를 실행하고, 모두 성공하면 같은 실행의 `release` job이 `.github/workflows/goreleaser.yml`을 불러 그 커밋을 배포합니다.
  - 배포는 그 커밋의 `CHANGELOG.md` 맨 위 version으로 annotated tag `v<X.Y.Z>`를 만들고 GitHub Release를 만듭니다. 릴리스 절차에서는 tag를 만들거나 push하지 않습니다.
  - 그 version의 GitHub Release가 이미 있으면 아무것도 만들거나 게시하지 않습니다.
- `ci`는 `master` push와 tag push에서는 실행하지 않습니다.

## 브랜치

- `feature/working`: 평소 작업하는 로컬 전용 브랜치입니다. push하지 않습니다. 릴리스가 끝나면 릴리스 커밋을 가리킵니다.
- `v<major>`(`v1`, `v2`, `v3` …): 그 major의 마지막 릴리스 커밋을 가리킵니다. 릴리스할 때 `feature/working`으로 fast-forward하고 그 위에 릴리스 커밋을 만들어 push합니다. major의 첫 릴리스에서 만들고, 이전 major의 브랜치는 옮기지 않습니다.
- `master`: 배포에 성공한 최신 릴리스 커밋을 가리킵니다. 배포가 성공한 뒤 `v<major>`로 fast-forward합니다.

- 브랜치는 모두 fast-forward로만 옮깁니다.
- 브랜치 `v<major>`와 tag `v<X.Y.Z>`는 이름이 비슷하므로, push할 때는 `refs/heads/v<major>`와 `refs/tags/v<X.Y.Z>`처럼 전체 이름으로 적습니다.
- 같은 이름의 브랜치와 tag는 만들지 않습니다.

## 릴리스 절차

- 아래 절차는 프로젝트 skill `/versionup [patch|minor|major]`(Codex에서는 `$versionup`)로 실행할 수 있습니다.
- skill은 `feature/working`에서 실행하며, 2단계, 5단계, 7단계 앞에서 사용자에게 확인을 받습니다.
- 확인을 거절하면 skill은 그 자리에서 멈추고, 남은 단계의 명령을 알려 줍니다.

1. 시작하기 전에 아래를 확인합니다. 하나라도 맞지 않으면 아무것도 바꾸지 않습니다.
   - 작업 트리가 깨끗합니다.
   - `git fetch --tags origin` 뒤에 로컬 `master`가 `origin/master`와 같습니다.
   - 마지막 릴리스 tag의 GitHub Release가 있습니다. 없으면 지난 배포가 끝나지 않은 것이므로 [배포 흐름](#배포-흐름)의 복구 절차를 먼저 따릅니다.
   - `master`, 그리고 있다면 `v<major>`와 `origin/v<major>`가 모두 `feature/working`의 조상입니다.
   - `go vet ./...`와 `go test ./...`가 통과합니다.
2. `v<major>`를 `feature/working`으로 옮깁니다. 이 명령은 `v<major>`가 없으면 새로 만들고, 있으면 fast-forward만 합니다.

   ```sh
   git fetch . refs/heads/feature/working:refs/heads/v<major>
   git switch v<major>
   ```

3. 저장소 루트 `CHANGELOG.md`에 `## [<version>] - <YYYY-MM-DD>` 절을 쓰고, 바뀐 내용을 영어로 적습니다. `## [Unreleased]` 절이 있으면 그 절을 새 version 절로 바꿉니다. 새 절의 본문(다음 `## ` 제목 앞까지)이 GitHub Release의 노트가 됩니다. `go run ./scripts/releasenotes <version>`이 성공하고 그 본문을 출력하면 배포도 같은 노트를 뽑습니다.
4. 이 변경을 `v<major>`에 커밋합니다. tag는 달지 않습니다.
5. `v<major>`를 push합니다. `ci`의 테스트가 모두 성공하면 배포가 시작됩니다.

   ```sh
   git push origin refs/heads/v<major>
   ```

6. 이 push의 `ci` 실행이 끝날 때까지 기다리고, 실행이 성공했으며 GitHub Release `v<version>`이 만들어졌는지 확인합니다. 실패했으면 `master`를 옮기지 않고 [배포 흐름](#배포-흐름)의 복구 절차를 따릅니다.

   ```sh
   gh run list --workflow ci --branch v<major> --event push --commit <릴리스 커밋> --limit 1 --json databaseId,status,conclusion,url
   gh run watch <databaseId> --exit-status
   gh release view v<version>
   ```

7. `master`를 릴리스 커밋으로 옮겨 push하고, 로컬 `feature/working`도 옮깁니다. `master` push에서는 `ci`가 실행되지 않습니다.

   ```sh
   git switch master
   git merge --ff-only v<major>
   git push origin refs/heads/master
   git switch feature/working
   git merge --ff-only v<major>
   ```

### major를 올릴 때

- Go는 major 2 이상인 version의 모듈을 `/v<major>` 접미사가 붙은 경로로 찾습니다.
- major를 올리기 전에 `go.mod`의 모듈 경로와 그 경로를 쓰는 import, `.goreleaser.yaml`의 ldflags, README의 `go install` 명령을 새 major로 바꿔 먼저 커밋합니다.
- versionup skill은 모듈 경로가 새 version의 major와 맞지 않으면 아무것도 바꾸지 않고 멈춥니다.
- 새 major의 첫 릴리스에서는 2단계가 `v<major>` 브랜치를 새로 만듭니다.

## 배포 흐름

- `v<major>` push의 `ci`에서 `test` job이 모든 OS에서 성공해야 `release` job이 시작합니다. 테스트가 하나라도 실패하면 아무것도 게시하지 않습니다.
- `release` job은 `go vet`과 `go test`를 다시 실행하지 않고, push한 커밋을 게시합니다.
- 같은 `v<major>`에 다시 push해도 진행 중인 실행은 취소되지 않고, 새 실행은 앞 실행이 끝난 뒤 시작합니다.

`release` job이 부르는 `goreleaser.yml`은 아래 순서로 실행합니다.

1. `CHANGELOG.md`에서 맨 위 `## [X.Y.Z]` 절의 version을 읽습니다. 그 major가 push한 브랜치의 major와 다르면 실패합니다. GitHub Release `v<version>`이 이미 있으면 여기서 끝냅니다.
2. `go run ./scripts/releasenotes <version>`으로 그 version 절의 본문을 뽑습니다. 본문이 비었거나 제목이 날짜로 끝나지 않으면 여기서 실패합니다.
3. push한 커밋에 annotated tag `v<version>`을 달아 push합니다.
   - 이전 실행이 게시하기 전에 실패해 tag가 다른 커밋에 남아 있으면, tag를 이 커밋으로 옮깁니다.
4. goreleaser가 뽑은 본문을 노트로 GitHub Release를 만들고, `.goreleaser.yaml` 설정대로 배포물을 게시합니다.

테스트나 1–3단계에서 실패했으면 아래 순서로 다시 배포합니다.

1. `feature/working`을 릴리스 커밋으로 옮긴 뒤, 그 위에서 문제를 고쳐 커밋합니다. `CHANGELOG.md`의 version은 그대로 둡니다.

   ```sh
   git switch feature/working
   git merge --ff-only v<major>
   ```

2. `v<major>`를 고친 커밋으로 fast-forward해 push합니다. 테스트가 성공하면 배포가 다시 시작하고, 고치기 전 커밋에 남은 tag를 고친 커밋으로 옮깁니다.

   ```sh
   git fetch . refs/heads/feature/working:refs/heads/v<major>
   git push origin refs/heads/v<major>
   ```

3. 릴리스 절차 6단계와 7단계를 실행합니다.

- 커밋을 고치지 않아도 되는 실패(네트워크 오류 등)는 GitHub Actions에서 실패한 `ci` 실행의 실패한 job을 다시 실행합니다.
- 4단계에서 GitHub Release가 만들어진 뒤 실패하면, 다시 실행해도 1단계에서 끝납니다. 이때는 `gh release delete v<version>`으로 Release를 지운 뒤 실패한 job을 다시 실행합니다.
