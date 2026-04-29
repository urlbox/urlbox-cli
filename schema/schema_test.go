// schema/schema_test.go
package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/schema"
)

func TestRenderSchema_NotEmpty(t *testing.T) {
	if len(schema.RenderJSON) == 0 {
		t.Fatal("schema.RenderJSON is empty")
	}
}

func TestRenderSchema_ParseableJSON(t *testing.T) {
	var obj map[string]any
	if err := json.Unmarshal(schema.RenderJSON, &obj); err != nil {
		t.Fatalf("schema.RenderJSON is not valid JSON: %v", err)
	}
	if _, ok := obj["properties"]; !ok {
		t.Fatalf("schema missing top-level 'properties' key; keys=%v", keys(obj))
	}
}

func TestRenderSchema_HasCoreFields(t *testing.T) {
	var obj map[string]any
	if err := json.Unmarshal(schema.RenderJSON, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props, ok := obj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("'properties' is not an object: %T", obj["properties"])
	}
	required := []string{"url", "format", "width", "height", "full_page"}
	for _, key := range required {
		if _, ok := props[key]; !ok {
			t.Errorf("schema is missing core field %q", key)
		}
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
