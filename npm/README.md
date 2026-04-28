# @urlbox/cli

The official CLI for the [Urlbox](https://urlbox.com) screenshot and web automation API.

## Install

```
npm install -g @urlbox/cli
```

## Usage

```sh
urlbox --version
urlbox --help
urlbox commands                       # list every command + flag
urlbox commands --output-format json  # machine-readable for agents
urlbox auth --api-key sec_xxxxxx      # save your API key
urlbox doctor                         # verify install + auth
urlbox skill show                     # one-page agent guide
```

All commands support `--output-format json|text|quiet` and a built-in `--jq <expr>` filter (no external `jq` binary needed).

## How it works

The npm package is a thin wrapper around the native Go binary. During `postinstall`, it downloads the pre-built binary for your platform and architecture (macOS, Linux, or Windows; amd64 or arm64) from the GitHub release. Running `urlbox` then spawns that binary with your arguments.

## Documentation

Full documentation and additional install methods: [github.com/urlbox/urlbox-cli](https://github.com/urlbox/urlbox-cli)

## License

MIT
