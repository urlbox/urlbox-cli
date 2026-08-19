# @urlbox/cli

The official CLI for [Urlbox](https://urlbox.com), the website screenshot API. Render screenshots, PDFs, videos, and extracted content from any URL or raw HTML.

## Install

```sh
npm install -g @urlbox/cli
```

## Quick start

```sh
# Sign in once through your browser (CI/headless: set URLBOX_API_SECRET instead)
urlbox login

# Render a page and save it
urlbox render https://example.com --output home.png
```

In CI, skip `login` — it needs a browser. Either set `URLBOX_API_SECRET` (your project's secret, `ubx_sk_…`, from the [dashboard](https://urlbox.com/dashboard/projects)), or persist it once with `printf %s "$URLBOX_API_SECRET" | urlbox auth --api-secret-stdin`.

Every command speaks JSON when piped, supports a built-in `--jq <expr>` filter, and returns a clear exit code.

## How it works

This npm package is a thin wrapper around the native Go binary. On `postinstall` it downloads the pre-built binary for your platform (macOS, Linux, or Windows; amd64 or arm64) from the GitHub release, and `urlbox` spawns it with your arguments.

## Documentation

Full docs, more install methods, and the command reference: [urlbox.com/docs/cli](https://urlbox.com/docs/cli) and [github.com/urlbox/urlbox-cli](https://github.com/urlbox/urlbox-cli).

## License

MIT
