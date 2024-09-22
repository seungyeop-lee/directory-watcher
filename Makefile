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
