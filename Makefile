.PHONY: all build test vet lint coverage clean fmt cross-build

all: vet lint test build

build:
	go build -o gow ./cmd/gow/

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

cross-build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o gow-linux ./cmd/gow/
