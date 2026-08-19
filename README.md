# Urlbox CLI

The official command-line interface for [Urlbox](https://urlbox.com), the website screenshot API. Render screenshots, PDFs, videos, and extracted content from any URL or raw HTML — straight from your terminal, a CI pipeline, or an AI agent. Bring your own AI to analyse renders, S3 storage to store them securely, and proxies to change a render's point of view.

Read our API's [docs](https://urlbox.com/docs) here. Are you an AI agent? Those docs are available as markdown files [here](https://urlbox.com/llms.txt).

Every command speaks JSON for your AI agents, and points you at the next step. The full CLI docs are live at [urlbox.com/docs/cli](https://urlbox.com/docs/cli).

## Install

```sh
# npm (cross-platform)
npm install -g @urlbox/cli

# macOS (Homebrew)
brew install urlbox/tap/urlbox

# Windows (Scoop)
scoop bucket add urlbox https://github.com/urlbox/homebrew-tap
scoop install urlbox

# Go
go install github.com/urlbox/urlbox-cli/cmd/urlbox@latest
```

Linux `.deb`/`.rpm`/`.apk` packages and a `curl | sh` installer are covered in [the install docs](https://urlbox.com/docs/cli/install).

Confirm it worked:

```sh
urlbox doctor
```

## Quick start

```sh
# Sign in once through your browser
urlbox login

# Render a page and save it
urlbox render https://example.com --output home.png

# Sign a render URL locally, no API call https://urlbox.com/docs/api/rest-api-vs-render-links#render-links
urlbox link --url https://example.com --output-format quiet
```

You can check out the full walkthrough at [urlbox.com/docs/cli/quickstart](https://urlbox.com/docs/cli/quickstart).

## Authentication

```sh
urlbox login
```

Your browser opens, you approve, and the CLI stores your session plus the active project's render credential — renders work immediately.

In CI and headless environments, where the browser can't open, authenticate with the render secret directly (it looks like `ubx_sk_…`, found under your project in the [dashboard](https://urlbox.com/dashboard/projects)):

```sh
URLBOX_API_SECRET=ubx_sk_… urlbox render https://example.com      # stateless — nothing written

printf %s "$URLBOX_API_SECRET" | urlbox config profile create default --api-secret-stdin
urlbox config profile create default --api-secret-file /run/secrets/urlbox   # or read it from disk
```

`config profile create` is the headless bootstrap: it works on a machine with no config at all, writes mode 0600, and also takes `--api-key` (needed by `link`) and `--api-host`. Avoid the bare `--api-secret`, which leaks into `ps` and shell history. `--api-secret-stdin` / `--api-secret-file` also work as a one-shot override on any single command.

When more than one source is present, the highest-priority wins: **command flag → environment variable → a per-repo `.urlbox/config.json` overlay → your stored config**.

Verify what the CLI resolved with `urlbox doctor`. More detail at [urlbox.com/docs/cli/authentication](https://urlbox.com/docs/cli/authentication).

## Commands

### Rendering

```sh
urlbox render https://example.com --format png --output home.png
urlbox render --json '{"url":"https://example.com","format":"pdf","full_page":true}'
```

`render` captures a page as a screenshot, PDF, video, or extracted content. Options merge in the order **preset → `--json` → flags**, so a typed flag always wins.

| Command | Does |
|---------|------|
| `render <url>` | Capture a page in any format |
| `screenshot <url>` (alias `shot`) | `render --format png` |
| `pdf <url>` | `render --format pdf --full-page` |
| `video <url>` | `render --format mp4` |

Handy render flags:

- `--preset <name>` layers in defaults before anything else. Built-in presets: `mobile` (iPhone viewport), `desktop` (1920×1080), `pdf-a4` (PDF on A4), `article` (block ads, retina, wait for most requests).
- `--dry-run` validates the merged payload without calling the API.
- `--curl` prints the equivalent curl command with the secret redacted.
- `--open` opens the result in your browser after rendering.
- `--output <path>` saves the file. Paths are sandboxed to the current directory — escapes and symlinks pointing outside it are rejected.

`--json` is a passthrough: any current or future API option works through it, and the Urlbox API validates it. Typed flags are checked locally before any network call. See [urlbox.com/docs/cli/rendering](https://urlbox.com/docs/cli/rendering) and the full option list at [urlbox.com/docs/cli/json-and-schema](https://urlbox.com/docs/cli/json-and-schema).

### Signed links

```sh
urlbox link --url https://example.com --output-format quiet
```

`link` builds an HMAC-SHA256 signed render URL with pure local crypto — no API call, no render. Useful for embedding in templates, emails, or static sites. It needs both the publishable API key and the API secret. More at [urlbox.com/docs/cli/signed-links](https://urlbox.com/docs/cli/signed-links).

### Async renders

```sh
urlbox render https://example.com --async --webhook-url https://hooks.example/cb
urlbox status ps_abc123 --wait
```

Pass `--async` to queue a render and get a `renderId` back immediately. `status` checks it, and `status --wait` polls (every 2s by default) until it reaches a terminal state — `succeeded` or `failed`. Webhooks and long-running renders are covered at [urlbox.com/docs/cli/async-and-webhooks](https://urlbox.com/docs/cli/async-and-webhooks).

### Account and context

```sh
urlbox login                 # sign in
urlbox whoami                # who am I, and which org/project is active (alias: me)
urlbox orgs list             # organisations you belong to (alias: org)
urlbox orgs select acme      # switch the active org
urlbox projects list         # projects in the active org
urlbox usage                 # render usage for the current period
```

| Command | Does |
|---------|------|
| `login` / `logout` | Sign in through the browser; sign out and revoke this device's session |
| `whoami` (alias `me`) | Show the signed-in user and active org/project |
| `orgs list` / `orgs select` | List or switch your active organisation (`--project` finishes the switch in one step) |
| `projects list` / `select` / `show` | Browse and switch the active project |
| `projects create` / `rename` / `enable` / `disable` / `delete` | Manage projects |
| `projects defaults show` / `set` / `remove` | Manage a project's default render options |
| `usage` | Render usage summary for the active org |

Deletes and disables prompt for confirmation; pass `--yes` to skip the prompt (agents should).

Switching organisation clears the stored render credential — it belongs to a project in the organisation you are leaving. The CLI picks the new organisation's project back up automatically when there is exactly one; when there are several, pass `urlbox orgs select <org> --project <name>` to land both in one step, or follow up with `urlbox projects select`. Details at [urlbox.com/docs/cli/configuration](https://urlbox.com/docs/cli/configuration).

### Org resources

Storage credentials, proxy pools, and LLM credentials belong to the organisation. Create one once, then assign it to any project.

```sh
urlbox storage list
urlbox storage create prod --provider aws_s3 --bucket b --region us-east-1 --key k --secret s
urlbox projects storage assign my-project prod
```

| Group | Verbs |
|-------|-------|
| `storage` | `list` `show` `create` `update` `delete` |
| `proxies` (alias `proxy`) | `list` `show` `create` `update` `delete` |
| `llm` | `list` `show` `create` `update` `delete` `test` `models` |
| `projects <kind> assign` / `unassign` | Attach or detach a project's `storage`, `proxy`, or `llm` credential |

Secrets are masked by default in both text and JSON — pass `--reveal` on `list` or `show` to unmask. Deletes are retype-to-confirm; `--yes` skips the prompt. A target resolves by name or id (`store_…`, `pool_…`, `llm_…`).

### Utilities

| Command | Does |
|---------|------|
| `config get` / `set` / `path` | Read and write the stored config |
| `config profile create` | Store credentials without a browser — the CI, container, and agent path |
| `schema render` | Print the JSON Schema of every render option |
| `commands` | List every command and flag (human table, or JSON when piped) |
| `skill show` / `install` | Print or install the agent skill (see below) |
| `doctor` | Check version, config, session, credentials, and API reachability |
| `dashboard` | Open the Urlbox dashboard in your browser |
| `upgrade` | Update to the latest version via the detected install method |
| `version` | Print the version, commit, and build date |

Troubleshooting guide: [urlbox.com/docs/cli/troubleshooting](https://urlbox.com/docs/cli/troubleshooting). Full reference: [urlbox.com/docs/cli/command-reference](https://urlbox.com/docs/cli/command-reference).

## Output

Every command returns one of three formats via `--output-format`:

| Format | Default when | Shape |
|--------|--------------|-------|
| `text` | stdout is a terminal | Human-readable, with colour |
| `json` | stdout is piped | Full envelope |
| `quiet` | — | Raw `data` only, no envelope |

Success and error responses use a stable envelope:

```json
{ "ok": true,  "command": "render", "data": {}, "summary": "...", "breadcrumbs": [] }
{ "ok": false, "command": "render", "error": "...", "code": "...", "hint": "..." }
```

Data goes to stdout, messages and warnings go to stderr, so piping stays clean. The `NO_COLOR` environment variable disables colour. Filter any response inline with the built-in `--jq <expr>` flag — no external `jq` binary needed:

```sh
urlbox doctor --output-format json --jq '.data.checks[] | select(.status != "ok")'
```

Each error `code` maps to a fixed exit code:

| Code | Exit | | Code | Exit |
|------|------|---|------|------|
| `usage` | 1 | | `rate_limit` | 6 |
| `validation` | 2 | | `conflict` | 7 |
| `auth` | 3 | | `server` | 10 |
| `forbidden` | 4 | | `network` | 11 |
| `not_found` | 5 | | `timeout` | 11 |

Renders retry automatically on 429, 5xx, and network errors — up to 3 retries (4 attempts total), with backoff. Disable with `--no-retry`, cap with `--max-retries`. Timeouts are never retried; raise `--timeout` or switch to `--async`. More at [urlbox.com/docs/cli/output-and-scripting](https://urlbox.com/docs/cli/output-and-scripting).

## For AI agents

The CLI is built to be driven by an agent. Everything is discoverable and non-interactive:

```sh
# Install the skill so your agent auto-discovers the CLI next session
urlbox skill install --target claude-code --scope user --yes

# Discover the surface
urlbox commands --output-format json    # every command and flag
urlbox render --help --agent            # structured JSON help
urlbox schema render                    # every render option and its type
```

Pipe any command and it defaults to JSON. Add `--yes` to skip confirmation prompts, `URLBOX_API_SECRET` to authenticate without a browser, and the global `--profile <name>` to pin a whole credential set. Pin context up front with `urlbox login --org <o> --project <p>`, or `urlbox orgs select <o> --project <p>` when switching later — both avoid the interactive picker, which refuses to run without a terminal rather than hanging. Skill install targets are `claude-code`, `cursor`, `codex`, and `opencode`. Setup guide: [urlbox.com/docs/cli/ai-agents](https://urlbox.com/docs/cli/ai-agents).

## Versioning

**v1.1.0 is the current stable release**, and the CLI follows [SemVer](https://semver.org) from here. `SURFACE.txt` is the contract: nothing listed in it is removed or renamed within the v1 line without a major bump — new commands and flags can arrive in any minor release.

Earlier `0.x` and `1.0.x` versions were the pre-stable line and are superseded; `npm install -g @urlbox/cli` always resolves to the current release.

## Development

```sh
make build    # build to bin/urlbox
make test     # run tests with the race detector
make ci       # fmt-check, lint, test, build, surface-check
```

`SURFACE.txt` is the canonical contract of every command and flag; a CI check fails if it drifts. Run `make surface-snapshot` after intentionally adding a command or flag, and commit the update alongside the code.

## License

MIT
