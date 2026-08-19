# Text-surface consistency audit — 2026-08-14

Rule being audited: lists render tables; details render KV; mutations render a
summary naming what the user named, plus KV of what changed. Every row driven
live against production on the current branch build.

## Already consistent (no change)

| Command | Text output today |
|---|---|
| `version`, `config get/set/path` | scalar one-liners — correct for scalar commands |
| `whoami`, `usage`, `login` | summary + KV block |
| `orgs list`, `projects list` | summary + table with `●` active marker |
| `doctor` | summary (`✗` on failure) + per-check table with hints |
| `link` | summary + KV incl. full signed URL |
| `logout` | summary — nothing else to show |

## Bugs found during audit (fix already dispatched, one commit in flight)

| Site | Defect |
|---|---|
| `projects show` | KV never renders: reads response through a nonexistent nested `"project"` key; summary shows raw id; `webhookKey` unmasked → KV + name summary + masked key + `--reveal` |
| `projects defaults show` | reports `0 default options` when defaults EXIST — same wrong nested read. Write path verified fine in production (`defaultOptions` persists) |
| `projects defaults set --merge` | same nested read → merge silently becomes overwrite |

## Deviations for approval (D1–D6)

| # | Command(s) | Today | Proposed target |
|---|---|---|---|
| D1 | `orgs select`, `projects select` | summary only | summary + KV of the new context (org, project, render-credential status) |
| D2 | `projects create` | `Created project audit-tmp` | + KV (name, id) so the id is visible without a follow-up call |
| D3 | `rename`/`enable`/`disable`/`delete`/`defaults set/remove` | summaries print the raw id (`Disabled proj_example`) though the user typed a name | summaries name the project (`Disabled audit-tmp2 (proj_…)`) |
| D4 | `defaults set` | `Set 1 default options` | grammar: `1 default option` / `N default options`; plus KV of the resulting defaults |
| D5 | `render`/`screenshot`/`pdf`/`video --dry-run` (shipped surface) | summary only; validated payload invisible in text | + KV of the validated options |
| D6 | `schema render` (shipped surface) | prints a title, no schema | print the schema body in text mode |

Out of scope, unchanged: `commands`, `skill`, `status`, `dashboard`, `upgrade`,
`auth` (bespoke outputs, working; `auth` dies in Plan 2 anyway).

Plan 2 rule: every new command (storage/proxies/llm) ships against the
consistency rule above from day one — lists=table, show=KV+masking, mutations=
named summary + changed-state KV.
