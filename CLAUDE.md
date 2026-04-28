# Urlbox CLI — Developer Rules

This file is auto-loaded by Claude Code when working in this repo.

## Spec
Canonical: `/Users/gustavomeneses/code/urlbox/urlbox-mono/urlbox-cli-final-spec.md`

## Non-negotiables
- **TDD from commit one.** Failing test → minimal implementation → green → refactor → commit.
- **Lint must be clean.** `make lint` and `make fmt-check` before every commit.
- **Stdout for data, stderr for human messages.** Never mix.
- **Every command, subcommand, and flag must appear in `SURFACE.txt`.**
  Adding new entries is fine; removing or renaming triggers `make surface-check` failure.
  After an intentional surface change, run `make surface-snapshot` and commit `SURFACE.txt` with the code.
- **Never commit autonomously.** Show diff + proposed message, wait for approval.

## Workflow when adding a command
1. Write failing tests in `*_test.go` (envelope shape, exit codes, edge cases).
2. Implement minimum code to pass.
3. `make ci` (fmt-check, lint, test, build, surface-check).
4. `make surface-snapshot` to refresh `SURFACE.txt`.
5. Update `skills/SKILL.md` if the command is agent-relevant.
6. Update `README.md` (and `npm/README.md` if user-facing).

## Output contract (Phase 1)
- Every command output uses `internal/output.Envelope` (success) or `ErrorEnvelope` (failure).
- Errors map to closed-set `ErrorCode` → exit code (see `internal/output/errors.go`).
- Breadcrumbs (`output.Breadcrumb{Action, Cmd}`) point agents at next reasonable command.

## Useful entrypoints when navigating the code
- Command tree: `internal/cmd/root.go` (`newRootCmd` registers subcommands)
- Output envelope/formatters: `internal/output/{envelope,format,errors,tty,style}.go`
- Surface snapshot: `internal/surface/snapshot.go`
- Config (XDG-aware key storage): `internal/config/config.go`
- Embedded skill: `skills/SKILL.md` + `skills/skills.go`
