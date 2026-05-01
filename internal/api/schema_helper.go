package api

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/urlbox/urlbox-cli/schema"
)

// schemaState holds the lazily-parsed render schema. Parsed once, read forever.
var schemaState struct {
	once  sync.Once
	props map[string]schemaProperty
}

type schemaProperty struct {
	Enum []string `json:"enum"`
}

// EnumValuesFor returns a comma-separated list of valid enum values for the
// given top-level render-option field. Returns "" when the field has no enum
// or doesn't exist (string/number fields fall back to the natural-language
// description in the help text).
//
// The schema is parsed once on first call; subsequent calls are concurrency-safe.
func EnumValuesFor(field string) string {
	schemaState.once.Do(parseSchema)
	p, ok := schemaState.props[field]
	if !ok {
		return ""
	}
	return strings.Join(p.Enum, ", ")
}

func parseSchema() {
	var s struct {
		Properties map[string]schemaProperty `json:"properties"`
	}
	if err := json.Unmarshal(schema.RenderJSON, &s); err != nil {
		// Schema is embedded at build time; if it fails to parse, that's a
		// build-time bug. Fall through silently — EnumValuesFor returns "".
		return
	}
	schemaState.props = s.Properties
}
