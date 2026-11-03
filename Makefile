# config.example.yml이 감시하는 폴더. test/는 .gitignore 대상이라 clone에는 없으므로
# 예시를 쓰는 실행 타깃이 먼저 만든다.
# 목록은 config.example.yml의 `  - path: '...'` 줄에서 읽는다(두 칸 들여쓰기, 작은따옴표, 같은 줄 주석 없음).
EXAMPLE_WATCH_DIRS := $(shell sed -n "s/^  - path: '\(.*\)'/\1/p" config.example.yml)

.PHONY: example-watch-dirs
example-watch-dirs:
	mkdir -p $(EXAMPLE_WATCH_DIRS)

.PHONY: run
run: example-watch-dirs
	go run main.go -c config.example.yml

.PHONY: runi
runi: example-watch-dirs
	go run main.go -c config.example.yml --log-level=INFO

.PHONY: rund
rund: example-watch-dirs
	go run main.go -c config.example.yml --log-level=DEBUG

.PHONY: release-test-snapshot
release-test-snapshot:
	goreleaser release --skip publish --clean --snapshot

.PHONY: release-test
release-test:
	goreleaser release --skip publish --clean

DOCKER_VERSION ?= latest
.PHONY: rund-docker
rund-docker: example-watch-dirs
	docker run --rm \
	-v $(PWD)/config.example.yml:/config.yml \
	-v $(PWD)/test:/test \
	ghcr.io/seungyeop-lee/directory-watcher:$(DOCKER_VERSION) -c /config.yml -l DEBUG

.PHONY: e2e-local
e2e-local:
	go test -count=1 -v ./e2e/scenarios

.PHONY: e2e-linux
e2e-linux:
	go run ./e2e/linux

# ARGS holds runner flags, then -- and go test flags. $(value) keeps the $ of
# a -run pattern from being expanded by make.
.PHONY: e2e-windows-up
e2e-windows-up:
	go run ./e2e/windows up $(value ARGS)

.PHONY: e2e-windows
e2e-windows:
	go run ./e2e/windows test $(value ARGS)

.PHONY: e2e-windows-down
e2e-windows-down:
	go run ./e2e/windows down $(value ARGS)

.PHONY: e2e-windows-all
e2e-windows-all:
	go run ./e2e/windows all $(value ARGS)

.PHONY: test
test:
	go test -race ./...
