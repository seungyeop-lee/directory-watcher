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
- The Docker CLI and a running Linux Docker engine, only to run the E2E scenarios in a Linux container with `make e2e-linux`.

To run the watcher from source with [`config.example.yml`](config.example.yml), run this at the repository root:

```sh
make run
```

`make rund` does the same with DEBUG logs. Both first create the watched folders that `config.example.yml` lists.

## Verification

Run these at the repository root after changing code:

| Command | When to run | What it runs |
|---|---|---|
| `go build ./...` | Always | Builds every package |
| `go vet ./...` | Always | Runs `go vet` on every package |
| `go test -count=1 -race ./...` | Always | Unit tests, the root tests that check documents against code, and the E2E scenarios in `e2e/scenarios` on the current OS, with the race detector |
| `make smoke-example` | You changed `config.example.yml`, or how the watcher starts, runs hooks, or stops | Runs the watcher with `config.example.yml` and checks that watching starts, that creating a file runs the `onChange` hooks, and that SIGTERM runs the `onFinishWatch` hooks and exits with code 0 |

CI runs the first three commands on Linux, Windows, and macOS, and `make smoke-example` on Linux.

To run the E2E scenarios in a Linux container or on a Windows VM, or to add a scenario, see the [E2E guide](e2e/README.md).

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): subject`, for example `chore(ci): master, v<major> 브랜치와 tag push에서는 ci를 실행하지 않음`. Write the subject in Korean. The scope names the affected area, such as `e2e`, `ci`, or `windows`. Omit the scope for changes that are not tied to one area.
