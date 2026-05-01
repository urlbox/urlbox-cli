# @urlbox/cli

The official CLI for the [Urlbox](https://urlbox.com) screenshot and web automation API.

## Install

```
npm install -g @urlbox/cli
```

## Usage

```sh
urlbox auth --api-secret sec_xxxxxx                       # one-time
urlbox render https://example.com --output home.png       # capture & save
urlbox screenshot https://example.com --output home.png   # alias: --format png
urlbox pdf https://example.com --output home.pdf          # alias: --format pdf --full-page
urlbox video https://example.com --output home.mp4        # alias: --format mp4

urlbox render https://example.com --dry-run               # preview payload, no API call
urlbox render https://example.com --curl                  # paste-able curl, secret redacted
urlbox render https://example.com --open                  # open result in browser
urlbox render https://example.com --preset article        # news/article preset
urlbox render https://example.com --timeout 3m            # raise per-attempt budget (default 60s)

# Self-discovery
urlbox commands --output-format json                      # full command catalog
urlbox render --help --agent                              # structured JSON help
urlbox schema render                                      # JSON Schema of render options
urlbox skill show                                         # one-page agent guide

# Diagnostics
urlbox doctor                                             # version + config + auth + reachability
```

All commands support `--output-format json|text|quiet` and a built-in `--jq <expr>` filter (no external `jq` binary needed). Payloads sent via `--json` are validated client-side against the same schema before any network call.

Multi-account workflows use named profiles (`urlbox --profile <name> ...`); see `urlbox config profile --help`.

## How it works

The npm package is a thin wrapper around the native Go binary. During `postinstall`, it downloads the pre-built binary for your platform and architecture (macOS, Linux, or Windows; amd64 or arm64) from the GitHub release. Running `urlbox` then spawns that binary with your arguments.

## Documentation

Full documentation and additional install methods: [github.com/urlbox/urlbox-cli](https://github.com/urlbox/urlbox-cli)

## License

MIT
