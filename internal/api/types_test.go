package api_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func TestResponse_JSON_RoundTrip(t *testing.T) {
	// `data` payload uses camelCase to match the locked Urlbox API wire
	// format (renderUrl, renderId — see urlbox-mono apps/api).
	src := `{
		"ok": true,
		"command": "render",
		"data": {"renderId": "ps_abc", "renderUrl": "https://cdn2.urlbox.io/x.png", "size": 245632},
		"summary": "Rendered example.com as PNG (245 KB)",
		"breadcrumbs": [{"action": "save", "cmd": "urlbox render https://example.com --output out.png"}]
	}`
	var got api.Response
	if err := json.Unmarshal([]byte(src), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.OK {
		t.Errorf("OK=false, want true")
	}
	if got.Command != "render" {
		t.Errorf("Command=%q, want %q", got.Command, "render")
	}
	if got.Data["renderId"] != "ps_abc" {
		t.Errorf("Data.renderId=%v", got.Data["renderId"])
	}
	if got.Summary != "Rendered example.com as PNG (245 KB)" {
		t.Errorf("Summary mismatch: %q", got.Summary)
	}
	if len(got.Breadcrumbs) != 1 {
		t.Fatalf("len(Breadcrumbs)=%d, want 1", len(got.Breadcrumbs))
	}
	if got.Breadcrumbs[0].Action != "save" {
		t.Errorf("Breadcrumbs[0].Action=%q", got.Breadcrumbs[0].Action)
	}

	out, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTrip api.Response
	if err := json.Unmarshal(out, &roundTrip); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if roundTrip.Data["renderId"] != got.Data["renderId"] {
		t.Errorf("round-trip lost renderId")
	}
}

func TestResponse_Error_FieldsOmitWhenEmpty(t *testing.T) {
	// A success response should not emit "error", "code", "hint" keys.
	r := api.Response{
		OK:      true,
		Command: "render",
		Data:    map[string]any{"renderId": "ps_abc"},
		Summary: "ok",
	}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	for _, k := range []string{`"error"`, `"code"`, `"hint"`} {
		if strings.Contains(s, k) {
			t.Errorf("success response leaked key %s; got %s", k, s)
		}
	}
}

// Compile-time interface satisfaction probe: a no-op stub here proves the
// interface methods are exactly the three documented (Render, RenderAsync,
// Status) and have the documented signatures.
type stubClient struct{}

func (stubClient) Render(_ context.Context, _ map[string]any) (*api.Response, error) {
	return nil, nil
}

func (stubClient) RenderAsync(_ context.Context, _ map[string]any) (*api.Response, error) {
	return nil, nil
}

func (stubClient) Status(_ context.Context, _ string) (*api.Response, error) {
	return nil, nil
}

var _ api.Client = stubClient{}
