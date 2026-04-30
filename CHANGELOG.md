# Changelog

All notable changes to the `urlbox` CLI are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [SemVer](https://semver.org/spec/v2.0.0.html).

## v0.5.0 — 2026-04-30

### Added

- `urlbox config` command tree:
  - `urlbox config path` — print the resolved config-file path.
  - `urlbox config get <key>` / `urlbox config set <key> <value>` — read/write
    a single value with smart profile-target resolution: with one profile
    `--profile` is implicit; with two or more it's required and the error
    lists configured names.
  - `urlbox config profile create <name>` (with `--api-host`, `--api-secret`,
    `--api-key`).
  - `urlbox config profile list` — JSON-friendly, marks the default, masks
    secrets.
  - `urlbox config profile default <name>` — switch the default; refuses
    unknown names.
  - `urlbox config profile delete <name>` — refuses the default or the only
    remaining profile.
- Multi-profile configuration: each profile carries `api_key`, `api_secret`,
  and `api_host`. The priority chain is **flag > env > repo > profile >
  stored default**, with per-field provenance available to callers.
- Per-repo overlay at `.urlbox/config.json`: walks from CWD up, stops at
  `$HOME` so a stray overlay outside your project tree can't silently affect
  production renders.
- New persistent flag: `--profile <name>` on every command. Honors
  `URLBOX_PROFILE` and `URLBOX_API_HOST` environment variables.
- `urlbox auth` (no args, on a TTY) now prompts once for the API secret with
  terminal echo disabled. The non-interactive path (`--api-key <secret>`) is
  unchanged and remains the recommended contract for agents and CI.
- `urlbox schema render` — print the public JSON Schema 2020-12 describing the
  render request payload (152 properties, generated from the dashboard
  allowlist, embedded in the binary). Use `--jq` to drill into specific
  fields.
- Client-side payload validation (active when payloads land in a future
  release): 1 MiB cap, control-character rejection, fuzzy correction for
  unknown options ("did you mean … ?"), and full JSON Schema validation
  against the embedded schema.

### Changed

- `urlbox auth` now writes into the resolved profile (creates one if none
  exists) instead of overwriting a single top-level `api_key`.
- Config saves are now atomic (temp file + rename), preserving 0600 perms.
- Phase 1 single-key config files are migrated transparently in memory and
  rewritten on first save. No user action required.

### Dependencies

- Added direct dep `golang.org/x/term v0.27.0` for masked terminal input.
  Pinned to the same major version as `x/text` to keep the Go floor at
  `1.24.0`.
- Added direct dep `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2` for JSON
  Schema 2020-12 validation.

## v0.3.0 — 2026-04-28

### Added

- Surface stability gate: every command, subcommand, and flag is recorded in
  `SURFACE.txt` and CI fails on breaking changes.
- Discoverability banner on TTY runs.
- `--help --agent` — structured JSON help on any command.
- `--jq <expr>` — built-in jq filter over envelopes (no external `jq` binary).
- `urlbox auth` — save API key to `~/.config/urlbox/config.json` (mode 0600).
- `urlbox doctor` — diagnose install, config, network, and credentials.
- `urlbox commands` — list every command + flag, JSON or text.
- `urlbox skill show` — print the embedded `SKILL.md` agent guide.
- Distribution via Homebrew, npm, and `install.sh`.

## v0.2.0 and earlier

Pre-public release. See git history for details.
