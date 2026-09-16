# Account Management Port — Plan 2: Credential Resources + Auth Sweep Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land spec slices 4–5 in this repo: the org-owned credential-resource groups (`storage`, `proxies`, `llm`), the `projects storage|proxy|llm assign|unassign` surface with the create→assign flow, and the auth removal sweep (delete `urlbox auth`, session-aware `doctor`, reference sweep, unified login hint).

**Architecture:** Behaviour is ported from `/Users/arnoldcubici-jones/Code/work/cli` (read-only; `SRC` below); endpoints and response shapes are pinned from the mono contracts in `/Users/arnoldcubici-jones/Code/work/urlbox-mono/packages/domain/src` (authoritative where source and mono disagree). Every command is written in this repo's landed idioms: cobra + `output.Envelope` with `SetTable`/`SetKV` (view replaces summary on ok), `loadSession`/`requireActiveOrg`, `resolveNameOrID`/`fetchList`, `prompt.SelectOne`/`TypeToConfirm`/`Confirm` via the shared `confirmPrompt` hook, `api.SessionClient`.

**Tech Stack:** Go 1.23, cobra, lipgloss (landed renderers), `internal/api/apitest` fakes.

## Global Constraints

- TDD from the first step of every task: failing test → minimal implementation → green.
- `make ci` (fmt-check, lint, test, build, surface-check) green at the end of every task; `make surface-snapshot` + commit `SURFACE.txt` alongside any surface change. PATH note: gofumpt/golangci-lint/gotestsum live in `$(go env GOPATH)/bin`.
- **Production-shape fixtures only.** Every fixture that represents an API response uses the shape pinned from the mono contract (named per task below). Inventing a fixture shape is a defect: flat-vs-wrapped mismatches shipped three bugs in Plan 1. When a task needs a shape not pinned here, STOP and verify against the mono contract first.
- Visual rules (landed formatter): lists render via `env.SetTable` (bordered table), details via `env.SetKV` (bordered 2-col box, labels uppercased centrally); the view replaces the summary when ok; mutations without a view keep named summaries. Errors keep summaries.
- Confirm gates: retype-the-name (`prompt.TypeToConfirm`) for deletes; y/n (`prompt.Confirm`) for disable-grade actions; `--yes` bypasses every prompt; non-TTY (or json/quiet format) without `--yes` errors `ErrUsage` naming `--yes`, never hangs. Tests inject via the shared `confirmPrompt` hook (`SetConfirmPromptForTest`) and `SetOrgsProjectPickForTest`-style hooks.
- stdout = structured data only; stderr = human messages. Errors only from the closed set in `internal/output/errors.go`; every CLIError hint non-empty (guard test); pretty-printed JSON in assertions (`"key": "value"` with a space); `_, _ =` on bare Fprint/Fprintln.
- ZERO code comments — exception: linter-enforced doc comments on exported symbols (keep new symbols unexported wherever possible); `//nolint` only where genuinely required, no prose beyond the directive.
- One commit per task: explicit-path staging (never `git add -A`), conventional subject under 72 chars, authored `git -c user.name="Arnold Cubici-Jones" -c user.email="108676317+AJCJ1@users.noreply.github.com" commit -m "..."`. Never push. Work continues on `feat/account-management` (draft PR #1).
- Do-not-modify throughout: `dashboard.go`, `skill.go`, `commands.go`, `upgrade.go`, `render*.go` (the `internal/cmd` render family), `screenshot.go`, `pdf.go`, `video.go`, `status.go`, `link.go`. `auth.go`, `auth_preflight.go`, and `doctor.go` are in scope for **Task 7 only** (Task 1 relocates `maskSecret` out of `auth.go` — that single mechanical move is sanctioned in Task 1).
- Profiles stay undocumented: no new help text or docs mention profiles beyond the inherited `--profile` flag.

**Shorthand:** `SRC = /Users/arnoldcubici-jones/Code/work/cli` (read-only reference). `MONO = /Users/arnoldcubici-jones/Code/work/urlbox-mono` (read-only). Target repo root is this repo.

## Pinned endpoint + shape reference (verified against MONO contracts 2026-08-17)

All paths are CLI-side (the oRPC contracts mount under `/v2`). Org-scoped paths use the stored public org id (`sess.Profile.ActiveOrg`).

| Operation | Method + path | Body (request) | Response |
|---|---|---|---|
| storage list | `GET /v2/organisation/{org}/storage-credentials` | — | `{"storageCredentials": [StorageCredentialDto…]}` |
| storage create | `POST` same | flat body: `type` (`s3`\|`azure`), `name`, s3: `provider bucket region endpoint key secret privateBucket objectLock`, azure: `accountName containerName sasToken`, common: `cdnHost` | `StorageCredentialDto` |
| storage get/update/delete | `GET/PATCH/DELETE /v2/organisation/{org}/storage-credentials/{cred}` | PATCH = partial flat body | Dto / Dto / `{"id":"store_…","deleted":true}` |
| proxies list | `GET /v2/organisation/{org}/proxies` | — | `{"proxies": [ProxyDto…]}` |
| proxies create | `POST` same | `{"name": "...", "proxies": [{"url": "..."}, …]}` (`name` optional per entry) | `ProxyDto` |
| proxies get/update/delete | `GET/PATCH/DELETE /v2/organisation/{org}/proxies/{proxy}` | PATCH = **FULL body** (`name` + entire `proxies` list — the server reconciles the pool to exactly what's sent) | Dto / Dto / `{"id":"pool_…","deleted":true}` |
| llm list | `GET /v2/organisation/{org}/llm-credentials` | — | `{"llmCredentials": [LlmCredentialDto…]}` |
| llm create | `POST` same | flat body: `name`, `provider`, optional `model prompt systemPrompt maxTokens temperature output schema baseUrl azure* aws* gcp* apiKey` | `LlmCredentialDto` |
| llm get/update/delete | `GET/PATCH/DELETE /v2/organisation/{org}/llm-credentials/{cred}` | PATCH = partial, **no `provider`** (immutable) | Dto / Dto / `{"id":"llm_…","deleted":true}` |
| llm test | `POST /v2/organisation/{org}/llm-credentials/{cred}/test` | `{}` | `{"ok": true}` or `{"ok": false, "error": "..."}` (may carry `azureApiProtocol`) |
| llm models | `POST /v2/organisation/{org}/llm-credentials/{cred}/models` | `{}` | `{"models": ["id1","id2",…]}` |
| assign storage | `PUT /v2/organisation/{org}/projects/{project}/storage-credential` | `{"storageCredentialId": "store_…"}` | flat `ProjectDto` |
| unassign storage | `DELETE` same | — | flat `ProjectDto` |
| assign llm | `PUT …/projects/{project}/llm-credential` | `{"llmCredentialId": "llm_…"}` | flat `ProjectDto` |
| unassign llm | `DELETE` same | — | flat `ProjectDto` |
| assign proxy | `PUT …/projects/{project}/proxy` | `{"proxyId": "pool_…"}` | flat `ProjectDto` |
| unassign proxy | `DELETE` same | — | flat `ProjectDto` |

DTO field pins (fixtures MUST match): `StorageCredentialDto` = flat `{id store_…, type, name, provider|null, bucket|null, region|null, endpoint|null, privateBucket, objectLock, accountName|null, containerName|null, cdnHost|null, key|null, secret|null, sasToken|null, assignedProjectIds: [], createdAt}` — secrets returned IN FULL (CLI masks on display). `ProxyDto` = `{id pool_…, name, proxies: [{id proxy_…, name, url}], assignedProjectIds, createdAt}` — URLs returned verbatim (CLI masks password portion on display). `LlmCredentialDto` = flat, provider-specific nullable fields, secrets (`apiKey`, `awsAccessKeyId`, `awsSecretAccessKey`, `awsSessionToken`, `gcpServiceAccountJson`) IN FULL. Public-id prefixes: `store_`, `pool_` (pool — NOT `proxy_`, which is the per-entry id), `llm_`, `proj_`.

**Spec supersessions (source implementation wins, matching the standing rule):** the spec's storage list columns ("ID/NAME/BUCKET/VISIBILITY/OBJECT-LOCK/CDN/ASSIGNED") are stale — the source's landed table is `BUCKET / ID / PROVIDER / ENDPOINT / KEY / ASSIGNED` (key masked) and that is what ports. Visibility/object-lock/CDN stay in `show`.

---

### Task 1: Masking foundation — relocate `maskSecret`, transplant `maskProxyURL`

**Files:**
- Create: `internal/cmd/masking.go`
- Create: `internal/cmd/masking_test.go`
- Modify: `internal/cmd/auth.go` (DELETE the `maskSecret` function only — no other change)

**Interfaces:**
- Consumes: nothing new.
- Produces: `maskSecret(s string) string` (moved verbatim, same behaviour: `<first4>…<last2>`, `***` for short values — callers in `config.go`, `projects.go` keep compiling unchanged); `maskProxyURL(raw string, reveal bool) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/masking_test.go`:

```go
package cmd

import "testing"

func TestMaskProxyURLMasksOnlyPassword(t *testing.T) {
	got := maskProxyURL("http://user:hunter2@proxy.example.com:8080", false)
	want := "http://user:****@proxy.example.com:8080"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMaskProxyURLRevealReturnsRaw(t *testing.T) {
	raw := "http://user:hunter2@proxy.example.com:8080"
	if got := maskProxyURL(raw, true); got != raw {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestMaskProxyURLNoPasswordUntouched(t *testing.T) {
	raw := "http://proxy.example.com:8080"
	if got := maskProxyURL(raw, false); got != raw {
		t.Fatalf("got %q want %q", got, raw)
	}
}

func TestMaskProxyURLSchemelessWithAtFullMasks(t *testing.T) {
	got := maskProxyURL("user:hunter2@proxy.example.com:8080", false)
	if got == "user:hunter2@proxy.example.com:8080" {
		t.Fatalf("schemeless credential string must not pass through unmasked")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestMaskProxyURL' -v`
Expected: FAIL — `undefined: maskProxyURL`.

- [ ] **Step 3: Implement**

Create `internal/cmd/masking.go`. Move `maskSecret` VERBATIM from `internal/cmd/auth.go:313` (delete it there), then add the transplant, adapted from `SRC/internal/cmd/proxies.go:70-92` — the only adaptation: the source's `maskSensitive(raw, false)` full-mask fallback becomes this repo's `maskSecret(raw)`:

```go
package cmd

import (
	"net/url"
	"strings"
)

func maskSecret(s string) string {
	// moved verbatim from auth.go — do not edit the body
}

func maskProxyURL(raw string, reveal bool) string {
	if reveal {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return maskSecret(raw)
	}
	if parsed.User == nil {
		if strings.Contains(raw, "@") {
			return maskSecret(raw)
		}
		return raw
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.UserPassword(parsed.User.Username(), "****")
	}
	return strings.Replace(parsed.String(), "%2A%2A%2A%2A", "****", 1)
}
```

Cross-check the transplant against `SRC/internal/cmd/proxies_test.go` — carry over any `maskProxyURL` cases there that Step 1 misses (add them to `masking_test.go` verbatim).

- [ ] **Step 4: Verify green**

Run: `go test ./internal/cmd/ -run 'TestMaskProxyURL' -v` → PASS. Then the full package: `go test ./internal/cmd/` → PASS (proves the `maskSecret` move broke no caller).

- [ ] **Step 5: Gates + commit**

Run: `make ci` → green; `make surface-snapshot` → no SURFACE.txt change.
Commit exactly: `internal/cmd/masking.go internal/cmd/masking_test.go internal/cmd/auth.go` — subject `refactor(cli): move maskSecret out of auth.go, add proxy URL masking`.

---

### Task 2: Plumbing — `PutJSON`, `credKind`, `resolveCredArg`, payload builders, create→assign machinery

**Files:**
- Modify: `internal/api/session_client.go` (add `PutJSON` to `SessionClient` and to the `SessionAPI` interface)
- Modify: `internal/api/session_client_test.go`
- Create: `internal/cmd/credkind.go`, `internal/cmd/credkind_test.go`
- Create: `internal/cmd/credbody.go`, `internal/cmd/credbody_test.go`
- Modify: every test fake implementing `SessionAPI` (`fakeSession` in `login_resolve_test.go` and any sibling) gains a `PutJSON` method.

**Interfaces:**
- Consumes: `api.SessionAPI`, `resolveNameOrID`, `fetchList`, `toNameIDs`, `nameID`, `prompt.SelectOne` via `promptPick`.
- Produces (later tasks rely on these exact names):
  - `PutJSON(ctx context.Context, path string, body, out any) error` on `SessionClient` + `SessionAPI`.
  - `type credKind struct { noun, group, pathPart, bodyKey, prefix, listPath, listKey string; matchFields []string }` with package vars `storageKind`, `proxyKind`, `llmKind`; methods `assignPath(org, project string) string`, `listPath2(org string) string` (name it `orgListPath` — see code), `resourcePath(org, id string) string`.
  - `resolveCredArg(items []map[string]any, arg string, kind credKind) (nameID, *output.CLIError)`.
  - `buildStorageBody(jsonBody string, f storageFlags, requireCreate bool) (map[string]any, *output.CLIError)` and `type storageFlags` (cobra-adapted: plain strings + `cmd.Flags().Changed` tracking — see code), `buildProxyCreateBody`, `mergeProxyUpdate`, `buildLlmBody`, `type llmFlags`.
  - `maybeAssignAfterCreate(ctx, client api.SessionAPI, org string, kind credKind, createdID, assignTo string, interactive bool) assignOutcome` + `type assignOutcome struct { Attempted bool; Project nameID; Err error }`.

Adaptations from source (`SRC/internal/cmd/credentials.go:305-420`, `storage.go:262-400`, `proxies.go:117-160`, `llm.go:179-265`), all mechanical: `map[string]interface{}` → `map[string]any`; the source's `stringValue`-set flag tracking becomes cobra `Flags().Changed("name")` checks; errors become `*output.CLIError` (`ErrUsage` for missing/invalid flags, hints naming the exact flags); `prompt.SelectOne` reached through the injectable `promptPick`/`confirmPrompt`-style hook so tests never draw.

- [ ] **Step 1: Failing tests — `PutJSON`**

Append to `internal/api/session_client_test.go` (mirror the existing `PatchJSON`-style test: apitest server asserting method PUT, path, bearer header, body echo):

```go
func TestSessionClientPutJSONSendsBearerAndBody(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	defer srv.Close()
	c := NewSessionClient(srv.URL(), "sess_tok")
	var out map[string]any
	if err := c.PutJSON(context.Background(), "/v2/organisation/org_1/projects/proj_1/proxy", map[string]string{"proxyId": "pool_1"}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := srv.Requests()[0]
	if req.Method != "PUT" || req.Path != "/v2/organisation/org_1/projects/proj_1/proxy" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}
	if !strings.Contains(req.Body, `"proxyId":"pool_1"`) {
		t.Fatalf("body missing proxyId: %s", req.Body)
	}
}
```

(Adapt the request-inspection calls to the exact `apitest` helper names used by the sibling tests in that file — read them first; they are the authority on the harness API.)

- [ ] **Step 2: Verify RED** — `go test ./internal/api/ -run TestSessionClientPutJSON -v` → FAIL `undefined` / missing method.

- [ ] **Step 3: Implement `PutJSON`** in `internal/api/session_client.go`: one method on `SessionClient` calling the existing `doJSON` with `http.MethodPut` (exactly parallel to `PatchJSON`), and add the signature to `SessionAPI`. Update every fake in `internal/cmd` implementing `SessionAPI` with a recording `PutJSON` (parallel to their `PatchJSON`). Verify GREEN, then run `go build ./...` to catch any fake missing the method.

- [ ] **Step 4: Failing tests — credkind + resolveCredArg + builders**

Create `internal/cmd/credkind_test.go`:

```go
package cmd

import (
	"strings"
	"testing"
)

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
```

Create `internal/cmd/credbody_test.go` (transplant coverage of `SRC` builder/validator semantics — production body shapes from the pinned table):

```go
package cmd

import (
	"strings"
	"testing"
)

func TestBuildStorageBodyProviderDrivesType(t *testing.T) {
	f := storageFlags{name: "s3 cred", provider: "cloudflare_r2", bucket: "b", region: "auto", key: "k", secret: "s", endpoint: "https://x.r2.cloudflarestorage.com", cdnHost: "cdn.example.com",
		set: map[string]bool{"name": true, "provider": true, "bucket": true, "region": true, "key": true, "secret": true, "endpoint": true, "cdn-host": true}}
	body, cliErr := buildStorageBody("", f, true)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if body["type"] != "s3" || body["provider"] != "cloudflare_r2" {
		t.Fatalf("type/provider: %v %v", body["type"], body["provider"])
	}
}

func TestBuildStorageBodyAzureDropsProvider(t *testing.T) {
	f := storageFlags{name: "az", provider: "azure", accountName: "acct", containerName: "cont", sasToken: "sv=…",
		set: map[string]bool{"name": true, "provider": true, "account-name": true, "container-name": true, "sas-token": true}}
	body, cliErr := buildStorageBody("", f, true)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if body["type"] != "azure" {
		t.Fatalf("type: %v", body["type"])
	}
	if _, ok := body["provider"]; ok {
		t.Fatalf("azure body must not carry provider")
	}
}

func TestBuildStorageBodyS3MissingFieldsErrors(t *testing.T) {
	f := storageFlags{name: "x", provider: "aws_s3", set: map[string]bool{"name": true, "provider": true}}
	_, cliErr := buildStorageBody("", f, true)
	if cliErr == nil || !strings.Contains(cliErr.Message, "--bucket") {
		t.Fatalf("want required-fields error, got %v", cliErr)
	}
}

func TestBuildStorageBodyUpdateSendsOnlyChanged(t *testing.T) {
	f := storageFlags{region: "eu-west-1", set: map[string]bool{"region": true}}
	body, cliErr := buildStorageBody("", f, false)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if len(body) != 1 || body["region"] != "eu-west-1" {
		t.Fatalf("partial body wrong: %v", body)
	}
}

func TestMergeProxyUpdateCarriesExistingForward(t *testing.T) {
	existing := map[string]any{"name": "eu pool", "proxies": []any{
		map[string]any{"id": "proxy_1", "name": "one", "url": "http://u:p@a:1"},
	}}
	body := mergeProxyUpdate(existing, "", nil, map[string]bool{})
	if body["name"] != "eu pool" {
		t.Fatalf("name not carried: %v", body["name"])
	}
	proxies := body["proxies"].([]map[string]any)
	if len(proxies) != 1 || proxies[0]["url"] != "http://u:p@a:1" || proxies[0]["name"] != "one" {
		t.Fatalf("existing entries not carried: %v", proxies)
	}
}

func TestMergeProxyUpdateReplacesWholeListWhenURLsGiven(t *testing.T) {
	existing := map[string]any{"name": "eu pool", "proxies": []any{map[string]any{"url": "http://old:1"}}}
	body := mergeProxyUpdate(existing, "", []string{"http://new:1", "http://new:2"}, map[string]bool{})
	proxies := body["proxies"].([]map[string]any)
	if len(proxies) != 2 || proxies[0]["url"] != "http://new:1" {
		t.Fatalf("list not replaced: %v", proxies)
	}
}

func TestBuildLlmBodyUpdateRejectsProvider(t *testing.T) {
	f := llmFlags{provider: "openai", set: map[string]bool{"provider": true}}
	_, cliErr := buildLlmBody("", f, false)
	if cliErr == nil || !strings.Contains(cliErr.Message, "immutable") {
		t.Fatalf("want provider-immutable error, got %v", cliErr)
	}
}

func TestBuildLlmBodyBedrockRequiresAwsFields(t *testing.T) {
	f := llmFlags{name: "b", provider: "amazon-bedrock", set: map[string]bool{"name": true, "provider": true}}
	_, cliErr := buildLlmBody("", f, true)
	if cliErr == nil || !strings.Contains(cliErr.Message, "awsRegion") {
		t.Fatalf("want bedrock required-fields error, got %v", cliErr)
	}
}
```

- [ ] **Step 5: Verify RED** — both new test files fail to compile (`undefined` symbols). Run: `go test ./internal/cmd/ -run 'TestCredKind|TestResolveCredArg|TestBuild|TestMergeProxy' -v`.

- [ ] **Step 6: Implement `internal/cmd/credkind.go`**

```go
package cmd

import (
	"context"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type credKind struct {
	noun        string
	group       string
	pathPart    string
	bodyKey     string
	prefix      string
	listPath    string
	listKey     string
	matchFields []string
}

var (
	storageKind = credKind{noun: "storage credential", group: "storage", pathPart: "storage-credential", bodyKey: "storageCredentialId", prefix: "store_", listPath: "storage-credentials", listKey: "storageCredentials", matchFields: []string{"bucket", "containerName"}}
	proxyKind   = credKind{noun: "proxy pool", group: "proxy", pathPart: "proxy", bodyKey: "proxyId", prefix: "pool_", listPath: "proxies", listKey: "proxies"}
	llmKind     = credKind{noun: "LLM credential", group: "llm", pathPart: "llm-credential", bodyKey: "llmCredentialId", prefix: "llm_", listPath: "llm-credentials", listKey: "llmCredentials"}
)

func (k credKind) orgListPath(org string) string {
	return "/v2/organisation/" + org + "/" + k.listPath
}

func (k credKind) resourcePath(org, id string) string {
	return k.orgListPath(org) + "/" + id
}

func (k credKind) assignPath(org, project string) string {
	return "/v2/organisation/" + org + "/projects/" + project + "/" + k.pathPart
}

func resolveCredArg(items []map[string]any, arg string, kind credKind) (nameID, *output.CLIError) {
	if strings.HasPrefix(arg, kind.prefix) {
		for _, m := range items {
			if valueOrEmpty(m["id"]) == arg {
				return nameID{ID: arg, Name: valueOrEmpty(m["name"])}, nil
			}
		}
		return nameID{ID: arg}, nil
	}
	fields := append([]string{"name"}, kind.matchFields...)
	var matches []nameID
	for _, m := range items {
		for _, f := range fields {
			v := valueOrEmpty(m[f])
			if v != "" && strings.EqualFold(v, arg) {
				matches = append(matches, nameID{ID: valueOrEmpty(m["id"]), Name: valueOrEmpty(m["name"])})
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nameID{}, output.NewCLIError(output.ErrNotFound, "no "+kind.noun+" matching \""+arg+"\"",
			"List them with `urlbox "+kind.listPath+" list`… ") // match nameid.go's exact hint pattern: reuse its wording with the kind's group command
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nameID{}, output.NewCLIError(output.ErrValidation, "\""+arg+"\" matches multiple "+kind.noun+"s",
			"Use one of the ids instead: "+strings.Join(ids, ", "))
	}
}

type assignOutcome struct {
	Attempted bool
	Project   nameID
	Err       error
}

func maybeAssignAfterCreate(ctx context.Context, client api.SessionAPI, org string, kind credKind, createdID, assignTo string, interactive bool) assignOutcome {
	assign := func(project nameID) assignOutcome {
		if err := client.PutJSON(ctx, kind.assignPath(org, project.ID), map[string]string{kind.bodyKey: createdID}, nil); err != nil {
			return assignOutcome{Attempted: true, Project: project, Err: err}
		}
		return assignOutcome{Attempted: true, Project: project}
	}
	if assignTo != "" {
		projects, err := fetchList(ctx, client, "/v2/projects", "projects")
		if err != nil {
			return assignOutcome{Attempted: true, Err: err}
		}
		project, cliErr := resolveNameOrID(assignTo, "proj_", toNameIDs(projects), "project")
		if cliErr != nil {
			return assignOutcome{Attempted: true, Err: cliErr}
		}
		return assign(project)
	}
	if !interactive {
		return assignOutcome{}
	}
	projects, err := fetchList(ctx, client, "/v2/projects", "projects")
	if err != nil || len(projects) == 0 {
		return assignOutcome{}
	}
	rows := toNameIDs(projects)
	options := make([]string, 0, len(rows)+1)
	options = append(options, "Don't assign")
	for _, p := range rows {
		options = append(options, p.Name)
	}
	idx, err := promptPick("Assign to a project?", options, 0)
	if err != nil || idx == 0 {
		return assignOutcome{}
	}
	return assign(rows[idx-1])
}
```

Fix the `ErrNotFound` hint to reuse the exact established wording pattern from `internal/cmd/nameid.go` with the kind's group (`urlbox storage list` / `urlbox proxies list` / `urlbox llm list`): `"List them with `urlbox <group or listPath> list`, then pass a name or id."` — final string per kind: storage → `urlbox storage list`, proxy → `urlbox proxies list`, llm → `urlbox llm list`. Add a `listCmd string` field to `credKind` if that is cleaner than deriving; tests in Task 3–5 assert the hint text.

- [ ] **Step 7: Implement `internal/cmd/credbody.go`** — cobra-adapted ports of `SRC/internal/cmd/storage.go:262-368` (`s3Providers`, `s3ProvidersNeedingEndpoint`, `buildStorageBody`, `validateStorageCreate`), `SRC/internal/cmd/proxies.go:117-160` (`buildProxyCreateBody`, `mergeProxyUpdate`), `SRC/internal/cmd/llm.go:179-251` (`buildLlmBody`, `validateLlmCreate`). Flag structs carry plain string/bool fields plus a `set map[string]bool` populated by the command layer from `cmd.Flags().Changed`:

```go
type storageFlags struct {
	name, provider, bucket, region, endpoint, key, secret, cdnHost string
	accountName, containerName, sasToken                           string
	privateBucket, objectLock                                      bool
	set                                                            map[string]bool
}

type llmFlags struct {
	name, provider, apiKey, model, baseURL string
	set                                    map[string]bool
}
```

Port the bodies preserving EXACT semantics: `--json` parsed first (reuse the landed JSON-object parsing used by `projects defaults set` — same error wording), changed flags merged on top using the wire field names from the pinned table (`cdnHost`, `accountName`, `privateBucket`…); the provider→type derivation and azure `delete(body, "provider")`; validation messages verbatim from source (they name the flags); update path = only-changed keys; `mergeProxyUpdate(existing map[string]any, name string, urls []string, set map[string]bool)` carrying name/entries forward exactly as `SRC` does (entry `name` preserved). All errors are `*output.CLIError` — validation failures `ErrUsage` with the flag-naming text as the MESSAGE and a usage hint (non-empty).

- [ ] **Step 8: Verify GREEN** — the Step-4/5 tests pass; full package passes; `make ci` green; `make surface-snapshot` no change.

- [ ] **Step 9: Commit** — paths: `internal/api/session_client.go internal/api/session_client_test.go internal/cmd/credkind.go internal/cmd/credkind_test.go internal/cmd/credbody.go internal/cmd/credbody_test.go` plus any fake-updated test files. Subject: `feat(cli): credential-resource plumbing (PutJSON, kinds, bodies)`.

---

### Task 3: `storage` group

**Files:**
- Create: `internal/cmd/storage.go`, `internal/cmd/storage_test.go`
- Modify: `internal/cmd/root.go` (register `newStorageCmd()` — alphabetical between `status`/`upgrade` per the existing ordering convention — verify against the file)
- Modify: `SURFACE.txt` (via snapshot)

**Interfaces:**
- Consumes: Task 1–2 symbols, `loadSession`, `requireActiveOrg`, `writeEnvelope`/`writeEnvelopeWithQuietData`, `env.SetTable`/`SetKV`, `prompt.TypeToConfirm` via the shared confirm hook, `confirmPrompt` gating pattern from `projects.go` delete.
- Produces: `newStorageCmd() *cobra.Command`; unexported `storageListRows`, `storageDetailPairs`, `storageProviderLabel`, `storageEndpointCell`, `assignedCount` (shared — put `assignedCount` in `credkind.go` if not already there from Task 2).

Command surface (mirror `projects.go`'s group construction exactly — group + subcommands, `RunE` returning `*output.CLIError`, no-args group prints help):

```
urlbox storage list
urlbox storage show <name-or-id> [--reveal]
urlbox storage create --name … --provider … [typed flags] [--json …] [--assign-to <project>]
urlbox storage update <name-or-id> [typed flags] [--json …]
urlbox storage delete <name-or-id> [--yes]
```

- [ ] **Step 1: Failing tests** — `internal/cmd/storage_test.go`. Fixtures use the PINNED `StorageCredentialDto` shape (flat, `{"storageCredentials": […]}` list wrapper, secrets in full). Required tests (follow the request-recording fake/apitest pattern of `projects_crud_test.go` — read it first, reuse its helpers where exported within the package):

```go
package cmd

import (
	"strings"
	"testing"
)

const storageListJSON = `{"storageCredentials":[
 {"id":"store_1","type":"s3","name":"prod","provider":"aws_s3","bucket":"prod-bucket","region":"us-east-1","endpoint":null,"privateBucket":false,"objectLock":false,"accountName":null,"containerName":null,"cdnHost":null,"key":"AKIAFAKEFAKEFAKE","secret":"sk_fake_secret_value","sasToken":null,"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"},
 {"id":"store_2","type":"azure","name":"az","provider":null,"bucket":null,"region":null,"endpoint":null,"privateBucket":true,"objectLock":false,"accountName":"acct","containerName":"cont","cdnHost":null,"key":null,"secret":null,"sasToken":"sv=fake","assignedProjectIds":[],"createdAt":"2026-08-02T00:00:00.000Z"}]}`

func TestStorageListRendersTableMaskedKey(t *testing.T) {
	// scripted GET /v2/organisation/org_compat/storage-credentials → storageListJSON
	// text mode: assert table contains "prod-bucket", "store_1", "AWS S3",
	// "AWS S3 (default)" endpoint cell, the MASKED key (contains "…" or stars, NOT "AKIAFAKEFAKEFAKE"),
	// azure row shows "cont" in BUCKET column and "Azure" provider label, assigned counts "1" and "0".
}

func TestStorageShowMasksSecretsRevealUnhides(t *testing.T) {
	// show store_1 text mode: KV contains NAME/ID/PROVIDER/BUCKET/VISIBILITY public;
	// key+secret masked; with --reveal full values present.
}

func TestStorageCreateSendsProviderDrivenBody(t *testing.T) {
	// create --name x --provider cloudflare_r2 --bucket b --region auto --key k --secret s --endpoint e --cdn-host c
	// asserts POST body has "type":"s3","provider":"cloudflare_r2" and all fields; response fixture = single Dto (flat).
}

func TestStorageUpdatePartialPatch(t *testing.T) {
	// update store_1 --region eu-west-1 → PATCH body exactly {"region":"eu-west-1"}.
	// (resolution by prefixed id makes no list call — assert single PATCH request)
}

func TestStorageDeleteRequiresYesOffTTY(t *testing.T) {
	// non-TTY, no --yes → ErrUsage naming --yes, no DELETE issued.
}

func TestStorageDeleteWithYes(t *testing.T) {
	// --yes → DELETE /v2/organisation/org_compat/storage-credentials/store_1; response {"id":"store_1","deleted":true}; summary names the credential.
}

func TestStorageCreateAssignTo(t *testing.T) {
	// create … --assign-to proj_1 → after POST, a PUT /projects/proj_1/storage-credential with {"storageCredentialId":"store_<created>"}.
	// envelope data carries assigned project (nil when no assign).
}

func TestStorageNotFoundNameHint(t *testing.T) {
	// show "nope" with fixture list → ErrNotFound, hint contains "urlbox storage list".
}
```

Write these as REAL tests (the comment bodies above specify the exact assertions — expand them following the concrete style of `projects_crud_test.go`; every assertion listed is required).

- [ ] **Step 2: Verify RED** — `go test ./internal/cmd/ -run TestStorage -v` → compile failures/undefined.

- [ ] **Step 3: Implement `internal/cmd/storage.go`**

Structure (complete skeleton — bodies follow the named sources):

```go
package cmd

func newStorageCmd() *cobra.Command // group; Short "Manage org storage credentials"; help Long in house style with Examples

func runStorageList(cmd *cobra.Command, args []string) error
// loadSession → requireActiveOrg → fetchList(ctx, client, storageKind.orgListPath(org), storageKind.listKey)
// env := NewEnvelope("storage list", map[string]any{"storageCredentials": items}, fmt.Sprintf("%d storage credentials", len(items)), nil)
// env.SetTable([]string{"BUCKET", "ID", "PROVIDER", "ENDPOINT", "KEY", "ASSIGNED"}, storageListRows(items), -1)
// writeEnvelopeWithQuietData(cmd, env, strconv.Itoa(len(items)))

func runStorageShow(cmd *cobra.Command, args []string) error
// list → resolveCredArg → GET resourcePath when arg was a bare unknown id OR reuse the matched list item (source fetches the item from the LIST it already has: findByID; if prefixed-id not in list, GET the resource directly and 404 maps naturally)
// env.SetKV(storageDetailPairs(item, reveal)); summary "Storage credential <name>"; quiet scalar = id

func runStorageCreate(cmd *cobra.Command, args []string) error
// buildStorageBody(json, flags, true) → PostJSON(orgListPath) → created Dto (flat)
// outcome := maybeAssignAfterCreate(ctx, client, org, storageKind, createdID, assignTo, interactiveText(cmd))
// envelope data {"credential": created, "assigned": …}; summary "Created storage credential <name>" (+ " (assigned to <project>)" when assigned); assign errors → ErrServer-mapped CLIError with the source's "created … but could not assign it" message

func runStorageUpdate(cmd *cobra.Command, args []string) error   // PATCH partial
func runStorageDelete(cmd *cobra.Command, args []string) error   // TypeToConfirm gate exactly like projects delete (same hook, same non-TTY ErrUsage naming --yes), DELETE, summary "Deleted storage credential <name>"
```

Port `storageListRows`/`storageProviderLabel`/`storageEndpointCell`/`storageDetailPairs`/`assignedCount` from `SRC/internal/cmd/storage.go:70-212` (mechanical `any` + `maskSecret` adaptations; detail pairs keep the source's field order and conditional omissions; masked fields use `maskSecret` unless `--reveal`). `interactiveText(cmd)` = the exact gating used by `projects create`'s switch prompt (resolved format is text AND the confirm/pick path is live) — reuse that helper, do not duplicate the logic (extract it in this task if it is currently inline; name it `interactiveText`).

- [ ] **Step 4: Verify GREEN**, `make ci`, `make surface-snapshot` (SURFACE.txt gains the full `urlbox storage …` tree — verify every subcommand + flag line appears; `--reveal`, `--assign-to`, `--yes`, typed create flags).

- [ ] **Step 5: Commit** — `internal/cmd/storage.go internal/cmd/storage_test.go internal/cmd/root.go SURFACE.txt` (+ any helper extraction file). Subject: `feat(cli): storage credential commands`.

---

### Task 4: `proxies` group

**Files:**
- Create: `internal/cmd/proxies.go`, `internal/cmd/proxies_test.go`
- Modify: `internal/cmd/root.go` (register; group aliases `proxies`/`proxy` — cobra `Aliases: []string{"proxy"}`), `SURFACE.txt`

**Interfaces:** consumes Tasks 1–2 (`proxyKind`, `maskProxyURL`, `buildProxyCreateBody`, `mergeProxyUpdate`); produces `newProxiesCmd()`.

Surface:

```
urlbox proxies list
urlbox proxies show <name-or-id> [--reveal]
urlbox proxies create --name … --url … [--url …] [--assign-to <project>]
urlbox proxies update <name-or-id> [--name …] [--url … --url …]
urlbox proxies delete <name-or-id> [--yes]
```

- [ ] **Step 1: Failing tests** — fixtures per pinned `ProxyDto`:

```go
const proxyListJSON = `{"proxies":[
 {"id":"pool_1","name":"eu","proxies":[{"id":"proxy_1","name":"one","url":"http://user:hunter2@p1.example.com:8080"},{"id":"proxy_2","name":"","url":"http://p2.example.com:8080"}],"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"}]}`
```

Required tests (expand fully, `projects_crud_test.go` style):
- `TestProxiesListShowsCountNeverURLs` — table row `pool_1 / eu / 2 / 1`; output does NOT contain `hunter2` or `p1.example.com`.
- `TestProxiesShowMasksPasswordOnly` — KV contains `http://user:****@p1.example.com:8080` and the passwordless URL untouched; `--reveal` shows `hunter2`.
- `TestProxiesCreateBody` — `--name eu --url http://a:1 --url http://b:2` → POST `{"name":"eu","proxies":[{"url":"http://a:1"},{"url":"http://b:2"}]}`.
- `TestProxiesUpdateReplacesWholeList` — update with two `--url` → PATCH body carries exactly those two; update with only `--name` → PATCH carries existing entries forward (fixture pool fetched first — assert the GET happens).
- `TestProxiesDeleteGates` — retype/`--yes` parity with storage.
- Help-text test: `urlbox proxies update --help` Long contains the whole-list-replacement warning sentence.

- [ ] **Step 2–4: RED → implement → GREEN.** `runProxiesUpdate` fetches the existing pool first (GET resource), then `mergeProxyUpdate`, then PATCH with the FULL body — this is the pinned reconcile semantic; the update help Long states: "The server replaces the pool's proxy list with exactly what you send: passing any --url replaces the whole list; omitting --url keeps the existing list." List/show/create/delete mirror Task 3's shapes (`proxies list` quiet scalar = count; show KV per `SRC/internal/cmd/proxies.go:94-116` with `maskProxyURL`). `make ci` + snapshot.

- [ ] **Step 5: Commit** — subject `feat(cli): proxy pool commands`.

---

### Task 5: `llm` group

**Files:**
- Create: `internal/cmd/llm.go`, `internal/cmd/llm_test.go`
- Modify: `internal/cmd/root.go`, `SURFACE.txt`

Surface:

```
urlbox llm list
urlbox llm show <name-or-id> [--reveal]
urlbox llm create --name … --provider … [--api-key … --model … --base-url …] [--json …] [--assign-to <project>]
urlbox llm update <name-or-id> [flags] [--json …]
urlbox llm delete <name-or-id> [--yes]
urlbox llm test <name-or-id>
urlbox llm models <name-or-id>
```

- [ ] **Step 1: Failing tests** — fixture per pinned `LlmCredentialDto` (flat; include `apiKey` full value to prove masking):

Required tests: list table `ID/NAME/PROVIDER/MODEL/ASSIGNED`; show masks `apiKey`/AWS/GCP secrets with `--reveal` unhiding; create body via typed flags (`--api-key` → `apiKey`, `--base-url` → `baseUrl`) + `--json` long-tail merge; update partial; update with `--provider` → client-side `ErrUsage` "provider is immutable after create — create a new credential instead"; bedrock create without AWS fields → the source's exact required-fields message; delete gates; `TestLlmTestOkAndError` — POST `…/llm_1/test` with `{"ok":true}` → summary `Connection OK`, exit 0; `{"ok":false,"error":"bad key"}` → summary contains `bad key`, non-zero exit mapping per source (`ok:false` envelope, exit 1); `TestLlmModels` — POST `…/llm_1/models` `{"models":["gpt-a","gpt-b"]}` → table (single `MODEL` column) with both rows.

- [ ] **Step 2–4: RED → implement → GREEN.** Port `llmListRows`/`llmDetailPairs`/`llmTestMessage` from `SRC/internal/cmd/llm.go:60-132,394-402` (secret-field table drives masking). `llm test` and `llm models` PostJSON `{}` bodies. `make ci` + snapshot.

- [ ] **Step 5: Commit** — subject `feat(cli): llm credential commands`.

---

### Task 6: `projects storage|proxy|llm assign|unassign`

**Files:**
- Modify: `internal/cmd/projects.go` (three sub-groups on the existing projects command)
- Create: `internal/cmd/projects_assign_test.go`
- Modify: `SURFACE.txt`

**Interfaces:** consumes `credKind.assignPath`, `resolveCredArg`, `resolveProjectArg` (landed), `PutJSON`/`DeleteJSON`.

Surface (per kind `k` in storage/proxy/llm):

```
urlbox projects storage assign <project> <cred>
urlbox projects storage unassign <project>
urlbox projects proxy assign <project> <pool>
urlbox projects proxy unassign <project>
urlbox projects llm assign <project> <cred>
urlbox projects llm unassign <project>
```

- [ ] **Step 1: Failing tests** — for each kind: assign resolves BOTH positionals (project by name via `/v2/projects` list; cred by name via the kind's org list — fixtures per pinned shapes), then `PUT assignPath` with `{bodyKey: credID}`; response = flat `ProjectDto` fixture; summary `Assigned <cred-name> to <project-name>`. Unassign → `DELETE assignPath`, summary `Unassigned the <noun> from <project-name>`. Not-found and ambiguity paths reuse Task 2 errors (one test per class, not per kind). JSON mode envelope carries the returned project.

- [ ] **Step 2–4: RED → implement → GREEN.** One generic constructor `newProjectsCredSubCmd(kind credKind)` producing the sub-group — registered three times inside `newProjectsCmd`; no per-kind copy-paste. `make ci` + snapshot (SURFACE.txt gains all six paths + inherited flags).

- [ ] **Step 5: Commit** — subject `feat(cli): project credential assign/unassign commands`.

---

### Task 7: Auth removal sweep

**Files:**
- Delete: `internal/cmd/auth.go`, `internal/cmd/auth_test.go`
- Modify or delete: `internal/cmd/auth_preflight.go` (+ test) — FIRST verify with `grep -rn "requireSecret\|auth_preflight" internal/` which commands still consume it: the render/link/status preflight ("run `urlbox auth`" hints) STAYS but its hint text changes to the unified login hint; delete only if genuinely orphaned, otherwise keep the file and update its hints.
- Modify: `internal/cmd/doctor.go`, `internal/cmd/doctor_test.go` (+ `doctor_internal_test.go` if present)
- Modify: `internal/cmd/login.go` (browser-open gate → resolved format), `internal/cmd/login_test.go` (+ quiet test)
- Modify: `internal/cmd/root.go` (unregister auth), `internal/cmd/error_hints_test.go` pinned strings if they reference `urlbox auth`, any `render_hint.go`-adjacent hint constants that mention `urlbox auth` (hint TEXT updates in non-command files are in scope; do not otherwise touch the render family's logic)
- Modify: `README.md`, `npm/README.md`, `skills/SKILL.md` (remove/replace every `urlbox auth` reference with the login story; `URLBOX_API_SECRET` stays as the CI path)
- Create: `internal/cmd/login_hint.go` (or add to `session_helpers.go`): a single `const loginHint = "Run `urlbox login` to sign in."` and `const notLoggedInMsg = "not logged in — run `urlbox login`"` — then replace the three existing wording variants (grep: `urlbox login` in `internal/`) so every auth-class error uses these two strings.
- Modify: `SURFACE.txt` (auth removals recorded)

Doctor's session-aware check set (keeps the existing table rendering and per-check glyph/hint mechanics; exit codes and JSON schema unchanged in SHAPE — the checks themselves change):

| check | ok when | hint on failure |
|---|---|---|
| `version`, `install_method`, `config_file`, `dns`, `api_reachable` | unchanged | unchanged |
| `session` (replaces `api_secret`-only view) | `session_token` present AND `GET /v1/auth/get-session` returns a user | `loginHint` |
| `active_org` / `active_project` | profile fields set | "Run `urlbox orgs select` / `urlbox projects select`" |
| `render_credential` | `api_secret` present (env or profile) | `loginHint` (env `URLBOX_API_SECRET` note for CI) |
| `auth` (credential validity probe) | unchanged mechanics, hint → `loginHint` |

Not logged in at all → the session/org/project/render checks report their failure states (doctor still exits per its existing worst-check rule); doctor NEVER requires login to run.

- [ ] **Step 1: Failing tests.** Doctor: logged-out config → `session` check fails with `loginHint`, exit code preserved per existing doctor exit rules; logged-in fixture (scripted get-session) → all new checks ok. Login gate: `urlbox login > file` with TTY-stdin fake (resolved format json) must NOT invoke the browser opener (inject the opener recorder) and must NOT prompt; new `TestLoginQuietPrintsEmailScalar` (scripted full flow, `--output-format quiet` → bare email line). Sweep guards: a NEW test `TestNoAuthCommandRemains` asserting `Execute(["auth"])` yields the unknown-command error, and extend the ghost-command/hint guard so no production string contains `urlbox auth` (grep-style test over `internal/` matching the repo's existing hint-guard test mechanics).
- [ ] **Step 2: RED** — new tests fail (auth still registered, hints still varied, doctor still secret-shaped).
- [ ] **Step 3: Implement** the deletions, doctor rework, hint unification, gate fix, docs sweep. The three docs files: replace auth sections with the login flow (mirror `README.md`'s existing Authentication section wording — it is already login-first; npm/README and SKILL.md align to it; no profile mentions).
- [ ] **Step 4: GREEN + gates.** `make ci`; `make surface-snapshot` — SURFACE.txt LOSES the `urlbox auth…` block (removals are expected and deliberate; surface-check passes because the snapshot is regenerated and committed).
- [ ] **Step 5: Commit** — paths: the deleted/modified files + docs + SURFACE.txt. Subject: `feat(cli)!: remove urlbox auth in favour of login`.

---

### Task 8: Compat extension, verification pass, in-repo docs for the new groups

**Files:**
- Modify: `internal/cmd/compat_session_config_test.go` (the two-state suite gains cases exercising the NEW commands' guard behaviour: `storage list`, `proxies list`, `llm list` in the legacy (logged-out) state → identical clean auth errors in both shapes where applicable — the two states differ on session fields, so for these commands assert the LEGACY state errors `not_logged_in` and the POST-LOGIN state fails only at the network boundary of the fake host: pin exit codes and stderr equality per the suite's mechanics; follow the suite's existing exemption pattern for state-dependent cells)
- Modify: `README.md`, `npm/README.md`, `skills/SKILL.md` (add the three groups + assign/unassign to the command documentation, house tone, no profiles)
- Create: `docs/superpowers/verification/2026-08-17-plan2-agent-layer.md` (the agent-layer verification record)
- Modify: `docs/superpowers/verification/2026-08-14-plan1-manual-production-checklist.md` — append a "Plan 2" section: storage/proxies/llm lifecycle with a throwaway credential each (create → list → show → show --reveal → update → assign → unassign → delete), `llm test`/`llm models` against a real credential if one exists, `urlbox auth` now errors, doctor's new checks logged-in AND logged-out.

- [ ] **Step 1:** compat additions RED→GREEN.
- [ ] **Step 2:** docs edits; `make ci`.
- [ ] **Step 3: Agent-layer verification (mandatory, recorded):** build the binary; drive EVERY command in text mode against production — the full Plan-1 set (regression) plus every new group command using a throwaway storage credential/proxy pool/llm credential where creatable without real third-party secrets (storage/proxy fine with fake values — creation is server-side validated but accepts unverified values; `llm test` may legitimately fail on a fake key: record the failure output as the expected shape). Record command + output + PASS/FAIL per cell in the verification doc. Any FAIL stops the line.
- [ ] **Step 4: Commit** — subject `test(cli): compat + verification for credential resources`.

---

## Execution notes

- Tasks 3, 4, 5 are structurally parallel but MUST run sequentially (shared `root.go`, shared SURFACE.txt, per-task commits).
- The reviewer of each task gets the pinned-shape table above as part of its constraints; a fixture that deviates from a pinned shape is an Important finding.
- Carried Plan-1 acceptances that Plan 2 deliberately does NOT reopen: `resolve.go` provenance duplication (accepted), whoami raw-id-on-error display (accepted), logout retry-budget test cost (accepted).

## Self-review

1. **Spec coverage (slices 4–5):** storage/proxies/llm CRUD ✓ (T3–5), llm test/models ✓ (T5), assign/unassign + create→assign + `--assign-to` ✓ (T2/T6, wired in T3–5 creates), auth removal + doctor session-world + reference sweep ✓ (T7), unified hint ✓ (T7), login gate + quiet test (carried items) ✓ (T7), compat + verification + docs ✓ (T8). `storage test`/`proxies test` correctly absent (spec: out of scope, no endpoints). Spec's stale storage-columns line superseded with a note ✓.
2. **Placeholder scan:** Task 3 Step 1 test bodies are specified as required-assertion lists with an explicit expansion instruction rather than full Go for all eight — each names its exact assertions and fixtures; no TBDs remain. Tasks 4–6 reference Task 3's expansion rule with their own required-assertion lists. All transplant code is inlined or pinned to SRC file:line.
3. **Type consistency vs landed files:** `SessionAPI` currently lacks `PutJSON` — T2 adds it and updates fakes ✓; `resolveNameOrID` returns `*output.CLIError` ✓ matched; `fetchList(ctx, client, path, key)` signature ✓; `prompt.Confirm(title string) (bool, error)` exists ✓ (T7 doctor does not need it; disable-grade gates not present in Plan 2's surface — deletes use TypeToConfirm) ✓; `maskSecret(s string)` single-arg ✓ (reveal handled at call sites, matching landed `projects show`).
