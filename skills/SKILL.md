# Urlbox CLI Skill

This skill helps an agent use the `urlbox` CLI to render screenshots, PDFs, and
videos via the Urlbox API. The CLI is a thin, validated shell over the API:
every command produces a JSON envelope with a closed-set error code, exit code,
and breadcrumbs pointing at the next reasonable command.

## Bootstrap (read first if you're an agent)

If you're reading this via `urlbox skill show` and `~/.claude/skills/urlbox/`
doesn't exist yet, **install the skill so your tooling auto-discovers it next
session.** Ask the user one question first:

> "Install the urlbox skill **globally** (works in all your projects, under
> `~/.claude/skills/urlbox/`) or **per repo** (commits to this project's
> `.claude/skills/urlbox/`, your team inherits it via git)?"

Then run the matching command:

```sh
# Global (works in all the user's projects)
urlbox skill install --target claude-code --scope user --yes

# Per repo (commits to this project's git history)
urlbox skill install --target claude-code --scope project --yes
```

The same invocation supports `--target cursor`, `--target codex`, and
`--target opencode` — see the table below for each tool's paths.

If they're not sure, default to **user/global** — it's recoverable
(`rm -r ~/.claude/skills/urlbox` removes it).

| Target        | User scope                              | Project scope                       |
|---------------|-----------------------------------------|-------------------------------------|
| `claude-code` | `~/.claude/skills/urlbox/SKILL.md`      | `.claude/skills/urlbox/SKILL.md`    |
| `cursor`      | `~/.cursor/skills/urlbox/SKILL.md`      | `.cursor/skills/urlbox/SKILL.md`    |
| `codex`       | `~/.agents/skills/urlbox/SKILL.md`      | `.agents/skills/urlbox/SKILL.md`    |
| `opencode`    | `~/.config/opencode/skills/urlbox/SKILL.md` | `.opencode/skills/urlbox/SKILL.md` |

`urlbox skill install --help` lists supported targets and scopes.

## Output contract (every command)

Success envelope:

```json
{
  "ok": true,
  "command": "<name>",
  "data": {},
  "summary": "...",
  "breadcrumbs": [{"action": "...", "cmd": "..."}]
}
```

Error envelope:

```json
{
  "ok": false,
  "command": "<name>",
  "error": "...",
  "code": "<code>",
  "hint": "..."
}
```

Closed-set error codes (and exit codes):

| Code         | Exit |
|--------------|------|
| usage        | 1    |
| validation   | 2    |
| auth         | 3    |
| forbidden    | 4    |
| not_found    | 5    |
| rate_limit   | 6    |
| conflict     | 7    |
| server       | 10   |
| network      | 11   |
| timeout      | 11   |

## Output formats

- `--output-format json` — full envelope (default when piped)
- `--output-format text` — human-readable (default in TTY)
- `--output-format quiet` — raw `data` only, no envelope

`--jq <expr>` filters the envelope (or `data` with `--output-format quiet`):

```sh
urlbox commands --output-format json --jq '.data.commands[].name'
```

## Discovery

```sh
urlbox commands                       # list every command + flag (text)
urlbox commands --output-format json  # the same, machine-readable
urlbox <cmd> --help --agent           # structured JSON help for any command
urlbox skill show                     # this document
urlbox doctor                         # verify install/config/network/auth
```

Every command in this CLI is listed in the repo's `SURFACE.txt`; that file is
the authoritative contract and is enforced in CI. New flags or commands won't
silently disappear between versions without a major version bump.

## Field not exposed by a flag? Use `--json`

The render command exposes ~30 of the most common Urlbox API options as
flags (`--width`, `--full-page`, `--block-ads`, etc.). **For anything else,
use `--json '{...}'`.** Every API option in the schema is settable this
way — including options the dashboard exposes that the CLI hasn't grown a
dedicated flag for yet (e.g. `video_scroll`, `video_scroll_distance`,
`save_html`, `cookies`, hundreds more).

```sh
# Discover the full set of valid keys (152 fields as of v0.8.0)
urlbox schema render --jq '.data.properties | keys'

# Drill into one field's contract
urlbox schema render --jq '.data.properties.video_scroll'

# Pass any option through --json
urlbox render --json '{"url":"https://example.com","video_scroll":true,"video_scroll_distance":1200,"video_scroll_duration":1800}'

# --json composes with flags (flags win on conflict)
urlbox render https://example.com --json '{"video_scroll":true}' --output movie.mp4 --format mp4
```

As of v0.9.0 the embedded JSON Schema is **documentation, not a gate**:
`--json` is a passthrough — anything you pass goes to the Urlbox API
verbatim, and the API is the authoritative validator. Typos that look
like documented options trigger a stderr warning ("did you mean:
<closest>?") but the request still goes through; agents read the
warning and decide whether to re-run with the suggested spelling.

This means **`--json` always works for any current or future API
option** — no need to wait for a CLI release when the dashboard adds
something new. The trade-off: errors arrive after a network round-trip
instead of locally.

**Decision tree for agents:** if the option you need is in
`urlbox render --help`, use the typed flag (fast local feedback,
strict enum/type validation). Otherwise reach for `--json '{...}'`
(passthrough, the API decides). Never guess — `urlbox schema render`
documents the well-known options, but the API accepts more.

## Available commands

| Command                          | Purpose                                                |
|----------------------------------|--------------------------------------------------------|
| `urlbox auth`                    | Save API secret (`--api-secret <secret>`; or interactive) |
| `urlbox commands`                | List every command + flag                              |
| `urlbox config get <key>`        | Read a config value                                    |
| `urlbox config set <key> <val>`  | Write a config value                                   |
| `urlbox config path`             | Print the resolved config-file path                    |
| `urlbox config profile create`   | Create a named profile (`--api-host`, `--api-secret`, ...) |
| `urlbox config profile list`     | List all profiles, mark the default                    |
| `urlbox config profile default`  | Switch the default profile                             |
| `urlbox config profile delete`   | Delete a non-default profile                           |
| `urlbox dashboard`               | Open the Urlbox dashboard in the user's browser        |
| `urlbox doctor`                  | Diagnose install, config, network, credentials         |
| `urlbox link`                    | Generate an HMAC-signed render URL with no API call    |
| `urlbox render <url>`            | Capture a screenshot, PDF, or video of a web page      |
| `urlbox screenshot <url>`        | Alias for `render --format png` (also `urlbox shot`)   |
| `urlbox pdf <url>`               | Alias for `render --format pdf --full-page`            |
| `urlbox video <url>`             | Alias for `render --format mp4`                        |
| `urlbox schema render`           | Print the JSON Schema for the render request payload   |
| `urlbox skill`                   | Show this skill content (`urlbox skill show`)          |
| `urlbox status <renderId>`       | Check / poll the status of an async render             |
| `urlbox upgrade`                 | Self-update via detected install method                |

## render: capture a URL

`urlbox render <url>` captures a screenshot, PDF, or video. Three input
modes are supported and merge in this order (later wins):
**preset → --json → --flag values**.

### Quickstart

```sh
# Simplest: positional URL, default format (png via screenshot alias)
urlbox screenshot https://example.com --output home.png

# Explicit render with format flag
urlbox render https://example.com --format pdf --full-page

# Full payload via --json (preferred for non-trivial config — agents use this)
urlbox render --json '{"url":"https://example.com","format":"png","width":1920,"full_page":true}'

# --json from stdin or a file
echo '{"url":"https://example.com"}' | urlbox render --json -
urlbox render --json @opts.json

# Built-in presets layer in defaults
urlbox render https://example.com --preset mobile          # iPhone viewport
urlbox render https://example.com --preset desktop         # 1920x1080
urlbox render https://example.com --preset pdf-a4          # PDF + A4
urlbox render https://example.com --preset article         # block ads, retina, mostrequestsfinished (news/article workflows)

# Preview the validated payload without calling the API (no credit burn)
urlbox render https://example.com --format pdf --dry-run

# Generate a copy-pasteable curl command (secret redacted)
urlbox render https://example.com --curl

# Save the rendered asset to disk (path sandboxed to CWD)
urlbox render https://example.com --output screenshot.png

# Open the result in the browser after rendering
urlbox render https://example.com --open

# Async: queue and return a renderId; poll with `urlbox status <id> --wait`
urlbox render https://example.com --async --webhook-url https://hooks.example/cb
```

### Self-discovery

- `urlbox schema render` — full JSON Schema for valid render options.
- `urlbox schema render --jq '.data.properties.url'` — drill into one field.
- `urlbox render --help --agent` — structured JSON help for any command.

### Retries

The CLI retries automatically on 429 / 5xx / generic network errors
(3 attempts, 1s/2s/4s backoff with ±20% jitter, respects Retry-After).
Disable with `--no-retry`; cap with `--max-retries N`.

**Timeouts are NOT retried.** A `context.DeadlineExceeded` produces an
error envelope with `code: "timeout"` (literal string `"timeout"`) and
a hint listing three recovery paths: retry the same command, raise
`--timeout`, or switch to `--async --webhook-url`. The agent picks the
strategy — heavy renders are slow on every attempt, so silent auto-retry
rarely helps.

### Timeout

`--timeout duration` (default `60s`) sets the per-attempt budget for the
render call. Raise it for heavy `--full-page` renders or news/article
sites. For very long renders, prefer `--async --webhook-url <url>` so
the CLI doesn't block.

### Upstream errors

If the rendered page itself returned an HTTP error (login wall, captcha,
rate limit), `data.upstreamOk` is `false` and `data.upstreamStatus` carries
the code. The summary line warns. Don't treat the bytes as authoritative —
the render likely captured a captcha page rather than the target content.

### Error codes (closed set)

| Code         | Exit | Meaning                                                    |
|--------------|------|------------------------------------------------------------|
| `usage`      | 1    | bad flags / missing url                                    |
| `validation` | 2    | payload failed schema validation; see `hint` for the fix   |
| `auth`       | 3    | missing/invalid API secret; run `urlbox auth --api-secret` |
| `forbidden`  | 4    | account/plan doesn't allow this feature                    |
| `not_found`  | 5    | endpoint or render ID unknown                              |
| `rate_limit` | 6    | retry budget exhausted; back off and retry                 |
| `conflict`   | 7    | request conflicts with in-flight state                     |
| `server`     | 10   | Urlbox API error; try again later                          |
| `network`    | 11   | no connection / DNS error; run `urlbox doctor`             |
| `"timeout"`  | 11   | render exceeded `--timeout` budget; raise it or use `--async` |

### `urlbox schema render`

Print the JSON Schema describing the render request payload. Use this to
discover every valid option and its type.

- `urlbox schema render` — full schema in the standard envelope.
- `urlbox schema render --output-format quiet` — raw schema only (no envelope).
- `urlbox schema render --jq '.data.properties.url'` — drill into a specific field.

## link: sign a render URL (no API call)

`urlbox link` produces an HMAC-SHA256 signed render URL **without** calling
the API. It's pure local crypto — useful for embedding URLs in templates,
emails, or static sites, and for inspecting the canonical query a render
request would use.

URL shape: `https://api.urlbox.com/v1/<api_key>/<token>/<format>?<canonical_query>`

```sh
# Minimal — positional URL via --url
urlbox link --url https://example.com

# Full payload via --json (same merge rules as render: --json then flags)
urlbox link --json '{"url":"https://example.com","width":1920,"full_page":true}' --format png

# Raw URL only (one line, no envelope) — pipe-friendly for templates
urlbox link --url https://example.com --output-format quiet
```

Requires both the **publishable API key** AND the **API secret**. If a
profile is missing one, the error envelope tells you which (`auth` code).
If you actually want the rendered asset, use `urlbox render` instead —
`link` never touches the network.

## status: check an async render

`urlbox status <renderId>` looks up the state of an async render queued by
`urlbox render --async`. One-shot by default; pass `--wait` to poll until
the render reaches a terminal state.

```sh
# One-shot snapshot
urlbox status ps_abc123

# Poll every 5s until terminal (default --timeout 60s)
urlbox status ps_abc123 --wait

# Custom cadence — long renders, slower polling to spare API quota
urlbox status ps_abc123 --wait --timeout 5m --poll-interval 10s
```

Terminal statuses:

- `succeeded` → exit 0, `data.renderUrl` points at the asset.
- `failed` / `error` → exit 10, envelope's `error` describes why.

Non-terminal states (`created`, `retrying`, `processing`) without `--wait`
return `ok: true` with a breadcrumb suggesting `urlbox status <id> --wait`.
With `--wait`, the deadline is governed by `--timeout`; if it elapses
before a terminal state, the envelope is `usage` / exit 1 with a hint to
raise `--timeout` or re-run later.

## dashboard: open the Urlbox dashboard

`urlbox dashboard` opens https://urlbox.com/dashboard in the user's
default browser. On headless boxes (no `DISPLAY` / `WAYLAND_DISPLAY` on
Linux, or unsupported OS) it prints the URL to stderr instead and still
emits the standard envelope on stdout — so agents and pipelines always
get `data.url` regardless of host.

```sh
urlbox dashboard
urlbox dashboard --output-format json --jq '.data.url'
```

Exit codes: 0 on success (browser launched or URL printed); 10 if the OS
browser handler returned an error (URL is in the hint).

## Common workflows

```sh
# Fire-and-poll: kick off an async render, then wait for it
urlbox render https://example.com --async --output-format quiet \
  | jq -r .data.renderId \
  | xargs urlbox status --wait

# Sign a URL for a CDN / template (no API call, no credit burn)
urlbox link --json '{"url":"https://example.com","width":1920}' --output-format quiet

# Open the dashboard
urlbox dashboard
```

## Validation contract

The CLI's embedded JSON Schema (`schema/render.json`, ~150 documented
options) is **documentation, not a gate** — it powers typed flags,
help text, fuzzy-typo suggestions, and `urlbox schema render`. The
Urlbox API is the authoritative validator.

**Typed flags** (`--width`, `--format`, `--wait-until`, ...) get
strict client-side validation:
- Type checks via Cobra (e.g. `--width=abc` → local error).
- Enum checks against the schema (e.g. `--wait-until invalid` → local
  error with "Allowed: ..." hint).

**`--json` is a passthrough:**
- Unknown keys, known-but-bad-type values, missing-required fields —
  all go to the API. Structured `InvalidOptions` responses (with
  field names) come back when the API rejects.
- Unknown keys that fuzzy-match a documented option emit a stderr
  warning (`warning: unknown option "fromat" — did you mean
  "format"?`) but the request is still sent verbatim. Agents read
  the warning and decide whether to re-run with the suggestion.

**Silent-passthrough caveat:** unknown `--json` keys with no fuzzy
match to a documented option pass to the API silently — the CLI does
NOT warn. If you need to verify which keys the API recognized, use
`urlbox schema render` to check the documented set, or inspect the
returned envelope's `data` for the API's interpretation.

Local hard errors (always reject before any API call):
1. Payloads larger than 1 MiB.
2. URL-like fields with control characters (below 0x20 or 0x7F).
3. Malformed JSON.

All local validation failures use error code `"validation"` (exit
code 2). API-side validation failures arrive as exit code 2 with the
same `"validation"` envelope, plus the API's structured field-level
detail.

## Configuration

The CLI resolves credentials from this priority chain (highest first):

1. CLI flag (`--profile <name>`)
2. Env vars (`URLBOX_API_SECRET`, `URLBOX_PROFILE`, `URLBOX_API_HOST`)
3. The named profile (selected via `--profile`, `URLBOX_PROFILE`, or `default_profile`)
4. Per-repo overrides at `.urlbox/config.json` (walks from CWD up; stops at `$HOME`)
5. The global default profile in `~/.config/urlbox/config.json`

The CLI ships **production-host only**. To target a custom host, set `api_host`
on a profile directly: `urlbox --profile <name> config set api_host https://...`.

### Multiple profiles

`config set` and `config get` adapt to the profile count:

- **0 profiles:** `config set` errors with "No profiles configured" — run `urlbox auth --api-secret <secret>` to bootstrap.
- **1 profile:** `--profile` is implicit; `urlbox config set api_secret sk_xxx` Just Works.
- **2+ profiles:** `--profile` is required; the error lists configured names.

The top-level `default_profile` key is exempt from this rule, but `config set
default_profile <name>` requires `<name>` to exist as a profile.

## Authentication

### For agents and CI (non-interactive)

All options below are agent-safe — none prompts. Listed in order of
secret-hygiene; prefer the higher items.

```sh
# A — pipe on stdin (no argv leak, no shell-history exposure)
printf %s "$URLBOX_API_SECRET" | urlbox auth --api-secret-stdin

# B — read from a file (handy when the secret already lives on disk)
urlbox auth --api-secret-file /run/secrets/urlbox

# C — env var (never touches the config file)
URLBOX_API_SECRET=<secret> urlbox render <url>

# D — argv flag (leaks into `ps` and shell history; emits a TTY warning)
urlbox auth --api-secret <secret>

urlbox doctor --output-format json # JSON envelope: ok/not-ok
```

`--api-secret-stdin` and `--api-secret-file` are accepted by every
command that takes `--api-secret` (auth, render, status, link,
config profile create, and the render aliases screenshot/pdf/video).
Mutually exclusive — pass at most one.

### For humans on a TTY

```sh
urlbox auth                        # prompts once for the secret with masked echo
```

Saves to `~/.config/urlbox/config.json` (mode 0600), under the default profile
(creates one if none exists). Verify with `urlbox doctor`.

## Coming next

Future surface additions land here as they ship; the skill file gains
examples and decision trees alongside each new command.
