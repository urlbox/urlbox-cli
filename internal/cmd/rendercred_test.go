package cmd

import (
	"context"
	"testing"
)

func TestPickAPICredentialSkipsRevoked(t *testing.T) {
	creds := []map[string]any{
		{"apiKey": "pk_revoked", "apiSecret": "sk_revoked", "revoked": true},
		{"apiKey": "pk_live", "apiSecret": "sk_live", "revoked": false},
	}
	got := pickAPICredential(creds)
	if got.key != "pk_live" || got.secret != "sk_live" {
		t.Fatalf("got %+v", got)
	}
	empty := pickAPICredential([]map[string]any{{"revoked": true, "apiKey": "pk", "apiSecret": "sk"}})
	if empty.key != "" || empty.secret != "" {
		t.Fatalf("all-revoked must be empty, got %+v", empty)
	}
}

func TestEnsureRenderCredentialReturnsExisting(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[{"apiKey":"pk_have","apiSecret":"sk_have","revoked":false}]}`,
	}}
	cred, issued, err := ensureRenderCredential(context.Background(), f, "org_1", "proj_1", false, neverPick)
	if err != nil || issued || cred.key != "pk_have" || cred.secret != "sk_have" {
		t.Fatalf("got %+v issued=%v err=%v", cred, issued, err)
	}
	if len(f.posts) != 0 {
		t.Fatalf("must not issue when a credential exists")
	}
}

func TestEnsureRenderCredentialAutoIssuesNonInteractive(t *testing.T) {
	f := &fakeSession{
		gets: map[string]string{
			"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[]}`,
		},
		postResponses: map[string]string{
			"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiKey":"pk_new","apiSecret":"sk_new"}`,
		},
	}
	cred, issued, err := ensureRenderCredential(context.Background(), f, "org_1", "proj_1", false, neverPick)
	if err != nil || !issued || cred.key != "pk_new" || cred.secret != "sk_new" {
		t.Fatalf("got %+v issued=%v err=%v", cred, issued, err)
	}
}

func TestEnsureRenderCredentialInteractiveSkip(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[]}`,
	}}
	skip := func(_ string, options []string, _ int) (int, error) { return 1, nil }
	cred, issued, err := ensureRenderCredential(context.Background(), f, "org_1", "proj_1", true, skip)
	if err != nil || issued || cred.key != "" || cred.secret != "" {
		t.Fatalf("skip must return empty: %+v %v %v", cred, issued, err)
	}
	if len(f.posts) != 0 {
		t.Fatal("skip must not issue")
	}
}
