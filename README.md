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
# One-time: store your API secret
urlbox auth --api-secret sec_xxxxxxxxxxxx

# Render a URL — saves screenshot.png to the current directory
urlbox render https://example.com --output screenshot.png

# Aliases: same pipeline, different default format
urlbox screenshot https://example.com --output home.png    # PNG
urlbox pdf https://example.com --output home.pdf           # PDF + full page
urlbox video https://example.com --output home.mp4         # MP4

# Preview the merged payload without making an API call (no credit burn)
urlbox render https://example.com --format pdf --dry-run

# Generate a copy-pasteable curl command (secret redacted)
urlbox render https://example.com --curl

# Verify install, config, and credentials
urlbox doctor

# Self-discovery for agents
urlbox commands --output-format json    # full command catalog
urlbox render --help --agent            # structured JSON help
urlbox schema render                    # JSON Schema of every render option
```

## Commands

### `render`

Capture a URL as a screenshot, PDF, or video.

```sh
# Simplest — positional URL, format flag, optional output
urlbox render https://example.com --format png --output home.png

# Full payload via --json (preferred for non-trivial config)
urlbox render --json '{"url":"https://example.com","format":"pdf","width":1920,"full_page":true}'

# --json from stdin or @file
echo '{"url":"https://example.com"}' | urlbox render --json -
urlbox render --json @opts.json

# Built-in presets layer in defaults (preset < json < flags)
urlbox render https://example.com --preset mobile          # iPhone viewport
urlbox render https://example.com --preset desktop         # 1920×1080
urlbox render https://example.com --preset pdf-a4          # PDF + A4 page

# Preview without calling the API
urlbox render https://example.com --dry-run

# Generate an equivalent curl command (secret redacted as $URLBOX_API_SECRET)
urlbox render https://example.com --curl

# Open the result in your browser after rendering
urlbox render https://example.com --open

# Async: queue and return a renderId
urlbox render https://example.com --async --webhook-url https://hooks.example/cb
```

**Aliases** (thin wrappers; share the entire render pipeline):

- `urlbox screenshot <url>` (also `urlbox shot`) — pre-sets `--format png`.
- `urlbox pdf <url>` — pre-sets `--format pdf --full-page`.
- `urlbox video <url>` — pre-sets `--format mp4`.

User-supplied flags override the alias defaults: `urlbox screenshot foo.com --format webp`.

**Reliability:** the CLI retries automatically on 429 / 5xx (3 attempts,
1s/2s/4s backoff with ±20% jitter, respects `Retry-After`). Disable with
`--no-retry`; cap with `--max-retries N`.

**Output sandbox:** `--output <path>` is canonicalized and asserted to stay
under the current working directory. Parent escapes (`../`), absolute paths
outside CWD, and symlinks pointing outside CWD are rejected.

### `auth`

Saves your Urlbox API secret to `~/.config/urlbox/config.json` (mode 0600). The
env var `URLBOX_API_SECRET` takes precedence at runtime if both are set.

```sh
# Non-interactive (preferred for agents and CI)
urlbox auth --api-secret sec_xxxxxxxxxxxx

# Interactive (humans on a TTY) — prompts once with masked echo
urlbox auth
```

The interactive path is gated on stdin AND stderr being TTYs, so headless
agents and piped invocations always require `--api-secret`.

### `config`

Inspect and modify the persisted configuration. Supports multiple named
profiles for working across accounts.

```sh
# Read / write a value
urlbox config get api_secret
urlbox config set api_host https://api.urlbox.com

# Show where the config file lives
urlbox config path

# Profile management
urlbox config profile list
urlbox config profile create work --api-secret sec_work
urlbox config profile default work
urlbox config profile delete work
```

**Profile-target resolution for `config set` / `config get`** (per-profile keys):

- 0 profiles: errors with "No profiles configured" — bootstrap with `urlbox auth`.
- 1 profile: `--profile` is implicit; `config set api_secret sk_xxx` Just Works.
- 2+ profiles: `--profile` is required; the error lists configured names.

`default_profile` is top-level and exempt from this rule, but the named profile
must exist (you can't set a dangling default).

### `commands`

Lists all available commands, their descriptions, and flags.

In a terminal, output is a human-readable table. When piped or with `--output-format json`, output is a structured JSON catalog suitable for agent and script consumption.

```
$ urlbox commands
Available commands:

  auth        Configure API credentials
  commands    List all available commands
  config      Inspect and modify CLI configuration
  doctor      Check installation, configuration, network, and credentials
  pdf         Render a URL as PDF (alias for `render --format pdf --full-page`)
  render      Render a URL to a screenshot, PDF, video, or other format
  schema      Print JSON Schemas describing Urlbox API payloads
  screenshot  Capture a screenshot (alias for `render --format png`)
  skill       Agent skill content
  upgrade     Update urlbox to the latest version
  video       Render a URL as MP4 video (alias for `render --format mp4`)

Use "urlbox <command> --help" for more information about a command.
```

### `doctor`

Diagnoses installation, configuration, network, and credential issues. Runs
seven checks: version, install method, config file, API secret, DNS, API
reachability, and credential validity. Exits non-zero if any check fails.

```sh
urlbox doctor
urlbox doctor --output-format json --jq '.data.checks[] | select(.status != "ok")'
```

### `schema`

Prints the JSON Schema that describes an Urlbox API payload. Use this to
discover every valid option and its type — handy for agents building requests
or for humans exploring the available render options.

```sh
# Full schema in the standard envelope
urlbox schema render

# Discover render options
urlbox schema render --jq '.data.properties | keys'

# Raw schema only (no envelope)
urlbox schema render --output-format quiet
```

When `--json` is used to send payloads (Phase 4 onward), this same schema is
applied for client-side validation before any network call. Validation
failures return error code `validation` (exit code 2). See `urlbox skill show`
for the full validation contract.

### `skill`

Prints the embedded `SKILL.md` — a one-page agent guide describing the output
contract, error codes, discovery commands, and authentication flow. Useful as
context for an LLM agent.

```sh
urlbox skill show
```

### `upgrade`

Updates urlbox to the latest version. Automatically detects how you installed it (Homebrew, Scoop, npm, or Go) and runs the appropriate update command. If the install method can't be detected, it prints all available upgrade commands so you can pick the right one.

## Output Formats

All commands support three output formats via the `--output-format` flag:

| Format | Flag | Description |
|--------|------|-------------|
| `text` | `--output-format text` | Human-readable with colors. Default in a terminal. |
| `json` | `--output-format json` | Full JSON envelope with `ok`, `command`, `data`, `summary`, and `breadcrumbs` fields. Default when piped. |
| `quiet` | `--output-format quiet` | Raw data only (no envelope wrapper). |

**Auto-detection:** When no `--output-format` flag is given, the CLI uses `text` if stdout is a TTY (interactive terminal) and `json` if stdout is piped to another program. This means scripts and agents get structured JSON by default without any extra flags.

The `NO_COLOR` environment variable is respected — when set, terminal colors are disabled.

## Filtering output with `--jq`

Every command supports a built-in `--jq <expr>` flag, powered by [gojq](https://github.com/itchyny/gojq). The expression runs over the JSON envelope, or against `.data` when combined with `--output-format quiet`. No external `jq` binary required.

```sh
# Pull a single field
urlbox commands --output-format json --jq '.data.commands[].name'

# Filter to failing doctor checks only
urlbox doctor --output-format json --jq '.data.checks[] | select(.status != "ok")'

# Use --output-format quiet to run jq directly against .data
urlbox commands --output-format quiet --jq '.commands | length'
```

## Agent integration

Three discovery layers built specifically for LLM agents:

```sh
# 1. Full command catalog as JSON
urlbox commands --output-format json

# 2. Structured help for any command
urlbox commands --help --agent
urlbox doctor --help --agent

# 3. The CLI's agent skill (embedded as SKILL.md)
urlbox skill show
```

The CLI's exposed surface (every command and flag, at every level) is committed to `SURFACE.txt` and enforced in CI. New commands and flags can be added freely; renaming or removing one fails the surface gate, so downstream agents and scripts never break silently.

## Authentication

Three ways to provide your Urlbox API secret. The CLI picks the highest-priority
source available (highest first):

```sh
# 1. CLI flag — one-shot, doesn't touch the config file
urlbox --profile <name> render <url>

# 2. Env var — preferred for CI / containers
export URLBOX_API_SECRET=sec_xxxxxxxxxxxx

# 3. Persisted to ~/.config/urlbox/config.json (mode 0600)
urlbox auth --api-secret sec_xxxxxxxxxxxx
```

The full priority chain:

1. CLI flag (`--profile <name>`)
2. Env vars (`URLBOX_API_SECRET`, `URLBOX_PROFILE`, `URLBOX_API_HOST`)
3. The named profile (selected via `--profile`, `URLBOX_PROFILE`, or the
   stored `default_profile`)
4. Per-repo overrides at `.urlbox/config.json` (walks from CWD up to `$HOME`)
5. The global default profile in `~/.config/urlbox/config.json`

Multiple profiles let you keep credentials per account, environment, or repo.
See `urlbox config profile --help` for management commands.

Verify with `urlbox doctor`.

## Development

| Target | Description |
|--------|-------------|
| `make ci` | Run all checks: fmt-check, lint, test, build, surface-check |
| `make test` | Run tests with race detector |
| `make e2e` | Run end-to-end tests |
| `make e2e-verbose` | Run E2E tests with colored output |
| `make lint` | Run golangci-lint |
| `make fmt` | Format with gofumpt |
| `make build` | Build binary to `bin/urlbox` |
| `make surface-snapshot` | Regenerate `SURFACE.txt` from the built binary |
| `make surface-check` | Fail if `SURFACE.txt` is stale or has breaking changes |
| `make clean` | Remove `bin/` and `dist/` |

`SURFACE.txt` is the canonical contract of every command and flag. Run `make surface-snapshot` after intentionally adding a flag or command, then commit the updated file alongside the code change.

## License

MIT
