// internal/validation/validate.go
package validation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/schema"
)

// sanitizedStringFields enumerates the string fields whose values must pass
// SanitizeStringField (control-char rejection) before passthrough to the API.
// Currently just URL-like fields.
var sanitizedStringFields = map[string]struct{}{
	"url": {},
}

var (
	compiledSchema    *jsonschema.Schema
	compiledSchemaErr error
	knownTopLevelKeys []string
	compileSchemaOnce sync.Once
)

// warningsMu guards lastWarnings. The slice holds non-fatal stderr-bound
// messages produced by the most recent ValidatePayload call (e.g. fuzzy-typo
// suggestions for unknown options that are passed through verbatim).
var (
	warningsMu   sync.Mutex
	lastWarnings []string
)

// ResetWarnings clears the package-level warning buffer. ValidatePayload
// invokes this at the top of every call; callers wiring warnings into stderr
// (e.g. the render command) generally don't need to call it directly.
func ResetWarnings() {
	warningsMu.Lock()
	defer warningsMu.Unlock()
	lastWarnings = nil
}

// LastWarnings returns a copy of the warnings produced by the most recent
// ValidatePayload call. The render command (B6) reads this and prints each
// entry to stderr; tests assert against the slice directly.
func LastWarnings() []string {
	warningsMu.Lock()
	defer warningsMu.Unlock()
	if len(lastWarnings) == 0 {
		return nil
	}
	out := make([]string, len(lastWarnings))
	copy(out, lastWarnings)
	return out
}

// appendWarning is the internal mutator. Use ResetWarnings to clear and
// LastWarnings to read.
func appendWarning(s string) {
	warningsMu.Lock()
	defer warningsMu.Unlock()
	lastWarnings = append(lastWarnings, s)
}

// loadSchema compiles the embedded render schema once per process and returns
// the compiled schema, the sorted list of known top-level property names, and
// any compile error (sticky across callers).
//
// As of v0.9.0 the compiled schema is no longer used by ValidatePayload as a
// gate (see the ValidatePayload comment). It is kept loaded because the schema
// surface command and other introspection paths depend on it, and the
// known-key list it produces still drives fuzzy typo suggestions.
func loadSchema() (*jsonschema.Schema, []string, error) {
	compileSchemaOnce.Do(func() {
		s, keys, err := loadSchemaFrom(schema.RenderJSON)
		compiledSchema = s
		knownTopLevelKeys = keys
		if err != nil {
			compiledSchemaErr = fmt.Errorf("embedded render schema: %w", err)
		}
	})
	return compiledSchema, knownTopLevelKeys, compiledSchemaErr
}

// loadSchemaFrom is the testable inner: compiles a JSON Schema from arbitrary
// bytes, returns the compiled schema, sorted top-level property keys, and
// error. Callers with stable embedded schemas should use loadSchema (memoized).
func loadSchemaFrom(b []byte) (*jsonschema.Schema, []string, error) {
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, nil, fmt.Errorf("decode schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("render.json", raw); err != nil {
		return nil, nil, fmt.Errorf("register schema: %w", err)
	}
	s, err := compiler.Compile("render.json")
	if err != nil {
		return nil, nil, fmt.Errorf("compile schema: %w", err)
	}
	var keys []string
	if props, ok := raw["properties"].(map[string]any); ok {
		keys = make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	return s, keys, nil
}

// ValidatePayload runs the v0.9.0 "schema as documentation" pipeline over a
// raw --json payload.
//
// Order of checks:
//  1. ResetWarnings (clears any leftover state from a previous call)
//  2. SanitizeRaw (size cap; local hard error)
//  3. JSON parse (local hard error)
//  4. Per-field sanitize for known sensitive string fields, e.g. url control
//     characters (local hard error)
//  5. Unknown-key warning recorder — emits a non-fatal warning via
//     appendWarning when an unknown key fuzzy-matches a known option;
//     silent passthrough when no match. Never returns an error.
//
// Notably ABSENT: a schema gate. v0.9.0 removed the compiled.Validate(payload)
// call so --json passes through to the API verbatim — the API is the single
// source of truth for option validation and emits structured errors. The
// embedded schema remains informational: it powers typed flags, help text,
// and the typo-suggestion list. See spec at
// ~/.claude/specs/2026-05-05-cli-schema-as-documentation-design.md (§5.3).
//
// Returns the parsed payload as map[string]any on success. Returns
// (nil, *output.CLIError) only for the four local hard errors above (size
// cap, malformed JSON, control characters, embedded-schema build defect).
func ValidatePayload(b []byte) (map[string]any, *output.CLIError) {
	ResetWarnings()

	if err := SanitizeRaw(b); err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, output.NewCLIError(
			output.ErrValidation,
			"Payload is not valid JSON: "+err.Error(),
			"Pass valid JSON. Example: --json '{\"url\":\"https://example.com\"}'",
		)
	}

	_, known, schemaErr := loadSchema()
	if schemaErr != nil {
		// schemaErr already starts with "embedded render schema:" — don't
		// re-prefix or we get "Embedded render schema is unavailable:
		// embedded render schema: ...".
		return nil, output.NewCLIError(
			output.ErrServer,
			"Build defect — "+schemaErr.Error(),
			"Please open an issue.",
		)
	}

	// Per-field sanitize for known sensitive string fields.
	for fieldName := range sanitizedStringFields {
		if v, ok := payload[fieldName]; ok {
			if s, ok := v.(string); ok {
				if cliErr := SanitizeStringField(fieldName, s); cliErr != nil {
					return nil, cliErr
				}
			}
		}
	}

	// Record warnings for unknown keys that look like typos. Never returns
	// an error: anything goes through to the API.
	recordUnknownKeyWarnings(payload, known)

	return payload, nil
}

// recordUnknownKeyWarnings walks payload keys, finds those not present in
// the schema's known top-level property set, and runs each through
// ClosestMatch. For every fuzzy-matchable typo it appends a friendly stderr
// warning via appendWarning; truly novel keys (no fuzzy hit) are silent.
//
// Multiple matchable typos collapse into a single summary warning. Unknown
// keys are ALWAYS passed through verbatim — the warning is informational,
// not a correction. The API decides.
func recordUnknownKeyWarnings(payload map[string]any, known []string) {
	if len(known) == 0 {
		return
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, k := range known {
		knownSet[k] = struct{}{}
	}

	type match struct {
		name       string
		suggestion string
	}
	var matches []match
	for key := range payload {
		if _, ok := knownSet[key]; ok {
			continue
		}
		if suggestion, ok := ClosestMatch(key, known); ok {
			matches = append(matches, match{name: key, suggestion: suggestion})
		}
		// No fuzzy match → silent passthrough.
	}
	if len(matches) == 0 {
		return
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].name < matches[j].name
	})

	if len(matches) == 1 {
		m := matches[0]
		appendWarning(fmt.Sprintf(
			`unknown option %q — did you mean %q? (sending verbatim; the API will decide)`,
			m.name, m.suggestion,
		))
		return
	}

	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, fmt.Sprintf(`%s → %q`, m.name, m.suggestion))
	}
	appendWarning(
		"unknown options — did you mean: " + strings.Join(parts, ", ") +
			"? (sending verbatim; the API will decide)",
	)
}
