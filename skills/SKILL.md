# Urlbox CLI Skill

This skill helps an agent use the `urlbox` CLI to render screenshots, PDFs, and
videos via the Urlbox API. The CLI is a thin, validated shell over the API:
every command produces a JSON envelope with a closed-set error code, exit code,
and breadcrumbs pointing at the next reasonable command.

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

## Available commands

| Command                          | Purpose                                                |
|----------------------------------|--------------------------------------------------------|
| `urlbox auth`                    | Save API secret (`--api-key <secret>`; or interactive) |
| `urlbox commands`                | List every command + flag                              |
| `urlbox config get <key>`        | Read a config value                                    |
| `urlbox config set <key> <val>`  | Write a config value                                   |
| `urlbox config path`             | Print the resolved config-file path                    |
| `urlbox config profile create`   | Create a named profile (`--api-host`, `--api-secret`, ...) |
| `urlbox config profile list`     | List all profiles, mark the default                    |
| `urlbox config profile default`  | Switch the default profile                             |
| `urlbox config profile delete`   | Delete a non-default profile                           |
| `urlbox doctor`                  | Diagnose install, config, network, credentials         |
| `urlbox schema render`           | Print the JSON Schema for the render request payload   |
| `urlbox skill`                   | Show this skill content (`urlbox skill show`)          |
| `urlbox upgrade`                 | Self-update via detected install method                |

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

- **0 profiles:** `config set` errors with "No profiles configured" — run `urlbox auth --api-key <secret>` to bootstrap.
- **1 profile:** `--profile` is implicit; `urlbox config set api_secret sk_xxx` Just Works.
- **2+ profiles:** `--profile` is required; the error lists configured names.

The top-level `default_profile` key is exempt from this rule, but `config set
default_profile <name>` requires `<name>` to exist as a profile.

## Authentication

### For agents and CI (preferred — non-interactive)

```sh
urlbox auth --api-key <secret>     # one-liner, never prompts
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

- `urlbox render <url>` — sync/async screenshot, PDF, video
- `urlbox status <id>` — async render status
- `urlbox link <url>` — HMAC-signed render URL (no API call)

When these arrive, this skill file will gain examples and decision trees.
