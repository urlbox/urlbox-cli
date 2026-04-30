# Changelog

All notable changes to the `urlbox` CLI are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [SemVer](https://semver.org/spec/v2.0.0.html).

## v0.5.0 — 2026-04-30

This release combines two phases of work that were committed on `main` but not
released separately. v0.4.0 was skipped intentionally — Phase 2 (schema +
validation) and Phase 3 (multi-profile config + auth) ship together.

### Added

#### Multi-profile configuration (Phase 3)

- New `Config` shape: `default_profile` + `profiles` map, each profile carries
  `api_key`, `api_secret`, `api_host`. Phase 1 single-key files migrate
  transparently in memory and are rewritten on first save.
- Atomic save (temp file + rename) preserves 0600 perms.
- Per-repo overlay at `.urlbox/config.json`: walks from CWD up, stops at
  `$HOME` so a stray overlay outside your project tree can never silently
  affect production renders.
- `Resolve(opts)` priority chain: flag > env > repo > profile > stored default.
  Per-field provenance via `Resolved.Source`.
- New persistent flag: `--profile <name>` on every command. Honors
  `URLBOX_PROFILE` and `URLBOX_API_HOST` env vars.

#### `urlbox config` command tree (Phase 3)

- `urlbox config path` — prints the resolved config-file path.
- `urlbox config get <key>` / `urlbox config set <key> <value>` — read/write a
  single value. Smart profile-target resolution: with one profile `--profile`
  is implicit; with two or more it's required and the error lists configured
  names.
- `urlbox config profile create <name>` (with `--api-host`, `--api-secret`,
  `--api-key`).
- `urlbox config profile list` — JSON-friendly, marks the default, masks
  secrets.
- `urlbox config profile default <name>` — switch the default; refuses unknown
  names.
- `urlbox config profile delete <name>` — refuses the default or the only
  remaining profile.

#### `urlbox auth` masked-secret prompt (Phase 3)

- `urlbox auth` (no args, on a TTY) now prompts once for the API secret with
  terminal echo disabled via `golang.org/x/term`. The non-interactive path
  (`--api-key <secret>`) is unchanged and remains the recommended contract for
  agents and CI.

#### Public render JSON Schema (Phase 2)

- `schema/render.json` (152 properties) — JSON Schema 2020-12 generated from
  the dashboard allowlist, embedded via `go:embed`. `additionalProperties:
  false`, `oneOf [url, html]`. No internal hidden options leaked.
- `urlbox schema render` — print the schema in the standard envelope, raw, or
  filtered through `--jq`.

#### Validation (Phase 2)

- `internal/validation` package, exposed when payloads land in Phase 4:
  - 1 MiB payload cap, control-character rejection (<0x20 or 0x7F).
  - Fuzzy correction for unknown top-level options ("did you mean … ?").
  - Full JSON Schema 2020-12 validation against the embedded render schema.

### Changed

- `urlbox auth` now writes into the resolved profile (creates one if none
  exists) instead of overwriting a single top-level `api_key`.
- `Save` is atomic.

### Dependencies

- New direct dep: `golang.org/x/term v0.27.0` — for `term.ReadPassword` /
  `term.IsTerminal`. Pinned to the same major version as `x/text` to keep the
  Go floor at `1.24.0`.
- New direct dep: `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2` — JSON
  Schema 2020-12 validator (Phase 2).

### Removed

- The `Config{APIKey string}` single-key shape from Phase 1. The legacy field
  is read transparently and migrated; no user action required.

## v0.3.0 — 2026-04-28

Phase 1: Agent Contract & Discovery. Surface stability gate (`SURFACE.txt`),
discoverability banner, `--help --agent`, `--jq` filter, `urlbox auth`,
`urlbox doctor`, embedded `SKILL.md`, READMEs. Distribution via Homebrew, npm,
and `install.sh`.

## v0.2.0 and earlier

Pre-public release. See git history for details.
