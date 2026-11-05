# Contributing to Directory Watcher

Bug reports and feature ideas are welcome as GitHub issues. Pull requests are not accepted: the repository settings allow only collaborators to open them.

## Issues

- To report a bug, open a [Bug report](https://github.com/seungyeop-lee/directory-watcher/issues/new?template=bug_report.yml) issue.
- To suggest a new feature or a change, open a [Feature request](https://github.com/seungyeop-lee/directory-watcher/issues/new?template=feature_request.yml) issue.
- To report a security vulnerability, follow [SECURITY.md](SECURITY.md). Do not open a public issue for it.

You can write issues in English or Korean.

## Language

Code comments, commit messages, [`AGENTS.md`](AGENTS.md), [`docs/release.md`](docs/release.md), and the project skills in `.claude/skills/` are written in Korean. `AGENTS.md` describes the language, document format, and test file naming rules. Read it, with a translator if needed, before changing code or documents.

Documents that have both an English `.md` and a Korean `.ko.md` version, such as `README.md` and `README.ko.md`, are changed together so that both keep the same sections and facts.

## Development setup

You need:

- Go, at or above the version in the `go` line of `go.mod`.
- `make` and `sh`, to run the `make` targets below.
- Only to run the E2E scenarios in a Linux container or on Windows, the Docker CLI with a running Linux Docker engine for `make e2e-linux`, and a UTM Windows VM on a Mac for `make e2e-windows-all`. See [Setup and run](e2e/README.md#setup-and-run) in the E2E guide.

To run the watcher from source with [`config.example.yml`](config.example.yml), run this at the repository root:

```sh
make run
```

`make rund` does the same with DEBUG logs. Both first create the watched folders that `config.example.yml` lists.

## Verification

Run these at the repository root:

| Command | When to run | What it runs |
|---|---|---|
| `make test-fast` | While you change code | `go test -short ./...`: unit tests and the root tests that check documents against code. It skips the E2E tests that launch the watcher, and packages whose files did not change report `(cached)` |
| `go build ./...` | Before each commit | Builds every package |
| `go vet ./...` | Before each commit | Runs `go vet` on every package |
| `go test -count=1 -race ./...` | Before each commit | Unit tests, the root tests that check documents against code, and the E2E scenarios in `e2e/scenarios` on the current OS, with the race detector. `make test` runs the same command |
| `make smoke-example` | You changed `config.example.yml`, or how the watcher starts, runs hooks, or stops | Runs the watcher with `config.example.yml` and checks that watching starts, that creating a file runs the `onChange` hooks, and that SIGTERM runs the `onFinishWatch` hooks and exits with code 0 |
| `make e2e-linux` and `make e2e-windows-all` | You changed code that behaves differently per OS, and before a release | Runs the E2E scenarios in a Linux container and on a UTM Windows VM |

- Do not drop `-count=1` from the commit check. The E2E tests build the watcher in a separate `go build`, so the Go test cache does not see changes to the watcher source and can report an old result.
- Code that behaves differently per OS includes these paths:
  - `internal/process/`: process groups, signals, and Windows Job Objects.
  - `internal/app/domain/command/shell_*.go`: the default shell and how hook commands are passed to it.
  - `cmd/console_*.go`: console output encoding on Windows.
  - `internal/app/service/runner_watch_target.go`, `internal/app/service/runner_watch_target_tree.go`, and `internal/app/service/file_id_*.go`: handling of the fsnotify backend of each OS and file identity.
  - `e2e/harness/signal_*.go`: how the E2E harness stops the watcher and observes processes.
  - Any other code that branches on `runtime.GOOS` or has an OS-specific file name suffix or build constraint.

CI runs `go build ./...`, `go vet ./...`, and `go test -count=1 -race ./...` on Linux, Windows, and macOS, and `make smoke-example` on Linux. On Windows it also runs the extra E2E checks listed in [CI steps](e2e/docs/windows.md#ci-steps).

To run only some E2E scenarios, or to add a scenario, see the [E2E guide](e2e/README.md).

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): subject`, for example `fix(e2e): Windows 실행기가 게스트 파일을 모두 실행 ID나 SHA-256으로 확인함`. Write the subject in Korean. The scope names the affected area, such as `e2e`, `ci`, or `windows`. Omit the scope for changes that are not tied to one area.
