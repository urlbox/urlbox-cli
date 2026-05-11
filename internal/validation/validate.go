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
//  1. SanitizeRaw (size cap; local hard error)
//  2. JSON parse (local hard error)
//  3. Per-field sanitize for known sensitive string fields, e.g. url control
//     characters (local hard error)
//  4. Unknown-key warning collection — returns a non-fatal warning when an
//     unknown key fuzzy-matches a known option; silent passthrough when no
//     match. Never returns an error.
//
// Notably ABSENT: a schema gate. v0.9.0 removed the compiled.Validate(payload)
// call so --json passes through to the API verbatim — the API is the single
// source of truth for option validation and emits structured errors. The
// embedded schema remains informational: it powers typed flags, help text,
// and the typo-suggestion list. See spec at
// ~/.claude/specs/2026-05-05-cli-schema-as-documentation-design.md (§5.3).
//
// Returns (payload, warnings, nil) on success. Returns (nil, nil, *CLIError)
// for the local hard errors above. Warnings are owned by the caller — no
// package-global state is mutated (Arch I3 from Round 1 review).
func ValidatePayload(b []byte) (payload map[string]any, warnings []string, cliErr *output.CLIError) {
	if err := SanitizeRaw(b); err != nil {
		return nil, nil, err
	}

	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, nil, output.NewCLIError(
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
		return nil, nil, output.NewCLIError(
			output.ErrServer,
			"Build defect — "+schemaErr.Error(),
			"Please open an issue.",
		)
	}

	// Per-field sanitize for known sensitive string fields.
	for fieldName := range sanitizedStringFields {
		if v, ok := payload[fieldName]; ok {
			if s, ok := v.(string); ok {
				if cErr := SanitizeStringField(fieldName, s); cErr != nil {
					return nil, nil, cErr
				}
			}
		}
	}

	warnings = unknownKeyWarnings(payload, known)
	return payload, warnings, nil
}

// unknownKeyWarnings walks payload keys, finds those not present in the
// schema's known top-level property set, runs each through ClosestMatch,
// and returns one warning per fuzzy-matchable typo collapsed into a single
// summary line when there are 2+. Truly novel keys (no fuzzy hit) are silent.
//
// Unknown keys are ALWAYS passed through verbatim — the warning is
// informational, not a correction. The API decides.
//
// Pure function — no package-globals; callers own the returned slice.
func unknownKeyWarnings(payload map[string]any, known []string) []string {
	if len(known) == 0 {
		return nil
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
	}
	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].name < matches[j].name
	})

	if len(matches) == 1 {
		m := matches[0]
		return []string{fmt.Sprintf(
			`unknown option %q — did you mean %q? (sending verbatim; the API will decide)`,
			m.name, m.suggestion,
		)}
	}

	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, fmt.Sprintf(`%s → %q`, m.name, m.suggestion))
	}
	return []string{
		"unknown options — did you mean: " + strings.Join(parts, ", ") +
			"? (sending verbatim; the API will decide)",
	}
}
