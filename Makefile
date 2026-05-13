.PHONY: all build run run-dev init install test test-integration lint fmt clean

APP_NAME = deviceos
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "0.1.0-dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X github.com/lohtbrok/deviceos/internal/version.Version=$(VERSION)"

all: fmt lint build

build:
	go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

run: build
	./bin/$(APP_NAME) start

run-dev:
	go run $(LDFLAGS) ./cmd/$(APP_NAME) start

init:
	go run $(LDFLAGS) ./cmd/$(APP_NAME) init

install: build
	@echo "Installing $(APP_NAME) to ~/.local/bin/..."
	@mkdir -p ~/.local/bin
	cp bin/$(APP_NAME) ~/.local/bin/$(APP_NAME)
	@echo "Done. Make sure ~/.local/bin is in your PATH."

test:
	go test ./... -count=1

test-integration:
	go test -tags=integration -count=1 -timeout=180s ./tests/integration/

test-integration-verbose:
	go test -tags=integration -count=1 -v -timeout=180s ./tests/integration/

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

clean:
	rm -rf bin/ data/
