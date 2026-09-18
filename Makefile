# SKALL Build & Release Automation

VERSION ?= 1.0.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "release")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
PKG_VERSION := github.com/Harish-vinayagam/Skall/internal/version

LDFLAGS := -s -w \
	-X $(PKG_VERSION).Version=$(VERSION) \
	-X $(PKG_VERSION).GitCommit=$(GIT_COMMIT) \
	-X $(PKG_VERSION).BuildDate=$(BUILD_DATE)

.PHONY: all build test test-race fmt vet release-linux-amd64 release-linux-arm64 clean

all: build

# Standard local development build
build:
	mkdir -p bin
	CGO_ENABLED=1 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/skall ./cmd/skall

# Run all unit and integration tests
test:
	go test -count=1 ./...

# Run all tests under the Go race detector
test-race:
	go test -race -count=1 ./...

# Code formatting check and format
fmt:
	go fmt ./...

# Static analysis
vet:
	go vet ./...

# Reproducible release binary for Linux AMD64
release-linux-amd64:
	mkdir -p bin
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/skall-linux-amd64 ./cmd/skall

# Release binary for Linux ARM64 (requires cross-compiler if running on non-ARM host)
release-linux-arm64:
	mkdir -p bin
	@if [ "$$(uname -m)" = "aarch64" ] || [ "$$(uname -m)" = "arm64" ]; then \
		CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/skall-linux-arm64 ./cmd/skall; \
	elif command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then \
		CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go build -trimpath -ldflags="$(LDFLAGS)" -o bin/skall-linux-arm64 ./cmd/skall; \
	else \
		echo "Cross-compilation to Linux ARM64 requires aarch64-linux-gnu-gcc or running natively on ARM64."; \
		exit 1; \
	fi

# Clean up build artifacts
clean:
	rm -rf bin/
