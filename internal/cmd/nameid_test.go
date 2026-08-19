package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

type stubListAPI struct {
	resp map[string]any
	err  error
}

func (s stubListAPI) GetJSON(_ context.Context, _ string, out any) error {
	if s.err != nil {
		return s.err
	}
	if dst, ok := out.(*map[string]any); ok {
		*dst = s.resp
	}
	return nil
}

func (s stubListAPI) PostJSON(context.Context, string, any, any) error  { return nil }
func (s stubListAPI) PatchJSON(context.Context, string, any, any) error { return nil }
func (s stubListAPI) PutJSON(context.Context, string, any, any) error   { return nil }
func (s stubListAPI) DeleteJSON(context.Context, string, any) error     { return nil }

func TestResolveNameOrIDPrefixedIDPassesThrough(t *testing.T) {
	rows := []nameID{{ID: "proj_known", Name: "Site"}}
	got, cli := resolveNameOrID("proj_unknown", "proj_", rows, "project")
	if cli != nil {
		t.Fatalf("unexpected error: %v", cli)
	}
	if got.ID != "proj_unknown" {
		t.Fatalf("id = %q, want passthrough", got.ID)
	}
	got, cli = resolveNameOrID("proj_known", "proj_", rows, "project")
	if cli != nil || got.Name != "Site" {
		t.Fatalf("known id should resolve row, got %+v %v", got, cli)
	}
}

func TestResolveNameOrIDMatchesNameCaseInsensitive(t *testing.T) {
	rows := []nameID{{ID: "proj_1", Name: "Production"}, {ID: "proj_2", Name: "Staging"}}
	got, cli := resolveNameOrID("pRoDuCtIoN", "proj_", rows, "project")
	if cli != nil || got.ID != "proj_1" {
		t.Fatalf("got %+v %v", got, cli)
	}
}

func TestResolveNameOrIDNoMatchIsNotFound(t *testing.T) {
	_, cli := resolveNameOrID("nope", "proj_", []nameID{{ID: "proj_1", Name: "A"}}, "project")
	if cli == nil || cli.Code != output.ErrNotFound {
		t.Fatalf("want not_found, got %v", cli)
	}
}

func TestResolveNameOrIDAmbiguityListsIDs(t *testing.T) {
	rows := []nameID{{ID: "proj_1", Name: "Dup"}, {ID: "proj_2", Name: "dup"}}
	_, cli := resolveNameOrID("dup", "proj_", rows, "project")
	if cli == nil || cli.Code != output.ErrValidation {
		t.Fatalf("want validation, got %v", cli)
	}
	if !strings.Contains(cli.Hint, "proj_1") || !strings.Contains(cli.Hint, "proj_2") {
		t.Fatalf("hint must list candidate ids, got %q", cli.Hint)
	}
}

func TestFetchListExtractsObjectsUnderKey(t *testing.T) {
	client := stubListAPI{resp: map[string]any{
		"projects": []any{
			map[string]any{"id": "proj_1", "name": "A"},
			"not-an-object",
			map[string]any{"id": "proj_2", "name": "B"},
		},
	}}
	out, err := fetchList(context.Background(), client, "/v2/projects", "projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 object rows, got %d: %+v", len(out), out)
	}
	if valueOrEmpty(out[0]["id"]) != "proj_1" || valueOrEmpty(out[1]["id"]) != "proj_2" {
		t.Fatalf("rows = %+v", out)
	}
}

func TestFetchListMissingKeyIsEmpty(t *testing.T) {
	client := stubListAPI{resp: map[string]any{}}
	out, err := fetchList(context.Background(), client, "/v2/projects", "projects")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %+v", out)
	}
}

func TestToNameIDs(t *testing.T) {
	rows := toNameIDs([]map[string]any{{"id": "proj_1", "name": "A"}, {"id": 7, "name": nil}})
	if rows[0] != (nameID{ID: "proj_1", Name: "A"}) {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1] != (nameID{}) {
		t.Fatalf("non-string fields must map to empty, got %+v", rows[1])
	}
}
