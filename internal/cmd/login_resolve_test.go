package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

type fakeSession struct {
	gets  map[string]string
	posts []struct {
		Path string
		Body any
	}
	puts []struct {
		Path string
		Body any
	}
	postResponses map[string]string
}

func (f *fakeSession) GetJSON(_ context.Context, path string, out any) error {
	body, ok := f.gets[path]
	if !ok {
		return output.NewCLIError(output.ErrNotFound, "no fake for "+path, "")
	}
	return json.Unmarshal([]byte(body), out)
}

func (f *fakeSession) PostJSON(_ context.Context, path string, body, out any) error {
	f.posts = append(f.posts, struct {
		Path string
		Body any
	}{path, body})
	if resp, ok := f.postResponses[path]; ok && out != nil {
		return json.Unmarshal([]byte(resp), out)
	}
	return nil
}

func (f *fakeSession) PatchJSON(_ context.Context, path string, body, out any) error { return nil }

func (f *fakeSession) PutJSON(_ context.Context, path string, body, out any) error {
	f.puts = append(f.puts, struct {
		Path string
		Body any
	}{path, body})
	return nil
}

func (f *fakeSession) DeleteJSON(_ context.Context, path string, out any) error { return nil }

func neverPick(string, []string, int) (int, error) {
	panic("picker must not be called")
}

func notInteractive(string, []string, int) (int, error) {
	return -1, errNotInteractivePick
}

func sessionJSON(email, activeID, publicID string) string {
	return `{"user":{"email":"` + email + `"},"session":{"activeOrganizationId":"` + activeID + `","activeOrganizationPublicId":"` + publicID + `"}}`
}

func TestResolveActiveOrgSingleOrgSilently(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v1/auth/organization/list": `[{"id":"7","name":"Acme","publicId":"org_acme"}]`,
		"/v1/auth/get-session":       sessionJSON("a@urlbox.com", "7", "org_acme"),
	}}
	got, cli := resolveActiveOrg(context.Background(), f, "", neverPick)
	if cli != nil {
		t.Fatalf("unexpected: %v", cli)
	}
	if got.publicID != "org_acme" || got.name != "Acme" || got.email != "a@urlbox.com" {
		t.Fatalf("got %+v", got)
	}
	if len(f.posts) != 1 || f.posts[0].Path != "/v1/auth/organization/set-active" {
		t.Fatalf("set-active not called: %+v", f.posts)
	}
	b, _ := json.Marshal(f.posts[0].Body)
	if !strings.Contains(string(b), `"organizationId":"7"`) {
		t.Fatalf("set-active must send the numeric id, sent %s", b)
	}
}

func TestResolveActiveOrgFlagMatchesPublicIDNumericIDAndName(t *testing.T) {
	list := `[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`
	for _, flag := range []string{"org_two", "2", "tWo"} {
		f := &fakeSession{gets: map[string]string{
			"/v1/auth/organization/list": list,
			"/v1/auth/get-session":       sessionJSON("a@urlbox.com", "2", "org_two"),
		}}
		got, cli := resolveActiveOrg(context.Background(), f, flag, neverPick)
		if cli != nil || got.publicID != "org_two" {
			t.Fatalf("flag %q: got %+v %v", flag, got, cli)
		}
	}
}

func TestResolveActiveOrgUnknownFlagErrors(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v1/auth/organization/list": `[{"id":"1","name":"One","publicId":"org_one"}]`,
	}}
	_, cli := resolveActiveOrg(context.Background(), f, "nope", neverPick)
	if cli == nil || cli.Code != output.ErrNotFound {
		t.Fatalf("want not_found, got %v", cli)
	}
}

func TestResolveActiveOrgMultiplePicks(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v1/auth/organization/list": `[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`,
		"/v1/auth/get-session":       sessionJSON("a@urlbox.com", "2", "org_two"),
	}}
	pick := func(_ string, options []string, _ int) (int, error) {
		if len(options) != 2 {
			t.Fatalf("options = %v", options)
		}
		return 1, nil
	}
	got, cli := resolveActiveOrg(context.Background(), f, "", pick)
	if cli != nil || got.name != "Two" {
		t.Fatalf("got %+v %v", got, cli)
	}
}

func TestResolveActiveOrgNonInteractiveNamesFlag(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v1/auth/organization/list": `[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`,
	}}
	_, cli := resolveActiveOrg(context.Background(), f, "", notInteractive)
	if cli == nil || cli.Code != output.ErrUsage || !strings.Contains(cli.Hint, "--org") {
		t.Fatalf("want usage error naming --org, got %v", cli)
	}
}

func TestResolveActiveOrgZeroOrgs(t *testing.T) {
	f := &fakeSession{gets: map[string]string{"/v1/auth/organization/list": `[]`}}
	_, cli := resolveActiveOrg(context.Background(), f, "", neverPick)
	if cli == nil || cli.Code != output.ErrNotFound {
		t.Fatalf("want not_found, got %v", cli)
	}
}

func TestResolveActiveProjectMatrix(t *testing.T) {
	zero := &fakeSession{gets: map[string]string{"/v2/projects": `{"projects":[]}`}}
	got, count, cli := resolveActiveProject(context.Background(), zero, "", neverPick)
	if cli != nil || got.ID != "" || count != 0 {
		t.Fatalf("zero projects: got %+v count=%d %v", got, count, cli)
	}

	one := &fakeSession{gets: map[string]string{"/v2/projects": `{"projects":[{"id":"proj_1","name":"Only"}]}`}}
	got, count, cli = resolveActiveProject(context.Background(), one, "", neverPick)
	if cli != nil || got.ID != "proj_1" || count != 1 {
		t.Fatalf("one project: got %+v count=%d %v", got, count, cli)
	}

	many := &fakeSession{gets: map[string]string{"/v2/projects": `{"projects":[{"id":"proj_1","name":"A"},{"id":"proj_2","name":"B"}]}`}}
	got, count, cli = resolveActiveProject(context.Background(), many, "", func(_ string, _ []string, _ int) (int, error) { return 1, nil })
	if cli != nil || got.ID != "proj_2" || count != 2 {
		t.Fatalf("picker path: got %+v count=%d %v", got, count, cli)
	}

	got, count, cli = resolveActiveProject(context.Background(), many, "b", neverPick)
	if cli != nil || got.ID != "proj_2" || count != 2 {
		t.Fatalf("flag path: got %+v count=%d %v", got, count, cli)
	}

	// The count travels with the error too — `orgs select` reports "N projects".
	_, count, cli = resolveActiveProject(context.Background(), many, "", notInteractive)
	if cli == nil || cli.Code != output.ErrUsage || !strings.Contains(cli.Hint, "--project") {
		t.Fatalf("want usage error naming --project, got %v", cli)
	}
	if count != 2 {
		t.Fatalf("ambiguous path must still report the count, got %d", count)
	}
}

func TestActiveOrgNameFallbacks(t *testing.T) {
	named := &fakeSession{gets: map[string]string{
		"/v1/auth/get-session":       sessionJSON("a@urlbox.com", "7", "org_x"),
		"/v1/auth/organization/list": `[{"id":"7","name":"Acme","publicId":"org_x"}]`,
	}}
	if got := activeOrgName(context.Background(), named); got != "Acme" {
		t.Fatalf("got %q", got)
	}
	none := &fakeSession{gets: map[string]string{
		"/v1/auth/get-session": sessionJSON("a@urlbox.com", "", ""),
	}}
	if got := activeOrgName(context.Background(), none); got != "(none)" {
		t.Fatalf("got %q", got)
	}
}
