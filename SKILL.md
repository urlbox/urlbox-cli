---
name: urlbox-cli
version: 1.0.0
description: High-level guide for using the Urlbox CLI as an agent-friendly interface to the Urlbox screenshot and web automation API.
---

# Urlbox CLI

`urlbox` is an agent-friendly command-line interface for the Urlbox API.

It is designed to work well for both humans and agents:

- predictable flags for common operations
- `--dry-run` validation before network calls
- `--output-format json` for structured single-command output
- `--output-format ndjson` for incremental batch output
- schema inspection via `urlbox schema render`
- direct support for URLs or raw HTML input

## What It Can Generate

From a URL or raw HTML, the CLI can request:

- screenshots: `png`, `jpg`, `webp`, `avif`
- document/vector output: `pdf`, `svg`
- videos: `mp4`, `webm`
- extracted page content: `html`, `mhtml`, `md`

## Core Commands

- `urlbox render`
- `urlbox batch`
- `urlbox status`
- `urlbox jobs`
- `urlbox schema`
- `urlbox projects`
- `urlbox config`
- `urlbox auth`
- `urlbox usage`

## Agent-Friendly Patterns

Use a URL:

```bash
urlbox render https://example.com --format png --width 1440 --output-format json
```

Use raw HTML inline:

```bash
urlbox render --html '<html><body><h1>Hello</h1></body></html>' --format png --dry-run
```

Use raw HTML from stdin:

```bash
cat page.html | urlbox render --html - --format pdf --dry-run
```

Use an HTML file:

```bash
urlbox render --html-file ./fixtures/page.html --format pdf --full-page
```

Use async callback delivery:

```bash
urlbox render https://example.com \
  --async \
  --webhook-url https://hooks.example.com/urlbox \
  --output-format json
```

Use batch mode for many renders:

```bash
urlbox batch --file payload.json --output-format ndjson
```

Use batch mode with raw HTML:

```bash
urlbox batch --html-file ./fixtures/page.html --format svg --dry-run
```

## Recommended Workflow For Agents

1. Inspect the schema when you are unsure of option names.
2. Prefer `--json` for nested payloads.
3. Use `--dry-run` before expensive or high-volume runs.
4. Use `--async` plus `urlbox status` or `--webhook-url` for async workflows.
5. Use `--output-format ndjson` for `batch`.

## Schema Discovery

```bash
urlbox schema render
urlbox schema render.full_page
```

## HTML Input

The CLI supports both:

- `--html '<html>...</html>'`
- `--html -` to read HTML from stdin
- `--html-file path/to/file.html`

That means agents do not need to wrap HTML payloads inside raw JSON for the common case.

## Related Skill Files

- `skills/urlbox-render-debug/SKILL.md`
- `skills/urlbox-batch-screenshots/SKILL.md`
- `skills/urlbox-project-setup/SKILL.md`
