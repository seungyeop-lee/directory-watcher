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
	goreleaser release --skip-publish --rm-dist --snapshot

.PHONY: release-test
release-test:
	goreleaser release --skip-publish --rm-dist
