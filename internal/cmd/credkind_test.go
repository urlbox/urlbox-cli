package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestCreateNamePositionalOnly(t *testing.T) {
	name, cliErr := createName(&cobra.Command{}, []string{"prod"}, "")
	if cliErr != nil || name != "prod" {
		t.Fatalf("positional-only: name=%q err=%v", name, cliErr)
	}
}

func TestCreateNameFlagOnly(t *testing.T) {
	name, cliErr := createName(&cobra.Command{}, nil, "prod")
	if cliErr != nil || name != "prod" {
		t.Fatalf("flag-only: name=%q err=%v", name, cliErr)
	}
}

func TestCreateNameBothEqual(t *testing.T) {
	name, cliErr := createName(&cobra.Command{}, []string{"prod"}, "prod")
	if cliErr != nil || name != "prod" {
		t.Fatalf("both-equal: name=%q err=%v", name, cliErr)
	}
}

func TestCreateNameBothDifferentErrors(t *testing.T) {
	_, cliErr := createName(&cobra.Command{}, []string{"a"}, "b")
	if cliErr == nil {
		t.Fatalf("both-different must error")
	}
	if cliErr.Code != output.ErrUsage {
		t.Fatalf("code = %q, want %q", cliErr.Code, output.ErrUsage)
	}
	if !strings.Contains(cliErr.Message, "a") || !strings.Contains(cliErr.Message, "b") {
		t.Fatalf("message must name both %q and %q, got %q", "a", "b", cliErr.Message)
	}
	if cliErr.Hint == "" {
		t.Fatalf("conflict error must carry a hint")
	}
}

func TestCreateNameNeitherReturnsEmpty(t *testing.T) {
	name, cliErr := createName(&cobra.Command{}, nil, "")
	if cliErr != nil || name != "" {
		t.Fatalf("neither: name=%q err=%v", name, cliErr)
	}
}

func TestCredKindPaths(t *testing.T) {
	if got := storageKind.assignPath("org_1", "proj_1"); got != "/v2/organisation/org_1/projects/proj_1/storage-credential" {
		t.Fatalf("assignPath: %s", got)
	}
	if got := proxyKind.orgListPath("org_1"); got != "/v2/organisation/org_1/proxies" {
		t.Fatalf("orgListPath: %s", got)
	}
	if got := llmKind.resourcePath("org_1", "llm_9"); got != "/v2/organisation/org_1/llm-credentials/llm_9" {
		t.Fatalf("resourcePath: %s", got)
	}
}

func TestResolveCredArgMatchesBucketField(t *testing.T) {
	items := []map[string]any{
		{"id": "store_1", "name": "prod", "bucket": "prod-bucket"},
		{"id": "store_2", "name": "staging", "bucket": "stg-bucket"},
	}
	got, cliErr := resolveCredArg(items, "prod-bucket", storageKind)
	if cliErr != nil || got.ID != "store_1" {
		t.Fatalf("got %+v err %v", got, cliErr)
	}
}

func TestResolveCredArgAmbiguousListsIDs(t *testing.T) {
	items := []map[string]any{
		{"id": "pool_1", "name": "eu"},
		{"id": "pool_2", "name": "EU"},
	}
	_, cliErr := resolveCredArg(items, "eu", proxyKind)
	if cliErr == nil || !strings.Contains(cliErr.Hint, "pool_1") || !strings.Contains(cliErr.Hint, "pool_2") {
		t.Fatalf("want ambiguity error listing ids, got %v", cliErr)
	}
}

func TestResolveCredArgUnknownPrefixedIDPassesThrough(t *testing.T) {
	got, cliErr := resolveCredArg(nil, "llm_unknown", llmKind)
	if cliErr != nil || got.ID != "llm_unknown" {
		t.Fatalf("got %+v err %v", got, cliErr)
	}
}

func TestMaybeAssignAfterCreateAssignTo(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/projects": `{"projects":[{"id":"proj_1","name":"Site"}]}`,
	}}
	outcome := maybeAssignAfterCreate(context.Background(), f, "org_1", storageKind, "store_9", "Site", false)
	if !outcome.Attempted || outcome.Err != nil || outcome.Project.ID != "proj_1" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if len(f.puts) != 1 || f.puts[0].Path != "/v2/organisation/org_1/projects/proj_1/storage-credential" {
		t.Fatalf("put not made to assign path: %+v", f.puts)
	}
	body, _ := f.puts[0].Body.(map[string]string)
	if body["storageCredentialId"] != "store_9" {
		t.Fatalf("put body = %+v", f.puts[0].Body)
	}
}

func TestMaybeAssignAfterCreateNonInteractiveSkips(t *testing.T) {
	f := &fakeSession{gets: map[string]string{}}
	outcome := maybeAssignAfterCreate(context.Background(), f, "org_1", proxyKind, "pool_9", "", false)
	if outcome.Attempted || len(f.puts) != 0 {
		t.Fatalf("non-interactive with no --assign-to must not attempt: %+v puts=%+v", outcome, f.puts)
	}
}

func TestMaybeAssignAfterCreateInteractiveNoProjectsSkips(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/projects": `{"projects":[]}`,
	}}
	outcome := maybeAssignAfterCreate(context.Background(), f, "org_1", llmKind, "llm_9", "", true)
	if outcome.Attempted || len(f.puts) != 0 {
		t.Fatalf("interactive with zero projects must skip: %+v puts=%+v", outcome, f.puts)
	}
}

func TestResolveCredArgNotFoundHint(t *testing.T) {
	_, cliErr := resolveCredArg(nil, "nope", storageKind)
	if cliErr == nil || !strings.Contains(cliErr.Hint, "urlbox storage list") {
		t.Fatalf("want not-found hint naming the list command, got %v", cliErr)
	}
	_, cliErr = resolveCredArg(nil, "nope", proxyKind)
	if cliErr == nil || !strings.Contains(cliErr.Hint, "urlbox proxies list") {
		t.Fatalf("want proxies list hint, got %v", cliErr)
	}
	_, cliErr = resolveCredArg(nil, "nope", llmKind)
	if cliErr == nil || !strings.Contains(cliErr.Hint, "urlbox llm list") {
		t.Fatalf("want llm list hint, got %v", cliErr)
	}
}
