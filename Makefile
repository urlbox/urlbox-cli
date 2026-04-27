.PHONY: build test e2e e2e-verbose lint fmt fmt-check ci clean

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

e2e:
	gotestsum --format testdox -- -count=1 ./internal/cmd/ -run TestE2E $(ARGS)

e2e-verbose:
	@go test -count=1 -v ./internal/cmd/ -run TestE2E $(ARGS) 2>&1 | sed -E \
		-e 's/^(=== RUN .*)$$/\x1b[36m\1\x1b[0m/' \
		-e 's/^( *--- PASS:.*)$$/\x1b[32m\1\x1b[0m/' \
		-e 's/^( *--- FAIL:.*)$$/\x1b[31m\1\x1b[0m/' \
		-e 's/^(PASS)$$/\x1b[32;1m\1\x1b[0m/' \
		-e 's/^(FAIL.*)$$/\x1b[31;1m\1\x1b[0m/' \
		-e 's/^(ok .*)$$/\x1b[32m\1\x1b[0m/'

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
