# Urlbox CLI Agent Context

## Current Command Surface

The CLI currently exposes these top-level commands:

- `urlbox render`
- `urlbox batch`
- `urlbox status`
- `urlbox jobs`
- `urlbox schema`
- `urlbox projects`
- `urlbox config`
- `urlbox auth`
- `urlbox usage`

## Critical Rules

- Use `--dry-run` before mutation-heavy or high-volume work when it is available.
- Prefer `--json` for nested payloads like render options, defaults, storage, and LLM config.
- Prefer `--output-format json` for single-command automation.
- Prefer `--output-format ndjson` for `urlbox batch` so results can be processed incrementally.
- Use `urlbox schema render` before guessing render option names.
- For project defaults, validate locally first with `urlbox projects defaults set PROJECT_ID --dry-run`.
- `urlbox batch --bg` stores async render IDs in the local jobs registry; use `urlbox jobs` to inspect them.

## Current Limitations

- There is no interactive project editor yet.
- Webhook CRUD is internal-only for now and is not part of the public CLI surface.
- Project selection is explicit by project ID for project-setting commands.

## Common Patterns

- Screenshot: `urlbox render URL --format png --width 1920`
- SVG: `urlbox render URL --format svg`
- Screenshot from raw HTML: `urlbox render --html '<html>...</html>' --format png`
- PDF from stdin HTML: `cat page.html | urlbox render --html - --format pdf`
- PDF from HTML file: `urlbox render --html-file ./page.html --format pdf --full-page`
- Full-page PDF: `urlbox render URL --format pdf --full-page`
- Async render callback: `urlbox render URL --async --webhook-url https://hooks.example.com/urlbox`
- Validate a render payload: `urlbox render URL --json '{"full_page":true}' --dry-run`
- Batch screenshots: `urlbox batch --json '[{"url":"https://a.com"},{"url":"https://b.com"}]' --output-format ndjson`
- Batch from raw HTML file: `urlbox batch --html-file ./page.html --format svg --dry-run`
- Batch async callbacks: `urlbox batch --file payload.json --async --webhook-url https://hooks.example.com/urlbox --output-format ndjson`
- Batch matrix expansion: `urlbox batch --json '{"urls":["https://a.com"],"matrix":{"format":["png","pdf"]}}' --dry-run`
- Show render schema: `urlbox schema render`
- Check async status: `urlbox status RENDER_ID --wait`
- Configure project defaults: `urlbox projects defaults set PROJECT_ID --json '{"width":1920}'`
- Configure storage: `urlbox projects storage set PROJECT_ID --json '{...}'`
- Configure LLM: `urlbox projects llm set PROJECT_ID --json '{...}'`
