.PHONY: linux-build
linux-build:
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-linux-amd64 cmd/main.go

.PHONY: macos-build
macos-build:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-macos-amd64 cmd/main.go

.PHONY: windows-build
windows-build:
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-windows-amd64.exe cmd/main.go

.PHONY: build
build: linux-build macos-build windows-build

run:
	go run cmd/main.go -c config.yml
