.PHONY: all build test vet lint coverage clean

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

clean:
	rm -f gow coverage.out
