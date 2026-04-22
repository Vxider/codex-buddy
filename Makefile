APP_DIR := $(CURDIR)

.PHONY: build
build:
	go build -o bin/codex-buddy ./cmd/codex-buddy

.PHONY: install
install:
	./webserver/scripts/build-install.sh

.PHONY: install-uconsole
install-uconsole:
	./uconsole/scripts/build-install.sh

.PHONY: install-uconsole-ws281x
install-uconsole-ws281x:
	./uconsole/scripts/build-install.sh --ws281x

.PHONY: run
run:
	go run ./cmd/codex-buddy serve --config $(APP_DIR)/webserver/examples/config.example.json

.PHONY: fmt
fmt:
	gofmt -w ./cmd ./internal
