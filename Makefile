.PHONY: all build test vet lint coverage clean fmt cross-build docs

all: vet lint test build

build:
	go build -ldflags "-s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)" -o gow ./cmd/gow/

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

coverage:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

fmt:
	gofmt -w -s .

clean:
	rm -f gow gow-linux coverage.out

LDFLAGS := -s -w
GOOS = linux
GOARCH = amd64

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

cross-build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o gow-linux ./cmd/gow/

docs:
	go run ./cmd/gow/ generate-docs docs/
