---
name: urlbox-batch-screenshots
version: 1.0.0
description: Run batch screenshot or PDF jobs with dry-run validation, matrix expansion, and NDJSON output.
---

# Urlbox Batch Screenshots

## Use This For

- Taking the same screenshot across many URLs
- Rendering the same URLs in multiple widths or formats
- Agent workflows that need incremental machine-readable output

## Recommended Flow

1. Build the batch payload.
2. Validate it with `--dry-run`.
3. Run it with `--output-format ndjson`.

## Examples

Inline JSON array:

```bash
urlbox batch --dry-run --json '[
  {"url":"https://example.com","format":"png","width":1440},
  {"url":"https://example.com/pricing","format":"png","width":1440}
]'
```

Single HTML document:

```bash
urlbox batch --html '<html><body>Batch</body></html>' --format pdf --dry-run
```

Single HTML file:

```bash
urlbox batch --html-file ./page.html --format svg --dry-run
```

Single HTML document from stdin:

```bash
cat page.html | urlbox batch --html - --format pdf --dry-run
```

Matrix expansion:

```bash
urlbox batch --dry-run --json '{
  "urls": ["https://example.com", "https://example.com/pricing"],
  "matrix": {
    "format": ["png", "pdf"],
    "width": [1280, 1920]
  },
  "options": {
    "full_page": true
  }
}'
```

Incremental output for agents:

```bash
urlbox batch --json @payload.json --output-format ndjson
```

If shell `@file` expansion is unavailable, use:

```bash
urlbox batch --file payload.json --output-format ndjson
```

Async callbacks for each entry:

```bash
urlbox batch \
  --file payload.json \
  --async \
  --webhook-url https://hooks.example.com/urlbox \
  --output-format ndjson
```

## Guidance

- Prefer `--json` or `--file` over long flag lists for nested inputs.
- Use `--concurrency` conservatively; start with `1` or `2` if API behavior is unknown.
- Use `--async` or `--bg` when you need render IDs instead of immediate sync completion.
- Use `--webhook-url` when your async workflow relies on Urlbox callback delivery.
- Use `--bg` when you want local tracking via `urlbox jobs`.
