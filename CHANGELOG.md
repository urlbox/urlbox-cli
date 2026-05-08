# Changelog

All notable changes to the `urlbox` CLI are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project follows [SemVer](https://semver.org/spec/v2.0.0.html).

## v1.0.0 — 2026-05-08 — v1 GA

The first general-availability release of the Urlbox CLI. Closes the
original 5-phase spec end-to-end: agent contract, schema validation,
multi-profile config + auth, render + async, link, status, dashboard,
plus the four-target skill-install matrix (claude-code, cursor, codex,
opencode). Twenty-one commits since v0.9.0; zero new third-party
dependencies; surface frozen by `SURFACE.txt`.

### Added — new commands

- **`urlbox link`** — generate an HMAC-SHA256 signed render URL using
  local crypto only (no API call). Pure function over the canonical
  query string, lowercase-hex token. URL shape:
  `https://api.urlbox.com/v1/<api_key>/<token>/<format>?<query>`. Mirrors
  the customer-facing JS / PHP / Python / Ruby reference impls exactly;
  pinned tests against Python-derived fixtures cover simple, multi-key
  sorted, special-character, multi-value-array, null-value, and
  nested-object cases. Quiet mode emits the bare URL on stdout (pipeline
  friendly).
- **`urlbox status <renderId>`** — look up the status of an async render
  via `api.Client.Status`. Single-shot by default; `--wait` polls every
  `--poll-interval` (default 5s) until terminal (`succeeded`, `error`,
  `failed`) or `--timeout` (default 60s) elapses. `--profile` is
  threaded through to `config.Resolve`. Polling uses the new
  `internal/clock` package so tests run in microseconds.
- **`urlbox dashboard`** — opens `https://urlbox.com/dashboard` in the
  user's default browser. Falls back to printing the URL on stderr (and
  emitting the standard envelope on stdout) when no graphical session is
  detected (Linux without `DISPLAY`/`WAYLAND_DISPLAY`, or unsupported
  OS).

### Added — skill-install matrix

- `urlbox skill install --target {cursor,codex,opencode}` joins the
  existing `claude-code` target. Verified paths against each tool's
  current docs:
  - `cursor` → `~/.cursor/skills/urlbox/SKILL.md` (or
    `.cursor/skills/urlbox/SKILL.md` for project scope)
  - `codex` → `~/.agents/skills/urlbox/SKILL.md` (cross-agent dir, per
    Codex docs) / `.agents/skills/urlbox/SKILL.md`
  - `opencode` → `~/.config/opencode/skills/urlbox/SKILL.md` /
    `.opencode/skills/urlbox/SKILL.md`

### Added — agent UX

- `Did you mean "X"?` typo suggestions on unknown commands and flags
  (Fizzy item 5). `urlbox rendr` → suggests `render`; `urlbox render
  --output-formart json` → suggests `--output-format`. Reuses
  `internal/validation.ClosestMatch` so behavior is consistent with the
  v0.9.0 `--json` typo suggestions.
- Static regression guard `TestNoEmptyCLIErrorHints` (Fizzy item 1):
  every production `output.NewCLIError(code, msg, hint)` call must pass
  a non-empty hint. Build fails if a future commit drops one. Filled
  seven previously-empty hints with concrete recovery paths.

### Fixed

- **Security:** `urlbox render --output` rejects pre-existing leaf
  symlinks (`os.Lstat` probe before open). Closes a
  "render-to-planted-symlink" arbitrary-file-write hazard. Parent-
  directory symlinks were already defended in v0.7.0; this completes
  the story.
- `RetryDo` returns the last observed response on context cancel
  mid-sleep instead of `nil`, with body replaced by `http.NoBody` for
  safe reads.
- `urlbox auth` interactive-prompt newline now lands on the
  cobra-injected stderr writer (test-capturable; matches the rest of
  the codebase).
- `urlbox render --profile <name>` actually honours `--profile`. The
  flag was registered persistently on root from v0.5.0 but render's
  `buildRenderClient` was missing the wire-through to
  `config.Resolve`. Status, link, and the new commands had it; render
  was the odd one out.

### Changed

- `--output-format` now rejects unknown values explicitly with `code:
  "usage"` (exit 1) instead of silently falling through to JSON.
  `--output-format ndjson` returns a clear "not yet implemented; coming
  in a future release alongside `urlbox batch`" message — previously it
  produced JSON and confused users expecting NDJSON.
- `urlbox upgrade` now emits the standard envelope on stdout while
  streaming package-manager progress on stderr. Agents piping
  `--output-format json` get a structured description of the install
  method, current version, binary path, and a `verify` breadcrumb;
  humans see the brew/scoop/npm/go output as before.
- `urlbox render --json '<malformed>'` exits with code `validation`
  (exit 2) instead of `usage` (exit 1). Matches `link`'s behavior and
  `internal/validation.ValidatePayload`'s convention. Malformed JSON is
  a payload-content problem, not a flag-shape problem.

### Tests / Coverage

- `internal/config` coverage: 85.0% → 88.8%. New tests cover `Save`'s
  rename / mkdirAll / createTemp failure branches, `Load`'s
  malformed-JSON branch, and `APISecretSource`'s legacy-only fallback.
- `internal/validation` coverage: 92.2% → 95.0%. `loadSchema` factored
  into a testable `loadSchemaFrom(b []byte)` so decode/register/compile
  error branches are reachable without fighting `sync.Once`.
- New `internal/clock` package (~70 lines, stdlib-only) with `Clock`
  interface and a race-clean `FakeClock` driving the status `--wait`
  tests in microseconds.

### Documentation

- New `urlbox link` / `urlbox status` / `urlbox dashboard` sections in
  `skills/SKILL.md`, `README.md`, and `npm/README.md`. SKILL.md gains a
  "Common workflows" section (fire-and-poll, sign-a-URL, open-dashboard).
- Skill install table in SKILL.md and README documents all four
  targets with verified upstream paths.
- `RetryDo`, `loadSchema`, `defaultAuthSecretReader` godoc updated to
  reflect new contracts.

### v1 surface stability promise

`SURFACE.txt` is the contract. Adding new commands or flags is
backwards-compatible. Removing or renaming anything in `SURFACE.txt`
requires a major version bump. From v1.0 onward, every breaking change
to commands, flags, envelope shape, or exit codes ships in a 2.0
release, not a point release.

### Exit code mapping (frozen for v1.0)

The closed-set error codes and their exit codes:

| Exit code | Code           |
|-----------|----------------|
| 0         | success        |
| 1         | usage          |
| 2         | validation     |
| 3         | auth           |
| 4         | forbidden      |
| 5         | not_found      |
| 6         | rate_limit     |
| 7         | conflict       |
| 10        | server         |
| 11        | network / timeout |

This mapping has been stable since v0.3.0; v1.0 freezes it.

### Known limitations (post-1.0 polish, not blockers)

- `did_you_mean` for unknown commands only walks immediate subcommands
  of root. `urlbox config gett` falls through to the generic hint
  without a suggestion. Documented in `suggestUnknownCommand`'s godoc.
- `did_you_mean` for unknown flags unions all flag names across the
  command tree. A typo on one command can match a flag from an
  unrelated command (e.g. `urlbox auth status --widht` would suggest
  `--width`, a render flag). Documented in `suggestUnknownFlag`'s
  godoc.
- Both helpers parse cobra/pflag error strings via `strings.Index`. A
  `go get -u cobra/pflag` PR should re-verify the prefix constants in
  `internal/cmd/root.go`.

### Fizzy verifications closed

- Item 1 (breadcrumbs/hint on every error) — regression test landed.
- Item 2 (numeric ID parent-scoping) — closed by Phase 5 design (opaque
  `renderId`, no integer ambiguity).
- Item 3 (skill install non-interactive) — closed in v0.8.1.
- Item 4 (identifier consistency) — closed by Phase 5 design (`link`
  emits a `renderId`-shaped output that `status` accepts verbatim).
- Item 5 (did_you_mean) — regression test landed.

### Convention H

`make smoke` green against `api.urlbox.com`: 6/6 tests pass (sync
render, async queue, auth fail, three v0.9.0 passthrough cases).

### Notes

- Twenty-one commits since v0.9.0 across three workstreams (sweep, link/
  status/dashboard, skill targets) plus two bulletproofing follow-ups.
- Zero new third-party dependencies. `go.mod` and `go.sum` unchanged
  since `cc1bbd0` (v0.9.0 merge).
- Cross-compile verified for `linux/amd64`, `linux/arm64`,
  `windows/amd64`. Race + stress (count=10) clean.

## v0.9.0 — 2026-05-05

Schema-as-documentation. The embedded `schema/render.json` becomes
informational, the Urlbox API becomes the authoritative validator, and
`--json` becomes a passthrough that always works for any current or future
API option.

### BREAKING

- **`--json` no longer gates unknown keys or known-bad types.** Anything
  passed via `--json` flows to the API verbatim; the API returns structured
  field-level errors. Previously the CLI rejected unknown keys and bad types
  locally with `code: "validation"` (exit 2). If you relied on the local
  rejection — e.g. catching typos in CI — switch to a `--dry-run` followed
  by `urlbox schema render` introspection, or grep stderr for
  `^warning:` lines (which still surface fuzzy-matchable typos).
- **Hand-patched `video_scroll*` schema entries removed.** They were never
  in the dashboard's docs source (`apps/dashboard/src/lib/options.ts`) and
  flow through the new `--json` passthrough now. To use them:
  `urlbox render <url> --json '{"video_scroll": true, "video_scroll_distance": 800}'`.

### Added

- Embedded `schema/render.json` regenerated from urlbox-mono's generator
  with `additionalProperties: true`. Schema is now strictly informational —
  it powers typed flags, `--help` text, `urlbox schema render`, and
  fuzzy-typo suggestions, but does not gate API requests.
- Stderr warnings for `--json` keys that fuzzy-match a documented option:
  `warning: unknown option "fromat" — did you mean "format"? (sending
  verbatim; the API will decide)`. Multiple typos collapse into one summary
  warning. Keys with no fuzzy match pass silently.
- Typed-flag enum enforcement at the Cobra layer for `--format` and
  `--wait-until`. Invalid values error locally with an `Allowed: ...` hint
  (and a "Did you mean: ..." prefix when fuzzy-matchable). Same `--json`
  values are passthrough — the contract split is explicit and consistent.

### Changed

- `internal/validation.ValidatePayload` no longer calls `schema.Validate()`.
  The pipeline now runs: `ResetWarnings → SanitizeRaw (size) → JSON parse →
  field-sanitize (URL control chars) → recordUnknownKeyWarnings (warn,
  never error)`. Local hard errors are unchanged.
- New exported helpers: `validation.ResetWarnings()` and
  `validation.LastWarnings()` for callers that need to drain warnings to
  stderr (the render command does this after every `ValidatePayload`).
- New exported helper: `api.EnumSliceFor(field)` returns the enum values
  for a schema field as `[]string`. Used by the Cobra-layer enum validator.

### Documentation

- New "Validation contract" section in `SKILL.md`, `README.md`, and
  `npm/README.md` explaining the typed-flag-vs-`--json` model, the warning
  behavior, and the local-hard-error list.
- `TestSkill_DocumentsRenderSurface` extended to pin "Validation contract"
  and "passthrough" as required strings (regression guard against silent
  doc regressions).

### Notes

- The CLI release ships independently of the urlbox-mono PR landing the
  generator and auto-PR workflow. The schema in this release was generated
  locally from urlbox-mono's generator branch run against `main`. Once the
  upstream PR merges, future schema regenerations flow automatically (a PR
  opens in this repo whenever `apps/dashboard/src/lib/options.ts` changes).
- Schema completeness debt: ~50 options visible in the dashboard UI are
  not yet in `apps/dashboard/src/lib/options.ts`. They work today via
  `--json`. The dashboard team backfills as they go; the auto-PR workflow
  picks them up incrementally.

## v0.8.1 — 2026-05-01

Agent self-bootstrap — closes the discovery gap where the embedded SKILL.md
lived only in the Go binary, with no file on disk for tooling like Claude
Code to auto-load.

### Added

- `urlbox skill install --target claude-code --scope <user|project> [--yes]`
  writes the embedded `SKILL.md` to the well-known skill directory of the
  agent tooling. User scope: `~/.claude/skills/urlbox/SKILL.md`. Project
  scope: `.claude/skills/urlbox/SKILL.md` (commits to git so teammates
  inherit). On a TTY the command prompts; with `--yes` it runs cleanly
  under `< /dev/null` so agents can self-bootstrap.
- New top-of-`SKILL.md` "Bootstrap" section gives agents the decision tree
  for installation: ask the user user-vs-project, run the right command,
  default to user/global if unsure. Agents reading SKILL.md now know
  exactly how to register themselves with the user's tooling.
- Cursor / Codex / OpenCode targets are listed as TBD (paths vary per
  release; ship as those tools stabilize their skill-discovery conventions).

### Internal

- `supportedSkillTargets` map in `internal/cmd/skill.go` is the single
  source of truth for the target → path matrix. Adding a new target is
  one map entry + a test case.
- File perms `0o700` / `0o600` (gosec G301/G306). Skill files are
  user-private docs, not world-readable.

## v0.8.0 — 2026-05-01

Render UX & reliability hardening — addresses every friction point an
agentic field test of v0.7.0 surfaced.

### Breaking

- `--timeout` flag changed type from `int` (milliseconds, mapped to the
  Urlbox API's hard page-timeout option) to `duration` (the CLI's
  per-attempt render budget — see Added below). Old invocations like
  `--timeout 30000` will fail at flag parsing because `30000` is not a
  valid duration string. The hard page-timeout API option is still
  reachable via `--json '{"timeout": 30000}'`. Acceptable break: zero
  reported v0.7.0 users.

### Fixed

- `--wait-until` help text now lists the API's real enum values
  (`domloaded`, `mostrequestsfinished`, `requestsfinished`, `loaded`).
  v0.7.0 listed Puppeteer-style values that the API rejects — a 100%
  failure mode for any agent that trusted `--help`.
- `--output` envelopes now include `upstreamOk` and `upstreamStatus`
  when the API surfaces the rendered page's HTTP status. Catches the
  silent-failure mode where Reuters returned a 176 KB Datadome captcha
  PNG and the CLI reported `ok: true`.
- Render timeouts now produce a dedicated `code: "timeout"` envelope
  with an actionable hint listing three recovery paths: retry the same
  command, raise `--timeout`, or switch to `--async --webhook-url`.
  v0.7.0 returned `code: "network"` with the misleading "Check your
  internet connection" hint.
- Heavy `--full-page` renders no longer eat their entire retry budget
  on the first attempt. v0.7.0 wrapped runRender in a fixed 60s context
  with auto-retry on timeout — but each retry inherited a near-dead
  context and failed immediately. v0.8.0 drops auto-retry-on-timeout
  in favor of fail-fast + an honest hint, putting the recovery decision
  in the caller's hands.
- `schema/render.json` now accepts the five `video_scroll*` fields the
  dashboard exposes (`video_scroll`, `video_scroll_back`,
  `video_scroll_distance`, `video_scroll_duration`,
  `video_scroll_back_duration`). v0.7.0 had `additionalProperties: false`
  blocking them. Root cause: the schema-sync pipeline lives on
  urlbox-mono's `feature/urlbox-cli` branch and never landed on main; the
  dashboard later refactored its option metadata into per-component files,
  breaking the original allowlist source. Hand-patched until the broader
  sync rebuild lands.
- `urlbox auth` now points users at the dashboard URL where they can
  copy their API secret (https://urlbox.com/dashboard/projects). The URL
  appears in `--help`, the interactive pre-prompt, and the missing-secret
  error envelope's hint. Field report: agents were inventing wrong URLs
  ("urlbox.com/dashboard/api-secrets") because the CLI never said where
  the secret actually lives.
- `skills/SKILL.md` now teaches the `--json` fallback prominently. v0.7.0
  buried the "any field not in a flag is settable via --json" guidance
  deep in the render section; agents bounced off "no flag for that field"
  walls. New section "Field not exposed by a flag? Use `--json`" lands
  right after Discovery, with a decision tree and worked examples.

### Added

- `urlbox render --timeout duration` (default `60s`) — per-attempt
  budget for the render call. Raise for heavy pages, or prefer
  `--async --webhook-url` for very long renders.
- `urlbox render --preset article` — built-in preset for news/article
  workflows. Bundles `--block-ads --retina --wait-until mostrequestsfinished`
  and explicitly disables `--full-page` (heavy-page timeouts are the
  most common failure mode).
- New error code `timeout` (exit code 11, same family as `network`).
  Agents can branch on the `code` field to distinguish a render that
  exceeded its budget from a connection problem.

### Internal

- New `internal/api.EnumValuesFor(field)` helper reads the embedded
  render schema once and returns the comma-separated enum list. Used
  by the `--wait-until` help text; future enum-bound flags should
  use it too.
- New `internal/cmd.networkHint(err, isRenderCall, timeout)` classifies
  network-class errors and returns the right hint for the call site,
  echoing the actual timeout value so agents see exactly how long they
  waited.
- `internal/api.RetryDo` short-circuits on `output.ErrTimeout` — the
  retry budget is reserved for transient failures (5xx / network blip),
  not deadline-exhausted attempts.

## v0.7.0 — 2026-05-01

The `urlbox render` command ships. The CLI can now actually render screenshots,
PDFs, and videos against `api.urlbox.com`.

### Added

- `urlbox render <url>` — capture a screenshot, PDF, or video via the Urlbox API.
  Supports ~25 convenience flags (`--format`, `--width`, `--height`, `--full-page`,
  `--quality`, `--block-ads`, `--dark-mode`, `--retina`, `--wait-until`,
  `--user-agent`, `--webhook-url`, ...).
- `--json` accepts a literal JSON payload, `-` for stdin, or `@path` for a
  file. Merge order is **preset < json < flags** (last writer wins).
- `--dry-run` validates and prints the merged payload without calling the API
  (no credit burn).
- `--curl` prints a copy-pasteable curl command equivalent to the request,
  with the API secret redacted as `$URLBOX_API_SECRET`.
- `--output <path>` saves the rendered asset to disk. Path is canonicalized
  and asserted to stay under the current working directory — parent escapes
  (`../`), absolute paths outside CWD, and symlinks pointing outside CWD
  are all rejected with a `validation` error.
- `--open` launches the rendered URL in the user's default browser
  (`open` on macOS, `xdg-open` on Linux, `cmd /c start` on Windows).
- `--async` queues a render and returns immediately with a `renderId`.
  Pair with `--webhook-url` for production workflows.
- `--no-retry` / `--max-retries N` control the retry budget.
- `--preset <name>` for `mobile` (375×812 + iPhone UA), `desktop` (1920×1080),
  and `pdf-a4` (PDF format + A4 page size). Unknown name returns a usage
  error listing the available presets.
- Three thin alias commands sharing the entire render pipeline:
  - `urlbox screenshot <url>` (also `urlbox shot`) — `--format png` default.
  - `urlbox pdf <url>` — `--format pdf --full-page` default.
  - `urlbox video <url>` — `--format mp4` default.
- New `make smoke` target — Go tests gated behind a `smoke` build tag,
  exercise the real `api.urlbox.com` (sync render, async queue, auth-fail
  mapping). Required `URLBOX_API_SECRET` env var; never runs under `make ci`.

### Fixed

- The HTTPClient now correctly handles the API's nested error body
  (`{"error":{"code":"...","message":"..."}}`) and re-routes any `ApiKey*`
  error code to the `auth` exit code, regardless of the HTTP status. The
  previous code only handled flat `{"error":"..."}` strings and missed the
  re-routing — the smoke test surfaced this.

### Internal

- `internal/api` now exports `PathSync`, `PathAsync`, `PathStatus` so
  `--curl` and tests reference the same path constants the HTTPClient does.
- New `internal/browser` package: `Opener` interface + `OSOpener` +
  `NoopOpener`. Test-injectable via `cmd.SetOpenerForTest`.
- `internal/cmd/render_output.go`: `resolveOutputPath` (sandbox check
  with EvalSymlinks for parent-symlink defense), `downloadTo` (streaming
  download capped at 5 minutes via context.WithTimeout).

### Known limitations

- `--output` sandbox defends against **parent**-symlink escapes (a
  symlinked directory in the path) but not against a **leaf**-symlink
  (the destination file itself being a pre-existing symlink). The
  `O_TRUNC` open would still follow the symlink. Hardening requires
  `O_NOFOLLOW` or an `Lstat` probe; deferred. Realistic exploit chain
  is narrow (an attacker would need pre-existing write access to plant
  the symlink in CWD).

## v0.6.0 — 2026-04-30

Mostly internal plumbing for the upcoming `urlbox render` command. Two
user-visible changes: the auth flag rename and a doctor bug fix.

### Changed

- `urlbox auth --api-key` is now `urlbox auth --api-secret`. The flag
  matches the dashboard's terminology and the underlying field. v0.5.0
  configs aren't migrated — re-run `urlbox auth --api-secret <secret>`
  after upgrading.
- The auth flow now stores the secret in `Profiles[<name>].APISecret`
  (was `APIKey`). The `APIKey` field is reserved for the publishable
  key used by HMAC URL signing in a later release.
- `ResolveAPIKey()` / `APIKeySource()` in `internal/config` renamed to
  `ResolveAPISecret()` / `APISecretSource()`.

### Fixed

- `urlbox doctor`'s auth check now hits the correct `/v1/user/me`
  endpoint. Previously it was hitting `/v1/account`, which doesn't
  exist on the API — the check would always have failed against a
  real backend (it only "passed" because the test suite stubbed the
  response).
- Doctor's User-Agent now matches the rest of the CLI's HTTP traffic
  (`urlbox-cli/<ver> (<os>/<arch>) go/<go-ver>`).

### Added (internal)

These don't add user-visible commands yet — `urlbox render` lands in
the next release — but they're worth listing because they shape the
public API surface that's coming:

- `internal/api` package: mockable `Client` interface (`Render`,
  `RenderAsync`, `Status`); `HTTPClient` implementation with Bearer
  auth, TLS 1.2 minimum, configurable retry, and full status-code
  mapping (401→auth, 403→forbidden, 404→not_found, 409→conflict,
  429→rate_limit, 5xx→server, network→network).
- Retry with exponential backoff + jitter, respects `Retry-After`
  (seconds and HTTP-date forms). Disable with the upcoming
  `--no-retry` flag.
- Test scaffolding (`internal/api/apitest`) used by every API client
  test; never touches real `api.urlbox.com`.

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
