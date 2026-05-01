package cmd

import (
	"encoding/json"
	"strings"
)

// buildCurl returns a deterministic, copy-pasteable curl command equivalent
// to the request the render command would have made. The API secret is
// never printed — it's redacted as the literal text $URLBOX_API_SECRET so
// the user can paste the command into a shell with the env var set.
//
// The body is JSON with alpha-sorted keys (Go's encoding/json marshals
// maps in sorted-key order since 1.12, so this is implicit; nested maps
// inherit the same behavior because we recurse). Output is a single line —
// no shell continuations — per the project's preference for paste-friendly
// commands.
//
// Embedded single quotes in any string value are escaped via the standard
// '\” trick: close the open quote, emit a literal '\”, re-open.
func buildCurl(baseURL, path string, opts map[string]any) string {
	body, _ := json.Marshal(opts)

	var b strings.Builder
	b.WriteString("curl -X POST ")
	b.WriteString(quoteShell(baseURL + path))
	b.WriteString(" -H ")
	b.WriteString(quoteShell("Authorization: Bearer $URLBOX_API_SECRET"))
	b.WriteString(" -H ")
	b.WriteString(quoteShell("Content-Type: application/json"))
	b.WriteString(" -d ")
	b.WriteString(quoteShell(string(body)))
	return b.String()
}

// quoteShell wraps s in single quotes, escaping any embedded single quote
// using the POSIX-portable '\” trick.
func quoteShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
