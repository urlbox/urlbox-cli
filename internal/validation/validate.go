// internal/validation/validate.go
package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/schema"
)

// sanitizedStringFields enumerates the string fields whose values must pass
// SanitizeStringField (control-char rejection) before schema validation.
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
func loadSchema() (*jsonschema.Schema, []string, error) {
	compileSchemaOnce.Do(func() {
		var raw map[string]any
		if err := json.Unmarshal(schema.RenderJSON, &raw); err != nil {
			compiledSchemaErr = fmt.Errorf("decode embedded render schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("render.json", raw); err != nil {
			compiledSchemaErr = fmt.Errorf("register embedded render schema: %w", err)
			return
		}
		s, err := compiler.Compile("render.json")
		if err != nil {
			compiledSchemaErr = fmt.Errorf("compile embedded render schema: %w", err)
			return
		}
		compiledSchema = s

		if props, ok := raw["properties"].(map[string]any); ok {
			knownTopLevelKeys = make([]string, 0, len(props))
			for k := range props {
				knownTopLevelKeys = append(knownTopLevelKeys, k)
			}
			sort.Strings(knownTopLevelKeys)
		}
	})
	return compiledSchema, knownTopLevelKeys, compiledSchemaErr
}

// ValidatePayload runs the full validation pipeline over a raw --json payload.
//
// Order of checks:
//  1. SanitizeRaw (size cap)
//  2. JSON parse
//  3. Top-level fuzzy correction on unknown keys
//  4. Per-field sanitize for known sensitive string fields (e.g. url)
//  5. JSON Schema validation against the embedded render schema
//
// On success returns the parsed payload as map[string]any. On failure returns
// (nil, *output.CLIError) with code "validation" (or "server" if the embedded
// schema itself is unreadable, which would be a build defect).
func ValidatePayload(b []byte) (map[string]any, *output.CLIError) {
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

	compiled, known, schemaErr := loadSchema()
	if schemaErr != nil {
		return nil, output.NewCLIError(
			output.ErrServer,
			"Embedded render schema is unavailable: "+schemaErr.Error(),
			"This is a build defect. Please open an issue.",
		)
	}

	// Fuzzy-correct unknown top-level keys before formal schema validation —
	// gives nicer agent UX than schema's generic "additionalProperties" error.
	if cliErr := checkUnknownKeys(payload, known); cliErr != nil {
		return nil, cliErr
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

	// JSON Schema validation.
	if err := compiled.Validate(payload); err != nil {
		return nil, schemaErrorToCLIError(err)
	}

	return payload, nil
}

// checkUnknownKeys collects payload keys not present in the schema's known
// top-level property set, suggests fuzzy corrections, and returns a single
// validation CLIError summarising them. Returns nil when every key is known.
func checkUnknownKeys(payload map[string]any, known []string) *output.CLIError {
	if len(known) == 0 {
		return nil
	}
	knownSet := make(map[string]struct{}, len(known))
	for _, k := range known {
		knownSet[k] = struct{}{}
	}

	type unknown struct {
		name       string
		suggestion string
		hasMatch   bool
	}
	var unknowns []unknown
	for key := range payload {
		if _, ok := knownSet[key]; ok {
			continue
		}
		match, hasMatch := ClosestMatch(key, known)
		unknowns = append(unknowns, unknown{name: key, suggestion: match, hasMatch: hasMatch})
	}
	if len(unknowns) == 0 {
		return nil
	}

	sort.Slice(unknowns, func(i, j int) bool {
		return unknowns[i].name < unknowns[j].name
	})

	if len(unknowns) == 1 {
		u := unknowns[0]
		msg := "Unknown option: " + u.name
		if u.hasMatch {
			return output.NewCLIError(output.ErrValidation, msg, fmt.Sprintf(`Did you mean %q?`, u.suggestion))
		}
		return output.NewCLIError(output.ErrValidation, msg, "Run `urlbox schema render` to see all valid options.")
	}

	names := make([]string, 0, len(unknowns))
	parts := make([]string, 0, len(unknowns))
	anyMatch := false
	for _, u := range unknowns {
		names = append(names, u.name)
		if u.hasMatch {
			parts = append(parts, fmt.Sprintf(`%s → %q`, u.name, u.suggestion))
			anyMatch = true
		}
	}
	msg := "Unknown options: " + strings.Join(names, ", ")
	if anyMatch {
		return output.NewCLIError(
			output.ErrValidation,
			msg,
			"Did you mean: "+strings.Join(parts, ", ")+"?",
		)
	}
	return output.NewCLIError(output.ErrValidation, msg, "Run `urlbox schema render` to see all valid options.")
}

// schemaErrorToCLIError converts a jsonschema validation error into a
// validation-class CLIError, surfacing the offending field path in the hint.
func schemaErrorToCLIError(err error) *output.CLIError {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return output.NewCLIError(
			output.ErrValidation,
			"Payload validation failed: "+err.Error(),
			"",
		)
	}

	leaf := leafCause(ve)
	path := leafPath(leaf)
	hint := ""
	if path != "" {
		hint = "Field \"" + path + "\" failed validation. Run `urlbox schema render` to see its constraints."
	}
	detail := leafSummary(leaf)
	return output.NewCLIError(output.ErrValidation, "Payload validation failed: "+detail, hint)
}

// leafCause walks the error tree to the deepest first cause — the most
// specific failure to surface to the user.
func leafCause(ve *jsonschema.ValidationError) *jsonschema.ValidationError {
	for len(ve.Causes) > 0 {
		ve = ve.Causes[0]
	}
	return ve
}

// leafPath returns a dotted path for the leaf's instance location, e.g.
// "width" or "options.foo". Empty string for the document root.
func leafPath(ve *jsonschema.ValidationError) string {
	if len(ve.InstanceLocation) == 0 {
		return ""
	}
	return strings.Join(ve.InstanceLocation, ".")
}

// leafSummary renders a short, human-readable description of the leaf error.
// We rely on the upstream Error() formatter (which uses an English printer
// internally) to produce a single-line "at \"<ptr>\": <kind>" string for the
// leaf, then trim any trailing whitespace from the formatter.
func leafSummary(ve *jsonschema.ValidationError) string {
	return strings.TrimSpace(ve.Error())
}
