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

## Available commands (Phase 1)

| Command            | Purpose                                                |
|--------------------|--------------------------------------------------------|
| `urlbox auth`      | Set API key (`--api-key <key>`)                        |
| `urlbox commands`  | List every command + flag                              |
| `urlbox doctor`    | Diagnose install, config, network, credentials         |
| `urlbox skill`     | Show this skill content (`urlbox skill show`)          |
| `urlbox upgrade`   | Self-update via detected install method                |

## Authentication

Two ways to provide credentials, env var takes precedence:

1. `URLBOX_API_SECRET=sec_xxx` (recommended for CI / containers)
2. `urlbox auth --api-key sec_xxx` → writes to `~/.config/urlbox/config.json` (0600)

Verify with `urlbox doctor`.

## Coming next (Phase 4+)

- `urlbox render <url>` — sync/async screenshot, PDF, video
- `urlbox status <id>` — async render status
- `urlbox link <url>` — HMAC-signed render URL (no API call)

When these arrive, this skill file will gain examples and decision trees.
