.PHONY: build build-all test lint clean install vet

BINARY_NAME=berth
VERSION=$(shell git describe --tags --always --dirty)
LDFLAGS=-ldflags "-X github.com/berth-dev/berth/internal/cli.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/berth

build-all: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64 build-windows-amd64

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/berth-darwin-arm64 ./cmd/berth

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/berth-darwin-amd64 ./cmd/berth

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/berth-linux-amd64 ./cmd/berth

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/berth-linux-arm64 ./cmd/berth

build-windows-amd64:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/berth-windows-amd64.exe ./cmd/berth

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

install:
	go install $(LDFLAGS) ./cmd/berth
