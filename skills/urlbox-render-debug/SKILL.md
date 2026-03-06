---
name: urlbox-render-debug
version: 1.0.0
description: Debug Urlbox render requests by validating payloads locally, iterating on options, and switching between sync and async runs.
---

# Urlbox Render Debug

## Use This For

- Verifying render option names
- Catching invalid payloads before spending credits
- Iterating on timeouts, selectors, and viewport settings
- Switching from sync debugging to async polling
- Wiring render callbacks via `webhook_url`
- Rendering from inline HTML or an HTML file
- Generating document output like `pdf` or `svg`

## Recommended Flow

1. Inspect the schema for unknown options.
2. Build the payload with `--json`.
3. Run `--dry-run`.
4. Execute a sync render.
5. Switch to `--async` or `--bg` when the request is stable.

## Examples

Inspect the schema:

```bash
urlbox schema render
urlbox schema render.full_page
```

Validate locally:

```bash
urlbox render https://example.com --json '{
  "format": "png",
  "width": 1440,
  "full_page": true,
  "delay": 1500
}' --dry-run
```

Sync render:

```bash
urlbox render https://example.com --format png --width 1440 --full-page
```

SVG render:

```bash
urlbox render https://example.com --format svg
```

Render from inline HTML:

```bash
urlbox render \
  --html '<html><body><h1>Hello</h1></body></html>' \
  --format png \
  --dry-run
```

Render from stdin HTML:

```bash
cat page.html | urlbox render \
  --html - \
  --format pdf \
  --dry-run
```

Render from an HTML file:

```bash
urlbox render \
  --html-file ./fixtures/page.html \
  --format pdf \
  --full-page
```

Async render:

```bash
urlbox render https://example.com --json '{"format":"pdf","full_page":true}' --async
urlbox status RENDER_ID --wait
```

Async render with callback:

```bash
urlbox render https://example.com \
  --async \
  --webhook-url https://hooks.example.com/urlbox \
  --format png
```

Background job tracking:

```bash
urlbox render https://example.com --json '{"format":"png"}' --bg
urlbox jobs
urlbox jobs --wait
```

## Guidance

- If the payload is nested or non-trivial, prefer `--json`.
- If you get an “unknown option” error, check `urlbox schema render` instead of guessing.
- Use sync mode first for quick feedback.
- Use `--webhook-url` for documented async callbacks instead of the internal webhook CRUD surface.
- Use `--output` when you want a predictable saved file path in local debugging.
