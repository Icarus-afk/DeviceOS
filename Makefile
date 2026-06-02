.PHONY: all build run run-dev init install test test-integration test-integration-verbose lint fmt clean docker-build docker-run release

APP_NAME = deviceos
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "0.1.0")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -ldflags "-X github.com/lohtbrok/deviceos/internal/version.Version=$(VERSION) -X github.com/lohtbrok/deviceos/internal/version.Commit=$(COMMIT)"

all: fmt lint build

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/$(APP_NAME)

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

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t $(APP_NAME):latest \
		-t $(APP_NAME):$(VERSION) \
		.

docker-run:
	docker compose up -d --build

release:
	@echo "Building release binaries for linux/amd64, linux/arm64..."
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-$(VERSION)-linux-amd64 ./cmd/$(APP_NAME)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-$(VERSION)-linux-arm64 ./cmd/$(APP_NAME)
	@cd dist && sha256sum $(APP_NAME)-$(VERSION)-linux-amd64 $(APP_NAME)-$(VERSION)-linux-arm64 > checksums.txt
	@echo "Release binaries in dist/:"
	@ls -lh dist/
