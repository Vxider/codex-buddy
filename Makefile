APP_DIR := $(CURDIR)

.PHONY: build
build:
	go build -o bin/codex-buddy ./cmd/codex-buddy

.PHONY: test
test:
	go test ./...

.PHONY: install
install:
	./webserver/scripts/build-install.sh

.PHONY: install-uconsole
install-uconsole:
	./uconsole/scripts/build-install.sh

.PHONY: run
run:
	go run ./cmd/codex-buddy serve --config $(APP_DIR)/webserver/examples/config.example.json

.PHONY: fmt
fmt:
	gofmt -w $$(find cmd internal webserver uconsole -name '*.go' | sort)
