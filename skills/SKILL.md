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

If they're not sure, default to **user/global** — it's recoverable
(`rm -r ~/.claude/skills/urlbox` removes it).

| Target          | User scope                          | Project scope                |
|-----------------|-------------------------------------|------------------------------|
| `claude-code`   | `~/.claude/skills/urlbox/SKILL.md`  | `.claude/skills/urlbox/SKILL.md` |
| `cursor`        | TBD                                  | TBD                              |
| `codex`         | TBD                                  | TBD                              |
| `opencode`      | TBD                                  | TBD                              |

`urlbox skill install --help` lists supported targets and scopes. Cursor /
Codex / OpenCode targets are TBD — paths vary per release; they ship as
those tools stabilize their skill-discovery conventions.

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

The validator runs locally against the embedded JSON Schema, so
typos are caught before any API call (saves credits + tightens the
feedback loop). Unknown keys produce a `code: "validation"` envelope
with a "did you mean: <closest>" hint.

**Decision tree for agents:** if the option you need is in
`urlbox render --help`, use the flag; otherwise reach for `--json '{...}'`.
Never guess — `urlbox schema render` is the source of truth.

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
| `urlbox doctor`                  | Diagnose install, config, network, credentials         |
| `urlbox render <url>`            | Capture a screenshot, PDF, or video of a web page      |
| `urlbox screenshot <url>`        | Alias for `render --format png` (also `urlbox shot`)   |
| `urlbox pdf <url>`               | Alias for `render --format pdf --full-page`            |
| `urlbox video <url>`             | Alias for `render --format mp4`                        |
| `urlbox schema render`           | Print the JSON Schema for the render request payload   |
| `urlbox skill`                   | Show this skill content (`urlbox skill show`)          |
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

# Async: queue and return a renderId; poll with `urlbox status` (a future release)
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

## Validation

When `--json` is used (Phase 4 onward), payloads are validated before being
sent to the API:

1. **Size cap:** payloads larger than 1 MiB are rejected.
2. **Control characters:** URL-like fields with characters below 0x20 or 0x7F
   are rejected.
3. **Fuzzy correction:** unknown top-level options trigger a "did you mean
   <similar>?" suggestion when a close match exists.
4. **JSON Schema:** the full payload is validated against the embedded render
   JSON Schema (Draft 2020-12).

All validation failures use error code `"validation"` (exit code 2).

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

### For agents and CI (preferred — non-interactive)

```sh
urlbox auth --api-secret <secret>  # one-liner, never prompts
urlbox doctor --output-format json # JSON envelope: ok/not-ok
```

The env var `URLBOX_API_SECRET` takes precedence at runtime over any saved value
and never touches the config file.

### For humans on a TTY

```sh
urlbox auth                        # prompts once for the secret with masked echo
```

Saves to `~/.config/urlbox/config.json` (mode 0600), under the default profile
(creates one if none exists). Verify with `urlbox doctor`.

## Coming next

- `urlbox status <id>` — poll an async render to completion.
- `urlbox link <url>` — generate an HMAC-signed render URL with no API call.

When these arrive, this skill file will gain examples and decision trees.
