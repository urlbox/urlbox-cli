# Urlbox CLI

The official command-line interface for the [Urlbox](https://urlbox.com) screenshot and web automation API.

## Install

### macOS (Homebrew)

```
brew install urlbox/tap/urlbox
```

### Windows (Scoop)

```
scoop bucket add urlbox https://github.com/urlbox/homebrew-tap
scoop install urlbox
```

### Linux (deb/rpm/apk)

Download the appropriate package from the [latest release](https://github.com/urlbox/cli/releases/latest).

### Go

```
go install github.com/urlbox/cli/cmd/urlbox@latest
```

### Script (macOS/Linux)

```
curl -fsSL https://cli.urlbox.com/install.sh | sh
```

## Usage

```
urlbox --version
urlbox --help
```

## Upgrade

```
urlbox upgrade
```

## Development

```
make ci        # Run fmt-check, lint, test, build
make test      # Run tests with race detector
make lint      # Run golangci-lint
make fmt       # Format with gofumpt
make build     # Build binary to bin/urlbox
```

## License

MIT
