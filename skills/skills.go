// Package skills exposes the embedded SKILL.md as a Go string.
package skills

import _ "embed"

// Content is the embedded skill file content (Markdown).
//
//go:embed SKILL.md
var Content string
