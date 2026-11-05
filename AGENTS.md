# 언어

- 프로젝트의 기본 언어는 한국어다. 코드 주석, 커밋 메시지, 내부 문서는 한국어로 쓴다.
- 외부에 보이거나 배포되는 부분만 영어로 쓴다. CLI 출력·로그·오류 메시지, 릴리스 노트, `CONTRIBUTING.md`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/`의 issue template이 여기에 해당한다.
- 한국어판(`.ko.md`)과 영어판(`.md`)이 함께 있는 문서는 한쪽을 고치면 다른 쪽도 같이 고쳐, 두 판의 절 구성과 사실을 같게 유지한다.

# 문서 형식

- README, USAGE, CHANGELOG와 `docs/`, `e2e/` 아래 문서의 본문은 서술형 개조식으로 쓴다. 본문을 bullet 목록으로 쓰고, 항목마다 서술어로 끝나는 완결된 문장을 쓴다.
- 한 항목에는 사실 하나를 담고, 그 사실을 보충하는 세부 사항은 하위 bullet로 쓴다.
- 한국어 항목은 그 문서가 이미 쓰는 `합니다`체나 `한다`체로 끝내고, `~함`, `~임` 같은 명사형으로 끝내지 않는다.
- 순서가 있는 절차는 번호 목록으로 쓴다. 코드 블록, 표, 다이어그램, 인용 블록은 목록으로 바꾸지 않는다.
- 목록, 코드 블록, 표 바로 앞에서 그것을 소개하는 한 문장(예: "Then install the cask:")은 문단으로 둔다.

# 테스트 파일 이름

- 테스트 파일 `<이름>_test.go`는 같은 폴더에 있는 본체 파일 `<이름>.go`를 시험한다. `runner_watch_target_rename_test.go`처럼 본체 파일 이름에 주제를 덧붙인 테스트 파일은 만들지 않는다.
- `internal/app/service`에서 runner 전체를 띄워야만 확인되는 시나리오는 단계별 본체 파일의 테스트에 두지 않는다. 감시 대상 하나의 시나리오는 `runner_watch_target_test.go`에, 전체 runner의 시나리오는 `runner_test.go`에 둔다.
- 저장소 루트에서 문서와 코드가 같은지 보는 테스트는 예외라서, 같은 이름의 본체 파일이 없어도 된다.
- 한 패키지의 여러 테스트 파일이 함께 쓰는 helper는 그 패키지의 `helpers_test.go`에 둔다. 이 파일은 같은 이름의 본체 파일이 없어도 된다.

# 로그 레벨

- 로그 레벨(`-l`/`--log-level`)은 개발과 유지보수에 쓰는 설정이라서, 사용자가 신경 쓰지 않아도 되게 한다.
- `--help`는 이 flag를 `Maintenance Flags:` 절에 따로 보여 준다. 개발과 유지보수에 쓰는 flag를 새로 더하면 `cmd/cmd.go`의 `maintenanceFlags`에 정의한다.

# 검증

- Windows 동작을 확인해야 하면, Windows 환경이 없다고 판단하기 전에 `/Applications/UTM.app/Contents/MacOS/utmctl list`로 UTM VM을 찾는다. VM이 있으면 [UTM Windows VM에서 E2E 실행](e2e/docs/windows-utm.ko.md)을 읽고 `make e2e-windows-all ARGS="-vm=<VM 이름>"`으로 게스트에서 테스트를 실행한다. `./e2e/scenarios` 밖의 패키지를 함께 실행하는 방법과 이 VM으로 대신할 수 없는 Windows 검증도 그 문서에 있다.
- 심볼릭 링크를 만드는 테스트는 링크를 만들 수 없으면 SKIP한다. Windows 실행 출력에서 해당 테스트가 `--- SKIP` 없이 `--- PASS`인지 확인한다.

# CHANGELOG와 릴리스

- `CHANGELOG.md`는 `versionup` skill로 버전업할 때만 고친다. 기능을 추가하거나 버그를 고치는 커밋에서는 CHANGELOG에 항목을 적지 않고, 버전업할 때 지난 릴리스 tag 이후의 커밋을 모아 새 version 절에 한 번에 쓴다. `## [Unreleased]` 절이 남아 있으면 skill이 그 절을 새 version 절의 초안으로 쓴다.
- 릴리스할 때는 [릴리스](docs/release.md) 문서를 읽는다.

# 프로젝트 skill

- 프로젝트 skill은 `.claude/skills/<이름>/SKILL.md`에 둔다. 저장소 루트의 `.agents/skills`는 `../.claude/skills`를 가리키는 심볼릭 링크라서, Claude Code와 Codex가 같은 skill을 읽는다.
- skill 본문에는 특정 에이전트의 도구 이름을 쓰지 않고, 호출 방법은 Claude Code(`/<이름>`)와 Codex(`$<이름>`)를 함께 적는다.
- 사용자가 직접 불러야 하는 skill은 frontmatter에 `disable-model-invocation: true`를 두고, Codex용으로 skill 폴더의 `agents/openai.yaml`에 `policy.allow_implicit_invocation: false`를 둔다.
