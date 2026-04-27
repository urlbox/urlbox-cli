# Urlbox CLI

The official command-line interface for the [Urlbox](https://urlbox.com) screenshot and web automation API. Render screenshots, PDFs, videos, and extracted content from URLs or HTML.

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

### npm (cross-platform)

```
npm install -g @urlbox/cli
```

### Shell script (macOS/Linux)

```
curl -fsSL https://cli.urlbox.com/install.sh | sh
```

### Linux packages (deb/rpm/apk)

Download the appropriate package from the [latest release](https://github.com/urlbox/urlbox-cli/releases/latest).

### Go

```
go install github.com/urlbox/urlbox-cli/cmd/urlbox@latest
```

## Quick Start

```sh
# Check your installed version
urlbox --version

# See available commands and flags
urlbox --help

# List all commands (human-readable)
urlbox commands

# List all commands (machine-readable JSON)
urlbox commands --output-format json
```

## Commands

### `commands`

Lists all available commands, their descriptions, and flags.

In a terminal, output is a human-readable table. When piped or with `--output-format json`, output is a structured JSON catalog suitable for agent and script consumption.

```
$ urlbox commands
Available commands:

  commands  List all available commands
  upgrade   Update urlbox to the latest version

Use "urlbox <command> --help" for more information about a command.
```

### `upgrade`

Updates urlbox to the latest version. Automatically detects how you installed it (Homebrew, Scoop, npm, or Go) and runs the appropriate update command.

```
$ urlbox upgrade
Current version: v0.1.0
Install method: brew
Binary path: /opt/homebrew/bin/urlbox

Upgrading via Homebrew...
```

If the install method can't be detected, it prints all available upgrade commands so you can pick the right one.

## Output Formats

All commands support three output formats via the `--output-format` flag:

| Format | Flag | Description |
|--------|------|-------------|
| `text` | `--output-format text` | Human-readable with colors. Default in a terminal. |
| `json` | `--output-format json` | Full JSON envelope with `ok`, `command`, `data`, `summary`, and `breadcrumbs` fields. Default when piped. |
| `quiet` | `--output-format quiet` | Raw data only (no envelope wrapper). |

**Auto-detection:** When no `--output-format` flag is given, the CLI uses `text` if stdout is a TTY (interactive terminal) and `json` if stdout is piped to another program. This means scripts and agents get structured JSON by default without any extra flags.

The `NO_COLOR` environment variable is respected — when set, terminal colors are disabled.

## Development

| Target | Description |
|--------|-------------|
| `make ci` | Run all checks: fmt-check, lint, test, build |
| `make test` | Run tests with race detector |
| `make e2e` | Run end-to-end tests |
| `make e2e-verbose` | Run E2E tests with colored output |
| `make lint` | Run golangci-lint |
| `make fmt` | Format with gofumpt |
| `make build` | Build binary to `bin/urlbox` |
| `make clean` | Remove `bin/` and `dist/` |

## License

MIT
