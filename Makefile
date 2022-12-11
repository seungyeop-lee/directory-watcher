.PHONY: linux-build
linux-build:
	GOOS=linux GOARCH=386 go build -ldflags="-w -s" -o bin/directory-watcher-linux-386 main.go
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-linux-amd64 main.go
	GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o bin/directory-watcher-linux-arm64 main.go

.PHONY: macos-build
macos-build:
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-macos-amd64 main.go

.PHONY: windows-build
windows-build:
	GOOS=windows GOARCH=386 go build -ldflags="-w -s" -o bin/directory-watcher-windows-386.exe main.go
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s" -o bin/directory-watcher-windows-amd64.exe main.go

.PHONY: build
build: linux-build macos-build windows-build
	(cd bin && chmod +x *)

.PHONY: run
run:
	go run main.go -c config.example.yml

.PHONY: runi
runi:
	go run main.go -c config.example.yml --log-level=INFO

.PHONY: rund
rund:
	go run main.go -c config.example.yml --log-level=DEBUG
