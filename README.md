# Urlbox CLI

`urlbox` is the official command-line interface for the Urlbox screenshot and web automation API.

## Build

```bash
go build ./cmd/urlbox
```

## Install

```bash
go install github.com/urlbox/cli/cmd/urlbox@latest
```

## Commands

- `urlbox render https://example.com --format png`
- `urlbox render https://example.com --async --webhook-url https://hooks.example.com/urlbox`
- `urlbox batch --file urls.json --output-format ndjson`
- `urlbox projects list`
- `urlbox auth whoami`

## Development

The canonical development source lives in the Urlbox monorepo under `apps/cli/`.
This public repository is updated by an automated sync workflow from the monorepo.
