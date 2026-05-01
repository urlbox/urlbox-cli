package cmd

import (
	"sort"
	"strings"
)

// builtInPresets are the canonical defaults the spec ships with. Each preset
// is a partial render-options map; callers merge it in at the *lowest*
// precedence so per-repo config, --json, and flags can override every key.
//
// Adding a new preset:
//  1. Add an entry here.
//  2. Add a covering test in render_presets_test.go (pin the values).
//  3. Update SKILL.md / README so users know it exists.
var builtInPresets = map[string]map[string]any{
	"mobile": {
		"width":      375,
		"height":     812,
		"user_agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	},
	"desktop": {
		"width":  1920,
		"height": 1080,
	},
	"pdf-a4": {
		"format":        "pdf",
		"pdf_page_size": "A4",
	},
}

// applyPreset returns a fresh copy of the named preset's defaults, or nil
// when the name is unknown. The unknown-name case is surfaced to the user
// as a usage error by the caller (with the available names listed).
func applyPreset(name string) map[string]any {
	src, ok := builtInPresets[name]
	if !ok {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// availablePresetNames returns a stable, comma-separated list for use in
// hint text on unknown-preset errors.
func availablePresetNames() string {
	names := make([]string, 0, len(builtInPresets))
	for n := range builtInPresets {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
