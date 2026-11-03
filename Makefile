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

.PHONY: smoke-example
smoke-example: example-watch-dirs
	sh scripts/smoke-example.sh $(EXAMPLE_WATCH_DIRS)

# goreleaser는 publish를 건너뛰어도 scoops의 private_key 템플릿을 적용하므로, 비밀값이 없는 로컬에서는 빈 값을 넘긴다.
release-test-snapshot release-test: export SCOOP_BUCKET_SSH_KEY ?=

.PHONY: release-test-snapshot
release-test-snapshot:
	goreleaser release --skip publish --clean --snapshot

.PHONY: release-test
release-test:
	goreleaser release --skip publish --clean

# rund-docker는 지금 코드로 이미지를 빌드해 실행한다. 빌드 컨텍스트에는 goreleaser처럼
# Docker가 실행되는 아키텍처의 linux 바이너리를 linux/<아키텍처>/ 아래에 두고, entrypoint.sh와 함께 담는다.
RUND_DOCKER_CONTEXT := dist/rund-docker
RUND_DOCKER_IMAGE := directory-watcher:rund-docker
# config.example.yml 훅의 dir 목록. 이미지는 작업 폴더가 /라서 상대 경로 dir이 /<dir>로 풀리므로,
# 각 폴더를 /<dir>에 마운트해 로컬 실행과 같은 폴더에서 훅이 돌게 한다.
# 목록은 `- dir: "..."` 줄에서 읽는다(큰따옴표, 같은 줄 주석 없음).
EXAMPLE_HOOK_DIRS := $(sort $(shell sed -n 's/^ *- dir: "\(.*\)"/\1/p' config.example.yml))

.PHONY: rund-docker
rund-docker: example-watch-dirs
	arch=$$(docker version --format '{{.Server.Arch}}') && \
	CGO_ENABLED=0 GOOS=linux GOARCH=$$arch \
	go build -o $(RUND_DOCKER_CONTEXT)/linux/$$arch/directory-watcher .
	cp entrypoint.sh $(RUND_DOCKER_CONTEXT)/
	docker build -f Dockerfile -t $(RUND_DOCKER_IMAGE) $(RUND_DOCKER_CONTEXT)
	docker run --rm \
	-v $(PWD)/config.example.yml:/config.yml \
	-v $(PWD)/test:/test \
	$(foreach dir,$(EXAMPLE_HOOK_DIRS),-v $(PWD)/$(dir):/$(dir)) \
	$(RUND_DOCKER_IMAGE) -c /config.yml -l DEBUG

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
