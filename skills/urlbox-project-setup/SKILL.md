---
name: urlbox-project-setup
version: 1.0.0
description: Create and configure a Urlbox project from the CLI, including defaults, storage, proxy, and LLM settings.
---

# Urlbox Project Setup

## Use This For

- Creating a new project
- Applying project defaults
- Configuring storage, proxy, or LLM credentials
- Auditing project settings without opening the dashboard

## Recommended Order

1. Create the project.
2. Capture the project ID.
3. Configure defaults.
4. Configure storage or LLM if needed.
5. Verify with `projects show`.

## Examples

Create a project:

```bash
urlbox projects create "Agent Project" --output-format json
```

Set default render options:

```bash
urlbox projects defaults set PROJECT_ID --dry-run --json '{
  "width": 1920,
  "full_page": true,
  "format": "png"
}'
```

Then persist:

```bash
urlbox projects defaults set PROJECT_ID --json '{
  "width": 1920,
  "full_page": true,
  "format": "png"
}'
```

Configure S3-compatible storage:

```bash
urlbox projects storage set PROJECT_ID --json '{
  "provider": "aws_s3",
  "key": "KEY",
  "secret": "SECRET",
  "bucket": "renders",
  "region": "us-east-1"
}'
```

Test without saving:

```bash
urlbox projects storage test PROJECT_ID --json '{
  "provider": "aws_s3",
  "key": "KEY",
  "secret": "SECRET",
  "bucket": "renders",
  "region": "us-east-1"
}'
```

Configure an LLM:

```bash
urlbox projects llm set PROJECT_ID --json '{
  "provider": "anthropic",
  "key": "sk-ant-...",
  "model": "claude-sonnet-4-5-20250514"
}'
```

Configure a project proxy:

```bash
urlbox projects proxy set PROJECT_ID --url 'https://user:pass@proxy.example.com:8080'
```

Verify:

```bash
urlbox projects show PROJECT_ID --output-format json
```

## Guidance

- Use `projects defaults set --dry-run` before persisting defaults.
- Prefer `--json` for storage and LLM because those payloads are nested and provider-specific.
- Use `--reveal` only when you explicitly need to inspect sensitive values.
- Use explicit project IDs; there is no implicit “default project” workflow yet.
