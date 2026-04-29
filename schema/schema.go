// Package schema exposes the embedded JSON Schemas for Urlbox API payloads.
package schema

import _ "embed"

// RenderJSON is the embedded JSON Schema describing the render request payload.
// Sourced from urlbox-mono/packages/types/src/render/render.types.ts and synced
// via the .github/workflows/sync-cli-schema.yml workflow in the monorepo.
//
//go:embed render.json
var RenderJSON []byte
