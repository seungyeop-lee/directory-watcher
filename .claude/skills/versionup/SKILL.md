---
name: versionup
description: 사용자가 `/versionup`(Codex에서는 `$versionup`)으로 directory-watcher의 다음 릴리스를 만들어 달라고 직접 요청할 때만 사용합니다. 올릴 단계로 patch, minor, major 중 하나를 받거나 생략합니다.
argument-hint: "[patch|minor|major]"
disable-model-invocation: true
---

# versionup

`feature/working`의 커밋을 `master`로 옮기고, `master`에 `CHANGELOG.md`의 새 version 절을 담은 릴리스 커밋 하나를 만든 뒤, 그 커밋에 annotated tag `v<새 version>`을 달고 `v<새 version의 major>` 브랜치(아래에서 `v<major>`)를 그 커밋으로 옮깁니다. 이어서 `master`, `v<major>`, tag를 push하고 `feature/working`을 릴리스 커밋에 맞춥니다. tag가 push되면 `.github/workflows/goreleaser.yml`이 그 tag로 배포하고 GitHub Release를 만듭니다. 릴리스 규칙은 `docs/release.md`와 `goreleaser.yml`에 있고, 이 skill은 `docs/release.md` "릴리스 절차" 절을 대신 실행합니다. 시작하기 전에 그 둘을 읽고, 이 skill이 그 둘과 어긋나는 곳을 발견하면 skill에 맞추지 말고 멈춘 뒤 어긋난 곳을 사용자에게 알립니다.

**브랜치는 모두 fast-forward로만 옮기고, push와 브랜치 이동은 아래 세 확인에서 사용자가 동의한 뒤에만 합니다.**

- 확인 ①(5단계): `master`를 `feature/working`으로 옮기기 전
- 확인 ②(9단계): push하기 전
- 확인 ③(10단계): `feature/working`을 릴리스 커밋에 맞추기 전

각 확인에서는 그 단계가 정한 내용을 보여 주고, 진행할지 사용자에게 묻고 답을 기다립니다. 사용자가 분명히 동의했을 때만 진행합니다. 거절하거나 답이 분명하지 않으면 그 자리에서 멈추고, 그 단계의 "거절하면"대로 그때까지 한 일과 남은 단계의 명령을 알립니다. skill을 부를 때 함께 준 말은 확인에 대한 답으로 보지 않습니다.

모든 명령은 저장소 루트에서 실행합니다.

## 1. 인수 읽기

올릴 단계는 사용자가 skill을 부를 때 함께 준 말에서 읽습니다. Claude Code에서 `/versionup minor`로 부르면 skill 본문 끝에 `ARGUMENTS: minor`가 붙고, Codex에서는 `$versionup minor`처럼 사용자의 메시지에 들어 있습니다.

- `patch`, `minor`, `major` 중 하나이면 그 단계를 씁니다.
- 아무것도 없으면 3단계에서 커밋을 보고 단계를 고릅니다.
- 그 밖의 값이면 아무것도 바꾸지 않고 사용법 `/versionup [patch|minor|major]`(Codex에서는 `$versionup [patch|minor|major]`)를 알린 뒤 멈춥니다.

## 2. 실행 전 확인

아래 조건 중 하나라도 맞지 않으면 아무것도 바꾸지 않고, 맞지 않는 조건을 알린 뒤 멈춥니다.

1. `git branch --show-current`가 `feature/working`이나 `master`를 출력합니다. 이 브랜치를 실행 브랜치라고 부릅니다. 릴리스 커밋은 지금 `HEAD` 위에 만들어집니다.
2. `git status --porcelain`이 아무것도 출력하지 않습니다.
3. `git fetch --tags origin`이 성공합니다. 실패하면 출력을 보여 줍니다. 원격 브랜치와 tag를 받아 오지 못하면 아래 조건과 4단계의 충돌 여부를 확인할 수 없습니다.
4. `git rev-parse master`와 `git rev-parse origin/master`가 같은 커밋을 출력합니다. 다르면 로컬 `master`에 push하지 않은 커밋이 있거나 원격보다 뒤처진 것이므로, 사용자가 먼저 맞춰야 합니다.
5. `git merge-base --is-ancestor master HEAD`가 성공합니다. 실패하면 `master`를 실행 브랜치로 fast-forward할 수 없습니다.
6. 로컬 `feature/working`이 있으면(`git rev-parse -q --verify refs/heads/feature/working`이 성공하면) 아래 둘이 성공합니다. 10단계에서 `feature/working`과 `origin/feature/working`을 릴리스 커밋으로 fast-forward하기 때문입니다.
   - `git merge-base --is-ancestor feature/working HEAD`
   - `origin/feature/working`이 있으면 `git merge-base --is-ancestor origin/feature/working HEAD`
7. `git describe --tags --abbrev=0`이 출력한 마지막 릴리스 tag가 `v<X.Y.Z>` 형식이고 prerelease 부분(`-` 뒤)이 없습니다. 이 tag에서 `v`를 뺀 값이 지금 version입니다.
8. `go vet ./...`와 `go test ./...`가 통과합니다. 1–7이 모두 맞을 때 마지막으로 실행합니다. tag workflow도 게시 전에 같은 검사를 하므로, 여기서 실패하면 그대로 배포할 수 없습니다. 실패하면 실패한 검사와 출력을 알리고, 아직 아무것도 바꾸지 않았으므로 문제를 고쳐 커밋한 뒤 skill을 바로 다시 실행하면 된다고 안내합니다.

## 3. 올릴 단계 정하기

마지막 릴리스 이후의 커밋과 바뀐 파일을 읽습니다.

```sh
git log --format='--- %h %s%n%b' --name-only v<지금 version>..HEAD
```

`CHANGELOG.md`에 `## [Unreleased]` 절이 있으면 그 절도 읽습니다. 그 절의 항목은 6단계에서 새 version 절의 초안이 됩니다.

사용자에게 보이는 변경은 배포물이나 설치 방법을 바꾸는 커밋입니다. 아래 두 조건을 모두 만족하는 커밋만 셉니다. commit type(`chore`, `test` 등)이 아니라 커밋이 바꾼 내용으로 판단합니다.

- 배포물이나 설치 방법을 정하는 파일을 바꿨습니다.
  - 배포 바이너리에 들어가는 코드: `main.go`, `cmd/`, `internal/`의 테스트가 아닌 파일(`*_test.go`와 `testdata/`는 빼고). 의존성과 모듈 경로를 정하는 `go.mod`, `go.sum`도 여기에 듭니다.
  - 설정 형식: `directory-watcher.schema.json`
  - Docker 이미지: `Dockerfile`, `entrypoint.sh`
  - 배포 채널과 배포물 이름: `.goreleaser.yaml`, `README.md`의 "Installation" 절
- 사용자가 겪는 동작을 바꿨습니다: 실행 동작, CLI 출력과 로그, 설정 형식, 설치 명령과 `go install` 경로, 배포 채널(GitHub Release, ghcr Docker 이미지, Homebrew cask, scoop)과 그 이름이나 tag. 주석이나 테스트만 고친 커밋, 동작을 바꾸지 않는 리팩터링, 진단용으로 넣었다가 같은 릴리스 안에서 되돌린 코드는 세지 않습니다. 설치 절을 바꿨어도 설치 방법이 그대로인 커밋(예시만 더한 커밋 등)도 세지 않습니다. 제목만으로 판단할 수 없으면 `git show <hash>`로 diff를 읽습니다.

`e2e/`, `test/`, `scripts/`, `.github/`, `.claude/`, `.agents/`, `docs/`, `Makefile`, `config.example.yml`, `AGENTS.md`, 그리고 설치 안내가 아닌 README와 USAGE 같은 문서만 바꾼 커밋은 내부 변경이므로 세지 않습니다.

센 커밋과 `[Unreleased]` 절의 항목마다 단계를 정합니다.

- major: breaking change. 기존 설정, hook 명령, 설치 명령이나 경로가 다르게 동작하거나, 기존 사용자가 무언가를 바꿔야 계속 쓸 수 있는 변경입니다. 제목이 `type!:` 또는 `type(scope)!:`이거나 본문에 `BREAKING CHANGE`가 있는 커밋, `go.mod`의 모듈 경로 major를 올린 커밋도 여기에 듭니다.
- minor: 새 설정이나 기능을 더한 변경
- patch: 그 밖의 변경

이 가운데 가장 높은 단계를 고릅니다.

- 센 커밋도 `[Unreleased]` 항목도 없으면 릴리스할 변경이 없다고 알리고 멈춥니다. 인수를 받았어도 멈춥니다. CHANGELOG 절의 본문이 비면 tag workflow가 게시 전에 실패합니다.
- 인수 없이 실행했으면 고른 단계와 그 근거가 된 커밋(hash와 제목)이나 항목을 사용자에게 알립니다.
- 인수를 받았는데 판단한 단계와 다르면, 그 차이를 알리되 인수의 단계를 씁니다.

## 4. 새 version 계산과 확인

지금 version `X.Y.Z`에 단계를 적용합니다: patch는 `X.Y.(Z+1)`, minor는 `X.(Y+1).0`, major는 `(X+1).0.0`입니다. 새 version의 major가 `N`이면 `v<major>`는 `vN` 브랜치입니다.

아래 조건 중 하나라도 맞지 않으면 아무것도 바꾸지 않고 멈춥니다.

1. `go list -m`이 출력한 모듈 경로가 `/v<새 version의 major>`로 끝납니다. Go는 major 2 이상인 version의 모듈을 그 접미사가 붙은 경로로 찾으므로, 맞지 않으면 `go install`로 새 version을 설치할 수 없습니다. 멈출 때 지금 모듈 경로와 필요한 접미사를 알리고, 둘 중 하나를 고르게 안내합니다.
   - major로 올리려면 `go.mod`의 모듈 경로와 그 경로를 쓰는 import, `.goreleaser.yaml`의 ldflags, README의 `go install` 명령을 새 major로 바꿔 먼저 커밋합니다.
   - major로 올리지 않으려면 모듈 경로가 지금 major와 맞는지 확인하고, 모듈 경로를 이미 올렸다면 `major`로 다시 실행합니다.
2. `git rev-parse -q --verify refs/tags/v<새 version>`이 실패합니다. 성공하면 그 version은 이미 tag가 있으므로 멈춥니다. 2단계에서 원격 tag를 받아 왔으므로 이 확인은 원격 tag도 포함합니다.
3. `refs/heads/v<major>`와 `refs/remotes/origin/v<major>` 가운데 있는 것(`git rev-parse -q --verify <ref>`가 성공하는 것)마다 `git merge-base --is-ancestor <ref> HEAD`가 성공합니다. 릴리스 커밋은 `HEAD` 위에 만들어지므로, 이 조건이 맞아야 `v<major>`를 릴리스 커밋으로 fast-forward할 수 있습니다. 둘 다 없으면 이번이 그 major의 첫 릴리스이고, 8단계에서 `v<major>`를 새로 만듭니다.
4. `git for-each-ref refs/tags/v<major> refs/heads/v<새 version> refs/remotes/origin/v<새 version>`이 아무것도 출력하지 않습니다. 브랜치 `v<major>`와 같은 이름의 tag나, tag `v<새 version>`과 같은 이름의 브랜치가 있으면 같은 이름이 브랜치와 tag를 함께 가리키게 되므로 멈춥니다.

## 5. 확인 ①: `master`로 옮기기

실행 브랜치가 `master`이면 이 단계를 건너뜁니다. 실행 브랜치가 `feature/working`이고 `git rev-parse master`와 `git rev-parse HEAD`가 같으면, 옮길 커밋이 없으므로 확인 없이 `git switch master`만 실행합니다.

그 밖의 경우 `master`로 옮겨질 커밋 목록과 새 version을 보여 주고, `master`를 `feature/working`으로 fast-forward할지 묻습니다.

```sh
git log --oneline master..feature/working
```

동의하면 실행합니다.

```sh
git switch master
git merge --ff-only feature/working
```

**거절하면:** 아무것도 바꾸지 않았다고 알립니다. 남은 단계는 위 두 명령으로 `master`를 옮기는 일부터 시작하는 `docs/release.md` "릴리스 절차"의 2단계 이후이며, skill을 다시 실행해도 된다고 안내합니다.

## 6. CHANGELOG 절 쓰기

저장소 루트 `CHANGELOG.md`에 새 version의 절을 씁니다. 날짜는 `date +%F`의 오늘 날짜이고, 제목은 `## [<새 version>] - <오늘 날짜>`입니다.

- `## [Unreleased]` 절이 있으면 그 절을 새 version 절로 바꿉니다. 초안의 항목은 그대로 두고, 3단계에서 센 커밋 가운데 초안에 없는 변경만 더한 뒤, 절 제목을 새 version의 제목으로 바꿉니다. 다만 초안 항목이 지난 릴리스가 아니라 릴리스되지 않은 중간 상태와 비교해 쓰였으면, 지난 릴리스의 사용자가 겪는 변화로 고치고 11단계에서 고친 항목을 알립니다.
- 없으면 머리말 뒤, 가장 최근 version 절(첫 번째 `## [` 제목) 위에 새 절을 추가합니다.

본문은 영어로, 파일 머리말의 Keep a Changelog 1.1.0 형식을 따라 씁니다. 이 본문이 GitHub Release의 노트가 됩니다.

- 3단계에서 센 커밋만 적습니다. 내부 변경은 적지 않습니다.
- 변경을 `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security` 소제목(`### `) 아래 이 순서로 나누고, 항목이 없는 소제목은 넣지 않습니다. 초안에 이미 있는 소제목은 그대로 쓰고, 새 항목은 알맞은 소제목에 넣습니다.
- 커밋 메시지는 한국어이므로 그대로 옮기지 않습니다. 제목만으로 무엇이 바뀌었는지 알 수 없으면 `git show <hash>`로 diff를 읽고, 사용자가 CLI, 설정, 설치에서 겪는 변화를 영어 문장으로 다시 씁니다. 설정 키, 옵션, 절 이름은 `USAGE.md`와 `README.md`의 영어 표기를 씁니다.
- 기존 사용자가 해야 할 일이 있는 변경(설치 명령, 설정 문법 등)은 무엇을 바꿔야 하는지 함께 적습니다.
- 같은 기능을 고친 여러 커밋은 항목 하나로 합칩니다. 같은 릴리스 안에서 다른 커밋이 대체하거나 되돌린 변경은 최종 결과만 적습니다. 커밋 hash와 bd 이슈 ID는 적지 않습니다.
- 항목은 `AGENTS.md` "문서 형식" 절의 서술형 개조식으로 씁니다. 항목마다 서술어로 끝나는 완결된 문장을 쓰고, 한 항목에는 변경 하나를 담으며, 세부 사항은 하위 bullet로 씁니다.
- 문장 형식은 초안이나 이전 version 절을 따릅니다.

## 7. 검사

tag workflow가 릴리스 노트를 뽑는 명령으로 새 절을 확인합니다.

```sh
go run ./scripts/releasenotes <새 version>
```

이 명령은 `CHANGELOG.md`에 새 version 절이 없거나, 본문이 비었거나, 제목이 날짜로 끝나지 않으면 실패합니다. 성공하면 출력이 6단계에서 쓴 본문과 같은지 확인합니다.

실패하면 원인은 6단계에서 쓴 내용이므로, 실패 출력을 보고 6단계의 규칙대로 절을 고친 뒤 다시 실행합니다. 한 번만 고칩니다. 고친 뒤에도 실패하면 커밋하지 않고, 바꾼 파일을 그대로 둔 채 실패 출력을 알리고 멈춥니다. 이때 skill을 다시 실행하려면 `git restore CHANGELOG.md`로 파일을 먼저 되돌려야 하고, 5단계에서 `master`를 옮겼다면 `git switch feature/working`과 `git branch -f master origin/master`로 `master`도 되돌려야 한다고 함께 알립니다. 되돌리지 않으면 2단계의 4번 조건에서 멈춥니다.

## 8. 커밋, tag, `v<major>`

`CHANGELOG.md`만 stage해 커밋 하나로 만들고, 그 커밋에 annotated tag를 답니다. git hook을 건너뛰지 않습니다(`--no-verify` 금지). 이어서 로컬 `v<major>`를 그 커밋으로 옮깁니다. 이 단계는 확인 없이 진행합니다.

```sh
git add CHANGELOG.md
git commit -m "chore(release): <새 version> 릴리스의 CHANGELOG 절을 추가함"
git tag -a v<새 version> -m "v<새 version>"
git fetch . refs/heads/master:refs/heads/v<major>
```

마지막 명령은 로컬 `v<major>`를 `master`로 fast-forward하고, 로컬 `v<major>`가 없으면 `master`에 새로 만듭니다. fast-forward가 아니면 git이 `rejected`로 거절합니다. 거절되면 출력을 알리고 멈춥니다. 이때 커밋과 tag는 로컬에 남아 있고 아무것도 push하지 않았다고 함께 알립니다.

## 9. 확인 ②: push

아래 내용을 보여 줍니다.

- 새 CHANGELOG 절: 7단계의 `go run ./scripts/releasenotes <새 version>` 출력
- 릴리스 커밋의 hash와 제목, tag `v<새 version>`, `v<major>`가 가리키는 커밋: `git log -1 --format='%h %s' refs/heads/v<major>`
- 실행할 명령: `git push --atomic origin refs/heads/master refs/heads/v<major> refs/tags/v<새 version>`
- push하면 `.github/workflows/goreleaser.yml`이 tag `v<새 version>`으로 배포를 시작해 GitHub Release, ghcr Docker 이미지, Homebrew cask, scoop에 게시한다는 것

push할지 묻고, 동의하면 실행합니다.

```sh
git push --atomic origin refs/heads/master refs/heads/v<major> refs/tags/v<새 version>
```

ref를 `refs/heads/`와 `refs/tags/`로 적어 브랜치와 tag를 구분하고, `--atomic`으로 셋 중 하나라도 거절되면 아무것도 바꾸지 않습니다. push가 실패하면 출력을 알리고 멈춥니다. 이때 원격은 바뀌지 않았고, 로컬에는 커밋, tag, `v<major>`가 남아 있다고 함께 알립니다.

**거절하면:** 로컬 `master`에 릴리스 커밋을 만들고 tag `v<새 version>`을 달았으며 로컬 `v<major>`를 옮겼고, 아무것도 push하지 않았다고 알립니다. 남은 단계의 명령을 함께 알립니다.

```sh
git push --atomic origin refs/heads/master refs/heads/v<major> refs/tags/v<새 version>
git switch feature/working
git merge --ff-only master
git push origin refs/heads/feature/working
```

뒤의 세 줄은 로컬 `feature/working`이 있을 때만 알립니다. push하기 전에 커밋을 고치면 tag와 `v<major>`는 고치기 전 커밋에 남는다는 것도 알립니다. 이때는 `master`에서 `git tag -d v<새 version>`으로 tag를 지우고 `git branch -f v<major> master`로 `v<major>`를 고친 커밋으로 옮긴 뒤, 8단계의 `git tag` 명령으로 tag를 다시 답니다.

## 10. 확인 ③: `feature/working` 맞추기

로컬 `feature/working`이 없으면 이 단계를 건너뜁니다. 5단계를 건너뛰었어도 로컬 `feature/working`이 있으면 이 단계를 실행합니다.

`feature/working`을 `master`(릴리스 커밋)로 fast-forward하고 `origin/feature/working`에 push할지 묻습니다. 실행할 명령을 함께 보여 주고, 동의하면 실행합니다.

```sh
git switch feature/working
git merge --ff-only master
git push origin refs/heads/feature/working
```

push가 실패하면 출력을 알리고 멈춥니다. 릴리스는 이미 push되었고, 로컬 `feature/working`만 옮겨졌다고 함께 알립니다.

**거절하면:** 릴리스는 push되었고 `feature/working`은 옮기지 않았다고 알립니다. 지금 브랜치는 `master`입니다. 남은 단계는 위 세 명령입니다.

## 11. 안내

사용자에게 아래 내용을 알립니다.

- 커밋 hash와 제목, 만든 tag, `v<major>`가 가리키는 커밋, 지금 version과 새 version, 고른 단계와 그 근거가 된 커밋이나 항목, 6단계에서 고친 초안 항목
- push한 ref와 `feature/working`을 맞췄는지, 지금 브랜치
- 배포 진행은 GitHub Actions의 `goreleaser` workflow에서 확인합니다. 검사나 노트 추출에서 실패하면 `docs/release.md` "배포 흐름" 절의 복구 절차를 따릅니다.
