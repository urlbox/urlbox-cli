.PHONY: build test lint fmt fmt-check ci clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS  = -s -w \
	-X github.com/urlbox/urlbox-cli/internal/version.Version=$(VERSION) \
	-X github.com/urlbox/urlbox-cli/internal/version.Commit=$(COMMIT) \
	-X github.com/urlbox/urlbox-cli/internal/version.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/urlbox ./cmd/urlbox

test:
	go test -race -cover ./...

test-watch:
	gotestsum --watch ./...

lint:
	golangci-lint run ./...

fmt:
	gofumpt -w .

fmt-check:
	@test -z "$$(gofumpt -l .)" || (echo "Files need formatting:" && gofumpt -l . && exit 1)

ci: fmt-check lint test build

clean:
	rm -rf bin/ dist/
