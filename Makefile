APP_DIR := $(CURDIR)

.PHONY: build
build:
	go build -o bin/agent-buddy ./cmd/agent-buddy

.PHONY: test
test:
	go test ./...

.PHONY: install
install:
	./webserver/scripts/build-install.sh

.PHONY: install-uconsole
install-uconsole:
	./uconsole/scripts/build-install.sh

.PHONY: build-macos
build-macos:
	./macos/scripts/build.sh

.PHONY: run
run:
	go run ./cmd/agent-buddy serve --config $(APP_DIR)/webserver/examples/config.example.json

.PHONY: fmt
fmt:
	gofmt -w $$(find cmd internal webserver uconsole -name '*.go' | sort)
