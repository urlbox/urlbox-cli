# Account management + device login — port design

**Status:** DESIGN / approved in conversation, pending written review.
**Target repo:** `urlbox/urlbox-cli` (this repo — the shipped `@urlbox/cli`).
**Behaviour source:** the `urlbox/cli` repo, branch `feat/device-login` (PR #8, approved there), whose account-management surface was built against the deployed device-auth API. That implementation is the behaviour spec; this repo's conventions are the implementation spec. Where they pull apart, behaviour comes from the source and expression comes from this repo.

## Goal

Bring the session/account-management surface into the shipped CLI: browser device-flow `login`, session management, org/project context, `/v2` management commands, and org-owned credential resources (storage, proxies, LLM) — with every pre-existing command provably unaffected.

## Scope

**Added (all net-new here):**

- `login`, `logout`, `whoami` (alias `me`)
- `orgs list|select` (alias `org`)
- `projects` (alias `project`): list/select/create/update/delete + `defaults` (on `/v2`) — one group; `select` sets the active project
- `usage`
- `storage`, `proxies`, `llm` groups: list/show/create/update/delete; `llm test`, `llm models`
- `projects storage|proxy|llm assign|unassign`; create→assign prompt; `--assign-to`

**Removed:** the `auth` command (see "The auth removal sweep").

**Not ported:** `batch`, `jobs`, `webhook` (they exist only in the source repo; explicitly out).

**Untouched in behaviour:** `render`, `screenshot`/`pdf`/`video`, `link`, `status`, `dashboard`, `skill`, `commands`, `schema`, `upgrade`, `version` — and all packaging/release machinery (goreleaser, npm, brew, scoop). New commands ship with the next ordinary release. Two existing commands are deliberately *extended*, not preserved byte-for-byte: `config` (learns the new profile keys) and `doctor` (learns the session world; loses its `urlbox auth` references in the sweep).

## Credential model

Two credentials, two clients, one profile:

| Surface | Credential | Commands |
|---|---|---|
| Rendering (`/v1` render, status) | API secret (`api_secret`, as today) | `render` family, `status`, `link` |
| Management (`/v2` + better-auth) | Session token (`session_token`, new) | everything added above |

- `Profile` gains `session_token`, `active_org` (`org_…` public id), `active_project` (`proj_…` public id). Real struct fields so every save path preserves them; the config validators accept them.
- A second, session-authenticated constructor joins the existing client in `internal/api`, riding the same retry/backoff, status→error-code mapping, timeouts, and user-agent. A command constructs the client for its surface and physically cannot send the wrong credential.
- `login` stores the fetched render secret in the profile's existing `api_secret` field — the field the render pipeline already reads. No render-path code changes.

## The login flow

1. `POST /v1/auth/device/code` (`client_id: "urlbox-cli"`) → code + verification URL.
2. Print the code and URL, best-effort open the browser (house opener; failure is non-fatal — the URL is already printed).
3. Poll `POST /v1/auth/device/token` per the server's `interval`; honour `slow_down` backoff, `authorization_pending`, denial, and the `expires_in` window.
4. Store the session token (config write is lockfile-guarded; file mode 0600 as today).
5. Resolve the active org: `GET /v1/auth/organization/list`; one org → silent; several → picker (`--org <name-or-id>` bypasses). `POST /v1/auth/organization/set-active` with better-auth's internal id, then read the public id back from `GET /v1/auth/get-session` and store that.
6. Resolve the active project the same way (`--project` bypasses; zero projects → say so and continue).
7. Fetch the active project's render credential over the session (issuing one if the project has none) and store it as `api_secret`. Best-effort: failure warns but never fails the login.
8. Success envelope: email, org, project, render-credential status (`ready`/`issued`/`none`/`error`).

`logout`: `POST /v1/auth/sign-out` (revokes only this session), then clear `session_token`/`active_org`/`active_project` plus the login-installed `api_key`/`api_secret` locally (matching the source implementation). Server failure still clears local state, with a warning. `whoami`: `GET /v1/auth/get-session` + active context; JSON carries email and org/project ids/names.

## Management + credential resources (behaviour summary)

- Path-less `/v2` where the session's active org drives resolution (`/v2/projects`, `/v2/usage`, `/v2/me`); org-scoped `/v2/organisation/{org}/…` for credential resources and project sub-resources, using the stored public org id.
- Lists show an ASSIGNED count (in-use vs orphaned at a glance); proxy URLs never appear in lists (they embed passwords).
- `show` masks secrets (storage key/secret/SAS token, LLM API keys/cloud credentials, the password component of proxy URLs — only that component); `--reveal` unhides, matching the house pattern.
- `create`/`update` are flag-driven with `--json` for full/partial payloads; updates send only what was passed (partial PATCH); `proxies update` replaces the whole URL list (help text says so); LLM `--provider` is create-only.
- Positional resource/project arguments accept a public id (prefix match: `store_`/`pool_`/`llm_`/`proj_`) or a name; names resolve via the list; ambiguity errors listing the matching ids.
- After a TTY `create`: offer to assign to a project (skippable); `--assign-to` does it non-interactively; non-TTY without the flag: create, print, exit.
- Agent-hard requirements: every interactive affordance has a flag equivalent; every data command supports JSON output; deletes take `--yes`; non-TTY never hangs — pickers error naming the flag to pass.

## House-convention integration

Every new command complies with the existing machinery, no exceptions:

- **Output:** success/error envelopes; errors only from the closed ten-code set — "not logged in" and "session expired" map to `auth` (exit 3) with hint `Run \`urlbox login\``; name-resolution misses → `not_found`; ambiguous names → `validation` with the candidate ids in the hint. Breadcrumbs point at the natural next command (login → first screenshot). `--jq`, `--agent`, `--output-format`, `--profile` inherited from root. stdout = data, stderr = humans. Quiet-mode scalars defined per command at plan time.
- **Surface contract:** every command/flag lands in `SURFACE.txt` via `make surface-snapshot`, committed with the code, every slice.
- **Config:** all writes through the lockfile-guarded update path; new keys supported by `config get/set/show` with session token masked (`--reveal` to unhide).
- **Help:** house style — multi-paragraph Long, `Examples:` block, footgun warnings, exit codes documented where they vary.
- **Picker:** this repo's first interactive select component. Draws to stderr only, uses the existing lipgloss styles, respects `NO_COLOR`, returns a clean error on non-TTY naming the bypass flag. Used by login, `orgs select`, `project select`, create→assign.
- **Quality gates:** TDD throughout; `make ci` (fmt-check, lint, test, build, surface-check) green at every slice boundary.

## Transplants (approach C)

Command wrappers are written fresh in cobra + envelopes. Five framework-free logic pieces move across from the source repo *with their unit tests*, imports adjusted, wired into house plumbing:

1. Device-poll state machine (interval, `slow_down` backoff, pending, denial, expiry) — told time via `internal/clock` so tests fast-forward.
2. Login's org/project resolution sequence (zero/one/many × zero/one/many, and the internal-id → public-id translation through set-active + get-session).
3. Name-or-id resolution (prefix precedence, list lookup, ambiguity error listing ids).
4. Secret-masking rules (including password-only masking inside proxy URLs).
5. Flag → `/v2` payload mapping (partial-PATCH semantics, s3-vs-azure field sets, whole-list proxy replacement).

## The auth removal sweep

`urlbox auth` is removed: `login` is the interactive door; `URLBOX_API_SECRET` (already supported) is the CI/headless path. The removal is deliberate and will appear in the `SURFACE.txt` diff (that file makes removals reviewed, not forbidden). The sweep updates every reference to the old flow in the same PR: `doctor` checks and hints, error hints, help text, README, `npm/README.md`, and the agent skill. `doctor` additionally learns the session world: logged in, session valid, active org/project set, render credential present.

## Profiles: kept as plumbing, undocumented

The multi-profile machinery is untouched — it is load-bearing for every existing command (all credential resolution routes through it; the shipped on-disk config format is profile-shaped; 57 surface lines and the most-tested config logic depend on it). Session fields live inside a profile, exactly as the source implementation does. But profiles are not documented to users: no docs-site coverage, no mention in new help text beyond the inherited `--profile` flag. Multi-account support ("log into two accounts side by side") exists silently; publicising it is a possible docs-only follow-up, expected never to be needed (one account reaches many orgs).

## Compatibility & regression protection (the "don't break render" clause)

`login` writes into the same file every existing command reads. Protections, built in slice 1 before anything else stacks on them:

- New fields added to the config struct and validators so no save path drops them and no read path rejects them.
- A compatibility suite runs every pre-existing command against two config states — legacy (no session fields) and post-login — asserting identical behaviour: `render` dry-run, `screenshot`, `link` signing, `status`, `doctor`, `config get`/`config path`/`config profile list`, `dashboard`, `schema`, `commands`, `version`.
- e2e binary test for the login → render sequence.
- Each existing command's config interaction is enumerated in the implementation plan; none assumed.

## Verification protocol (two layers, both mandatory)

1. **Agent layer, per slice:** drive every pre-existing command one at a time in a real terminal against both config states and record actual output — alongside the automated suites and `make ci`.
2. **Human layer, merge gate:** a written manual checklist — exact commands with expected results, login through render through credential groups — run by hand against production. Nothing is declared working from tests alone.

## Build order

1. **Foundation** — config fields, session client, `login`, `logout`; compatibility suite exists and passes. Real login works end-to-end against production at the end of this slice.
2. **Context** — `whoami`/`me`, `orgs list|select`, `project list|select`, the picker.
3. **Management** — `projects` CRUD + defaults, `usage`.
4. **Credential resources** — `storage`/`proxies`/`llm`, assign/unassign, create→assign, `--assign-to`.
5. **Auth sweep** — remove `auth`, update all references, final surface snapshot.

Delivered as one PR on a feature branch here, slices as reviewable commits.

## Out of scope / follow-ups

- `batch`, `jobs`, `webhook` — not ported.
- `storage test` / `proxies test` — no `/v2` endpoints yet (LLM is the only resource with test/models); follow-up CLI PR when the endpoints ship.
- Docs site + blog updates — resume after this PR merges (task list already prepared in the mono).
- Retirement of the source repo and its PR #8 — separate conversation, not this PR.
- Publicising multi-account profiles — docs-only follow-up, if ever.

## Endpoints consumed (all deployed in production today)

`POST /v1/auth/device/code` · `POST /v1/auth/device/token` · `GET /v1/auth/organization/list` · `POST /v1/auth/organization/set-active` · `GET /v1/auth/get-session` · `POST /v1/auth/sign-out` · `GET /v2/me` · `GET /v2/usage` · `GET/POST… /v2/projects` and org-scoped project routes incl. render-defaults · `/v2/organisation/{org}/storage-credentials[/{id}]` · `/v2/organisation/{org}/proxies[/{id}]` · `/v2/organisation/{org}/llm-credentials[/{id}]` + `/test` + `/models` · `PUT/DELETE /v2/organisation/{org}/projects/{project}/storage-credential|proxy|llm-credential` · the project api-credential fetch/issue routes used by login step 7. The mono needs zero changes; exact paths are pinned from the source implementation at plan time.
