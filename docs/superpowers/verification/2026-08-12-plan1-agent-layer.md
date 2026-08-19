# Plan 1 — Agent-layer verification (2026-08-12)

Final agent-layer verification for Plan 1 (account management port: login /
logout / whoami / orgs / projects / usage + session config keys). This doc is
the input to the human-layer checklist that gates the Plan 2 release.

## Environment

- Branch: `feat/account-management` (tip `e1db0d3`, working tree contains the
  Task 16 changes: config session keys, docs sweep).
- Binary under test: `bin/urlbox` built from this tree via
  `go build -o bin/urlbox ./cmd/urlbox` (go1.24.0 darwin/arm64).
- Gates before this run: `make ci` green (fmt-check, lint 0 issues,
  `go test -race -cover ./...` all pass, build, surface-check),
  `make surface-snapshot` produced no `SURFACE.txt` diff (the new commands were
  already snapshotted by Tasks 9–15; Task 16 added no new flags/commands).

## Two config states

Constructed in scratch `XDG_CONFIG_HOME` directories.

**Legacy (pre-login shape)** — `api_key` + `api_secret` only:

```json
{
  "default_profile": "default",
  "profiles": {
    "default": {
      "api_key": "ubx_pk_legacy_key",
      "api_secret": "ubx_sk_legacy_secret_abcdef"
    }
  }
}
```

**Post-login shape** — plus `session_token` / `active_org` / `active_project`:

```json
{
  "default_profile": "default",
  "profiles": {
    "default": {
      "api_key": "ubx_pk_legacy_key",
      "api_secret": "ubx_sk_legacy_secret_abcdef",
      "session_token": "sess_tok_post_login_9988776655",
      "active_org": "org_postlogin",
      "active_project": "proj_postlogin"
    }
  }
}
```

## PART A — Pre-existing commands, both states

Every pre-existing command from the brief was run against BOTH config states and
compared byte-for-byte (stdout AND stderr AND exit code). **Any behavioural
difference between the two states for a pre-existing command is a STOP-the-line
bug.** None was found.

| Command                                                      | Exit | stdout parity legacy vs post | Result |
|--------------------------------------------------------------|------|------------------------------|--------|
| `version`                                                    | 0    | identical                    | PASS   |
| `commands --output-format json`                              | 0    | identical                    | PASS   |
| `schema render --output-format json \| head -5`              | 0    | identical                    | PASS   |
| `render https://example.com --dry-run --output-format json`  | 0    | identical                    | PASS   |
| `screenshot https://example.com --dry-run --output-format json` | 0 | identical                    | PASS   |
| `pdf https://example.com --dry-run --output-format json`     | 0    | identical                    | PASS   |
| `render https://example.com --curl`                          | 0    | identical                    | PASS   |
| `link https://example.com`                                   | 0    | identical                    | PASS   |
| `config path`                                                | 0    | path differs by design*      | PASS   |
| `config get api_secret`                                      | 0    | identical                    | PASS   |
| `config profile list`                                        | 0    | identical                    | PASS   |
| `doctor`                                                     | 3    | identical (config-file path normalized)** | PASS |
| `dashboard --output-format json`                             | 0    | identical                    | PASS   |

\* `config path` prints its own `XDG_CONFIG_HOME` path — the only per-state
difference is the path value itself; shape and exit code are identical.

\*\* `doctor` reaches the real API to validate credentials; both states carry the
same (fake) secret so the `auth` check fails identically (HTTP 400 "Api Key does
not exist", exit 3). After normalizing the state-specific `config_file` path, the
two envelopes are byte-identical.

### Representative actual output (legacy state)

`bin/urlbox version`

```json
{
  "ok": true,
  "command": "version",
  "data": { "commit": "none", "date": "unknown", "version": "dev" },
  "summary": "urlbox dev (commit: none, built: unknown)"
}
```

`bin/urlbox render https://example.com --curl`

```json
{
  "ok": true,
  "command": "render",
  "data": {
    "curl": "curl -X POST 'https://api.urlbox.com/v1/screenshot' -H 'Authorization: Bearer $URLBOX_API_SECRET' -H 'Content-Type: application/json' -d '{\"url\":\"https://example.com\"}'"
  },
  "summary": "Equivalent curl command (no API call made)",
  "breadcrumbs": [{ "action": "run", "cmd": "urlbox render <url>" }]
}
```

`bin/urlbox config get api_secret` (masked by default):

```json
{
  "ok": true,
  "command": "config get",
  "data": { "key": "api_secret", "profile": "default", "value": "ubx_…ef" },
  "summary": "api_secret = \"ubx_…ef\""
}
```

`bin/urlbox config profile list`:

```json
{
  "ok": true,
  "command": "config profile list",
  "data": {
    "default": "default",
    "profiles": [
      { "api_host": "", "is_default": true, "masked_secret": "ubx_…ef", "name": "default" }
    ]
  },
  "summary": "1 profile(s); default = \"default\""
}
```

`bin/urlbox doctor` (exit 3 — auth check fails on the fake secret; identical
across both states):

```json
{
  "ok": false,
  "command": "doctor",
  "data": {
    "checks": [
      { "name": "version", "status": "ok", "message": "dev" },
      { "name": "install_method", "status": "warn", "message": "unknown", "hint": "Install via brew, scoop, npm, or curl install.sh for upgrade support" },
      { "name": "config_file", "status": "ok", "message": "<XDG>/urlbox/config.json" },
      { "name": "api_secret", "status": "ok", "message": "configured (file)" },
      { "name": "dns", "status": "ok", "message": "api.urlbox.com resolves" },
      { "name": "api_reachable", "status": "ok", "message": "HTTP 200 from https://api.urlbox.com" },
      { "name": "auth", "status": "fail", "message": "API returned 400: Api Key does not exist", "hint": "Re-run `urlbox auth --api-secret <secret>` with a valid secret, or check `urlbox config get api_secret --reveal` against the dashboard." }
    ],
    "status": "fail"
  },
  "summary": "Some checks failed — see hints for next steps",
  "breadcrumbs": [{ "action": "auth", "cmd": "urlbox auth --api-secret <secret>" }]
}
```

`bin/urlbox dashboard --output-format json`:

```json
{
  "ok": true,
  "command": "dashboard",
  "data": { "url": "https://urlbox.com/dashboard" },
  "summary": "Dashboard URL emitted (no browser launched in machine-readable mode)",
  "breadcrumbs": [{ "action": "copy", "cmd": "https://urlbox.com/dashboard" }]
}
```

## PART B — New commands, logged-out state

`login` was interrupted with SIGINT (Ctrl-C) after the code prints;
`whoami` / `usage` / `orgs list` / `projects list` were run against the legacy
(no-session) state. Each session command must produce the `auth` error envelope,
exit 3.

`bin/urlbox login` (SIGINT after code prints):

```
[stderr]
Your code: HTEB4RLW
Open this URL to continue: https://urlbox.com/device?user_code=HTEB4RLW
Waiting for approval…
[stdout] (empty)
```

Human messages (code + verification URL + "Waiting…") go to **stderr**; stdout
stays empty. On interrupt the config is left untouched — no `session_token` is
written. Real device flow (hits the live API to mint the device code).

| Command (logged-out)                | Exit | Envelope | Result |
|-------------------------------------|------|----------|--------|
| `whoami --output-format json`       | 3    | auth     | PASS   |
| `usage --output-format json`        | 3    | auth     | PASS   |
| `orgs list --output-format json`    | 3    | auth     | PASS   |
| `projects list --output-format json`| 3    | auth     | PASS   |

Actual (identical shape for all four; `command` field differs):

```json
{
  "ok": false,
  "command": "whoami",
  "error": "not logged in — run `urlbox login`",
  "code": "auth",
  "hint": "Run `urlbox login` to sign in via your browser."
}
```

Text-mode (`--output-format text`) error path for the same four commands — empty
stdout, `Error:` + `Hint:` to stderr, exit 3:

```
[stderr]
Error: not logged in — run `urlbox login`
Hint: Run `urlbox login` to sign in via your browser.
```

## PART C — New commands, success layout (mock API, post-login)

To record the human/text-mode success layout (the envelope text formatter path)
and the JSON data shape, the four session commands were driven against a local
mock API (`URLBOX_API_HOST=http://127.0.0.1:8791`) with a valid `session_token`.
This exercises the success formatter that the logged-out state cannot reach.

### Text mode (human layout) — the deferred "record text-mode output" item

```
### $ bin/urlbox whoami --output-format text
✓ Signed in as dev@example.com — org Acme Inc          (exit 0)

### $ bin/urlbox orgs list --output-format text
✓ 2 organisations — active: Acme Inc                    (exit 0)

### $ bin/urlbox projects list --output-format text
✓ 2 projects — active: Website Shots                    (exit 0)

### $ bin/urlbox usage --output-format text
✓ Renders used: 1240 / 5000                             (exit 0)
```

The text formatter renders a single `✓ <summary>` line to stdout on success.

### JSON data shape (same runs)

`whoami`:

```json
{
  "ok": true,
  "command": "whoami",
  "data": {
    "email": "dev@example.com",
    "org": { "id": "org_postlogin", "name": "Acme Inc" },
    "project": { "id": "proj_postlogin", "name": "Website Shots" }
  },
  "summary": "Signed in as dev@example.com — org Acme Inc"
}
```

`orgs list`:

```json
{
  "ok": true,
  "command": "orgs list",
  "data": {
    "organisations": [
      { "active": true, "id": "org_postlogin", "name": "Acme Inc" },
      { "active": false, "id": "org_globex", "name": "Globex" }
    ]
  },
  "summary": "2 organisations — active: Acme Inc"
}
```

`projects list`:

```json
{
  "ok": true,
  "command": "projects list",
  "data": {
    "projects": [
      { "active": true, "id": "proj_postlogin", "name": "Website Shots" },
      { "active": false, "id": "proj_other", "name": "Marketing" }
    ]
  },
  "summary": "2 projects — active: Website Shots"
}
```

`usage`:

```json
{
  "ok": true,
  "command": "usage",
  "data": {
    "current_period_end": "2026-08-31",
    "current_period_start": "2026-08-01",
    "render_quota": 5000,
    "renders_used": 1240
  },
  "summary": "Renders used: 1240 / 5000"
}
```

## PART D — New config keys (session_token / active_org / active_project)

Run against the post-login state.

| Action                                            | Result | Note |
|---------------------------------------------------|--------|------|
| `config get session_token`                        | `sess…55` (exit 0) | masked by default (like `api_secret`) |
| `config get session_token --reveal`               | `sess_tok_post_login_9988776655` (exit 0) | `--reveal` unhides |
| `config get active_org`                           | `org_postlogin` (exit 0) | plain string |
| `config get active_project`                       | `proj_postlogin` (exit 0) | plain string |
| `config set active_project proj_newvalue`         | exit 0 | persisted; quiet read returns `"proj_newvalue"` |
| `config set session_token sess_tok_rotated_xyz789`| exit 0 | validated via `ValidateSecretValue`; echo masked to `sess…89`; raw value persisted on disk |
| `config get bogus_key`                            | exit 1, `usage` | hint lists all seven keys |

`config get session_token` (masked):

```json
{
  "ok": true,
  "command": "config get",
  "data": { "key": "session_token", "profile": "default", "value": "sess…55" },
  "summary": "session_token = \"sess…55\""
}
```

`config get session_token --reveal`:

```json
{
  "ok": true,
  "command": "config get",
  "data": { "key": "session_token", "profile": "default", "value": "sess_tok_post_login_9988776655" },
  "summary": "session_token = \"sess_tok_post_login_9988776655\""
}
```

`config set session_token …` — echo masked, raw value persisted:

```json
{
  "ok": true,
  "command": "config set",
  "data": { "key": "session_token", "profile": "default", "value": "sess…89" },
  "summary": "session_token set in profile \"default\"",
  "breadcrumbs": [{ "action": "verify", "cmd": "urlbox config get session_token" }]
}
```

On disk after the set (raw, unmasked — masking is display-only):

```json
"session_token": "sess_tok_rotated_xyz789",
"active_project": "proj_newvalue"
```

`config get bogus_key` — unknown-key error with the updated hint:

```json
{
  "ok": false,
  "command": "config get",
  "error": "Unknown config key: bogus_key",
  "code": "usage",
  "hint": "Supported: api_key, api_secret, api_host, default_profile, session_token, active_org, active_project"
}
```

## PART E — Deferred verification: picker draws on stderr

The interactive picker (`internal/prompt.SelectOne`, huh/bubbletea) was driven
under a real PTY (via `expect`), with the child process's **stdout (fd1)
redirected to a plain file** and its **stderr (fd2) redirected to a separate
file**. stdin stayed the PTY so the `IsTerminal(stdin)` gate passes and
bubbletea initializes.

Options presented: `["Acme", "Globex", "Initech"]`, active index 1 (Globex).
Enter accepted the active option.

Result:

- **fd1 (stdout)** — 14 bytes, exactly `CHOSE_INDEX=1\n`. Zero terminal/UI
  escape codes leaked to stdout (verified by hexdump). stdout stayed clean data.
- **fd2 (stderr)** — 503 bytes containing bubbletea's terminal-init sequences
  (`ESC[?25l`, `ESC[?2004h`, `ESC[?1004h`) AND the rendered list. ANSI-stripped:

```
┃ Choose an org
┃   Acme
┃ > Globex (current)
┃   Initech
```

**Conclusion: the picker renders on stderr; stdout carries only the structured
result.** This is guaranteed at the library level — huh's `Form.Run` passes
`tea.WithOutput(os.Stderr)` (huh@v1.0.0 `form.go:112`), and `SelectOne` does not
override it. The `CHOSE_INDEX=1` on stdout also confirms the select completed and
returned the correct index.

### `ACCESSIBLE=1` and accessible mode (record only — no code change)

The upstream note is that huh falls back to **stdout** in accessible mode. Two
cases were recorded (same PTY + split-fd setup):

- **`ACCESSIBLE=1` set:** huh@v1.0.0 does NOT treat `ACCESSIBLE` as the trigger
  (that env var was the trigger in older huh releases). The normal interactive
  TUI still rendered on **stderr** (fd2 = 503 bytes with the box UI); stdout
  stayed clean (`CHOSE_INDEX=1`). No behaviour change vs the default.

- **`TERM=dumb` (huh@v1.0.0's actual accessible trigger, `form.go:124`):** the
  plain numbered accessible prompt rendered on **stdout** (fd1 = 101 bytes):

  ```
  Choose an org
  1. Acme
  2. Globex (current)
  3. Initech
  Enter a number between 1 and 3:
  ```

  fd2 (stderr) was empty. This is the documented accessible-mode fallback:
  in accessible mode huh writes to `cmp.Or(f.output, os.Stdout)` (`form.go:672`)
  and `SelectOne` leaves `f.output` unset, so it lands on stdout. **Recorded per
  the brief; no code change made.** Accessible mode is opt-in and only reached
  under a `TERM=dumb` terminal, which is not the default agent/CI path.

## Summary

- Pre-existing commands: **identical across both config states** (no
  STOP-the-line behavioural difference). PASS on every cell.
- New session commands logged-out: **auth envelope, exit 3** on all four. PASS.
- `login`: prints code + URL to stderr, waits, interrupt leaves config
  untouched. PASS.
- New session commands success layout (text + JSON) recorded via mock API.
- New config keys: get/set/mask/reveal/validate all correct; unknown-key hint
  updated. PASS.
- Picker: **renders on stderr, stdout stays clean data**. Accessible-mode
  fallback to stdout recorded (TERM=dumb), no code change.
