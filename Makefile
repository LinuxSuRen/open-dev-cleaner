BINARY ?= open-dev-cleaner
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build test fmt vet clean release

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)" -o bin/$(BINARY) .

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin dist

release:
	goreleaser release --clean

snapshot:
	goreleaser build --snapshot --clean
