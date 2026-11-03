.PHONY: run
run:
	go run main.go -c config.example.yml

.PHONY: runi
runi:
	go run main.go -c config.example.yml --log-level=INFO

.PHONY: rund
rund:
	go run main.go -c config.example.yml --log-level=DEBUG

.PHONY: release-test-snapshot
release-test-snapshot:
	goreleaser release --skip publish --clean --snapshot

.PHONY: release-test
release-test:
	goreleaser release --skip publish --clean

DOCKER_VERSION ?= latest
.PHONY: rund-docker
rund-docker:
	docker run --rm \
	-v $(PWD)/config.example.yml:/config.yml \
	-v $(PWD)/test:/test \
	ghcr.io/seungyeop-lee/directory-watcher:$(DOCKER_VERSION) -c /config.yml -l DEBUG

.PHONY: e2e e2e-linux e2e-windows-up e2e-windows e2e-windows-down e2e-windows-all
e2e:
	go test -count=1 -v ./e2e/scenarios

e2e-linux:
	go run ./e2e/linux

# ARGS holds runner flags, then -- and go test flags. $(value) keeps the $ of
# a -run pattern from being expanded by make.
e2e-windows-up:
	go run ./e2e/windows up $(value ARGS)

e2e-windows:
	go run ./e2e/windows test $(value ARGS)

e2e-windows-down:
	go run ./e2e/windows down $(value ARGS)

e2e-windows-all:
	go run ./e2e/windows all $(value ARGS)

.PHONY: test
test:
	go test -race ./...

.PHONY: beads-ui
beads-ui:
	bdui start --open
