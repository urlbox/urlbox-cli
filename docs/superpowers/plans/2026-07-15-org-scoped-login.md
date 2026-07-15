# Organisation-Scoped Login (`urlbox login`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace paste-a-secret onboarding with a browser device-flow login: `urlbox login` opens the dashboard, the user approves and picks an org + project, and the CLI ends up holding a session token plus that project's render credential — with `urlbox org list/use` and `urlbox project list/use` for switching afterwards.

**Architecture:** A new management client (`internal/api/mgmt.go`) speaks to the better-auth device-flow endpoints (`/v1/auth/device/*`), the org endpoints (`/v1/auth/organization/*`), and the org-scoped `/v2` discovery endpoints (projects, api-credentials), authenticating with `Authorization: Bearer <session token>`. `urlbox login` runs the flow and writes five new fields onto the existing `config.Profile`; the project's `ubx_sk_` secret lands in the *existing* `api_secret` field, so the entire render pipeline (`config.Resolve` → `api.NewHTTPClient`) is untouched. Org/project switching commands re-discover over `/v2` and rewrite the profile.

**Tech Stack:** Go, cobra, existing repo machinery: `internal/output` envelopes + closed error codes, `internal/config` (XDG multi-profile, `config.Update` file-locked writes), `internal/browser` opener, `httptest` for fakes.

**Server dependency:** The mono-repo amendment plan (`urlbox-mono` `docs/superpowers/plans/2026-07-15-device-auth-session-redeem.md`) must be deployed first. This plan consumes exactly: `POST /v1/auth/device/code`, `POST /v1/auth/device/token`, `POST /v2/device-grants/redeem` (Bearer) → `{organisation:{id,name}, project:{id,name}}`, `GET /v1/auth/organization/list` (rows include `publicId`), `POST /v1/auth/organization/set-active`, `GET /v2/organisation/{org}/projects`, `GET /v2/organisation/{org}/projects/{project}/api-credentials`, `POST /v1/auth/sign-out`.

## Design decisions (normative — check any implementation against these)

1. **The command is `urlbox login`, top-level.** NOT `urlbox auth login`. Its sibling is `urlbox logout`. `urlbox auth` (paste-a-secret) stays byte-for-byte as-is — it is the headless/CI path forever (device flow needs a browser) and `SURFACE.txt` forbids removals anyway.
2. **`client_id` is exactly `"urlbox-cli"`** (the server's `validateClient` rejects everything else).
3. **Org + project are chosen in the browser** during approval. The CLI does not prompt for them during `login`; it receives them from redeem.
4. **The render path is untouched.** The fetched `ubx_sk_` secret is stored in `profile.api_secret` (and `ubx_` in `api_key`); explicit credentials (flag > env > repo overlay) still beat the profile via the unchanged `config.Resolve` precedence. No lazy credential fetch at render time.
5. **One org per profile.** `urlbox login --profile work` keeps orgs side by side; `urlbox org use` re-points the *current* profile (server `set-active` + local rewrite + project re-discovery). `login` may create the target profile if it doesn't exist (unlike `auth`, which requires it — login is the onboarding entry point).
6. **Explicit org paths on `/v2`.** After login the CLI always calls `/v2/organisation/{org_…}/...` with the stored org public id — never the path-less public surface — so behavior can't silently depend on server-side active-org state. `set-active` is still called (at redeem server-side, and by `org use`) to keep the session consistent.
7. **Session expiry is a first-class error**: any 401 from a Bearer-authenticated call maps to `ErrAuth` with the hint "Your session has expired — run `urlbox login`". Session tokens live only in the 0600 config file, are masked everywhere (`maskSecret`), and `config get session_token` requires `--reveal`.
8. **`logout`** signs out server-side (best-effort) and clears every login-written field (`session_token`, `org_*`, `project_*`, `api_key`, `api_secret`) from the profile, preserving `api_host`.
9. **All new surface is additive** — `SURFACE.txt` gains entries, loses none.

## Global Constraints

(From repo `CLAUDE.md` — binding for every task.)

- **TDD from commit one.** Failing test → minimal implementation → green → refactor → commit.
- `make lint` and `make fmt-check` clean before every commit.
- **Stdout for data, stderr for human messages.** Prompts, spinners, "opening browser…" chatter → stderr.
- Every command output uses `output.Envelope` / `ErrorEnvelope`; errors map to the closed `output.ErrorCode` set; breadcrumbs point at the next command.
- Every new command/flag must appear in `SURFACE.txt` (`make surface-snapshot` after the surface stabilizes, commit the file with the code).
- **Never commit autonomously.** Each task's commit step means: show the diff + proposed message, wait for approval.
- `make ci` (fmt-check, lint, test, build, surface-check) must pass at the end of every task.

---

### Task 1: Profile schema — session + org + project fields

**Files:**
- Modify: `internal/config/profile.go`
- Modify: `internal/cmd/config.go` (the two `switch key` blocks at ~lines 585 and 602 — `config get` / `config set` key handling)
- Test: `internal/config/profile_test.go`, `internal/cmd/config_test.go` (extend)

**Interfaces:**
- Produces: `config.Profile` gains `SessionToken`, `OrgID`, `OrgName`, `ProjectID`, `ProjectName` (JSON keys `session_token`, `org_id`, `org_name`, `project_id`, `project_name`). Every later task reads/writes these exact names.

- [ ] **Step 1: Write failing tests**

In `internal/config/profile_test.go`:

```go
func TestProfile_LoginFieldsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := config.Save(&config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{"default": {
			APIKey:       "ubx_pub",
			APISecret:    "ubx_sk_secret",
			SessionToken: "sess_token_value",
			OrgID:        "org_abc123",
			OrgName:      "Acme",
			ProjectID:    "prj_def456",
			ProjectName:  "Website",
		}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := cfg.Profiles["default"]
	if p.SessionToken != "sess_token_value" || p.OrgID != "org_abc123" ||
		p.OrgName != "Acme" || p.ProjectID != "prj_def456" || p.ProjectName != "Website" {
		t.Errorf("login fields did not round-trip: %+v", p)
	}
}

func TestProfile_IsEmpty_ConsidersLoginFields(t *testing.T) {
	if (config.Profile{SessionToken: "x"}).IsEmpty() {
		t.Error("profile with session token reported empty")
	}
}
```

In `internal/cmd/config_test.go` (follow the file's existing get/set test shape):

```go
func TestConfigGet_SessionToken_MaskedWithoutReveal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedProfile(t, config.Profile{SessionToken: "sess_supersecretvalue"}) // reuse/add the file's seeding helper
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "get", "session_token", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), "sess_supersecretvalue") {
		t.Error("session token leaked without --reveal")
	}
}

func TestConfigSet_SessionToken_Rejected(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "session_token", "x", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("config set session_token should be rejected — it is login-managed")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "urlbox login") {
		t.Errorf("rejection should point at urlbox login; got %s", combined)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ ./internal/cmd/ -run 'TestProfile_Login|TestProfile_IsEmpty_Considers|TestConfigGet_SessionToken|TestConfigSet_SessionToken' -v`
Expected: FAIL (unknown fields / unknown key).

- [ ] **Step 3: Implement**

`internal/config/profile.go`:

```go
// Package-level doc unchanged.
package config

// Profile is one named credential set persisted in Config.Profiles.
//
// The api_key/api_secret pair is the project render credential — either
// pasted via `urlbox auth` or fetched by `urlbox login`/`project use`.
// The session/org/project fields exist only on logged-in profiles and are
// written exclusively by login/org/project commands (config set rejects
// them; see internal/cmd/config.go).
type Profile struct {
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`
	APIHost   string `json:"api_host,omitempty"`

	// SessionToken is the better-auth session obtained by `urlbox login`
	// (Authorization: Bearer …). Management-plane only — never sent to
	// render endpoints. ~7-day lifetime with sliding refresh; expiry maps
	// to ErrAuth "run urlbox login".
	SessionToken string `json:"session_token,omitempty"`
	OrgID        string `json:"org_id,omitempty"`   // org_… public id
	OrgName      string `json:"org_name,omitempty"`
	ProjectID    string `json:"project_id,omitempty"` // prj_… public id
	ProjectName  string `json:"project_name,omitempty"`
}

// IsEmpty reports whether the profile has no stored state at all.
func (p Profile) IsEmpty() bool {
	return p == Profile{}
}
```

`internal/cmd/config.go` — in the `config get` key switch, add arms alongside `api_secret` (same masking + `--reveal` behavior as `api_secret` for the token; plain values for the rest):

```go
	case "session_token":
		// masked unless --reveal, exactly like api_secret
	case "org_id", "org_name", "project_id", "project_name":
		// plain string value
```

(Copy the exact masking/reveal code the `api_secret` arm uses — same helper, same envelope shape.)

In the `config set` key switch, reject the login-managed keys:

```go
	case "session_token", "org_id", "org_name", "project_id", "project_name":
		return output.NewCLIError(
			output.ErrUsage,
			"\""+key+"\" is managed by urlbox login",
			"Run `urlbox login` to sign in, `urlbox org use <org>` to switch organisation, or `urlbox project use <project>` to switch project.",
		)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ ./internal/cmd/ -run 'TestProfile_Login|TestProfile_IsEmpty_Considers|TestConfigGet_SessionToken|TestConfigSet_SessionToken' -v`
Expected: PASS. Then `make ci`.

- [ ] **Step 5: Propose commit**

Show diff + message, wait for approval:
```
feat(config): profile fields for org-scoped login (session token, org, project)
```

---

### Task 2: Management API client (`internal/api/mgmt.go`)

**Files:**
- Create: `internal/api/mgmt.go`
- Test: `internal/api/mgmt_test.go`

**Interfaces:**
- Consumes: `output.NewCLIError` + error codes, `BuildUserAgent`, `version.Version`.
- Produces (later tasks call these exact signatures):

```go
const ClientID = "urlbox-cli"

func NewMgmtClient(baseURL, sessionToken string) *MgmtClient

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}
func (c *MgmtClient) StartDeviceFlow(ctx context.Context) (*DeviceCodeResponse, error)
func (c *MgmtClient) PollDeviceToken(ctx context.Context, deviceCode string, interval time.Duration,
	sleep func(context.Context, time.Duration) error) (string, error)

type NamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type RedeemResult struct {
	Organisation NamedRef `json:"organisation"`
	Project      NamedRef `json:"project"`
}
func (c *MgmtClient) Redeem(ctx context.Context, deviceCode string) (*RedeemResult, error)

type Organization struct {
	ID       FlexID `json:"id"` // better-auth numeric pk (string or number in JSON)
	PublicID string `json:"publicId"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}
func (c *MgmtClient) ListOrgs(ctx context.Context) ([]Organization, error)
func (c *MgmtClient) SetActiveOrg(ctx context.Context, organizationID string) error

type MgmtProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
func (c *MgmtClient) ListProjects(ctx context.Context, org string) ([]MgmtProject, error)

type APICredential struct {
	ID        string `json:"id"`
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
	Revoked   bool   `json:"revoked"`
}
func (c *MgmtClient) ListCredentials(ctx context.Context, org, project string) ([]APICredential, error)

func (c *MgmtClient) SignOut(ctx context.Context) error
```

- [ ] **Step 1: Write failing tests**

`internal/api/mgmt_test.go` — one `httptest.Server` with a `http.ServeMux` faking the real endpoints:

```go
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func noSleep(context.Context, time.Duration) error { return nil }

func TestMgmt_StartDeviceFlow_SendsClientID(t *testing.T) {
	var gotClientID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/device/code" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotClientID = body["client_id"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev_12345678901234567890", "user_code": "ABCD-EFGH",
			"verification_uri": "https://urlbox.com/device",
			"verification_uri_complete": "https://urlbox.com/device?user_code=ABCD-EFGH",
			"expires_in": 900, "interval": 5,
		})
	}))
	t.Cleanup(srv.Close)

	dc, err := api.NewMgmtClient(srv.URL, "").StartDeviceFlow(context.Background())
	if err != nil {
		t.Fatalf("StartDeviceFlow: %v", err)
	}
	if gotClientID != api.ClientID || api.ClientID != "urlbox-cli" {
		t.Errorf("client_id=%q, want urlbox-cli", gotClientID)
	}
	if dc.UserCode != "ABCD-EFGH" || dc.Interval != 5 || dc.VerificationURIComplete == "" {
		t.Errorf("bad parse: %+v", dc)
	}
}

func TestMgmt_PollDeviceToken_PendingThenSuccess(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "sess_tok", "token_type": "Bearer"})
	}))
	t.Cleanup(srv.Close)

	tok, err := api.NewMgmtClient(srv.URL, "").PollDeviceToken(
		context.Background(), "dev_12345678901234567890", 5*time.Second, noSleep)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if tok != "sess_tok" {
		t.Errorf("token=%q", tok)
	}
	if calls.Load() != 3 {
		t.Errorf("calls=%d, want 3", calls.Load())
	}
}

func TestMgmt_PollDeviceToken_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	}))
	t.Cleanup(srv.Close)
	_, err := api.NewMgmtClient(srv.URL, "").PollDeviceToken(
		context.Background(), "dev_12345678901234567890", time.Second, noSleep)
	if err == nil {
		t.Fatal("denied poll should error")
	}
}

func TestMgmt_BearerAndPaths(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"apiCredentials": []map[string]any{
			{"id": "cred_1", "apiKey": "ubx_k", "apiSecret": "ubx_sk_s", "revoked": false},
		}})
	}))
	t.Cleanup(srv.Close)
	creds, err := api.NewMgmtClient(srv.URL, "sess_tok").
		ListCredentials(context.Background(), "org_a", "prj_b")
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if gotAuth != "Bearer sess_tok" {
		t.Errorf("auth=%q", gotAuth)
	}
	if gotPath != "/v2/organisation/org_a/projects/prj_b/api-credentials" {
		t.Errorf("path=%q", gotPath)
	}
	if len(creds) != 1 || creds[0].APISecret != "ubx_sk_s" {
		t.Errorf("creds=%+v", creds)
	}
}

func TestMgmt_Unauthorized_MapsToAuthErrorWithLoginHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	_, err := api.NewMgmtClient(srv.URL, "stale").ListOrgs(context.Background())
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("want session-expired auth error, got %v", err)
	}
}

func TestMgmt_ListOrgs_FlexibleIDAndPublicID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// better-auth may emit the numeric pk as a number or string.
		_, _ = w.Write([]byte(`[{"id": 42, "publicId": "org_a", "name": "Acme", "slug": "acme"},
			{"id": "43", "publicId": "org_b", "name": "Beta", "slug": "beta"}]`))
	}))
	t.Cleanup(srv.Close)
	orgs, err := api.NewMgmtClient(srv.URL, "tok").ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if string(orgs[0].ID) != "42" || string(orgs[1].ID) != "43" || orgs[0].PublicID != "org_a" {
		t.Errorf("orgs=%+v", orgs)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/api/ -run TestMgmt -v`
Expected: FAIL — package has no `MgmtClient`.

- [ ] **Step 3: Implement `internal/api/mgmt.go`**

```go
// Package api — management-plane client for the org-scoped login flow.
//
// MgmtClient talks to the better-auth endpoints (/v1/auth/…) and the
// org-scoped /v2 discovery endpoints, authenticating with the session
// token as `Authorization: Bearer …`. It is deliberately separate from
// HTTPClient: render traffic authenticates with the project secret and
// must never carry the session token.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// ClientID is the OAuth device-flow client identifier. The server's
// validateClient hook rejects any other value.
const ClientID = "urlbox-cli"

// Management endpoint paths (mono: better-auth basePath /v1/auth; oRPC /v2).
const (
	PathDeviceCode   = "/v1/auth/device/code"
	PathDeviceToken  = "/v1/auth/device/token"
	PathOrgList      = "/v1/auth/organization/list"
	PathOrgSetActive = "/v1/auth/organization/set-active"
	PathSignOut      = "/v1/auth/sign-out"
	PathDeviceRedeem = "/v2/device-grants/redeem"
)

const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// MgmtClient is the management-plane HTTP client.
type MgmtClient struct {
	BaseURL      string
	SessionToken string
	UserAgent    string
	HTTP         *http.Client
}

// NewMgmtClient builds a client. sessionToken may be empty for the
// pre-login device-flow calls.
func NewMgmtClient(baseURL, sessionToken string) *MgmtClient {
	return &MgmtClient{
		BaseURL:      baseURL,
		SessionToken: sessionToken,
		UserAgent:    BuildUserAgent(version.Version),
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// FlexID tolerates better-auth emitting the numeric pk as a JSON number
// or string depending on adapter/serializer.
type FlexID string

// UnmarshalJSON accepts both `42` and `"42"`.
func (f *FlexID) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	*f = FlexID(s)
	return nil
}

// DeviceCodeResponse is the RFC 8628 device authorization response.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// NamedRef is an {id, name} pair as returned by redeem.
type NamedRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RedeemResult is the payout of POST /v2/device-grants/redeem.
type RedeemResult struct {
	Organisation NamedRef `json:"organisation"`
	Project      NamedRef `json:"project"`
}

// Organization is one row of GET /v1/auth/organization/list.
type Organization struct {
	ID       FlexID `json:"id"`
	PublicID string `json:"publicId"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
}

// MgmtProject is one row of GET /v2/organisation/{org}/projects.
type MgmtProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// APICredential is one row of GET …/projects/{project}/api-credentials.
type APICredential struct {
	ID        string `json:"id"`
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
	Revoked   bool   `json:"revoked"`
}

// StartDeviceFlow requests a device + user code pair.
func (c *MgmtClient) StartDeviceFlow(ctx context.Context) (*DeviceCodeResponse, error) {
	var dc DeviceCodeResponse
	if err := c.doJSON(ctx, http.MethodPost, PathDeviceCode,
		map[string]string{"client_id": ClientID}, &dc); err != nil {
		return nil, err
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return &dc, nil
}

// PollDeviceToken polls the token endpoint until approval, denial, expiry,
// or ctx cancellation. sleep is injected for testability; production
// callers pass sleepCtx below.
func (c *MgmtClient) PollDeviceToken(ctx context.Context, deviceCode string,
	interval time.Duration, sleep func(context.Context, time.Duration) error) (string, error) {
	for {
		req, err := c.newRequest(ctx, http.MethodPost, PathDeviceToken, map[string]string{
			"grant_type":  deviceGrantType,
			"device_code": deviceCode,
			"client_id":   ClientID,
		})
		if err != nil {
			return "", err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return "", output.NewCLIError(output.ErrNetwork, err.Error(),
				"Check your internet connection and the API host. Run `urlbox doctor`.")
		}
		var body struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusOK && decodeErr == nil && body.AccessToken != "":
			return body.AccessToken, nil
		case body.Error == "authorization_pending":
			// keep polling
		case body.Error == "slow_down":
			interval += 5 * time.Second
		case body.Error == "expired_token":
			return "", output.NewCLIError(output.ErrTimeout,
				"the device code expired before the request was approved",
				"Run `urlbox login` again and approve the request within 15 minutes.")
		case body.Error == "access_denied":
			return "", output.NewCLIError(output.ErrForbidden,
				"the login request was denied in the browser",
				"Run `urlbox login` again if this was a mistake.")
		default:
			return "", mapMgmtError(resp.StatusCode, body.Error)
		}
		if err := sleep(ctx, interval); err != nil {
			return "", output.NewCLIError(output.ErrTimeout,
				"gave up waiting for browser approval: "+err.Error(),
				"Run `urlbox login` again and approve the request in your browser.")
		}
	}
}

// SleepCtx is the production sleep for PollDeviceToken.
func SleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Redeem swaps the approved device code (+ Bearer session) for its
// org + project identity.
func (c *MgmtClient) Redeem(ctx context.Context, deviceCode string) (*RedeemResult, error) {
	var r RedeemResult
	if err := c.doJSON(ctx, http.MethodPost, PathDeviceRedeem,
		map[string]string{"deviceCode": deviceCode}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ListOrgs returns the user's organisations (better-auth rows incl. publicId).
func (c *MgmtClient) ListOrgs(ctx context.Context) ([]Organization, error) {
	var orgs []Organization
	if err := c.doJSON(ctx, http.MethodGet, PathOrgList, nil, &orgs); err != nil {
		return nil, err
	}
	return orgs, nil
}

// SetActiveOrg points the CLI session's active organisation at the given
// better-auth organization id (the numeric pk, not org_…).
func (c *MgmtClient) SetActiveOrg(ctx context.Context, organizationID string) error {
	return c.doJSON(ctx, http.MethodPost, PathOrgSetActive,
		map[string]string{"organizationId": organizationID}, nil)
}

// ListProjects lists the org's projects. org is the org_… public id.
func (c *MgmtClient) ListProjects(ctx context.Context, org string) ([]MgmtProject, error) {
	var out struct {
		Projects []MgmtProject `json:"projects"`
	}
	path := "/v2/organisation/" + url.PathEscape(org) + "/projects"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Projects, nil
}

// ListCredentials lists a project's render credentials (returns secrets).
func (c *MgmtClient) ListCredentials(ctx context.Context, org, project string) ([]APICredential, error) {
	var out struct {
		APICredentials []APICredential `json:"apiCredentials"`
	}
	path := "/v2/organisation/" + url.PathEscape(org) +
		"/projects/" + url.PathEscape(project) + "/api-credentials"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.APICredentials, nil
}

// SignOut revokes the session server-side.
func (c *MgmtClient) SignOut(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, PathSignOut, map[string]string{}, nil)
}

// newRequest builds a JSON request with UA + optional Bearer auth.
func (c *MgmtClient) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, output.NewCLIError(output.ErrUsage, "failed to encode request body", err.Error())
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, output.NewCLIError(output.ErrUsage, err.Error(), "")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.SessionToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.SessionToken)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	return req, nil
}

// doJSON sends and decodes into out (out may be nil to discard).
func (c *MgmtClient) doJSON(ctx context.Context, method, path string, body, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return output.NewCLIError(output.ErrNetwork, err.Error(),
			"Check your internet connection and the API host. Run `urlbox doctor`.")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		var apiBody struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiBody)
		msg := apiBody.Message
		if msg == "" {
			msg = apiBody.Error
		}
		return mapMgmtError(resp.StatusCode, msg)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return output.NewCLIError(output.ErrServer, "failed to parse API response", err.Error())
	}
	return nil
}

// mapMgmtError maps a management-plane HTTP failure to a typed CLIError.
// Distinct from mapStatusToCLIError (render plane): a 401 here means the
// SESSION is invalid, and the fix is `urlbox login`, not a new secret.
func mapMgmtError(status int, msg string) *output.CLIError {
	switch {
	case status == http.StatusUnauthorized:
		if msg == "" {
			msg = "your session has expired or been revoked"
		}
		return output.NewCLIError(output.ErrAuth, msg,
			"Run `urlbox login` to sign in again.")
	case status == http.StatusForbidden:
		if msg == "" {
			msg = "your account does not have access to this resource"
		}
		return output.NewCLIError(output.ErrForbidden, msg,
			"Check your role in this organisation on the dashboard.")
	case status == http.StatusNotFound:
		if msg == "" {
			msg = "not found"
		}
		return output.NewCLIError(output.ErrNotFound, msg,
			"Run `urlbox org list` / `urlbox project list` to see what exists.")
	case status == http.StatusTooManyRequests:
		if msg == "" {
			msg = "rate-limited by the Urlbox API"
		}
		return output.NewCLIError(output.ErrRateLimit, msg, "Retry shortly.")
	case status >= 500:
		if msg == "" {
			msg = fmt.Sprintf("Urlbox API returned HTTP %d", status)
		}
		return output.NewCLIError(output.ErrServer, msg,
			"Try again in a moment, or check status.urlbox.com.")
	default:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d from Urlbox API", status)
		}
		return output.NewCLIError(output.ErrUsage, msg,
			"Run `urlbox doctor` to verify connectivity and session state.")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -run TestMgmt -v`
Expected: PASS. Then `make ci`.

- [ ] **Step 5: Propose commit**

```
feat(api): management-plane client — device flow, orgs, projects, credentials
```

---

### Task 3: `urlbox login`

**Files:**
- Create: `internal/cmd/login.go`
- Create: `internal/cmd/write_envelope.go` (tiny shared success-envelope writer for the new commands; if an equivalent helper already exists in the package, use it instead and skip the new file)
- Modify: `internal/cmd/root.go` (register `newLoginCmd()`)
- Test: `internal/cmd/login_test.go`

**Interfaces:**
- Consumes: `api.NewMgmtClient` + Task 2 methods, `config.Update`, `browser.NewOSOpener().Open`, `maskSecret` (auth.go).
- Produces: `loginProfileName(cfg, flagProfile, envProfile) string` (flag > env > `cfg.DefaultProfile` > `"default"`); `writeEnvelope(cmd *cobra.Command, env *output.Envelope) error`. Tasks 4–6 reuse both.

- [ ] **Step 1: Write failing tests**

`internal/cmd/login_test.go`:

```go
package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
)

// fakeMgmtServer wires the full happy-path device flow: code → pending →
// token → redeem → credentials.
func fakeMgmtServer(t *testing.T) *httptest.Server {
	t.Helper()
	var tokenCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev_12345678901234567890", "user_code": "ABCD-EFGH",
			"verification_uri":          "https://urlbox.com/device",
			"verification_uri_complete": "https://urlbox.com/device?user_code=ABCD-EFGH",
			"expires_in":                900, "interval": 0, // interval 0 keeps the test fast
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, _ *http.Request) {
		if tokenCalls.Add(1) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "sess_tok"})
	})
	mux.HandleFunc("/v2/device-grants/redeem", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sess_tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organisation": map[string]string{"id": "org_a", "name": "Acme"},
			"project":      map[string]string{"id": "prj_b", "name": "Website"},
		})
	})
	mux.HandleFunc("/v2/organisation/org_a/projects/prj_b/api-credentials",
		func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"apiCredentials": []map[string]any{
				{"id": "cred_1", "apiKey": "ubx_key", "apiSecret": "ubx_sk_secret", "revoked": false},
			}})
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLogin_HappyPath_WritesProfileAndEnvelope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := fakeMgmtServer(t)
	t.Setenv(api.EnvAPIHost, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"login", "--no-open", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}

	// The user code and verification URL are human chatter → stderr.
	if !strings.Contains(stderr.String(), "ABCD-EFGH") {
		t.Errorf("stderr should show the user code; got %s", stderr.String())
	}
	if strings.Contains(stdout.String(), "ubx_sk_secret") || strings.Contains(stdout.String(), "sess_tok") {
		t.Error("secrets must not appear unmasked in the envelope")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	p := cfg.Profiles[cfg.DefaultProfile]
	if p.SessionToken != "sess_tok" || p.OrgID != "org_a" || p.OrgName != "Acme" ||
		p.ProjectID != "prj_b" || p.ProjectName != "Website" ||
		p.APIKey != "ubx_key" || p.APISecret != "ubx_sk_secret" {
		t.Errorf("profile not fully written: %+v", p)
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true || env["command"] != "login" {
		t.Errorf("envelope ok/command wrong: %v %v", env["ok"], env["command"])
	}
}

func TestLogin_Denied_FailsWithForbidden(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/device/code", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev_12345678901234567890", "user_code": "ABCD-EFGH",
			"verification_uri_complete": "https://urlbox.com/device?user_code=ABCD-EFGH",
			"expires_in":                900, "interval": 0,
		})
	})
	mux.HandleFunc("/v1/auth/device/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "access_denied"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv(api.EnvAPIHost, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"login", "--no-open", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("denied login must fail")
	}
	if cfg, _ := config.Load(); cfg != nil && len(cfg.Profiles) > 0 {
		for _, p := range cfg.Profiles {
			if p.SessionToken != "" {
				t.Error("denied login must not persist a session token")
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestLogin -v`
Expected: FAIL — `unknown command "login"`.

- [ ] **Step 3: Implement**

`internal/cmd/write_envelope.go`:

```go
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// writeEnvelope renders a success envelope honoring --output-format and
// --jq, the same plumbing every command repeats (see auth.go). New
// login/org/project commands share it here.
func writeEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if jqExpr != "" {
		return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
	}
	return output.NewFormatter(format, output.NewStylesForWriter(stdout)).WriteSuccess(stdout, env)
}
```

`internal/cmd/login.go`:

```go
// internal/cmd/login.go — `urlbox login`: browser device-flow sign-in.
//
// The user's org + project are chosen in the BROWSER during approval; the
// CLI receives them from redeem, then fetches the project's render
// credential over /v2 with the session token. The render pipeline itself
// never sees the session token (Design decision: render auth is the
// project secret, exactly as with `urlbox auth`).
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// browserOpener is swappable for tests (--no-open covers most cases; the
// override guards against a test environment ever launching a browser).
var browserOpener interface{ Open(string) error } = browser.NewOSOpener()

// loginProfileName resolves the profile login writes to: flag > env >
// default_profile > "default". Unlike `auth`, login CREATES a missing
// profile — it is the onboarding entry point. The repo overlay is
// deliberately ignored (write command; same rationale as auth).
func loginProfileName(cfg *config.Config, flagProfile, envProfile string) string {
	switch {
	case flagProfile != "":
		return flagProfile
	case envProfile != "":
		return envProfile
	case cfg.DefaultProfile != "":
		return cfg.DefaultProfile
	default:
		return "default"
	}
}

// loginHost resolves the API host for the device flow: env > existing
// profile api_host > default. (config.Resolve is not used because the
// target profile may not exist yet.)
func loginHost(cfg *config.Config, profileName string) string {
	if h := os.Getenv(config.EnvAPIHost); h != "" {
		return h
	}
	if p, ok := cfg.Profiles[profileName]; ok && p.APIHost != "" {
		return p.APIHost
	}
	return config.DefaultAPIHost
}

func newLoginCmd() *cobra.Command {
	var noOpen bool
	c := &cobra.Command{
		Use:   "login",
		Short: "Sign in with your browser (device flow)",
		Long: `Signs in to Urlbox with the OAuth device flow.

The CLI shows a one-time code and opens your browser at the dashboard's
device page. Sign in (if needed), choose the organisation and project the
CLI should use, and approve. The CLI then stores your session and the
chosen project's render credential in the local config profile.

Afterwards:
  urlbox org list / urlbox org use <org>            switch organisation
  urlbox project list / urlbox project use <id>     switch project
  urlbox render <url>                               render with the stored credential

Headless / CI environments should keep using ` + "`urlbox auth`" + ` with a
project API secret — the device flow requires a browser.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, cliErr := config.LoadOrCLIError()
			if cliErr != nil {
				return cliErr
			}
			flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
			profileName := loginProfileName(cfg, flagProfile, os.Getenv(config.EnvProfile))
			host := loginHost(cfg, profileName)
			client := api.NewMgmtClient(host, "")

			dc, err := client.StartDeviceFlow(cmd.Context())
			if err != nil {
				return err
			}

			errw := cmd.ErrOrStderr()
			_, _ = fmt.Fprintf(errw, "Your one-time code: %s\n", dc.UserCode)
			openURL := dc.VerificationURIComplete
			if openURL == "" {
				openURL = dc.VerificationURI
			}
			if !noOpen && isStderrTTY(errw) {
				_, _ = fmt.Fprintf(errw, "Opening %s in your browser…\n", openURL)
				if oerr := browserOpener.Open(openURL); oerr != nil {
					_, _ = fmt.Fprintf(errw, "Could not open a browser (%v).\n", oerr)
				}
			}
			_, _ = fmt.Fprintf(errw, "If the browser didn't open, visit: %s\n", openURL)
			_, _ = fmt.Fprintln(errw, "Waiting for approval…")

			ctx := cmd.Context()
			if dc.ExpiresIn > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(dc.ExpiresIn)*time.Second)
				defer cancel()
			}
			token, err := client.PollDeviceToken(ctx, dc.DeviceCode,
				time.Duration(dc.Interval)*time.Second, api.SleepCtx)
			if err != nil {
				return err
			}

			client.SessionToken = token
			redeemed, err := client.Redeem(cmd.Context(), dc.DeviceCode)
			if err != nil {
				return err
			}

			cred, cliErr2 := pickActiveCredential(cmd.Context(), client,
				redeemed.Organisation.ID, redeemed.Project.ID)
			if cliErr2 != nil {
				return cliErr2
			}

			if err := config.Update(func(c *config.Config) error {
				if c.Profiles == nil {
					c.Profiles = map[string]config.Profile{}
				}
				p := c.Profiles[profileName]
				p.SessionToken = token
				p.OrgID = redeemed.Organisation.ID
				p.OrgName = redeemed.Organisation.Name
				p.ProjectID = redeemed.Project.ID
				p.ProjectName = redeemed.Project.Name
				p.APIKey = cred.APIKey
				p.APISecret = cred.APISecret
				c.Profiles[profileName] = p
				if c.DefaultProfile == "" {
					c.DefaultProfile = profileName
				}
				return nil
			}); err != nil {
				return output.NewCLIError(output.ErrForbidden, "failed to save config", err.Error())
			}

			env := output.NewEnvelope(
				"login",
				map[string]any{
					"profile":       profileName,
					"organisation":  map[string]string{"id": redeemed.Organisation.ID, "name": redeemed.Organisation.Name},
					"project":       map[string]string{"id": redeemed.Project.ID, "name": redeemed.Project.Name},
					"api_key":       cred.APIKey,
					"masked_secret": maskSecret(cred.APISecret),
					"config_path":   config.Path(),
				},
				fmt.Sprintf("Logged in to %s — default project %s",
					redeemed.Organisation.Name, redeemed.Project.Name),
				[]output.Breadcrumb{
					{Action: "render", Cmd: "urlbox render <url>"},
					{Action: "switch project", Cmd: "urlbox project list"},
					{Action: "switch organisation", Cmd: "urlbox org list"},
				},
			)
			return writeEnvelope(cmd, env)
		},
	}
	c.Flags().BoolVar(&noOpen, "no-open", false, "Print the verification URL instead of opening a browser")
	return c
}

// pickActiveCredential fetches the project's credentials and returns the
// first non-revoked one.
func pickActiveCredential(ctx context.Context, client *api.MgmtClient, org, project string) (*api.APICredential, *output.CLIError) {
	creds, err := client.ListCredentials(ctx, org, project)
	if err != nil {
		if cli, ok := err.(*output.CLIError); ok {
			return nil, cli
		}
		return nil, output.NewCLIError(output.ErrServer, err.Error(), "")
	}
	for i := range creds {
		if !creds[i].Revoked {
			return &creds[i], nil
		}
	}
	return nil, output.NewCLIError(output.ErrConflict,
		"the chosen project has no active API credential",
		"Create or un-revoke a credential for this project on the dashboard, then run `urlbox project use "+project+"`.")
}
```

`internal/cmd/root.go` — register alphabetically with the others:

```go
	cmd.AddCommand(newLoginCmd())
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestLogin -v`
Expected: PASS. Then `make ci` (surface-check will fail until Task 8's snapshot — run `make surface-snapshot` now and commit `SURFACE.txt` with this task).

- [ ] **Step 5: Propose commit**

```
feat(login): urlbox login — browser device-flow sign-in
```

---

### Task 4: `urlbox org list` / `urlbox org use <org>`

**Files:**
- Create: `internal/cmd/org.go`
- Create: `internal/cmd/session.go` (shared logged-in-profile loader)
- Modify: `internal/cmd/root.go` (register `newOrgCmd()`)
- Test: `internal/cmd/org_test.go`

**Interfaces:**
- Consumes: Task 2 client, Task 3 `writeEnvelope`.
- Produces: `loadSessionProfile(cmd *cobra.Command) (*config.Config, string, config.Profile, string, *output.CLIError)` returning (config, profileName, profile, host, err) — Tasks 5 and 6 reuse it. Requires a non-empty `SessionToken` or returns `ErrAuth` "not logged in".

- [ ] **Step 1: Write failing tests**

`internal/cmd/org_test.go` (seed a logged-in profile, fake the endpoints):

```go
func seedLoggedInProfile(t *testing.T, host string) {
	t.Helper()
	if err := config.Save(&config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{"default": {
			APIHost: host, SessionToken: "sess_tok",
			OrgID: "org_a", OrgName: "Acme",
			ProjectID: "prj_b", ProjectName: "Website",
			APIKey: "ubx_key", APISecret: "ubx_sk_secret",
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestOrgList_MarksActiveOrg(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/organization/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sess_tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[{"id": 1, "publicId": "org_a", "name": "Acme", "slug": "acme"},
			{"id": 2, "publicId": "org_c", "name": "Corp", "slug": "corp"}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"org", "list", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env struct {
		Data struct {
			Organizations []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Active bool   `json:"active"`
			} `json:"organizations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(env.Data.Organizations) != 2 {
		t.Fatalf("orgs=%+v", env.Data.Organizations)
	}
	for _, o := range env.Data.Organizations {
		if (o.ID == "org_a") != o.Active {
			t.Errorf("active flag wrong for %s: %v", o.ID, o.Active)
		}
	}
}

func TestOrgList_NotLoggedIn_ErrAuthWithLoginHint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"org", "list", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("org list without a session must fail")
	}
	if !strings.Contains(stdout.String()+stderr.String(), "urlbox login") {
		t.Error("error should point at urlbox login")
	}
}

func TestOrgUse_SwitchesOrgAndDiscoversSingleProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var setActiveBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/organization/list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id": 1, "publicId": "org_a", "name": "Acme", "slug": "acme"},
			{"id": 2, "publicId": "org_c", "name": "Corp", "slug": "corp"}]`))
	})
	mux.HandleFunc("/v1/auth/organization/set-active", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		setActiveBody = string(b)
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/v2/organisation/org_c/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]string{
			{"id": "prj_z", "name": "Only"},
		}})
	})
	mux.HandleFunc("/v2/organisation/org_c/projects/prj_z/api-credentials",
		func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"apiCredentials": []map[string]any{
				{"id": "cred_9", "apiKey": "ubx_new", "apiSecret": "ubx_sk_new", "revoked": false},
			}})
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"org", "use", "org_c", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}
	if !strings.Contains(setActiveBody, `"organizationId":"2"`) {
		t.Errorf("set-active body=%s, want organizationId 2", setActiveBody)
	}
	cfg, _ := config.Load()
	p := cfg.Profiles["default"]
	if p.OrgID != "org_c" || p.OrgName != "Corp" ||
		p.ProjectID != "prj_z" || p.APISecret != "ubx_sk_new" {
		t.Errorf("profile after org use: %+v", p)
	}
}

func TestOrgUse_MultiProjectOrg_ClearsProjectAndBreadcrumbs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/organization/list", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id": 2, "publicId": "org_c", "name": "Corp", "slug": "corp"}]`))
	})
	mux.HandleFunc("/v1/auth/organization/set-active", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/v2/organisation/org_c/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]string{
			{"id": "prj_1", "name": "One"}, {"id": "prj_2", "name": "Two"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"org", "use", "org_c", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	cfg, _ := config.Load()
	p := cfg.Profiles["default"]
	if p.OrgID != "org_c" || p.ProjectID != "" || p.APISecret != "" {
		t.Errorf("multi-project org use must clear project + credential: %+v", p)
	}
	if !strings.Contains(stdout.String(), "urlbox project list") {
		t.Error("envelope should breadcrumb to `urlbox project list`")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run 'TestOrgList|TestOrgUse' -v`
Expected: FAIL — `unknown command "org"`.

- [ ] **Step 3: Implement**

`internal/cmd/session.go`:

```go
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// loadSessionProfile loads the config and resolves the logged-in profile
// for the management commands (org/project/logout). Precedence for the
// profile name matches auth/login: flag > env > default_profile >
// "default". The named profile must exist AND hold a session token.
// Host precedence: env > profile api_host > default.
func loadSessionProfile(cmd *cobra.Command) (*config.Config, string, config.Profile, string, *output.CLIError) {
	cfg, cliErr := config.LoadOrCLIError()
	if cliErr != nil {
		return nil, "", config.Profile{}, "", cliErr
	}
	flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
	name := loginProfileName(cfg, flagProfile, os.Getenv(config.EnvProfile))
	p, ok := cfg.Profiles[name]
	if !ok || p.SessionToken == "" {
		return nil, "", config.Profile{}, "", output.NewCLIError(
			output.ErrAuth,
			"not logged in",
			"Run `urlbox login` to sign in with your browser. (CI/headless: use `urlbox auth` with a project API secret instead.)",
		)
	}
	host := os.Getenv(config.EnvAPIHost)
	if host == "" {
		host = p.APIHost
	}
	if host == "" {
		host = config.DefaultAPIHost
	}
	return cfg, name, p, host, nil
}
```

`internal/cmd/org.go`:

```go
// internal/cmd/org.go — `urlbox org list` / `urlbox org use <org>`.
//
// Org switching is browser-less: the session token lists memberships,
// set-active re-points the CLI session, and project/credential state is
// re-discovered over /v2 with explicit org paths (never the path-less
// public surface — behavior must not depend on server-side active-org
// state; see the plan's Design decisions).
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newOrgCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "org",
		Short: "List and switch organisations",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(newOrgListCmd(), newOrgUseCmd())
	return c
}

func newOrgListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the organisations you belong to",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profileName, p, host, cliErr := loadSessionProfile(cmd)
			if cliErr != nil {
				return cliErr
			}
			orgs, err := api.NewMgmtClient(host, p.SessionToken).ListOrgs(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(orgs))
			for _, o := range orgs {
				rows = append(rows, map[string]any{
					"id":     o.PublicID,
					"name":   o.Name,
					"slug":   o.Slug,
					"active": o.PublicID == p.OrgID,
				})
			}
			env := output.NewEnvelope(
				"org list",
				map[string]any{"organizations": rows, "profile": profileName},
				fmt.Sprintf("%d organisation(s); active: %s", len(rows), p.OrgName),
				[]output.Breadcrumb{{Action: "switch", Cmd: "urlbox org use <org>"}},
			)
			return writeEnvelope(cmd, env)
		},
	}
}

func newOrgUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <org>",
		Short: "Switch the active organisation (public id, slug, or name)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, profileName, p, host, cliErr := loadSessionProfile(cmd)
			if cliErr != nil {
				return cliErr
			}
			client := api.NewMgmtClient(host, p.SessionToken)
			orgs, err := client.ListOrgs(cmd.Context())
			if err != nil {
				return err
			}
			target, cliErr2 := matchOrg(orgs, args[0])
			if cliErr2 != nil {
				return cliErr2
			}
			if err := client.SetActiveOrg(cmd.Context(), string(target.ID)); err != nil {
				return err
			}
			projects, err := client.ListProjects(cmd.Context(), target.PublicID)
			if err != nil {
				return err
			}

			data := map[string]any{
				"profile":      profileName,
				"organisation": map[string]string{"id": target.PublicID, "name": target.Name},
			}
			msg := "Switched to organisation " + target.Name
			crumbs := []output.Breadcrumb{{Action: "render", Cmd: "urlbox render <url>"}}

			updateErr := config.Update(func(c *config.Config) error {
				prof := c.Profiles[profileName]
				prof.OrgID, prof.OrgName = target.PublicID, target.Name
				if len(projects) == 1 {
					cred, credErr := pickActiveCredential(cmd.Context(), client, target.PublicID, projects[0].ID)
					if credErr != nil {
						return credErr
					}
					prof.ProjectID, prof.ProjectName = projects[0].ID, projects[0].Name
					prof.APIKey, prof.APISecret = cred.APIKey, cred.APISecret
					data["project"] = map[string]string{"id": projects[0].ID, "name": projects[0].Name}
					msg += " — default project " + projects[0].Name
				} else {
					// Ambiguous: clear the project + credential so a stale
					// secret from the previous org can never render against
					// the wrong project silently.
					prof.ProjectID, prof.ProjectName = "", ""
					prof.APIKey, prof.APISecret = "", ""
					msg += fmt.Sprintf(" — %d projects; pick one next", len(projects))
					crumbs = []output.Breadcrumb{{Action: "pick project", Cmd: "urlbox project list"}}
				}
				c.Profiles[profileName] = prof
				return nil
			})
			if updateErr != nil {
				if cli, ok := updateErr.(*output.CLIError); ok {
					return cli
				}
				return output.NewCLIError(output.ErrForbidden, "failed to save config", updateErr.Error())
			}
			return writeEnvelope(cmd, output.NewEnvelope("org use", data, msg, crumbs))
		},
	}
}

// matchOrg finds one org by public id, slug, or exact (case-insensitive)
// name. Ambiguity and misses are usage errors listing the candidates.
func matchOrg(orgs []api.Organization, arg string) (*api.Organization, *output.CLIError) {
	var matches []*api.Organization
	for i := range orgs {
		o := &orgs[i]
		if o.PublicID == arg || o.Slug == arg || strings.EqualFold(o.Name, arg) {
			matches = append(matches, o)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, output.NewCLIError(output.ErrNotFound,
			fmt.Sprintf("no organisation matches %q", arg),
			"Run `urlbox org list` to see your organisations (use the org_… id).")
	default:
		return nil, output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("%q matches more than one organisation", arg),
			"Use the unique org_… public id from `urlbox org list`.")
	}
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newOrgCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestOrgList|TestOrgUse' -v`
Expected: PASS. Then `make surface-snapshot && make ci`.

- [ ] **Step 5: Propose commit**

```
feat(org): urlbox org list / org use — browser-less organisation switching
```

---

### Task 5: `urlbox project list` / `urlbox project use <project>`

**Files:**
- Create: `internal/cmd/project.go`
- Modify: `internal/cmd/root.go` (register `newProjectCmd()`)
- Test: `internal/cmd/project_test.go`

**Interfaces:**
- Consumes: `loadSessionProfile`, `api.MgmtClient.ListProjects/ListCredentials`, `pickActiveCredential`, `writeEnvelope`.

- [ ] **Step 1: Write failing tests**

`internal/cmd/project_test.go` (reuse `seedLoggedInProfile` from Task 4's test file):

```go
func TestProjectList_MarksActiveProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/organisation/org_a/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sess_tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]string{
			{"id": "prj_b", "name": "Website"}, {"id": "prj_c", "name": "Docs"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"project", "list", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env struct {
		Data struct {
			Projects []struct {
				ID     string `json:"id"`
				Active bool   `json:"active"`
			} `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(env.Data.Projects) != 2 {
		t.Fatalf("projects=%+v", env.Data.Projects)
	}
	for _, p := range env.Data.Projects {
		if (p.ID == "prj_b") != p.Active {
			t.Errorf("active flag wrong for %s", p.ID)
		}
	}
}

func TestProjectUse_FetchesCredentialAndWritesProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/organisation/org_a/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]string{
			{"id": "prj_b", "name": "Website"}, {"id": "prj_c", "name": "Docs"},
		}})
	})
	mux.HandleFunc("/v2/organisation/org_a/projects/prj_c/api-credentials",
		func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"apiCredentials": []map[string]any{
				{"id": "cred_r", "apiKey": "ubx_r", "apiSecret": "ubx_sk_r", "revoked": true},
				{"id": "cred_a", "apiKey": "ubx_docs", "apiSecret": "ubx_sk_docs", "revoked": false},
			}})
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"project", "use", "prj_c", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}
	cfg, _ := config.Load()
	p := cfg.Profiles["default"]
	if p.ProjectID != "prj_c" || p.ProjectName != "Docs" ||
		p.APIKey != "ubx_docs" || p.APISecret != "ubx_sk_docs" {
		t.Errorf("profile after project use: %+v", p)
	}
	// Revoked credentials must be skipped.
	if p.APISecret == "ubx_sk_r" {
		t.Error("picked a revoked credential")
	}
}

func TestProjectUse_UnknownProject_NotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/organisation/org_a/projects", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]string{
			{"id": "prj_b", "name": "Website"},
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"project", "use", "prj_nope", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("unknown project must fail")
	}
	if !strings.Contains(stdout.String()+stderr.String(), "project list") {
		t.Error("error should breadcrumb to urlbox project list")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestProject -v`
Expected: FAIL — `unknown command "project"`.

- [ ] **Step 3: Implement `internal/cmd/project.go`**

```go
// internal/cmd/project.go — `urlbox project list` / `urlbox project use`.
//
// Projects are discovered with the session token against the profile's
// stored org (explicit /v2 org path). `use` stores the chosen project's
// render credential into the profile's api_key/api_secret — the exact
// fields `urlbox auth` writes — so the render pipeline needs no changes.
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newProjectCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "project",
		Short: "List and switch projects in the active organisation",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(newProjectListCmd(), newProjectUseCmd())
	return c
}

func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the active organisation's projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profileName, p, host, cliErr := loadSessionProfile(cmd)
			if cliErr != nil {
				return cliErr
			}
			if p.OrgID == "" {
				return output.NewCLIError(output.ErrUsage, "no active organisation on this profile",
					"Run `urlbox org use <org>` first (see `urlbox org list`).")
			}
			projects, err := api.NewMgmtClient(host, p.SessionToken).ListProjects(cmd.Context(), p.OrgID)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(projects))
			for _, pr := range projects {
				rows = append(rows, map[string]any{
					"id": pr.ID, "name": pr.Name, "active": pr.ID == p.ProjectID,
				})
			}
			env := output.NewEnvelope(
				"project list",
				map[string]any{
					"projects":     rows,
					"organisation": map[string]string{"id": p.OrgID, "name": p.OrgName},
					"profile":      profileName,
				},
				fmt.Sprintf("%d project(s) in %s", len(rows), p.OrgName),
				[]output.Breadcrumb{{Action: "switch", Cmd: "urlbox project use <project>"}},
			)
			return writeEnvelope(cmd, env)
		},
	}
}

func newProjectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <project>",
		Short: "Switch the default project (public id or name) and store its credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, profileName, p, host, cliErr := loadSessionProfile(cmd)
			if cliErr != nil {
				return cliErr
			}
			if p.OrgID == "" {
				return output.NewCLIError(output.ErrUsage, "no active organisation on this profile",
					"Run `urlbox org use <org>` first (see `urlbox org list`).")
			}
			client := api.NewMgmtClient(host, p.SessionToken)
			projects, err := client.ListProjects(cmd.Context(), p.OrgID)
			if err != nil {
				return err
			}
			var target *api.MgmtProject
			for i := range projects {
				if projects[i].ID == args[0] || strings.EqualFold(projects[i].Name, args[0]) {
					if target != nil {
						return output.NewCLIError(output.ErrUsage,
							fmt.Sprintf("%q matches more than one project", args[0]),
							"Use the unique prj_… public id from `urlbox project list`.")
					}
					target = &projects[i]
				}
			}
			if target == nil {
				return output.NewCLIError(output.ErrNotFound,
					fmt.Sprintf("no project matches %q in %s", args[0], p.OrgName),
					"Run `urlbox project list` to see the organisation's projects.")
			}
			cred, cliErr2 := pickActiveCredential(cmd.Context(), client, p.OrgID, target.ID)
			if cliErr2 != nil {
				return cliErr2
			}
			if err := config.Update(func(c *config.Config) error {
				prof := c.Profiles[profileName]
				prof.ProjectID, prof.ProjectName = target.ID, target.Name
				prof.APIKey, prof.APISecret = cred.APIKey, cred.APISecret
				c.Profiles[profileName] = prof
				return nil
			}); err != nil {
				return output.NewCLIError(output.ErrForbidden, "failed to save config", err.Error())
			}
			env := output.NewEnvelope(
				"project use",
				map[string]any{
					"profile":       profileName,
					"project":       map[string]string{"id": target.ID, "name": target.Name},
					"api_key":       cred.APIKey,
					"masked_secret": maskSecret(cred.APISecret),
				},
				"Default project is now "+target.Name,
				[]output.Breadcrumb{{Action: "render", Cmd: "urlbox render <url>"}},
			)
			return writeEnvelope(cmd, env)
		},
	}
}
```

Register in `root.go`: `cmd.AddCommand(newProjectCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestProject -v`
Expected: PASS. Then `make surface-snapshot && make ci`.

- [ ] **Step 5: Propose commit**

```
feat(project): urlbox project list / project use — project switching + credential fetch
```

---

### Task 6: `urlbox logout`

**Files:**
- Create: `internal/cmd/logout.go`
- Modify: `internal/cmd/root.go` (register `newLogoutCmd()`)
- Test: `internal/cmd/logout_test.go`

**Interfaces:**
- Consumes: `loadSessionProfile`, `api.MgmtClient.SignOut`, `writeEnvelope`.

- [ ] **Step 1: Write failing tests**

```go
func TestLogout_SignsOutAndClearsLoginFields(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var signedOut atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/sign-out", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sess_tok" {
			signedOut.Store(true)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !signedOut.Load() {
		t.Error("server-side sign-out not attempted")
	}
	cfg, _ := config.Load()
	p := cfg.Profiles["default"]
	if p.SessionToken != "" || p.OrgID != "" || p.OrgName != "" ||
		p.ProjectID != "" || p.ProjectName != "" || p.APIKey != "" || p.APISecret != "" {
		t.Errorf("login-written fields must be cleared: %+v", p)
	}
	if p.APIHost != srv.URL {
		t.Error("api_host must survive logout")
	}
}

func TestLogout_ServerUnreachable_StillClearsLocally(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seedLoggedInProfile(t, "http://127.0.0.1:1") // nothing listens here
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("logout must succeed locally even if revocation fails; exit=%d", exit)
	}
	cfg, _ := config.Load()
	if cfg.Profiles["default"].SessionToken != "" {
		t.Error("session token not cleared")
	}
}

func TestLogout_NotLoggedIn_Fails(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"logout"}, &stdout, &stderr); exit == 0 {
		t.Fatal("logout without a session must fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run TestLogout -v`
Expected: FAIL — `unknown command "logout"`.

- [ ] **Step 3: Implement `internal/cmd/logout.go`**

```go
// internal/cmd/logout.go — `urlbox logout`: revoke the session (best
// effort) and clear every login-written profile field. api_host survives;
// a manually-pasted `urlbox auth` secret on a DIFFERENT profile is
// untouched (logout only edits the resolved profile).
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and remove the stored session + credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profileName, p, host, cliErr := loadSessionProfile(cmd)
			if cliErr != nil {
				return cliErr
			}
			// Best-effort server-side revocation: local cleanup must not be
			// blocked by network state (the token still expires server-side).
			if err := api.NewMgmtClient(host, p.SessionToken).SignOut(cmd.Context()); err != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					"Warning: could not revoke the session server-side; it will expire on its own.")
			}
			if err := config.Update(func(c *config.Config) error {
				prof := c.Profiles[profileName]
				prof.SessionToken = ""
				prof.OrgID, prof.OrgName = "", ""
				prof.ProjectID, prof.ProjectName = "", ""
				prof.APIKey, prof.APISecret = "", ""
				c.Profiles[profileName] = prof
				return nil
			}); err != nil {
				return output.NewCLIError(output.ErrForbidden, "failed to save config", err.Error())
			}
			env := output.NewEnvelope(
				"logout",
				map[string]any{"profile": profileName, "config_path": config.Path()},
				"Logged out — session and stored credentials removed",
				[]output.Breadcrumb{{Action: "login", Cmd: "urlbox login"}},
			)
			return writeEnvelope(cmd, env)
		},
	}
}
```

Register in `root.go`: `cmd.AddCommand(newLogoutCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestLogout -v`
Expected: PASS. Then `make surface-snapshot && make ci`.

- [ ] **Step 5: Propose commit**

```
feat(logout): urlbox logout — revoke session + clear login-managed fields
```

---

### Task 7: Error hints + doctor session check

**Files:**
- Modify: `internal/cmd/auth_preflight.go` (the `requireSecret` hint)
- Modify: `internal/api/http_client.go` (the 401/auth-code hint in `mapStatusToCLIError`)
- Modify: `internal/cmd/doctor.go` (new `checkSession`; `runDoctorChecks` gains the profile)
- Test: `internal/cmd/auth_preflight_test.go`, `internal/cmd/doctor_test.go` (extend)

- [ ] **Step 1: Write failing tests**

```go
func TestRequireSecretHint_MentionsLoginFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("render without credentials must fail")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "urlbox login") {
		t.Errorf("missing-credential hint should lead with urlbox login; got %s", combined)
	}
}

func TestDoctor_SessionCheck_WarnsOnExpiredSession(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/get-session", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`null`)) // better-auth: no live session
	})
	// Keep doctor's other checks green enough: respond 200 everywhere else.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	seedLoggedInProfile(t, srv.URL)

	var stdout, stderr bytes.Buffer
	_ = cmd.Execute([]string{"doctor", "--output-format", "json"}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), `"session"`) {
		t.Fatalf("doctor should include a session check; got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "urlbox login") {
		t.Errorf("expired session should hint urlbox login; got %s", stdout.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cmd/ -run 'TestRequireSecretHint|TestDoctor_SessionCheck' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/cmd/auth_preflight.go` — replace the hint in `requireSecret`:

```go
	return output.NewCLIError(
		output.ErrAuth,
		"no API secret configured",
		"Run `urlbox login` to sign in with your browser (recommended), or `urlbox auth --api-secret <secret>` / URLBOX_API_SECRET for headless use. Get a secret from https://urlbox.com/dashboard/projects.",
	)
```

`internal/api/http_client.go` — the 401/auth-code arm's hint in `mapStatusToCLIError` becomes:

```go
			"Run `urlbox login` to sign in, or `urlbox auth --api-secret <secret>` to set a project secret directly. If you logged in a while ago, the project credential may have been rotated — `urlbox project use <project>` refreshes it.")
```

`internal/cmd/doctor.go` — add a check and thread the profile through. In the `RunE`, after loading `cfg` and building `resolved`, look up the resolved profile (`cfg.Profiles[resolved.Profile]`, zero value when absent) and pass it to `runDoctorChecks(ctx, resolved, profile)`. Append to the checks slice:

```go
		checkSession(ctx, host, profile),
```

```go
// checkSession verifies the login session when one is stored. Better-auth
// returns 200 with a JSON `null` body when the token no longer resolves.
func checkSession(ctx context.Context, host string, profile config.Profile) Check {
	if profile.SessionToken == "" {
		return Check{Name: "session", Status: "ok", Message: "skipped (not logged in)"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/v1/auth/get-session", http.NoBody)
	if err != nil {
		return Check{Name: "session", Status: "fail", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+profile.SessionToken)
	req.Header.Set("User-Agent", api.BuildUserAgent(version.Version))
	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		return Check{Name: "session", Status: "fail", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) != "null":
		return Check{Name: "session", Status: "ok",
			Message: "login session valid (org: " + profile.OrgName + ")"}
	case resp.StatusCode >= 500:
		return Check{Name: "session", Status: "warn",
			Message: fmt.Sprintf("API returned HTTP %d; cannot verify the session", resp.StatusCode)}
	default:
		return Check{Name: "session", Status: "warn",
			Message: "login session expired or revoked",
			Hint:    "Run `urlbox login` to sign in again. Renders still work while the stored project secret stays valid."}
	}
}
```

(Status is `warn`, not `fail`: an expired session doesn't break rendering, which runs on the stored project secret.)

- [ ] **Step 4: Run tests + full suite**

Run: `go test ./internal/cmd/ -run 'TestRequireSecretHint|TestDoctor_SessionCheck' -v`, then `make ci`.
Expected: PASS. Existing hint-assertion tests (e.g. `error_hints_test.go`, auth preflight tests) may assert the old wording — update those assertions to the new copy in the same commit.

- [ ] **Step 5: Propose commit**

```
feat(hints,doctor): steer missing/expired credentials at urlbox login; session check
```

---

### Task 8: Surface snapshot + docs

**Files:**
- Modify: `SURFACE.txt` (via `make surface-snapshot`)
- Modify: `skills/SKILL.md`, `README.md`, `npm/README.md`, `CHANGELOG.md`

- [ ] **Step 1: Snapshot the surface**

Run: `make surface-snapshot && make surface-check`
Expected new entries (with each command's inherited `--agent/--jq/--output-format/--profile` lines):
```
urlbox login
urlbox login --no-open
urlbox logout
urlbox org
urlbox org list
urlbox org use <org>
urlbox project
urlbox project list
urlbox project use <project>
```
No removals (CI enforces).

- [ ] **Step 2: Update docs**

- `skills/SKILL.md`: add a "Signing in" section — `urlbox login` (browser, interactive) vs `urlbox auth`/`URLBOX_API_SECRET` (headless/agents); agents on non-TTY environments must NOT attempt `login`; document `org list/use`, `project list/use`, the `session expired — run urlbox login` error, and that render behavior is unchanged.
- `README.md` + `npm/README.md`: quick-start switches to `urlbox login`; keep the paste-a-secret path documented under "CI / headless".
- `CHANGELOG.md`: entry for the release describing org-scoped login, new commands, and that existing `auth`/env/flag credential sources are unchanged.

- [ ] **Step 3: Full verification**

Run: `make ci`
Expected: all green. Then a manual end-to-end against a staging deployment of the mono `device-auth` branch: `urlbox login` (multi-org user), `urlbox org list`, `urlbox org use`, `urlbox project use`, `urlbox render https://example.com`, `urlbox doctor`, `urlbox logout`.

- [ ] **Step 4: Propose commit**

```
docs(surface,skill,readme): org-scoped login surface + onboarding docs
```

## Self-review checklist (for the plan executor / design reviewer)

- [ ] `urlbox login` is top-level; `urlbox auth` is unmodified (diff `internal/cmd/auth.go` against main → only hint-text changes from Task 7's neighbours, none in auth.go itself).
- [ ] `git diff main -- internal/cmd/render.go internal/api/http_client.go` shows hint-text changes ONLY — no render-path logic changes.
- [ ] `grep -rn "SessionToken" internal/api/http_client.go` returns nothing (render client never carries the session token).
- [ ] All `/v2` calls use explicit `/v2/organisation/{org}/…` paths (grep `"/v2/` in `internal/api/mgmt.go`; only `device-grants/redeem` is path-less).
- [ ] `SURFACE.txt` diff is additions-only.
- [ ] `config set session_token` (and org/project keys) is rejected; `config get session_token` masks without `--reveal`.
