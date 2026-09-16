# CLI render reporting — design

**Date:** 2026-09-11
**Status:** approved by Arnold (brainstorm session); implementation not started
**Branch:** new branch off latest `origin/main` (main tip `c9d96d6`, v1.2.0)
**Handoff:** `docs/superpowers/HANDOFF-2026-09-11-render-report-command.md`

Reporting a bad render from the CLI, via the mono's public `/v2` render-report
endpoints. Three pieces: a post-render hint line, `urlbox report <renderId>`,
and `urlbox support`.

## Rulings (Arnold, 2026-09-11)

- **Session-only auth.** The render-report routes accept only a logged-in
  session (device-flow bearer token) or a better-auth org API key the CLI
  doesn't hold. The project render secret (`ubx_sk_…`) is never accepted —
  verified live against production (401 as `Authorization: Bearer`, 401
  "Invalid API key" as `x-api-key`). This is the intended direction, not a
  limitation: session is the new way. Agents don't log in themselves — they
  ask the human to run the browser-launched `urlbox login`, then inherit the
  session from config.
- **Hint line, not a keypress.** The post-render affordance is one muted
  stderr line containing the ready-to-run report command. The process exits
  immediately; nothing listens for keys, nothing blocks.
- **Hint on every render response** — sync or async, success or failure —
  whenever the response yields a render id. Every render, no backoff, no
  config knob.
- **No last-render memory.** The CLI persists nothing after a render.
  `urlbox report` always takes an explicit renderId; the hint line in
  scrollback is the memory. No `--last` flag.
- **No JSON envelope changes.** The render id captured for the hint stays
  in-memory only. Sync JSON output keeps its current shape (no `renderId`
  field added). Agents that need the id use `--async` (its body carries it).
- **Limited reports:** category + comment only. `rects` deliberately out
  (dashboard's click-to-highlight concept).
- **Interactive flow collects both** category (house `SelectOne`) and a
  one-line comment — the API requires both.
- **`urlbox support` opens `https://urlbox.com/contact`.** (`/support`
  serves a 200 in production but has no page in the dashboard source; not
  the CLI's problem to solve.)
- Out of scope for this branch: the `respond` endpoint, a report *list*
  command, org-API-key auth, CLI-self-problem reporting, mining the archived
  `urlbox/cli` repo, any rating mechanic.

## API contract (pinned from mono origin/main 2026-09-11 — re-verify at build time)

`POST /v2/organisation/{org}/render-reports` — `{org}` is the organisation
public id (`org_…`, from the profile's `active_org`).

Body: `renderId` (string 1–255), `category` (enum below), `comment`
(string 1–2000, required), `rects` (optional; CLI never sends it).
Categories: `bot-detection`, `login-required`, `missing-content`,
`cookie-banner-or-popup`, `render-failed`, `other`.

Response DTO: `id` (`rpt_…`), `renderId`, `category`, `comment`, `rects`,
`state` (`open` | `in_review` | `resolved`), `outputUrl`, `outputFormat`,
`resolutionMessage`, `resolutionExampleOptions`, `resolutionExampleUrl`,
`hasUnseenResolution`, `createdAt`.

Semantics that shape CLI behaviour (verified in origin/main source):

- **The render must exist and belong to the caller's org**, else a uniform
  404 "Render not found" (no existence oracle). Renders expire from Mongo
  after ~60 days. (The handoff's "missing renderIds accepted by design" is
  stale — corrected.)
- **One report per render (upsert).** Re-creating while the report is `open`
  edits it in place. Once `in_review`/`resolved` it is locked → 409 CONFLICT
  "This report is being looked at by the team and can no longer be edited."
- Auth: session bearer via `Authorization: Bearer <session-token>` (same as
  every shipped account-management command). All member roles carry
  `renderReport: create + read`. Non-member org → 404.
- Side effects on create: opens a Crisp conversation tied to the reporter's
  email + Slack ping (soft-fail, never blocks filing).
- List exists (`GET`, paginated `{renderReports, page, totalPages}`) and
  `respond` exists (`POST …/{report}/respond`) — both out of CLI scope.

Live wire shapes captured 2026-09-11 (use as fixture sources):

- Sync success: body has NO render id; the id is the
  `x-urlbox-request-id` response header (e.g. `01a0906a-…_ps`), which
  resolves on `GET /v1/render/{id}` to `{"renderId": …, "status":
  "succeeded", "renderUrl": …}`.
- Sync failure (HTTP 400): no header; body carries the id:
  `{"error":{"message":"Invalid URL","code":"InvalidURLError"},
  "requestId":"01a09082-…_ps"}`. The failed render exists server-side
  (`status: "failed"` with `reason`), so it is reportable.
- Async accepted: body `{"status":"created","renderId":"…_pa",
  "statusUrl":"https://api.urlbox.com/v1/render/…"}`.
- Unauthenticated `/v2` error: `{"defined":false,"code":"UNAUTHORIZED",
  "status":401,"message":"Authentication required"}`.

## Piece 1 — the hint line

After any render-family command (`render`, `screenshot`, `pdf`, `video`)
receives an API response containing a render id, print one muted line to
stderr under the result/error:

```
✓ Rendered: https://renders.urlbox.com/…png
  Something wrong? urlbox report 01a0906a-…_ps
```

Wording is a placeholder; final copy is Arnold's, set at implementation
review before merge.

Where the id comes from, per response type (in-memory only, never persisted,
never added to envelopes):

| Response | Id source |
|---|---|
| Sync success | `x-urlbox-request-id` response header |
| Sync failure | `requestId` field in the error body |
| Async accepted | `renderId` in the body |
| Async submit failure | `requestId` in the error body, when present |

Gates — the hint prints only when ALL hold:

- resolved output format is `text` (never json/quiet)
- stderr is a TTY (`isStderrTTY`, same gate as the root banner)
- the response yielded a render id (auth/network/timeout failures without a
  `requestId` print nothing extra)

Never on `--dry-run`/`--curl` (no API call → no response), and NOT on
`urlbox status` (the render-time hint already covered that id).

The HTTP client must expose the response header + parsed error body to the
render command for this; that plumbing stays internal to the client/command
(`internal/api/http_client.go`, `internal/cmd/render.go`).

## Piece 2 — `urlbox report <renderId>`

Session command modelled on the account-management commands: guard =
`loadSession` + `requireActiveOrg`; call = `SessionClient.PostJSON` to
`/v2/organisation/{active_org}/render-reports` with
`{renderId, category, comment}`. Standard session retry flags
(`--max-retries`/`--no-retry`) attach; safe because create is an upsert.

Surface:

```
urlbox report <renderId> [--category <c>] [--comment <text>]
```

- `--category` — one of the six API values verbatim; validated client-side,
  invalid value → usage error listing the valid set.
- `--comment` — 1–2000 chars; validated client-side.
- Both flags present → no prompts (agent path).
- TTY + missing flag(s) → interactive: `prompt.SelectOne` over the six
  categories (human-readable labels, API values on the wire), then a
  one-line comment input (`huh` input, required non-empty). Interactive shows
  the renderId being reported before filing.
- Non-TTY + missing flag(s) → `ErrUsage` naming the missing flag; never
  hangs (`prompt.ErrNotInteractive` path).

Output (house rules — mutation with named summary, no view):

- text: `✓ Report filed for render <renderId>` (exact copy Arnold's)
- json: success envelope, `data` = the API report DTO verbatim
- quiet: the report public id (`rpt_…`) via `writeEnvelopeWithQuietData`
- no breadcrumb (no follow-up CLI command exists in this scope)

Error mapping (closed set):

| API | CLI code | Exit | Hint |
|---|---|---|---|
| 401 | `auth` | 3 | unified login hint (`loginHint`) |
| 404 org or render | `not_found` | 5 | render not found — expired (~60 days) or not from this org |
| 409 locked report | `conflict` | 7 | surface the API message (report is with the team) |
| 400/422 validation | `validation` | 2 | from API body |
| 429 / 5xx | `rate_limit` / `server` | 6 / 10 | standard mapping |

## Piece 3 — `urlbox support`

Clone of the `dashboard` command shape (`internal/cmd/dashboard.go` +
`internal/browser.Opener`), target `https://urlbox.com/contact`:

- text + TTY, not headless → open the browser; opener failure → `ErrServer`
  with the URL in the hint
- headless → print `Support URL: … (open in any browser)` to stderr, still
  emit the success envelope
- json/quiet → envelope only, never launches a browser
- no auth required

## Testing & delivery

- TDD failing-first per repo rules; `make ci` + `make surface-snapshot` per
  task; SURFACE.txt committed with the code (new entries: `report` + its
  flags, `support`).
- Fixtures are production-shape only, pinned from the live wire shapes in
  this spec (captured 2026-09-11) and the origin/main zod contract. No
  invented shapes.
- Prompt injection for tests follows the house `pickFunc` /
  `Set…ForTest` pattern; TTY overrides via `stderrTTYOverride` /
  `stdinTTYOverride`.
- `skills/SKILL.md` (agent-relevant command) and README updated.
- Live verification at Arnold's manual gate: needs a fresh `urlbox login`
  (his local session token is expired). Positive create against production
  files a real report (Crisp conversation + Slack ping) — do it once,
  deliberately, at the gate.
- Mono fumadocs + command reference ride a later effort, page-by-page with
  Arnold.
- Commits per repo rules: propose diff + message, wait for approval; author
  `Arnold Cubici-Jones <108676317+AJCJ1@users.noreply.github.com>`; squash
  to one before PR; never push without Arnold's word.
