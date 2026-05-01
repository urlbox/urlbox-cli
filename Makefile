.PHONY: build test e2e e2e-verbose lint fmt fmt-check surface-snapshot surface-check ci smoke clean

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

surface-snapshot: build
	./bin/urlbox surface > SURFACE.txt

surface-check: build
	@./bin/urlbox surface | diff SURFACE.txt - || (echo "SURFACE.txt is stale or surface has breaking changes. Run 'make surface-snapshot' to refresh." && exit 1)

ci: fmt-check lint test build surface-check

# Real-API smoke tests — gated behind the `smoke` build tag so `make ci`
# / `go test ./...` never trigger them. Each render burns one credit on
# the configured account; run deliberately at release-cut moments.
#
# Usage:  URLBOX_API_SECRET=ubx_sk_... make smoke
smoke:
	@test -n "$$URLBOX_API_SECRET" || (echo "URLBOX_API_SECRET must be set" && exit 1)
	go test -tags=smoke -count=1 -timeout=90s -v ./internal/api/...

clean:
	rm -rf bin/ dist/
