.PHONY: run-by-mac
run-by-mac:
	directory-watcher -c config.mac.yml

DOCKER_VERSION ?= latest
.PHONY: run-by-docker
run-by-docker:
	docker compose up --build && docker compose down
