.PHONY: linux-build
linux-build:
	GOOS=linux GOARCH=386 go build -ldflags="-w -s" -o bin/directory-watcher-linux-386 cmd/main.go
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-linux-amd64 cmd/main.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o bin/directory-watcher-linux-arm64 cmd/main.go

.PHONY: macos-build
macos-build:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-macos-amd64 cmd/main.go

.PHONY: windows-build
windows-build:
	GOOS=windows GOARCH=386 go build -ldflags="-w -s" -o bin/directory-watcher-windows-386.exe cmd/main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-windows-amd64.exe cmd/main.go

.PHONY: build
build: linux-build macos-build windows-build

.PHONY: run
run:
	go run cmd/main.go -c config.yml

.PHONY: runv
runv:
	go run cmd/main.go -c config.yml -v

.PHONY: rund
rund:
	go run cmd/main.go -c config.yml -d
