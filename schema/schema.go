// Package schema exposes the embedded JSON Schemas for Urlbox API payloads.
package schema

import _ "embed"

// RenderJSON is the embedded JSON Schema describing the render request payload.
// Generated from the Urlbox API's render option types and kept in sync
// automatically.
//
//go:embed render.json
var RenderJSON []byte
