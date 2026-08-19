# Plan 2 — Agent-layer verification (2026-08-18)

Agent-layer verification for Plan 2 (org-owned credential resources +
`urlbox auth` removal): the storage / proxies / llm command groups, the
`projects <kind> assign|unassign` sub-commands, doctor's new session-world
checks, and the auth-command removal. This doc is the input to the human-layer
manual checklist that gates the Plan 2 release.

## Environment

- Branch: `feat/account-management` (tip `963f807`, plus this task's test/doc
  changes in the working tree).
- Binary under test: `bin/urlbox` built from this tree via `make build`
  (`go build ... -o bin/urlbox ./cmd/urlbox`), go1.24 darwin/arm64.
- Gates: `make ci` green (fmt-check, lint 0 issues, `go test -race -cover
  ./...`, build, surface-check) and `make surface-snapshot` produced **no**
  `SURFACE.txt` diff — Plan 2's commands were snapshotted by their own tasks;
  Task 8 adds no new commands or flags.
- The machine carries a real logged-in session (`someone@example.com`, org
  `Example Org` / `org_example`, project `Default` / `proj_example`), used
  for the read-only and gate-behaviour drives against production. **No
  credentials were created, updated, assigned, or deleted against
  production** — every destructive/creating verb was driven only to its
  client-side gate (auth gate, usage gate, resolution failure, or the
  non-interactive confirmation refusal), which fires before any write leaves
  the CLI.

## Two config states

The logged-out drives use a scratch `XDG_CONFIG_HOME` holding the pre-login
(legacy) shape — `api_key` + `api_secret` only, with a deliberately fake
secret:

```json
{
  "default_profile": "default",
  "profiles": {
    "default": {
      "api_key": "pk_test_key",
      "api_secret": "ubx_sk_fake000000000000000000000000"
    }
  }
}
```

The logged-in drives use the machine's real config (session present).

## PART A — Plan-1 regression, logged out (text mode)

Every Plan-1 command re-driven against the legacy config to confirm Plan 2 did
not disturb them. `render`/`link`/`status` are session-independent (they use
`api_secret`); the account commands correctly hit the auth gate.

| Command | Exit | Output shape | Result |
|---------|------|--------------|--------|
| `version` | 0 | version envelope | PASS |
| `render https://example.com --dry-run --output-format text` | 0 | `✓ Dry run: payload validated, no API call made` | PASS |
| `screenshot https://example.com --dry-run --output-format text` | 0 | same dry-run line | PASS |
| `pdf https://example.com --dry-run --output-format text` | 0 | same dry-run line | PASS |
| `link https://example.com --output-format text` | 0 | boxed URL/FORMAT/KEY table | PASS |
| `config path --output-format text` | 0 | `✓ Config file path: …/lo/urlbox/config.json` | PASS |
| `whoami --output-format text` | 3 | `Error: not logged in — run \`urlbox login\`` | PASS |
| `orgs list --output-format text` | 3 | same not-logged-in error | PASS |
| `projects list --output-format text` | 3 | same not-logged-in error | PASS |
| `status render_abc123 --no-retry --output-format text` | 5 | `Error: Render render_abc123 not found` (real 404) | PASS |

## PART B — New groups, logged-out guard behaviour (text mode)

Every new command driven against the legacy config. All 21 return the unified
not-logged-in error and `auth` / exit 3 — the auth gate fires before any org
resolution or network call, so a logged-out caller can never leak a request.

| Command | Exit | Result |
|---------|------|--------|
| `storage list` | 3 | PASS |
| `proxies list` | 3 | PASS |
| `llm list` | 3 | PASS |
| `storage show foo` | 3 | PASS |
| `proxies show foo` | 3 | PASS |
| `llm show foo` | 3 | PASS |
| `storage create --name x --provider aws_s3 --bucket b --region r --key k --secret s` | 3 | PASS |
| `proxies create --name x --url http://u:p@h:8080` | 3 | PASS |
| `llm create --name x --provider openai --api-key sk-fake` | 3 | PASS |
| `storage update foo --region r2` | 3 | PASS |
| `proxies update foo --name y` | 3 | PASS |
| `llm update foo --model gpt-5` | 3 | PASS |
| `storage delete foo --yes` | 3 | PASS |
| `proxies delete foo --yes` | 3 | PASS |
| `llm delete foo --yes` | 3 | PASS |
| `llm test foo` | 3 | PASS |
| `llm models foo` | 3 | PASS |
| `projects storage assign proj cred` | 3 | PASS |
| `projects proxy assign proj cred` | 3 | PASS |
| `projects llm assign proj cred` | 3 | PASS |
| `projects storage unassign proj` | 3 | PASS |

Representative output (identical shape for all 21):

```
$ urlbox storage create --name x --provider aws_s3 --bucket b --region r --key k --secret s --output-format text
Error: not logged in — run `urlbox login`
Hint: Run `urlbox login` to sign in.
[exit=3]
```

## PART C — New groups, `--help` surfaces

`--help` printed for each group and each `projects <kind>` sub-group; every
verb is listed with its short description, and the group long-help carries the
"owned by the organisation, assigned to projects" framing and the
`--reveal` note.

| `--help` target | Verbs listed | Result |
|-----------------|--------------|--------|
| `storage` | create, delete, list, show, update | PASS |
| `proxies` (alias `proxy`) | create, delete, list, show, update | PASS |
| `llm` | create, delete, list, models, show, test, update | PASS |
| `projects storage` | assign, unassign | PASS |
| `projects proxy` | assign, unassign | PASS |
| `projects llm` | assign, unassign | PASS |

## PART D — New groups, gate behaviour against the live session

Driven with the machine's real session. Read-only lists are safe; the
write/resolve/confirm paths were driven only to their client-side gate.

### D.1 Read-only lists (safe, no writes)

The org has no credentials of any kind, so each list renders an empty table,
exit 0 — proving the authenticated read path reaches production cleanly.

| Command | Exit | Output | Result |
|---------|------|--------|--------|
| `storage list --output-format text` | 0 | empty `BUCKET/ID/PROVIDER/ENDPOINT/KEY/ASSIGNED` table | PASS |
| `proxies list --output-format text` | 0 | empty `ID/NAME/URLS/ASSIGNED` table | PASS |
| `llm list --output-format text` | 0 | empty `ID/NAME/PROVIDER/MODEL/ASSIGNED` table | PASS |

### D.2 Non-interactive delete confirmation gate

Driven with a `store_`/`pool_`/`llm_`-prefixed id (resolves without a list
lookup) and non-TTY stdin (`</dev/null`), no `--yes`. The retype-to-confirm
prompt refuses non-interactively **before** any DELETE is sent.

| Command (stdin: `/dev/null`, no `--yes`) | Exit | Output | Result |
|------------------------------------------|------|--------|--------|
| `storage delete store_nonexistent000` | 1 | `Error: deletion needs confirmation` / hint `Re-run with --yes …` | PASS |
| `proxies delete pool_nonexistent000` | 1 | same | PASS |
| `llm delete llm_nonexistent000` | 1 | same | PASS |

### D.3 Usage / validation / resolution gates

| Command | Exit | Output | Result |
|---------|------|--------|--------|
| `storage update store_x` (no fields) | 1 | `nothing to update — pass at least one field flag or --json` | PASS |
| `proxies update pool_x` (no fields) | 1 | `nothing to update — pass --name and/or --url` | PASS |
| `llm update llm_x` (no fields) | 1 | `nothing to update — pass at least one field flag or --json` | PASS |
| `storage create` (no flags) | 1 | `--name is required` | PASS |
| `proxies create` (no flags) | 1 | `--name is required` (hint: proxy pool) | PASS |
| `llm create` (no flags) | 1 | `--name is required` (hint: credential) | PASS |
| `storage show nonexistent-name` | 5 | `no storage credential matching "nonexistent-name"` | PASS |
| `llm test nonexistent-name` | 5 | `no LLM credential matching "nonexistent-name"` | PASS |
| `llm models nonexistent-name` | 5 | `no LLM credential matching "nonexistent-name"` | PASS |
| `projects storage assign nonexistent-project store_x` | 5 | `no project matching "nonexistent-project"` | PASS |

`llm test` / `models` against a real credential is deferred to the manual
checklist (no LLM credential exists on this account, and creating one would be
a production write). The resolution-failure shape above is the drivable
portion of those verbs at the agent layer.

## PART E — doctor's session-world checks, both states

`doctor` gained `session`, `active_org`, and `active_project` checks. Driven in
text mode in both states.

**Logged in** (real session), exit 0 — all ten checks pass:

```
│ ✓ │ session           │ signed in as someone@example.com  │
│ ✓ │ active_org        │ org_example                  │
│ ✓ │ active_project    │ proj_example                 │
```

**Logged out** (legacy config, fake secret), exit 3 — the three session-world
checks and `auth` fail with their hints, everything else stays green:

```
✗ Some checks failed — see hints for next steps
│ ✗ │ session           │ not logged in — run `urlbox login`         │ Run `urlbox login` to sign in.
│ ✗ │ active_org        │ no active organisation                     │ Run `urlbox orgs select` to choose one.
│ ✗ │ active_project    │ no active project                          │ Run `urlbox projects select` to choose one.
│ ✗ │ auth              │ API returned 400: Api Key does not exist   │ Run `urlbox login` to sign in. Or check `urlbox config get api_secret --reveal` …
```

The fake `api_secret` produced a real production `400 Api Key does not exist`
on the `auth` check — the expected shape for an unknown key. `doctorExitCode`
returns `auth` (exit 3) in both states because the worst failing check is
session/auth-class.

| doctor drive | Exit | Result |
|--------------|------|--------|
| logged in (real session) | 0 | PASS |
| logged out (legacy config) | 3 | PASS |

## PART F — `urlbox auth` removed

`auth` was removed in Plan 2 (login replaces it). It now resolves as an unknown
command before any session load, identical in both states.

```
$ urlbox auth --output-format text
Error: unknown command "auth" for "urlbox"
Hint: Run `urlbox <command> --help` for usage, or `urlbox commands` for the full surface.
[exit=1]
```

| Command | Exit | Result |
|---------|------|--------|
| `auth` (logged in) | 1 | PASS |
| `auth` (logged out) | 1 | PASS |

## Result

All drives PASS. No defect found; no source change made. The full lifecycle of
each credential group (create → assign → update → delete against production,
plus `llm test`/`models` against a real key) is handed to the human-layer
manual checklist, which owns the billable-safe production writes.
