# HANDOFF: CLI render reporting (fresh session, new feature)

**Repo:** urlbox-cli (this repo). New branch off latest `origin/main` — main has moved past PR #1; fetch first.
**Process:** brainstorm (Arnold, one question at a time) → spec → plan → subagent execution with per-task reviews → Arnold's manual gate. Nothing implements before the spec is approved.

## The feature

Reporting a bad render, from the CLI, via the mono's public render-report endpoints. Three pieces:

1. **Post-render affordance (interactive TTY only).** After a render result, a non-interruptive stderr affordance offers to report it — the interaction pattern is Claude Code's post-output feedback strip (ignorable, doesn't break the flow of work; a keypress starts the reporting flow). It must be: ignorable, never blocking, and completely absent in json/quiet/non-TTY. NOTE: the CLI currently stores nothing about past renders — remembering "the render that just happened" is a design question to solve fresh in the brainstorm.
2. **`urlbox report <renderId>` command.** Agent path: flags, JSON output, no prompts, house error codes. An agent that notices a bad render reports it cold.
3. **`urlbox support` command.** Opens the browser on the Urlbox support/contact page (house `browser` opener; print URL on headless). Small supplement, same feature branch.

**Arnold's rulings already made:**
- LIMITED reporting for now (his stated preference): category + comment. Area/region-specific reporting (the API's `rects`) is deliberately out — it's the dashboard's click-to-highlight concept and too tricky for a first CLI pass.
- Do NOT mine the archived `urlbox/cli` repo's old report-command branch. Ruled out — causes confusion. Design fresh.
- CLI-self-problem reporting (bugs in the CLI itself) is a SEPARATE future feature; not this branch beyond `urlbox support`.
- The 1–4 rating in Claude Code was an interaction-pattern EXAMPLE, not a requested rating feature. Do not build a rating.

## Pinned API contracts (from mono origin/main, 2026-09-11 — RE-VERIFY at build time; the mono is actively changing, Arnold has an in-flight render-reports worktree)

Base: `/v2`, org-scoped. Routes tagged public — callers: dashboard session, bearer session, or org API key (per the v2 controller header).

- `POST /v2/organisation/{org}/render-reports` — body: `renderId` (string 1–255; missing/expired renderIds are accepted by design), `category` (enum below), `comment` (string 1–2000, required), `rects` (optional, CLI sends none). Returns the report DTO.
- `GET /v2/organisation/{org}/render-reports` — filters: `renderId?`, `state?` (`open|in_review|resolved`), `category?`, `limit` (1–100, default 25).
- `POST /v2/organisation/{org}/render-reports/{report}/respond` — `report` id + `message` (1–2000). Probably out of scope for the CLI's first pass; note it exists.

Categories: `bot-detection`, `login-required`, `missing-content`, `cookie-banner-or-popup`, `render-failed`, `other`.

**MUST-VERIFY before design freezes:** which CLI credentials the routes actually accept — the login session token clearly works (bearer session); confirm whether a PROJECT render secret (`URLBOX_API_SECRET`, all an env-only agent has) is honoured as "org API key", or whether workflow 2 requires a session. This decides the command's auth guard and the docs story. Verify live against production, not from code reading alone.

## Open design questions for the brainstorm (Arnold rules on each)

1. The affordance: exact keypress, copy, and when it appears (every render? backoff? dismiss-forever?).
2. What the limited report captures interactively: category picker (house `prompt.SelectOne`) + one comment line? Or category only?
3. Last-render memory: where it lives (config? separate state file?), what it stores, retention. Keep minimal.
4. `urlbox report` flags: `--category`/`--comment` naming, `--last` sugar?, JSON output shape.
5. `urlbox support` target URL (urlbox.com/contact exists; confirm the canonical support page).

## Conventions (all binding, learned/ruled in the previous sessions)

- Zero code comments (linter-forced exported doc comments only). TDD failing-first. `make ci` + `make surface-snapshot` per task; SURFACE.txt committed with the code.
- Production-shape fixtures ONLY — pin every wire shape from the mono contracts / live API; invented fixtures repeatedly caused real bugs.
- Visual rules: lists = shared table renderer, details = KV box, view replaces summary on ok; named mutation summaries. Closed error-code set, non-empty hints, unified login hint. stdout = data, stderr = human.
- Agent rules: every interactive affordance has a flag equivalent; non-TTY never hangs; `--output-format json` everywhere; `--yes` bypasses prompts.
- No profiles in docs/help. Positioning: "website screenshot API", never "web automation"; customer word is "proxies". No em-dashes in customer-facing prose.
- Commits: per-task during execution, squash to one before PR; author `Arnold Cubici-Jones <108676317+AJCJ1@users.noreply.github.com>`; subjects <72 chars; NEVER push without Arnold's word.
- Propose-before-implement with Arnold ALWAYS: questions get answers, not edits; copy is his; decisions surface for veto, including knock-on wording. Validate findings against the real npm binary / live API before claiming anything (renders are free on his staff account).
- Docs for this feature (fumadocs CLI section + command reference) ride the same effort; page-by-page with Arnold.
