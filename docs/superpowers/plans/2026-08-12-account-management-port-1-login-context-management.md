# Account Management Port — Plan 1: Login, Context, Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring spec slices 1–3 of `docs/superpowers/specs/2026-08-12-account-management-port-design.md` into this repo: session config fields, the existing-command compatibility net, a session-authenticated API client, browser device-flow `login`/`logout`, `whoami`, `orgs`, `projects` (context + CRUD + defaults), and `usage`.

**Architecture:** Behaviour is ported from `/Users/arnoldcubici-jones/Code/work/cli` (branch `feat/device-login`, the behaviour spec); every command is written natively in this repo's idioms (cobra, `output.Envelope`/`CLIError` closed codes, `config.Resolve`/`Update`, `apitest` fakes, `internal/clock`). Five framework-free logic pieces are transplanted with their tests. Slices 4–5 (credential resources, auth sweep) are Plan 2 — do not touch `auth.go`, `doctor.go`, or storage/proxies/llm here.

**Tech Stack:** Go 1.23, cobra, charmbracelet/huh (new dep, picker only), lipgloss (present), httptest via `internal/api/apitest`, `internal/clock`.

## Global Constraints

- TDD from commit one: failing test → minimal implementation → green — every task, no exceptions.
- `make ci` (fmt-check, lint, test, build, surface-check) green at the end of every task; `make surface-snapshot` + commit `SURFACE.txt` alongside any surface change.
- stdout is for structured data only; stderr is for human messages — never mixed.
- Errors only from the closed set in `internal/output/errors.go`; "not logged in"/"session expired" → `output.ErrAuth` with hint `Run \`urlbox login\``.
- Every command writes envelopes via `writeEnvelope`/`writeEnvelopeWithQuietData` (`internal/cmd/config.go:614-650`); breadcrumbs point at the natural next command.
- gofumpt formatting (`make fmt`); zero code comments except where a constraint cannot be expressed in code.
- Profiles stay undocumented: no new help text mentions profiles beyond the inherited `--profile` flag.
- NO commits during tasks: mark the task complete and move on — all work accumulates uncommitted for a single review-gated commit at the end (this overrides the usual per-task commit convention).
- Do not modify: `auth.go`, `doctor.go`, `link.go`, `dashboard.go`, `skill.go`, `commands.go`, `upgrade.go`, `render*.go`, `screenshot.go`, `pdf.go`, `video.go`, `status.go` (Plan 2 owns the auth sweep; render-family behaviour must not change).

**Source-repo shorthand:** `SRC = /Users/arnoldcubici-jones/Code/work/cli` (read-only reference). Target repo root is this repo.

---

### Task 1: Profile session fields

**Files:**
- Modify: `internal/config/profile.go`
- Modify: `internal/config/resolve.go` (extract profile-selection helper)
- Test: `internal/config/profile_session_test.go` (create)

**Interfaces:**
- Produces: `Profile.SessionToken`, `Profile.ActiveOrg`, `Profile.ActiveProject` (JSON `session_token`, `active_org`, `active_project`); `config.ProfileName(flagProfile, envProfile string, overlay *RepoOverlay, cfg *Config) string` — the profile-selection chain (flag → repo overlay → env → default_profile → "default") reused by Resolve and by every session command in later tasks.

- [ ] **Step 1: Write the failing test**

Create `internal/config/profile_session_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSessionFieldsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := Save(&Config{
		DefaultProfile: "default",
		Profiles: map[string]Profile{"default": {
			APISecret:     "sk_live_1234567890",
			SessionToken:  "sess_tok_abcdef123456",
			ActiveOrg:     "org_01hxyz",
			ActiveProject: "proj_01habc",
		}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := loaded.Profiles["default"]
	if p.SessionToken != "sess_tok_abcdef123456" {
		t.Fatalf("session token dropped on roundtrip: %+v", p)
	}
	if p.ActiveOrg != "org_01hxyz" || p.ActiveProject != "proj_01habc" {
		t.Fatalf("active org/project dropped: %+v", p)
	}
	info, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", info.Mode().Perm())
	}
	if filepath.Dir(Path()) == "" {
		t.Fatal("empty config dir")
	}
}

func TestProfileIsEmptyCountsSessionFields(t *testing.T) {
	if (Profile{SessionToken: "tok"}).IsEmpty() {
		t.Fatal("profile with only a session token must not be IsEmpty")
	}
	if !(Profile{}).IsEmpty() {
		t.Fatal("zero profile must be IsEmpty")
	}
}

func TestProfileNameSelectionChain(t *testing.T) {
	cfg := &Config{DefaultProfile: "team", Profiles: map[string]Profile{"team": {}}}
	if got := ProfileName("flagged", "enved", &RepoOverlay{Profile: "repo"}, cfg); got != "flagged" {
		t.Fatalf("flag must win, got %q", got)
	}
	if got := ProfileName("", "enved", &RepoOverlay{Profile: "repo"}, cfg); got != "repo" {
		t.Fatalf("repo overlay must beat env, got %q", got)
	}
	if got := ProfileName("", "enved", nil, cfg); got != "enved" {
		t.Fatalf("env must beat default_profile, got %q", got)
	}
	if got := ProfileName("", "", nil, cfg); got != "team" {
		t.Fatalf("default_profile must beat literal default, got %q", got)
	}
	if got := ProfileName("", "", nil, &Config{}); got != "default" {
		t.Fatalf("fallback must be default, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestProfileSession|TestProfileIsEmpty|TestProfileName' -v`
Expected: FAIL — `p.SessionToken undefined`, `undefined: ProfileName` (compile errors are the failing state for new fields).

- [ ] **Step 3: Write minimal implementation**

In `internal/config/profile.go`, replace the whole file with:

```go
package config

type Profile struct {
	APIKey        string `json:"api_key,omitempty"`
	APISecret     string `json:"api_secret,omitempty"`
	APIHost       string `json:"api_host,omitempty"`
	SessionToken  string `json:"session_token,omitempty"`
	ActiveOrg     string `json:"active_org,omitempty"`
	ActiveProject string `json:"active_project,omitempty"`
}

func (p Profile) IsEmpty() bool {
	return p.APIKey == "" && p.APISecret == "" && p.APIHost == "" &&
		p.SessionToken == "" && p.ActiveOrg == "" && p.ActiveProject == ""
}
```

In `internal/config/resolve.go`, add after the `Source` type:

```go
func ProfileName(flagProfile, envProfile string, overlay *RepoOverlay, cfg *Config) string {
	switch {
	case flagProfile != "":
		return flagProfile
	case overlay != nil && overlay.Profile != "":
		return overlay.Profile
	case envProfile != "":
		return envProfile
	case cfg != nil && cfg.DefaultProfile != "":
		return cfg.DefaultProfile
	default:
		return "default"
	}
}
```

and replace the profile-selection `switch` inside `Resolve` (the one assigning `r.Profile, r.Source.Profile`) with:

```go
	r.Profile = ProfileName(opts.FlagProfile, opts.EnvProfile, opts.RepoOverlay, opts.Config)
	switch {
	case opts.FlagProfile != "":
		r.Source.Profile = "flag"
	case opts.RepoOverlay != nil && opts.RepoOverlay.Profile != "":
		r.Source.Profile = "repo"
	case opts.EnvProfile != "":
		r.Source.Profile = "env"
	case opts.Config != nil && opts.Config.DefaultProfile != "":
		r.Source.Profile = "default_profile"
	default:
		r.Source.Profile = "default"
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS, including every pre-existing config test (`resolve_test.go` must stay green — the extraction must not change selection behaviour).

- [ ] **Step 5: Run `make ci`**

Expected: green. `surface-check` unchanged (no command surface touched).

- [ ] **Step 6: Mark task complete — NO commit** (work accumulates for one review-gated commit at the end).

---

### Task 2: Existing-command compatibility suite

**Files:**
- Test: `internal/cmd/compat_session_config_test.go` (create)

**Interfaces:**
- Consumes: `cmd.Execute(args []string, stdout, stderr io.Writer) int` (root.go:36), `apitest.New`/`SuccessJSON`.
- Produces: `writeCompatConfig(t, dir, withSession bool)` and `compatCases()` used again by Task 16's verification step.

- [ ] **Step 1: Write the failing-then-guarding test**

Create `internal/cmd/compat_session_config_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const compatSecret = "sk_test_abcdefgh12345678"

func writeCompatConfig(t *testing.T, dir string, withSession bool) {
	t.Helper()
	profile := map[string]string{
		"api_key":    "pk_test_key",
		"api_secret": compatSecret,
	}
	if withSession {
		profile["session_token"] = "sess_tok_compat_123456"
		profile["active_org"] = "org_compat"
		profile["active_project"] = "proj_compat"
	}
	cfg := map[string]any{
		"default_profile": "default",
		"profiles":        map[string]any{"default": profile},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "urlbox"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "urlbox", "config.json"), b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

type compatCase struct {
	name string
	args []string
}

func compatCases() []compatCase {
	return []compatCase{
		{"render dry-run", []string{"render", "https://example.com", "--dry-run", "--output-format", "json"}},
		{"screenshot dry-run", []string{"screenshot", "https://example.com", "--dry-run", "--output-format", "json"}},
		{"pdf dry-run", []string{"pdf", "https://example.com", "--dry-run", "--output-format", "json"}},
		{"render curl", []string{"render", "https://example.com", "--curl", "--output-format", "json"}},
		{"link", []string{"link", "https://example.com", "--output-format", "json"}},
		{"config get secret", []string{"config", "get", "api_secret", "--output-format", "json"}},
		{"config path", []string{"config", "path", "--output-format", "quiet"}},
		{"config profile list", []string{"config", "profile", "list", "--output-format", "json"}},
		{"schema", []string{"schema", "render", "--output-format", "json"}},
		{"commands", []string{"commands", "--output-format", "json"}},
		{"version", []string{"version"}},
	}
}

func runCompat(t *testing.T, args []string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestSessionFieldsDoNotChangeExistingCommands(t *testing.T) {
	for _, tc := range compatCases() {
		t.Run(tc.name, func(t *testing.T) {
			legacyDir := t.TempDir()
			writeCompatConfig(t, legacyDir, false)
			t.Setenv("XDG_CONFIG_HOME", legacyDir)
			legacyOut, legacyErr, legacyCode := runCompat(t, tc.args)

			sessionDir := t.TempDir()
			writeCompatConfig(t, sessionDir, true)
			t.Setenv("XDG_CONFIG_HOME", sessionDir)
			sessionOut, sessionErr, sessionCode := runCompat(t, tc.args)

			if legacyCode != sessionCode {
				t.Fatalf("exit code changed: legacy=%d session=%d\nlegacy stderr: %s\nsession stderr: %s",
					legacyCode, sessionCode, legacyErr, sessionErr)
			}
			normalize := func(s, dir string) string {
				return bytes.NewBufferString(s).String()
			}
			if normalize(legacyOut, legacyDir) != normalize(sessionOut, sessionDir) &&
				tc.name != "config path" {
				t.Fatalf("stdout changed:\nlegacy:  %s\nsession: %s", legacyOut, sessionOut)
			}
		})
	}
}

func TestConfigSetPreservesSessionFields(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	_, stderr, code := runCompat(t, []string{"config", "set", "api_host", "https://api.urlbox.com"})
	if code != 0 {
		t.Fatalf("config set failed (%d): %s", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Profiles["default"]["session_token"] != "sess_tok_compat_123456" {
		t.Fatalf("config set dropped session_token: %s", b)
	}
}
```

- [ ] **Step 2: Run the suite**

Run: `go test ./internal/cmd/ -run 'TestSessionFields|TestConfigSetPreserves' -v`
Expected: PASS on a correct Task 1 (`config set` goes through `config.Update` → `Save`, which now round-trips the struct fields). If `TestConfigSetPreservesSessionFields` FAILS, Task 1's struct fields are wrong — fix there, not here. `config path` differs by tempdir path — the case is exempted from stdout comparison but still asserts equal exit codes.

- [ ] **Step 3: Adjust for repo reality**

The `link` and `config get` cases may legitimately differ from the exact args above if flags differ in this repo — before finalising, run each case manually (`go run ./cmd/urlbox <args>`) and fix ONLY the test's argument lists to the repo's real surface (never relax the equality assertions).

- [ ] **Step 4: Run `make ci`**

Expected: green.

- [ ] **Step 5: Mark task complete — NO commit.**

---

### Task 3: Session-authenticated API client

**Files:**
- Create: `internal/api/session_client.go`
- Test: `internal/api/session_client_test.go` (create)

**Interfaces:**
- Consumes: `RetryDo`, `DefaultRetryConfig`, `mapStatusToCLIError`, `BuildUserAgent` (all in `internal/api`), `output.CLIError`.
- Produces:
  - `api.NewSessionClient(baseURL, token string) *SessionClient`
  - `(*SessionClient) GetJSON(ctx context.Context, path string, out any) error`
  - `(*SessionClient) PostJSON(ctx context.Context, path string, body, out any) error`
  - `(*SessionClient) PatchJSON(ctx context.Context, path string, body, out any) error`
  - `(*SessionClient) DeleteJSON(ctx context.Context, path string, out any) error`
  - `(*SessionClient) DoRaw(ctx context.Context, method, path string, body any) (int, map[string]any, error)` — no error-code mapping; the device-poll loop (Task 4) reads RFC error strings from the raw body.
  - `type SessionAPI interface { GetJSON(ctx context.Context, path string, out any) error; PostJSON(ctx context.Context, path string, body, out any) error; PatchJSON(ctx context.Context, path string, body, out any) error; DeleteJSON(ctx context.Context, path string, out any) error }` — every command and transplant tests against this, not the concrete client.

- [ ] **Step 1: Write the failing test**

Create `internal/api/session_client_test.go`:

```go
package api

import (
	"context"
	"errors"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestSessionClientSendsBearerTokenAndUserAgent(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "sess_tok_123")
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/v1/auth/get-session", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer sess_tok_123" {
		t.Fatalf("auth header = %q", got)
	}
	if got := reqs[0].Header.Get("User-Agent"); got == "" {
		t.Fatal("missing User-Agent")
	}
	if reqs[0].Path != "/v1/auth/get-session" {
		t.Fatalf("path = %q", reqs[0].Path)
	}
}

func Test401MapsToAuthWithLoginHint(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{Status: 401, Body: `{"error":{"message":"unauthorized"}}`})
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "sess_expired")
	err := c.GetJSON(context.Background(), "/v2/usage", nil)
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("want CLIError, got %T %v", err, err)
	}
	if cli.Code != output.ErrAuth {
		t.Fatalf("code = %q, want auth", cli.Code)
	}
	if cli.Hint == "" || cli.Hint != "Run `urlbox login` — your session is missing or expired." {
		t.Fatalf("hint = %q", cli.Hint)
	}
}

func TestDoRawReturnsBodyWithoutMapping(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{Status: 400, Body: `{"error":"authorization_pending"}`})
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "")
	status, data, err := c.DoRaw(context.Background(), "POST", "/v1/auth/device/token", map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("DoRaw transport error: %v", err)
	}
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	if data["error"] != "authorization_pending" {
		t.Fatalf("data = %#v", data)
	}
}

func TestSessionClientRetries429(t *testing.T) {
	srv := apitest.New(apitest.RetryAfterSeconds(0), apitest.SuccessJSON(`{"fine":true}`))
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "tok")
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/v2/projects", &out); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if len(srv.Requests()) != 2 {
		t.Fatalf("want 2 requests (retry), got %d", len(srv.Requests()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/ -run 'TestSessionClient|Test401Maps|TestDoRaw' -v`
Expected: FAIL — `undefined: NewSessionClient`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/api/session_client.go`:

```go
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

type SessionAPI interface {
	GetJSON(ctx context.Context, path string, out any) error
	PostJSON(ctx context.Context, path string, body, out any) error
	PatchJSON(ctx context.Context, path string, body, out any) error
	DeleteJSON(ctx context.Context, path string, out any) error
}

type SessionClient struct {
	BaseURL   string
	Token     string
	UserAgent string
	Timeout   time.Duration
	Retry     RetryConfig
	HTTP      *http.Client
}

func NewSessionClient(baseURL, token string) *SessionClient {
	timeout := 30 * time.Second
	return &SessionClient{
		BaseURL:   baseURL,
		Token:     token,
		UserAgent: BuildUserAgent(version.Version),
		Timeout:   timeout,
		Retry:     DefaultRetryConfig(),
		HTTP: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

func (c *SessionClient) GetJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

func (c *SessionClient) PostJSON(ctx context.Context, path string, body, out any) error {
	if body == nil {
		body = map[string]string{}
	}
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

func (c *SessionClient) PatchJSON(ctx context.Context, path string, body, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, body, out)
}

func (c *SessionClient) DeleteJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, out)
}

func (c *SessionClient) doJSON(ctx context.Context, method, path string, body, out any) error {
	resp, respBody, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		cli := mapStatusToCLIError(resp, respBody)
		if cli.Code == output.ErrAuth {
			return output.NewCLIError(
				output.ErrAuth,
				cli.Message,
				"Run `urlbox login` — your session is missing or expired.",
			)
		}
		return cli
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return output.NewCLIError(output.ErrServer, "failed to parse API response", err.Error())
	}
	return nil
}

func (c *SessionClient) DoRaw(ctx context.Context, method, path string, body any) (int, map[string]any, error) {
	resp, respBody, err := c.send(ctx, method, path, body)
	if err != nil {
		return 0, nil, err
	}
	data := map[string]any{}
	if len(respBody) > 0 {
		if jerr := json.Unmarshal(respBody, &data); jerr != nil {
			return resp.StatusCode, map[string]any{}, nil
		}
	}
	return resp.StatusCode, data, nil
}

func (c *SessionClient) send(ctx context.Context, method, path string, body any) (*http.Response, []byte, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, output.NewCLIError(output.ErrUsage, "failed to encode request body", err.Error())
		}
		bodyBytes = b
	}
	send := func() (*http.Response, error) {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
		if err != nil {
			return nil, err
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.UserAgent)
		return c.HTTP.Do(req)
	}
	resp, err := RetryDo(ctx, c.Retry, send)
	if err != nil {
		code := output.ErrNetwork
		if errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			code = output.ErrTimeout
		}
		return nil, nil, output.NewCLIError(code, err.Error(),
			"Check your internet connection and the API host (URLBOX_API_HOST).")
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, nil, output.NewCLIError(output.ErrNetwork, readErr.Error(), "Check your internet connection.")
	}
	return resp, respBody, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/api/ -v`
Expected: PASS (new tests plus all pre-existing api tests). If `apitest.ScriptedResponse` literal field names differ, match `internal/api/apitest/server.go:46` exactly.

- [ ] **Step 5: Run `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 4: Transplant — device-poll state machine

**Files:**
- Create: `internal/deviceauth/poll.go`
- Test: `internal/deviceauth/poll_test.go` (create)
- Source reference: `SRC/internal/cmd/login.go:149-194` (`pollForToken`, `codeFromError`)

**Interfaces:**
- Consumes: `clock.Clock` (`internal/clock`), `output.CLIError`.
- Produces:
  - `type Exchange struct { AccessToken string; RFCCode string; Err error }`
  - `deviceauth.Poll(clk clock.Clock, interval, expiresIn int, exchange func() Exchange) (string, *output.CLIError)` — Task 9's login command calls this with a closure over `SessionClient.DoRaw`.

Behaviour ported verbatim from source: interval floor 5s; sleep-then-poll; `authorization_pending` continues; `slow_down` adds 5s to the interval; `access_denied` → auth error "Login denied."; `expired_token` or deadline → auth error "Code expired — run `urlbox login` again."; transport errors from exchange continue polling until the deadline (parity: source ignored non-RFC errors).

- [ ] **Step 1: Write the failing test**

Create `internal/deviceauth/poll_test.go`:

```go
package deviceauth

import (
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func runPoll(t *testing.T, interval, expiresIn int, script []Exchange) (string, *output.CLIError, *clock.FakeClock) {
	t.Helper()
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	i := 0
	exchange := func() Exchange {
		if i >= len(script) {
			t.Fatalf("poll exceeded script (%d calls)", i)
		}
		e := script[i]
		i++
		return e
	}
	type result struct {
		token string
		cli   *output.CLIError
	}
	done := make(chan result, 1)
	go func() {
		tok, cli := Poll(fc, interval, expiresIn, exchange)
		done <- result{tok, cli}
	}()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case r := <-done:
			return r.token, r.cli, fc
		case <-deadline:
			t.Fatal("poll did not finish")
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(10 * time.Second)
			}
		}
	}
}

func TestPollSucceedsAfterPending(t *testing.T) {
	tok, cli, _ := runPoll(t, 5, 300, []Exchange{
		{RFCCode: "authorization_pending"},
		{RFCCode: "authorization_pending"},
		{AccessToken: "sess_tok_win"},
	})
	if cli != nil {
		t.Fatalf("unexpected error: %v", cli)
	}
	if tok != "sess_tok_win" {
		t.Fatalf("token = %q", tok)
	}
}

func TestPollSlowDownBacksOff(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	calls := 0
	var gaps []time.Duration
	last := fc.Now()
	exchange := func() Exchange {
		gaps = append(gaps, fc.Since(last))
		last = fc.Now()
		calls++
		if calls == 1 {
			return Exchange{RFCCode: "slow_down"}
		}
		return Exchange{AccessToken: "tok"}
	}
	done := make(chan struct{})
	go func() {
		_, _ = Poll(fc, 5, 300, exchange)
		close(done)
	}()
	for {
		select {
		case <-done:
			if gaps[0] != 5*time.Second {
				t.Fatalf("first gap = %v, want 5s", gaps[0])
			}
			if gaps[1] != 10*time.Second {
				t.Fatalf("post-slow_down gap = %v, want 10s", gaps[1])
			}
			return
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(1 * time.Second)
			}
		}
	}
}

func TestPollDeniedStopsWithAuthError(t *testing.T) {
	_, cli, _ := runPoll(t, 5, 300, []Exchange{{RFCCode: "access_denied"}})
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth error, got %v", cli)
	}
	if cli.Message != "Login denied." {
		t.Fatalf("message = %q", cli.Message)
	}
}

func TestPollExpiredTokenStops(t *testing.T) {
	_, cli, _ := runPoll(t, 5, 300, []Exchange{{RFCCode: "expired_token"}})
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth error, got %v", cli)
	}
}

func TestPollDeadlineExpires(t *testing.T) {
	script := make([]Exchange, 4)
	for i := range script {
		script[i] = Exchange{RFCCode: "authorization_pending"}
	}
	_, cli, _ := runPoll(t, 5, 12, script)
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth expiry error, got %v", cli)
	}
}

func TestPollIntervalFloor(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	start := fc.Now()
	var firstGap time.Duration
	done := make(chan struct{})
	go func() {
		_, _ = Poll(fc, 0, 60, func() Exchange {
			firstGap = fc.Since(start)
			return Exchange{AccessToken: "tok"}
		})
		close(done)
	}()
	for {
		select {
		case <-done:
			if firstGap != 5*time.Second {
				t.Fatalf("gap with interval=0 is %v, want 5s floor", firstGap)
			}
			return
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(1 * time.Second)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/deviceauth/ -v`
Expected: FAIL — package does not exist / `undefined: Poll`.

- [ ] **Step 3: Write the implementation (transplanted logic, house error types)**

Create `internal/deviceauth/poll.go`:

```go
package deviceauth

import (
	"time"

	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type Exchange struct {
	AccessToken string
	RFCCode     string
	Err         error
}

func Poll(clk clock.Clock, interval, expiresIn int, exchange func() Exchange) (string, *output.CLIError) {
	if interval <= 0 {
		interval = 5
	}
	deadline := clk.Now().Add(time.Duration(expiresIn) * time.Second)
	for clk.Now().Before(deadline) {
		clk.Sleep(time.Duration(interval) * time.Second)
		e := exchange()
		if e.Err == nil && e.AccessToken != "" {
			return e.AccessToken, nil
		}
		switch e.RFCCode {
		case "authorization_pending", "":
			continue
		case "slow_down":
			interval += 5
			continue
		case "access_denied":
			return "", output.NewCLIError(output.ErrAuth, "Login denied.", "Approve the request in your browser, then run `urlbox login` again.")
		case "expired_token":
			return "", output.NewCLIError(output.ErrAuth, "Code expired — run `urlbox login` again.", "Device codes are short-lived; restart the login to get a fresh code.")
		}
	}
	return "", output.NewCLIError(output.ErrAuth, "Code expired — run `urlbox login` again.", "Device codes are short-lived; restart the login to get a fresh code.")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/deviceauth/ -race -v`
Expected: PASS. The `-race` flag is mandatory here (goroutine + FakeClock).

- [ ] **Step 5: Run `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 5: Transplant — name-or-id resolution + list helpers

(Ordered before the org/project resolution transplant because that transplant consumes `nameID`/`resolveNameOrID`/`fetchList` — a hard dependency; the spec's five-transplant list is otherwise unchanged.)

**Files:**
- Create: `internal/cmd/nameid.go`
- Test: `internal/cmd/nameid_test.go` (create)
- Source reference: `SRC/internal/cmd/credentials.go:80-115,174-187,240-256`

**Interfaces:**
- Consumes: `api.SessionAPI` (Task 3), `output.CLIError`.
- Produces (package `cmd`):
  - `type nameID struct { ID, Name string }`
  - `resolveNameOrID(arg, prefix string, rows []nameID, kind string) (nameID, *output.CLIError)` — prefix-id passthrough, case-insensitive name match, ambiguity → `ErrValidation` listing candidate ids in the hint, no match → `ErrNotFound`.
  - `toNameIDs(items []map[string]any) []nameID`
  - `fetchList(ctx context.Context, client api.SessionAPI, path, key string) ([]map[string]any, error)`
  - `valueOrEmpty(v any) string`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/nameid_test.go`:

```go
package cmd

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

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

func TestToNameIDs(t *testing.T) {
	rows := toNameIDs([]map[string]any{{"id": "proj_1", "name": "A"}, {"id": 7, "name": nil}})
	if rows[0] != (nameID{ID: "proj_1", Name: "A"}) {
		t.Fatalf("row0 = %+v", rows[0])
	}
	if rows[1] != (nameID{}) {
		t.Fatalf("non-string fields must map to empty, got %+v", rows[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestResolveNameOrID|TestToNameIDs' -v`
Expected: FAIL — `undefined: nameID`.

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/nameid.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type nameID struct {
	ID   string
	Name string
}

func valueOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

func resolveNameOrID(arg, prefix string, rows []nameID, kind string) (nameID, *output.CLIError) {
	if strings.HasPrefix(arg, prefix) {
		for _, r := range rows {
			if r.ID == arg {
				return r, nil
			}
		}
		return nameID{ID: arg}, nil
	}
	var matches []nameID
	for _, r := range rows {
		if strings.EqualFold(r.Name, arg) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nameID{}, output.NewCLIError(
			output.ErrNotFound,
			fmt.Sprintf("no %s matching %q", kind, arg),
			fmt.Sprintf("List them with `urlbox %ss list`, then pass a name or id.", kind),
		)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nameID{}, output.NewCLIError(
			output.ErrValidation,
			fmt.Sprintf("%q matches multiple %ss", arg, kind),
			"Use one of the ids instead: "+strings.Join(ids, ", "),
		)
	}
}

func toNameIDs(items []map[string]any) []nameID {
	rows := make([]nameID, len(items))
	for i, m := range items {
		rows[i] = nameID{ID: valueOrEmpty(m["id"]), Name: valueOrEmpty(m["name"])}
	}
	return rows
}

func fetchList(ctx context.Context, client api.SessionAPI, path, key string) ([]map[string]any, error) {
	var resp map[string]any
	if err := client.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	items, _ := resp[key].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestResolveNameOrID|TestToNameIDs' -v`
Expected: PASS.

- [ ] **Step 5: Run `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 6: Transplant — login org/project resolution

**Files:**
- Create: `internal/cmd/login_resolve.go`
- Test: `internal/cmd/login_resolve_test.go` (create)
- Source reference: `SRC/internal/cmd/login.go:227-381`, `SRC/internal/cmd/whoami.go:60-73`

**Interfaces:**
- Consumes: `api.SessionAPI`, `nameID`, `resolveNameOrID`, `fetchList`, `toNameIDs` (Task 5).
- Produces (package `cmd`):
  - `type pickFunc func(label string, options []string, active int) (int, error)` — Task 7's `prompt.SelectOne` satisfies it in production; tests inject stubs.
  - `type orgListRow struct { ID, Name, PublicID string }` (JSON `id`, `name`, `publicId`)
  - `type sessionResponse struct { User struct{ Email string }; Session struct{ ActiveOrganizationID, ActiveOrganizationPublicID string } }` (JSON tags as in source: `user.email`, `session.activeOrganizationId`, `session.activeOrganizationPublicId`)
  - `type resolvedOrg struct { publicID, name, email string }`
  - `matchOrg(orgs []orgListRow, arg string) (orgListRow, bool)`
  - `resolveActiveOrg(ctx context.Context, client api.SessionAPI, orgFlag string, pick pickFunc) (resolvedOrg, *output.CLIError)`
  - `resolveActiveProject(ctx context.Context, client api.SessionAPI, projectFlag string, pick pickFunc) (nameID, *output.CLIError)`
  - `activeOrgName(ctx context.Context, client api.SessionAPI) string`
  - `errNotInteractivePick = errors.New("not an interactive terminal")` — sentinel a pickFunc returns off-TTY; both resolvers map it to `ErrUsage` naming the bypass flag.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/login_resolve_test.go`:

```go
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
func (f *fakeSession) DeleteJSON(_ context.Context, path string, out any) error     { return nil }

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
	got, cli := resolveActiveProject(context.Background(), zero, "", neverPick)
	if cli != nil || got.ID != "" {
		t.Fatalf("zero projects: got %+v %v", got, cli)
	}

	one := &fakeSession{gets: map[string]string{"/v2/projects": `{"projects":[{"id":"proj_1","name":"Only"}]}`}}
	got, cli = resolveActiveProject(context.Background(), one, "", neverPick)
	if cli != nil || got.ID != "proj_1" {
		t.Fatalf("one project: got %+v %v", got, cli)
	}

	many := &fakeSession{gets: map[string]string{"/v2/projects": `{"projects":[{"id":"proj_1","name":"A"},{"id":"proj_2","name":"B"}]}`}}
	got, cli = resolveActiveProject(context.Background(), many, "", func(_ string, _ []string, _ int) (int, error) { return 1, nil })
	if cli != nil || got.ID != "proj_2" {
		t.Fatalf("picker path: got %+v %v", got, cli)
	}

	got, cli = resolveActiveProject(context.Background(), many, "b", neverPick)
	if cli != nil || got.ID != "proj_2" {
		t.Fatalf("flag path: got %+v %v", got, cli)
	}

	_, cli = resolveActiveProject(context.Background(), many, "", notInteractive)
	if cli == nil || cli.Code != output.ErrUsage || !strings.Contains(cli.Hint, "--project") {
		t.Fatalf("want usage error naming --project, got %v", cli)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestResolveActiveOrg|TestResolveActiveProject|TestActiveOrgName' -v`
Expected: FAIL — `undefined: resolveActiveOrg`.

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/login_resolve.go`:

```go
package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type pickFunc func(label string, options []string, active int) (int, error)

var errNotInteractivePick = errors.New("not an interactive terminal")

type orgListRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PublicID string `json:"publicId"`
}

type sessionResponse struct {
	User struct {
		Email string `json:"email"`
	} `json:"user"`
	Session struct {
		ActiveOrganizationID       string `json:"activeOrganizationId"`
		ActiveOrganizationPublicID string `json:"activeOrganizationPublicId"`
	} `json:"session"`
}

type resolvedOrg struct {
	publicID string
	name     string
	email    string
}

func matchOrg(orgs []orgListRow, arg string) (orgListRow, bool) {
	for _, o := range orgs {
		if o.PublicID == arg || o.ID == arg || strings.EqualFold(o.Name, arg) {
			return o, true
		}
	}
	return orgListRow{}, false
}

func resolveActiveOrg(ctx context.Context, client api.SessionAPI, orgFlag string, pick pickFunc) (resolvedOrg, *output.CLIError) {
	var orgs []orgListRow
	if err := client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	if len(orgs) == 0 {
		return resolvedOrg{}, output.NewCLIError(output.ErrNotFound,
			"your account has no organisation",
			"Create one in the dashboard at https://urlbox.com/dashboard, then run `urlbox login` again.")
	}
	chosen := orgs[0]
	if orgFlag != "" {
		match, ok := matchOrg(orgs, orgFlag)
		if !ok {
			return resolvedOrg{}, output.NewCLIError(output.ErrNotFound,
				fmt.Sprintf("no organisation matching %q", orgFlag),
				"Run `urlbox orgs list` to see your organisations.")
		}
		chosen = match
	} else if len(orgs) > 1 {
		names := make([]string, len(orgs))
		for i, o := range orgs {
			names[i] = o.Name
		}
		idx, err := pick("Select an organisation:", names, -1)
		if err != nil {
			if errors.Is(err, errNotInteractivePick) {
				return resolvedOrg{}, output.NewCLIError(output.ErrUsage,
					"multiple organisations and no interactive terminal",
					"Pass --org <name-or-id> to choose one non-interactively.")
			}
			return resolvedOrg{}, output.NewCLIError(output.ErrUsage, err.Error(), "")
		}
		chosen = orgs[idx]
	}
	if err := client.PostJSON(ctx, "/v1/auth/organization/set-active",
		map[string]string{"organizationId": chosen.ID}, nil); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	var session sessionResponse
	if err := client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	return resolvedOrg{
		publicID: session.Session.ActiveOrganizationPublicID,
		name:     chosen.Name,
		email:    session.User.Email,
	}, nil
}

func resolveActiveProject(ctx context.Context, client api.SessionAPI, projectFlag string, pick pickFunc) (nameID, *output.CLIError) {
	projects, err := fetchList(ctx, client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, asCLIError(err)
	}
	rows := toNameIDs(projects)
	if len(rows) == 0 {
		return nameID{}, nil
	}
	if projectFlag != "" {
		return resolveNameOrID(projectFlag, "proj_", rows, "project")
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	idx, perr := pick("Select the active project (used by render):", names, -1)
	if perr != nil {
		if errors.Is(perr, errNotInteractivePick) {
			return nameID{}, output.NewCLIError(output.ErrUsage,
				"multiple projects and no interactive terminal",
				"Pass --project <name-or-id>, or run `urlbox projects select` later.")
		}
		return nameID{}, output.NewCLIError(output.ErrUsage, perr.Error(), "")
	}
	return rows[idx], nil
}

func activeOrgName(ctx context.Context, client api.SessionAPI) string {
	var session sessionResponse
	if err := client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return "(none)"
	}
	activeID := session.Session.ActiveOrganizationID
	if activeID == "" {
		return "(none)"
	}
	var orgs []orgListRow
	if err := client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err == nil {
		for _, o := range orgs {
			if o.ID == activeID {
				return o.Name
			}
		}
	}
	if session.Session.ActiveOrganizationPublicID != "" {
		return session.Session.ActiveOrganizationPublicID
	}
	return "(none)"
}

func asCLIError(err error) *output.CLIError {
	var cli *output.CLIError
	if errors.As(err, &cli) {
		return cli
	}
	return output.NewCLIError(output.ErrServer, err.Error(), "")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestResolveActiveOrg|TestResolveActiveProject|TestActiveOrgName' -v`
Expected: PASS.

- [ ] **Step 5: Run `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 7: Picker component (internal/prompt)

**Files:**
- Create: `internal/prompt/prompt.go`
- Test: `internal/prompt/prompt_test.go` (create)
- Modify: `go.mod`/`go.sum` (add `github.com/charmbracelet/huh`)
- Source reference: `SRC/internal/prompt/prompt.go` (behaviour parity; theme adapted to this repo)

**Interfaces:**
- Produces (package `prompt`):
  - `prompt.ErrNotInteractive` (sentinel error)
  - `prompt.SelectOne(label string, options []string, active int) (int, error)` — satisfies Task 6's `pickFunc` via a thin adapter in the login command (Task 9).
  - `prompt.TypeToConfirm(title, expected string) error` — used by `projects delete` (Task 15) and Plan 2's credential deletes.
- House rules honoured: draws via huh (stderr-backed), colors from huh's charm theme (lipgloss/termenv → `NO_COLOR` respected automatically), non-TTY → `ErrNotInteractive`, never hangs.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/charmbracelet/huh@latest && go mod tidy`
Expected: `go.mod` gains `github.com/charmbracelet/huh`; build stays green (`go build ./...`).

- [ ] **Step 2: Write the failing test**

Create `internal/prompt/prompt_test.go` (non-TTY paths only — test stdin is never a terminal, which is exactly the guard under test; interactive navigation is covered by the manual checklist):

```go
package prompt

import (
	"errors"
	"testing"
)

func TestSelectOneNonTTYReturnsErrNotInteractive(t *testing.T) {
	_, err := SelectOne("pick:", []string{"a", "b"}, -1)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}

func TestSelectOneEmptyOptions(t *testing.T) {
	_, err := SelectOne("pick:", nil, -1)
	if err == nil {
		t.Fatal("want error for zero options")
	}
}

func TestTypeToConfirmNonTTY(t *testing.T) {
	err := TypeToConfirm("retype:", "expected")
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/prompt/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 4: Write the implementation**

Create `internal/prompt/prompt.go`:

```go
package prompt

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

var ErrNotInteractive = errors.New("not an interactive terminal")

func theme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Base = t.Focused.Base.MarginBottom(1)
	t.Blurred.Base = t.Blurred.Base.MarginBottom(1)
	return t
}

func SelectOne(label string, options []string, active int) (int, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return -1, ErrNotInteractive
	}
	if len(options) == 0 {
		return -1, errors.New("no options to choose from")
	}
	opts := make([]huh.Option[int], len(options))
	for i, o := range options {
		display := o
		if i == active {
			display = o + " (current)"
		}
		opts[i] = huh.NewOption(display, i)
	}
	choice := 0
	if active >= 0 && active < len(options) {
		choice = active
	}
	if err := huh.NewSelect[int]().
		Title(label).
		Options(opts...).
		Value(&choice).
		WithTheme(theme()).
		Run(); err != nil {
		return -1, err
	}
	return choice, nil
}

func TypeToConfirm(title, expected string) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ErrNotInteractive
	}
	var typed string
	if err := huh.NewInput().
		Title(title).
		Value(&typed).
		WithTheme(theme()).
		Run(); err != nil {
		return err
	}
	if strings.TrimSpace(typed) != expected {
		return fmt.Errorf("confirmation did not match %q — aborted", expected)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/prompt/ -v`
Expected: PASS.

- [ ] **Step 6: Run `make ci`** — expected green (lint may flag the new dep's indirect requirements; `go mod tidy` fixes).

- [ ] **Step 7: Mark task complete — NO commit.**

---

### Task 8: Transplant — render-credential fetch

**Files:**
- Create: `internal/cmd/rendercred.go`
- Test: `internal/cmd/rendercred_test.go` (create)
- Source reference: `SRC/internal/cmd/credentials.go:189-238`

**Interfaces:**
- Consumes: `api.SessionAPI`, `fetchList`, `valueOrEmpty` (Task 5), `pickFunc` (Task 6).
- Produces (package `cmd`):
  - `pickAPISecret(creds []map[string]any) string` — first non-revoked credential's `apiSecret`.
  - `fetchRenderSecret(ctx context.Context, client api.SessionAPI, org, project string) (string, error)` — `GET /v2/organisation/{org}/projects/{project}/api-credentials`, key `apiCredentials`.
  - `ensureRenderSecret(ctx context.Context, client api.SessionAPI, org, project string, interactive bool, pick pickFunc) (secret string, issued bool, err error)` — offers at a TTY, auto-issues otherwise (`POST` same path), never fails a skip.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/rendercred_test.go`:

```go
package cmd

import (
	"context"
	"testing"
)

func TestPickAPISecretSkipsRevoked(t *testing.T) {
	creds := []map[string]any{
		{"apiSecret": "sk_revoked", "revoked": true},
		{"apiSecret": "sk_live", "revoked": false},
	}
	if got := pickAPISecret(creds); got != "sk_live" {
		t.Fatalf("got %q", got)
	}
	if got := pickAPISecret([]map[string]any{{"revoked": true, "apiSecret": "x"}}); got != "" {
		t.Fatalf("all-revoked must be empty, got %q", got)
	}
}

func TestEnsureRenderSecretReturnsExisting(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[{"apiSecret":"sk_have","revoked":false}]}`,
	}}
	secret, issued, err := ensureRenderSecret(context.Background(), f, "org_1", "proj_1", false, neverPick)
	if err != nil || issued || secret != "sk_have" {
		t.Fatalf("got %q issued=%v err=%v", secret, issued, err)
	}
	if len(f.posts) != 0 {
		t.Fatalf("must not issue when a credential exists")
	}
}

func TestEnsureRenderSecretAutoIssuesNonInteractive(t *testing.T) {
	f := &fakeSession{
		gets: map[string]string{
			"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[]}`,
		},
		postResponses: map[string]string{
			"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiSecret":"sk_new"}`,
		},
	}
	secret, issued, err := ensureRenderSecret(context.Background(), f, "org_1", "proj_1", false, neverPick)
	if err != nil || !issued || secret != "sk_new" {
		t.Fatalf("got %q issued=%v err=%v", secret, issued, err)
	}
}

func TestEnsureRenderSecretInteractiveSkip(t *testing.T) {
	f := &fakeSession{gets: map[string]string{
		"/v2/organisation/org_1/projects/proj_1/api-credentials": `{"apiCredentials":[]}`,
	}}
	skip := func(_ string, options []string, _ int) (int, error) { return 1, nil }
	secret, issued, err := ensureRenderSecret(context.Background(), f, "org_1", "proj_1", true, skip)
	if err != nil || issued || secret != "" {
		t.Fatalf("skip must return empty: %q %v %v", secret, issued, err)
	}
	if len(f.posts) != 0 {
		t.Fatal("skip must not issue")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestPickAPISecret|TestEnsureRenderSecret' -v`
Expected: FAIL — `undefined: pickAPISecret`.

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/rendercred.go`:

```go
package cmd

import (
	"context"
	"errors"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func pickAPISecret(creds []map[string]any) string {
	for _, c := range creds {
		if revoked, _ := c["revoked"].(bool); revoked {
			continue
		}
		if secret := valueOrEmpty(c["apiSecret"]); secret != "" {
			return secret
		}
	}
	return ""
}

func apiCredentialsPath(org, project string) string {
	return "/v2/organisation/" + org + "/projects/" + project + "/api-credentials"
}

func fetchRenderSecret(ctx context.Context, client api.SessionAPI, org, project string) (string, error) {
	creds, err := fetchList(ctx, client, apiCredentialsPath(org, project), "apiCredentials")
	if err != nil {
		return "", err
	}
	return pickAPISecret(creds), nil
}

func ensureRenderSecret(ctx context.Context, client api.SessionAPI, org, project string, interactive bool, pick pickFunc) (string, bool, error) {
	secret, err := fetchRenderSecret(ctx, client, org, project)
	if err != nil || secret != "" {
		return secret, false, err
	}
	if interactive {
		idx, perr := pick("No render credential on this project — issue one?", []string{"Issue a new credential", "Skip"}, 0)
		if perr != nil && !errors.Is(perr, errNotInteractivePick) {
			return "", false, nil
		}
		if perr == nil && idx == 1 {
			return "", false, nil
		}
	}
	var created map[string]any
	if err := client.PostJSON(ctx, apiCredentialsPath(org, project), map[string]string{}, &created); err != nil {
		return "", false, err
	}
	return valueOrEmpty(created["apiSecret"]), true, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestPickAPISecret|TestEnsureRenderSecret' -v`
Expected: PASS.

- [ ] **Step 5: Run `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 9: `login` command

**Files:**
- Create: `internal/cmd/login.go`
- Create: `internal/cmd/session_helpers.go`
- Test: `internal/cmd/login_test.go` (create)
- Modify: `internal/cmd/root.go` (register `newLoginCmd()` in the `AddCommand` block, root.go:225-240)
- Source reference: `SRC/internal/cmd/login.go:32-147`

**Interfaces:**
- Consumes: `deviceauth.Poll` (Task 4), `resolveActiveOrg`/`resolveActiveProject` (Task 6), `prompt.SelectOne` (Task 7), `ensureRenderSecret` (Task 8), `config.ProfileName`/`config.Update` (Task 1), `api.NewSessionClient` (Task 3), `writeEnvelope`/`writeEnvelopeWithQuietData` (config.go:614-650), `internal/browser`.
- Produces (package `cmd`, reused by Tasks 10–15):
  - `sessionHost(cmd *cobra.Command) (host, profileName string, cliErr *output.CLIError)` — resolves API host + profile name through `config.Resolve` with the persistent `--profile` flag, `loadRepoOverlay()`, and env vars.
  - `loadSession(cmd *cobra.Command) (*sessionState, *output.CLIError)` where `type sessionState struct { Host, ProfileName string; Profile config.Profile; Client *api.SessionClient }` — returns `output.ErrAuth` ("not logged in — run `urlbox login`", hint names `urlbox login`) when the profile has no `SessionToken`.
  - `updateProfile(profileName string, mutate func(*config.Profile)) *output.CLIError` — lockfile-guarded write via `config.Update`.
  - `promptPick pickFunc` — adapter over `prompt.SelectOne` translating `prompt.ErrNotInteractive` → `errNotInteractivePick`.
  - Package test-injection vars mirroring status.go's pattern: `loginClock clock.Clock` (+`SetLoginClockForTest`/`ResetLoginClockForTest`), `loginOpener` (+`SetLoginOpenerForTest`/`ResetLoginOpenerForTest`).

- [ ] **Step 0: Read the opener pattern**

Read `internal/browser/opener.go` (104 lines) and the opener usage in `internal/cmd/dashboard.go`. Mirror the exact interface and injection-var pattern for `loginOpener` — if the interface method is not literally `Open(url string) error`, adapt the code below to the real signature; change nothing else.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/login_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/clock"
)

func advanceClockUntil(t *testing.T, fc *clock.FakeClock, done <-chan struct{}) {
	t.Helper()
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				if fc.WaitForSleeper(5 * time.Millisecond) {
					fc.Advance(10 * time.Second)
				}
			}
		}
	}()
}

func TestLoginFullFlowSingleOrgSingleProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device?code=ABCD-1234","interval":5,"expires_in":300}`),
		apitest.SuccessJSON(`{"access_token":"sess_tok_new"}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiSecret":"sk_fetched","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Email string `json:"email"`
			Org   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"org"`
			Project *struct {
				ID string `json:"id"`
			} `json:"project"`
			Render struct {
				Credential string `json:"credential"`
			} `json:"render"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not an envelope: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Data.Email != "a@urlbox.com" || env.Data.Org.ID != "org_acme" {
		t.Fatalf("envelope: %s", stdout.String())
	}
	if env.Data.Project == nil || env.Data.Project.ID != "proj_1" {
		t.Fatalf("project: %s", stdout.String())
	}
	if env.Data.Render.Credential != "ready" {
		t.Fatalf("render status: %s", stdout.String())
	}

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	p := cfg.Profiles["default"]
	if p["session_token"] != "sess_tok_new" || p["active_org"] != "org_acme" ||
		p["active_project"] != "proj_1" || p["api_secret"] != "sk_fetched" {
		t.Fatalf("profile after login: %#v", p)
	}

	reqs := srv.Requests()
	if reqs[0].Path != "/v1/auth/device/code" {
		t.Fatalf("first call %q", reqs[0].Path)
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"client_id":"urlbox-cli"`)) {
		t.Fatalf("device/code body: %s", reqs[0].Body)
	}
	if reqs[1].Path != "/v1/auth/device/token" {
		t.Fatalf("second call %q", reqs[1].Path)
	}
	if got := reqs[2].Header.Get("Authorization"); got != "Bearer sess_tok_new" {
		t.Fatalf("org list auth header %q", got)
	}
	if stderrStr := stderr.String(); !bytes.Contains([]byte(stderrStr), []byte("ABCD-1234")) {
		t.Fatalf("user code must print to stderr, got: %s", stderrStr)
	}
}

func TestLoginDeniedExitsAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device","interval":5,"expires_in":300}`),
		apitest.ScriptedResponse{Status: 400, Body: `{"error":"access_denied"}`},
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (auth)\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"code":"auth"`)) {
		t.Fatalf("error envelope: %s", stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestLogin -v`
Expected: FAIL — `undefined: SetLoginClockForTest` / unknown command "login".

- [ ] **Step 3: Write the session helpers**

Create `internal/cmd/session_helpers.go`:

```go
package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/prompt"
)

type sessionState struct {
	Host        string
	ProfileName string
	Profile     config.Profile
	Client      *api.SessionClient
}

func sessionHost(cmd *cobra.Command) (string, string, *output.CLIError) {
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return "", "", cfgErr
	}
	flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
	overlay, ovErr := loadRepoOverlay()
	if ovErr != nil {
		return "", "", ovErr
	}
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagProfile: flagProfile,
		EnvAPISecret: os.Getenv(config.EnvAPISecret),
		EnvAPIHost:   os.Getenv(config.EnvAPIHost),
		EnvProfile:   os.Getenv(config.EnvProfile),
		RepoOverlay:  overlay,
		Config:       cfg,
	})
	if rerr != nil {
		var cli *output.CLIError
		if errors.As(rerr, &cli) {
			return "", "", cli
		}
		return "", "", output.NewCLIError(output.ErrUsage, rerr.Error(), "Run `urlbox config path` to locate the config file.")
	}
	return resolved.APIHost, resolved.Profile, nil
}

func loadSession(cmd *cobra.Command) (*sessionState, *output.CLIError) {
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return nil, cliErr
	}
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return nil, cfgErr
	}
	profile := cfg.Profiles[profileName]
	if profile.SessionToken == "" {
		return nil, output.NewCLIError(
			output.ErrAuth,
			"not logged in",
			"Run `urlbox login` to sign in via your browser.",
		)
	}
	return &sessionState{
		Host:        host,
		ProfileName: profileName,
		Profile:     profile,
		Client:      api.NewSessionClient(host, profile.SessionToken),
	}, nil
}

func updateProfile(profileName string, mutate func(*config.Profile)) *output.CLIError {
	err := config.Update(func(c *config.Config) error {
		p := c.Profiles[profileName]
		mutate(&p)
		c.Profiles[profileName] = p
		if c.DefaultProfile == "" {
			c.DefaultProfile = profileName
		}
		return nil
	})
	if err == nil {
		return nil
	}
	var cli *output.CLIError
	if errors.As(err, &cli) {
		return cli
	}
	return output.NewCLIError(output.ErrForbidden, "could not write config: "+err.Error(),
		"Check the permissions of the config directory (`urlbox config path`).")
}

func promptPick(label string, options []string, active int) (int, error) {
	idx, err := prompt.SelectOne(label, options, active)
	if errors.Is(err, prompt.ErrNotInteractive) {
		return -1, errNotInteractivePick
	}
	return idx, err
}
```

- [ ] **Step 4: Write the login command**

Create `internal/cmd/login.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/deviceauth"
	"github.com/urlbox/urlbox-cli/internal/output"
)

var loginClock clock.Clock = clock.New()

func SetLoginClockForTest(c clock.Clock) { loginClock = c }

func ResetLoginClockForTest() { loginClock = clock.New() }

var loginOpener browser.Opener = browser.OSOpener{}

func SetLoginOpenerForTest(o browser.Opener) { loginOpener = o }

func ResetLoginOpenerForTest() { loginOpener = browser.OSOpener{} }

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri_complete"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

type loginFlags struct {
	org     string
	project string
}

func newLoginCmd() *cobra.Command {
	f := &loginFlags{}
	c := &cobra.Command{
		Use:   "login",
		Short: "Sign in via your browser (device flow)",
		Long: `Sign in to Urlbox via your browser.

Prints a short code and opens the approval page; once you approve, the CLI
stores a session for management commands, sets your active organisation and
project, and fetches the active project's render credential so render
commands work immediately.

CI and headless environments should set URLBOX_API_SECRET instead — the
device flow needs a browser.

Examples:
  urlbox login
  urlbox login --org acme --project production
  urlbox login --output-format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, f)
		},
	}
	c.Flags().StringVar(&f.org, "org", "", "Organisation to make active (name or id) — skips the picker")
	c.Flags().StringVar(&f.project, "project", "", "Project to make active (name or id) — skips the picker")
	return c
}

func runLogin(cmd *cobra.Command, f *loginFlags) error {
	ctx := context.Background()
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return cliErr
	}
	stderr := cmd.ErrOrStderr()
	anon := api.NewSessionClient(host, "")

	var code deviceCodeResponse
	if err := anon.PostJSON(ctx, "/v1/auth/device/code", map[string]string{"client_id": "urlbox-cli"}, &code); err != nil {
		return asCLIError(err)
	}

	fmt.Fprintf(stderr, "Your code: %s\n", code.UserCode)
	fmt.Fprintf(stderr, "Open this URL to continue: %s\n", code.VerificationURI)
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	if formatFlag != "json" && formatFlag != "quiet" {
		_ = loginOpener.Open(code.VerificationURI)
	}
	fmt.Fprintln(stderr, "Waiting for approval…")

	exchange := func() deviceauth.Exchange {
		status, data, err := anon.DoRaw(ctx, "POST", "/v1/auth/device/token", map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": code.DeviceCode,
			"client_id":   "urlbox-cli",
		})
		if err != nil {
			return deviceauth.Exchange{Err: err}
		}
		if status < 400 {
			return deviceauth.Exchange{AccessToken: valueOrEmpty(data["access_token"])}
		}
		return deviceauth.Exchange{RFCCode: valueOrEmpty(data["error"])}
	}
	token, pollErr := deviceauth.Poll(loginClock, code.Interval, code.ExpiresIn, exchange)
	if pollErr != nil {
		return pollErr
	}

	if cliErr := updateProfile(profileName, func(p *config.Profile) { p.SessionToken = token }); cliErr != nil {
		return cliErr
	}

	authed := api.NewSessionClient(host, token)
	org, orgErr := resolveActiveOrg(ctx, authed, f.org, promptPick)
	if orgErr != nil {
		return orgErr
	}
	if org.publicID != "" {
		if cliErr := updateProfile(profileName, func(p *config.Profile) { p.ActiveOrg = org.publicID }); cliErr != nil {
			return cliErr
		}
	}

	project, projErr := resolveActiveProject(ctx, authed, f.project, promptPick)
	if projErr != nil {
		return projErr
	}
	renderStatus := "none"
	if project.ID != "" {
		if cliErr := updateProfile(profileName, func(p *config.Profile) { p.ActiveProject = project.ID }); cliErr != nil {
			return cliErr
		}
		interactive := formatFlag != "json" && formatFlag != "quiet"
		secret, issued, err := ensureRenderSecret(ctx, authed, org.publicID, project.ID, interactive, promptPick)
		switch {
		case err != nil:
			fmt.Fprintf(stderr, "Logged in, but could not fetch the render credential: %v\n", err)
			renderStatus = "error"
		case secret != "":
			if cliErr := updateProfile(profileName, func(p *config.Profile) { p.APISecret = secret }); cliErr != nil {
				fmt.Fprintf(stderr, "Logged in, but could not save the render credential: %v\n", cliErr)
				renderStatus = "error"
			} else if issued {
				renderStatus = "issued"
			} else {
				renderStatus = "ready"
			}
		}
	} else {
		fmt.Fprintln(stderr, "No projects in this organisation yet.")
	}

	data := map[string]any{
		"email":   org.email,
		"org":     map[string]any{"id": org.publicID, "name": org.name},
		"project": nil,
		"render":  map[string]any{"credential": renderStatus},
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	summary := fmt.Sprintf("Logged in as %s — org %s", org.email, org.name)
	breadcrumbs := []output.Breadcrumb{{
		Action: "render",
		Cmd:    "urlbox screenshot https://example.com --output hello.png",
	}}
	env := output.NewEnvelope("login", data, summary, breadcrumbs)
	return writeEnvelopeWithQuietData(cmd, env, org.email)
}
```

- [ ] **Step 5: Register the command**

In `internal/cmd/root.go`, inside the `AddCommand` block (root.go:225-240), add in the alphabetical position:

```go
	cmd.AddCommand(newLoginCmd())
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestLogin' -race -v`
Expected: PASS both. If the compat suite (Task 2) now fails, login broke an existing path — fix before proceeding.

- [ ] **Step 7: Run `make surface-snapshot` then `make ci`**

Expected: `SURFACE.txt` gains `urlbox login` + its flags (`--org`, `--project`, inherited persistent flags); `ci` green.

- [ ] **Step 8: Mark task complete — NO commit.**

---

### Task 10: `logout` command

**Files:**
- Create: `internal/cmd/logout.go`
- Test: `internal/cmd/logout_test.go` (create)
- Modify: `internal/cmd/root.go` (register)
- Source reference: `SRC/internal/cmd/logout.go`

**Interfaces:**
- Consumes: `loadSession`-style config access (but logout must not hard-fail when not logged in), `updateProfile`, `api.NewSessionClient`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/logout_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func readProfileMap(t *testing.T, dir string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg.Profiles["default"]
}

func TestLogoutRevokesAndClears(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Path != "/v1/auth/sign-out" {
		t.Fatalf("requests: %+v", reqs)
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer sess_tok_compat_123456" {
		t.Fatalf("auth header %q", got)
	}
	p := readProfileMap(t, dir)
	for _, key := range []string{"session_token", "active_org", "active_project", "api_key", "api_secret"} {
		if p[key] != "" {
			t.Fatalf("%s not cleared: %#v", key, p)
		}
	}
}

func TestLogoutOfflineStillClearsLocally(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_HOST", "http://127.0.0.1:1")

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("offline logout must still succeed, exit %d\n%s", code, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("clearing local login anyway")) {
		t.Fatalf("expected warning on stderr, got: %s", stderr.String())
	}
	if p := readProfileMap(t, dir); p["session_token"] != "" {
		t.Fatalf("token not cleared: %#v", p)
	}
}

func TestLogoutWhenNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("logout without a session must be a no-op success, exit %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("envelope: %s", stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestLogout -v`
Expected: FAIL — unknown command "logout".

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/logout.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke this device's session",
		Long: `Sign out of Urlbox on this machine.

Revokes only this device's session server-side (your dashboard and other
devices stay signed in) and clears the stored session, active organisation,
active project, and render credential. If the server is unreachable the
local state is cleared anyway.

Examples:
  urlbox logout
  urlbox logout --output-format json`,
		Args: cobra.NoArgs,
		RunE: runLogout,
	}
}

func runLogout(cmd *cobra.Command, _ []string) error {
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return cliErr
	}
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return cfgErr
	}
	profile := cfg.Profiles[profileName]
	if profile.SessionToken == "" {
		env := output.NewEnvelope("logout", map[string]any{"logged_out": false}, "Not logged in.", nil)
		return writeEnvelope(cmd, env)
	}

	client := api.NewSessionClient(host, profile.SessionToken)
	if err := client.PostJSON(context.Background(), "/v1/auth/sign-out", map[string]string{}, nil); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: could not reach the server to revoke the session (%v); clearing local login anyway.\n", err)
	}

	if cliErr := updateProfile(profileName, func(p *config.Profile) {
		p.SessionToken = ""
		p.ActiveOrg = ""
		p.ActiveProject = ""
		p.APIKey = ""
		p.APISecret = ""
	}); cliErr != nil {
		return cliErr
	}

	env := output.NewEnvelope("logout", map[string]any{"logged_out": true}, "Logged out.", nil)
	return writeEnvelope(cmd, env)
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newLogoutCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestLogout -v`
Expected: PASS all three.

- [ ] **Step 5: Run `make surface-snapshot` then `make ci`** — expected green, `urlbox logout` in SURFACE.txt.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 11: `whoami` command (alias `me`)

**Files:**
- Create: `internal/cmd/whoami.go`
- Test: `internal/cmd/whoami_test.go` (create)
- Modify: `internal/cmd/root.go` (register)
- Source reference: `SRC/internal/cmd/whoami.go`

**Interfaces:**
- Consumes: `loadSession`, `activeOrgName`, `fetchList`, `toNameIDs`, `sessionResponse`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/whoami_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestWhoamiNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (auth)", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"code":"auth"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte("urlbox login")) {
		t.Fatalf("error envelope must carry auth code + login hint: %s", stdout.String())
	}
}

func TestWhoamiHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"}]}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	var env struct {
		Data struct {
			Email string `json:"email"`
			Org   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"org"`
			Project struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Data.Email != "a@urlbox.com" || env.Data.Org.ID != "org_acme" ||
		env.Data.Org.Name != "Acme" || env.Data.Project.ID != "proj_compat" {
		t.Fatalf("data: %s", stdout.String())
	}
}

func TestWhoamiExpiredSessionIsAuthError(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"user":{"email":""},"session":{"activeOrganizationId":"","activeOrganizationPublicId":""}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("dead token must exit 3, got %d\n%s", code, stdout.String())
	}
}

func TestMeAliasWorks(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"me", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("me alias must route to whoami, exit %d", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestWhoami|TestMeAlias' -v`
Expected: FAIL — unknown command "whoami".

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/whoami.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"me"},
		Short:   "Show the signed-in user and active context",
		Long: `Show who you are signed in as, plus the active organisation and project.

Examples:
  urlbox whoami
  urlbox whoami --output-format json`,
		Args: cobra.NoArgs,
		RunE: runWhoami,
	}
}

func runWhoami(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var session sessionResponse
	if err := sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return asCLIError(err)
	}
	if session.User.Email == "" {
		return output.NewCLIError(output.ErrAuth, "not logged in",
			"Your session has expired. Run `urlbox login` to sign in again.")
	}

	var project nameID
	if sess.Profile.ActiveProject != "" {
		if projects, err := fetchList(ctx, sess.Client, "/v2/projects", "projects"); err == nil {
			for _, r := range toNameIDs(projects) {
				if r.ID == sess.Profile.ActiveProject {
					project = r
					break
				}
			}
			if project.ID == "" {
				project = nameID{ID: sess.Profile.ActiveProject}
			}
		}
	}

	orgName := activeOrgName(ctx, sess.Client)
	data := map[string]any{
		"email": session.User.Email,
		"org": map[string]any{
			"id":   session.Session.ActiveOrganizationPublicID,
			"name": orgName,
		},
		"project": nil,
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	summary := fmt.Sprintf("Signed in as %s — org %s", session.User.Email, orgName)
	env := output.NewEnvelope("whoami", data, summary, nil)
	return writeEnvelopeWithQuietData(cmd, env, session.User.Email)
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newWhoamiCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run 'TestWhoami|TestMeAlias' -v`
Expected: PASS all four.

- [ ] **Step 5: Run `make surface-snapshot` then `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 12: `orgs` command group (alias `org`)

**Files:**
- Create: `internal/cmd/orgs.go`
- Test: `internal/cmd/orgs_test.go` (create)
- Modify: `internal/cmd/root.go` (register)
- Source reference: `SRC/internal/cmd/org.go` — read lines 95-215 fully before implementing `select`; the code below ports list + select faithfully (select = set-active, persist public id, then re-resolve project + render credential exactly like login steps 6-7, which is the spec's "select refreshes the stored render credential").

**Interfaces:**
- Consumes: `loadSession`, `matchOrg`, `resolveActiveProject`, `ensureRenderSecret`, `updateProfile`, `promptPick`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/orgs_test.go`:

```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestOrgsListMarksActive(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"2","activeOrganizationPublicId":"org_two"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"org_one"`, `"org_two"`, `"active":true`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("missing %s in: %s", want, stdout.String())
		}
	}
}

func TestOrgsSelectPositionalSwitchesAndRefreshesProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_9","name":"OtherOrgProj"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiSecret":"sk_other_org","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	p := readProfileMap(t, dir)
	if p["active_org"] != "org_one" || p["active_project"] != "proj_9" || p["api_secret"] != "sk_other_org" {
		t.Fatalf("profile after select: %#v", p)
	}
	reqs := srv.Requests()
	if reqs[1].Path != "/v1/auth/organization/set-active" {
		t.Fatalf("second call %q", reqs[1].Path)
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"organizationId":"1"`)) {
		t.Fatalf("set-active body: %s", reqs[1].Body)
	}
}

func TestOrgsSelectNonInteractiveWithoutArg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("want usage exit 1, got %d\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("name-or-id")) {
		t.Fatalf("error must name the positional: %s", stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestOrgs -v`
Expected: FAIL — unknown command "orgs".

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/orgs.go`:

```go
package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newOrgsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "orgs",
		Aliases: []string{"org"},
		Short:   "Manage the active organisation",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List your organisations",
		Args:  cobra.NoArgs,
		RunE:  runOrgsList,
	}
	sel := &cobra.Command{
		Use:   "select [name-or-id]",
		Short: "Set the active organisation",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runOrgsSelect,
	}
	c.AddCommand(list, sel)
	return c
}

func runOrgsList(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var orgs []orgListRow
	if err := sess.Client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return asCLIError(err)
	}
	var session sessionResponse
	_ = sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session)
	activeID := session.Session.ActiveOrganizationID

	rows := make([]map[string]any, len(orgs))
	activeName := ""
	for i, o := range orgs {
		active := o.ID != "" && o.ID == activeID
		if active {
			activeName = o.Name
		}
		rows[i] = map[string]any{"id": o.PublicID, "name": o.Name, "active": active}
	}
	summary := fmt.Sprintf("%d organisations", len(orgs))
	if activeName != "" {
		summary = fmt.Sprintf("%d organisations — active: %s", len(orgs), activeName)
	}
	env := output.NewEnvelope("orgs list", map[string]any{"organisations": rows}, summary, nil)
	return writeEnvelope(cmd, env)
}

func runOrgsSelect(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var orgs []orgListRow
	if err := sess.Client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return asCLIError(err)
	}
	if len(orgs) == 0 {
		return output.NewCLIError(output.ErrNotFound, "no organisations",
			"Create one in the dashboard at https://urlbox.com/dashboard.")
	}

	var chosen orgListRow
	if len(args) == 1 {
		match, ok := matchOrg(orgs, args[0])
		if !ok {
			return output.NewCLIError(output.ErrNotFound,
				fmt.Sprintf("no organisation matching %q", args[0]),
				"Run `urlbox orgs list` to see your organisations.")
		}
		chosen = match
	} else {
		names := make([]string, len(orgs))
		active := -1
		var session sessionResponse
		_ = sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session)
		for i, o := range orgs {
			names[i] = o.Name
			if o.ID == session.Session.ActiveOrganizationID {
				active = i
			}
		}
		idx, err := promptPick("Select the active organisation:", names, active)
		if err != nil {
			if errors.Is(err, errNotInteractivePick) {
				return output.NewCLIError(output.ErrUsage,
					"selection needs an interactive terminal",
					"Pass the organisation directly: `urlbox orgs select <name-or-id>`.")
			}
			return output.NewCLIError(output.ErrUsage, err.Error(), "")
		}
		chosen = orgs[idx]
	}

	if err := sess.Client.PostJSON(ctx, "/v1/auth/organization/set-active",
		map[string]string{"organizationId": chosen.ID}, nil); err != nil {
		return asCLIError(err)
	}
	var session sessionResponse
	if err := sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return asCLIError(err)
	}
	publicID := session.Session.ActiveOrganizationPublicID
	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) {
		p.ActiveOrg = publicID
		p.ActiveProject = ""
		p.APISecret = ""
	}); cliErr != nil {
		return cliErr
	}

	project, projErr := resolveActiveProject(ctx, sess.Client, "", func(label string, options []string, active int) (int, error) {
		return -1, errNotInteractivePick
	})
	renderStatus := "none"
	if projErr == nil && project.ID != "" {
		if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) { p.ActiveProject = project.ID }); cliErr == nil {
			if secret, issued, err := ensureRenderSecret(ctx, sess.Client, publicID, project.ID, false, promptPick); err == nil && secret != "" {
				if updateProfile(sess.ProfileName, func(p *config.Profile) { p.APISecret = secret }) == nil {
					if issued {
						renderStatus = "issued"
					} else {
						renderStatus = "ready"
					}
				}
			}
		}
	}
	if projErr == nil && project.ID == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "No projects in this organisation yet — run `urlbox projects select` after creating one.")
	}
	if projErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Several projects in this organisation — run `urlbox projects select` to pick one.")
	}

	data := map[string]any{
		"org":    map[string]any{"id": publicID, "name": chosen.Name},
		"render": map[string]any{"credential": renderStatus},
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	env := output.NewEnvelope("orgs select", data,
		fmt.Sprintf("Active organisation: %s", chosen.Name), nil)
	return writeEnvelopeWithQuietData(cmd, env, publicID)
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newOrgsCmd())`.

- [ ] **Step 4: Cross-check against source**

Read `SRC/internal/cmd/org.go:95-215`. If the source's `runOrgSelect` differs materially from the port above (beyond output plumbing), align the port to the source's behaviour and extend the tests to pin the difference. Known intended deltas (do not "fix"): envelopes replace RenderList tables; project re-resolution is non-interactive with a stderr pointer to `urlbox projects select`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestOrgs -v`
Expected: PASS all three.

- [ ] **Step 6: Run `make surface-snapshot` then `make ci`** — expected green.

- [ ] **Step 7: Mark task complete — NO commit.**

---

### Task 13: `projects` group — context (`list`, `select`)

**Files:**
- Create: `internal/cmd/projects.go`
- Test: `internal/cmd/projects_test.go` (create)
- Modify: `internal/cmd/root.go` (register)
- Source reference: `SRC/internal/cmd/projects.go:18-223` — read before implementing; port behaviour, express in envelopes.

**Interfaces:**
- Consumes: `loadSession`, `fetchList`, `toNameIDs`, `resolveNameOrID`, `ensureRenderSecret`, `updateProfile`, `promptPick`.
- Produces: `newProjectsCmd() *cobra.Command` (Use `projects`, Aliases `["project"]`) — Task 15 adds subcommands to this same constructor; `requireActiveOrg(sess *sessionState) (string, *output.CLIError)`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/projects_test.go`:

```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestProjectsListMarksActive(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"},{"id":"proj_2","name":"Side"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"proj_compat"`, `"proj_2"`, `"active":true`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("missing %s: %s", want, stdout.String())
		}
	}
}

func TestProjectsSelectPositionalRefreshesCredential(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"},{"id":"proj_2","name":"Side"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiSecret":"sk_side","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "select", "side", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_2" || p["api_secret"] != "sk_side" {
		t.Fatalf("profile after select: %#v", p)
	}
}

func TestProjectsSelectNonInteractiveWithoutArg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"A"},{"id":"proj_2","name":"B"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "select", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("want usage exit 1, got %d\n%s", code, stdout.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestProjects -v`
Expected: FAIL — unknown command "projects".

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/projects.go`:

```go
package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newProjectsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "projects",
		Aliases: []string{"project"},
		Short:   "Manage projects and the active project",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the active organisation's projects",
		Args:  cobra.NoArgs,
		RunE:  runProjectsList,
	}
	sel := &cobra.Command{
		Use:   "select [name-or-id]",
		Short: "Set the active project (used by render)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runProjectsSelect,
	}
	c.AddCommand(list, sel)
	return c
}

func requireActiveOrg(sess *sessionState) (string, *output.CLIError) {
	if sess.Profile.ActiveOrg == "" {
		return "", output.NewCLIError(output.ErrUsage, "no active organisation",
			"Run `urlbox orgs select` (or `urlbox login`) first.")
	}
	return sess.Profile.ActiveOrg, nil
}

func runProjectsList(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return asCLIError(err)
	}
	rows := make([]map[string]any, len(projects))
	activeName := ""
	for i, m := range projects {
		id := valueOrEmpty(m["id"])
		active := id != "" && id == sess.Profile.ActiveProject
		if active {
			activeName = valueOrEmpty(m["name"])
		}
		m["active"] = active
		rows[i] = m
	}
	summary := fmt.Sprintf("%d projects", len(rows))
	if activeName != "" {
		summary = fmt.Sprintf("%d projects — active: %s", len(rows), activeName)
	}
	env := output.NewEnvelope("projects list", map[string]any{"projects": rows}, summary, nil)
	return writeEnvelope(cmd, env)
}

func runProjectsSelect(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	projects, err := fetchList(ctx, sess.Client, "/v2/projects", "projects")
	if err != nil {
		return asCLIError(err)
	}
	rows := toNameIDs(projects)
	if len(rows) == 0 {
		return output.NewCLIError(output.ErrNotFound, "no projects in the active organisation",
			"Create one with `urlbox projects create <name>`.")
	}

	var chosen nameID
	if len(args) == 1 {
		var resErr *output.CLIError
		chosen, resErr = resolveNameOrID(args[0], "proj_", rows, "project")
		if resErr != nil {
			return resErr
		}
	} else {
		names := make([]string, len(rows))
		active := -1
		for i, r := range rows {
			names[i] = r.Name
			if r.ID == sess.Profile.ActiveProject {
				active = i
			}
		}
		idx, perr := promptPick("Select the active project (used by render):", names, active)
		if perr != nil {
			if errors.Is(perr, errNotInteractivePick) {
				return output.NewCLIError(output.ErrUsage,
					"selection needs an interactive terminal",
					"Pass the project directly: `urlbox projects select <name-or-id>`.")
			}
			return output.NewCLIError(output.ErrUsage, perr.Error(), "")
		}
		chosen = rows[idx]
	}

	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) { p.ActiveProject = chosen.ID }); cliErr != nil {
		return cliErr
	}
	renderStatus := "none"
	if org := sess.Profile.ActiveOrg; org != "" {
		formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
		interactive := formatFlag != "json" && formatFlag != "quiet"
		if secret, issued, err := ensureRenderSecret(ctx, sess.Client, org, chosen.ID, interactive, promptPick); err == nil && secret != "" {
			if updateProfile(sess.ProfileName, func(p *config.Profile) { p.APISecret = secret }) == nil {
				if issued {
					renderStatus = "issued"
				} else {
					renderStatus = "ready"
				}
			}
		}
	}

	data := map[string]any{
		"project": map[string]any{"id": chosen.ID, "name": chosen.Name},
		"render":  map[string]any{"credential": renderStatus},
	}
	env := output.NewEnvelope("projects select", data,
		fmt.Sprintf("Active project: %s", chosen.Name), nil)
	return writeEnvelopeWithQuietData(cmd, env, chosen.ID)
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newProjectsCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestProjects -v`
Expected: PASS all three.

- [ ] **Step 5: Run `make surface-snapshot` then `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 14: `usage` command

**Files:**
- Create: `internal/cmd/usage.go`
- Test: `internal/cmd/usage_test.go` (create)
- Modify: `internal/cmd/root.go` (register)
- Source reference: `SRC/internal/cmd/usage.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/usage_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestUsageHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"rendersUsed":120,"renderQuota":1000,"period":{"start":"2026-08-01","end":"2026-08-31"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"usage", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data["renders_used"] != float64(120) || env.Data["render_quota"] != float64(1000) {
		t.Fatalf("data: %#v", env.Data)
	}
	if env.Data["current_period_start"] != "2026-08-01" {
		t.Fatalf("period: %#v", env.Data)
	}
}

func TestUsageNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"usage", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestUsage -v`
Expected: FAIL — unknown command "usage".

- [ ] **Step 3: Write the implementation**

Create `internal/cmd/usage.go`:

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newUsageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "usage",
		Short: "Show the organisation's render usage for the current period",
		Args:  cobra.NoArgs,
		RunE:  runUsage,
	}
}

func runUsage(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	var usage struct {
		RendersUsed int `json:"rendersUsed"`
		RenderQuota int `json:"renderQuota"`
		Period      struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"period"`
	}
	if err := sess.Client.GetJSON(context.Background(), "/v2/usage", &usage); err != nil {
		return asCLIError(err)
	}
	data := map[string]any{
		"renders_used":         usage.RendersUsed,
		"render_quota":         usage.RenderQuota,
		"current_period_start": usage.Period.Start,
		"current_period_end":   usage.Period.End,
	}
	summary := fmt.Sprintf("Renders used: %d / %d", usage.RendersUsed, usage.RenderQuota)
	env := output.NewEnvelope("usage", data, summary, nil)
	return writeEnvelopeWithQuietData(cmd, env, fmt.Sprintf("%d", usage.RendersUsed))
}
```

Register in `internal/cmd/root.go`: `cmd.AddCommand(newUsageCmd())`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestUsage -v`
Expected: PASS both.

- [ ] **Step 5: Run `make surface-snapshot` then `make ci`** — expected green.

- [ ] **Step 6: Mark task complete — NO commit.**

---

### Task 15: `projects` CRUD + defaults

**Files:**
- Modify: `internal/cmd/projects.go` (extend `newProjectsCmd`)
- Test: `internal/cmd/projects_crud_test.go` (create)
- Source reference: `SRC/internal/cmd/projects.go:224-1003` — read `runProjectsShow/Create/Update/Rename/SetEnabled/Delete/Defaults*` before implementing; endpoints below are pinned from that file.

**Interfaces:**
- Consumes: `requireActiveOrg` (Task 13), `prompt.TypeToConfirm` (Task 7), everything from Task 13.
- Endpoints (pinned from source): create `POST /v2/projects` `{name}`; show `GET /v2/organisation/{org}/projects/{id}`; rename/enable/disable `PATCH /v2/organisation/{org}/projects/{id}` (`{"name":…}` / `{"enabled":…}`); delete `DELETE /v2/organisation/{org}/projects/{id}`; defaults read from the show endpoint's `defaultOptions`; defaults write `PATCH /v2/organisation/{org}/projects/{id}/render-defaults` `{"options": <object|null>}`.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/projects_crud_test.go`:

```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestProjectsCreate(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"project":{"id":"proj_new","name":"Fresh"}}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/projects" {
		t.Fatalf("request: %+v", reqs[0])
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"name":"Fresh"`)) {
		t.Fatalf("body: %s", reqs[0].Body)
	}
}

func TestProjectsRename(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Old"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","name":"New"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "rename", "old", "New", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"name":"New"`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDeleteRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Doomed"}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("delete without --yes off-TTY must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--yes")) {
		t.Fatalf("error must name --yes: %s", stdout.String())
	}
}

func TestProjectsDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "DELETE" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
}

func TestProjectsDefaultsSetAndRemove(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","defaultOptions":{"format":"png"}}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "defaults", "set", "main", "--json", `{"format":"png"}`, "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("set exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1/render-defaults" {
		t.Fatalf("set request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"format":"png"`)) {
		t.Fatalf("set body: %s", reqs[1].Body)
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{"projects", "defaults", "remove", "main", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("remove exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs = srv.Requests()
	if !bytes.Contains(reqs[3].Body, []byte(`"options":null`)) {
		t.Fatalf("remove body: %s", reqs[3].Body)
	}
}

func TestProjectsCrudNeedsActiveOrg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_HOST", "http://127.0.0.1:1")
	cfgPath := dir + "/urlbox/config.json"
	_ = cfgPath
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "set", "active_org", "", "--output-format", "json"}, &stdout, &stderr)
	_ = code
	stdout.Reset()
	code = Execute([]string{"projects", "rename", "proj_x", "New", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("no active org must be usage exit 1, got %d\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("orgs select")) {
		t.Fatalf("hint must name orgs select: %s", stdout.String())
	}
}
```

Note: `TestProjectsCrudNeedsActiveOrg` depends on Task 16's `config set active_org`; until Task 16 lands, blank the field by writing the fixture with `withSession` variant that omits `active_org` — add a third fixture writer if needed (`writeCompatConfigNoOrg`) instead of depending on Task 16 ordering. Implement whichever is reached first; the assertion (usage error naming `orgs select`) is the contract.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run 'TestProjectsCreate|TestProjectsRename|TestProjectsDelete|TestProjectsDefaults|TestProjectsCrudNeeds' -v`
Expected: FAIL — unknown subcommands.

- [ ] **Step 3: Extend `newProjectsCmd`**

Add to `internal/cmd/projects.go` (new subcommands appended inside `newProjectsCmd` after `c.AddCommand(list, sel)`, plus the run functions):

```go
	show := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one project",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectsShow,
	}
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project in the active organisation",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectsCreate,
	}
	rename := &cobra.Command{
		Use:   "rename <name-or-id> <new-name>",
		Short: "Rename a project",
		Args:  cobra.ExactArgs(2),
		RunE:  runProjectsRename,
	}
	enable := &cobra.Command{
		Use:   "enable <name-or-id>",
		Short: "Enable a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsSetEnabled(cmd, args, true)
		},
	}
	disable := &cobra.Command{
		Use:   "disable <name-or-id>",
		Short: "Disable a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsSetEnabled(cmd, args, false)
		},
	}
	var yes bool
	del := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsDelete(cmd, args, yes)
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "Skip the retype-to-confirm prompt")
	defaults := &cobra.Command{
		Use:   "defaults",
		Short: "Manage the project's default render options",
	}
	defaultsShow := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show default render options",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectsDefaultsShow,
	}
	var defaultsJSON string
	var defaultsMerge bool
	defaultsSet := &cobra.Command{
		Use:   "set <name-or-id> --json <object>",
		Short: "Set default render options",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsDefaultsSet(cmd, args, defaultsJSON, defaultsMerge)
		},
	}
	defaultsSet.Flags().StringVar(&defaultsJSON, "json", "", "Default options as a JSON object")
	defaultsSet.Flags().BoolVar(&defaultsMerge, "merge", false, "Merge into the existing defaults instead of replacing them")
	defaultsRemove := &cobra.Command{
		Use:   "remove <name-or-id>",
		Short: "Remove all default render options",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectsDefaultsRemove,
	}
	defaults.AddCommand(defaultsShow, defaultsSet, defaultsRemove)
	c.AddCommand(show, create, rename, enable, disable, del, defaults)
```

And the run functions (same file):

```go
func resolveProjectArg(cmd *cobra.Command, sess *sessionState, arg string) (nameID, *output.CLIError) {
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, asCLIError(err)
	}
	return resolveNameOrID(arg, "proj_", toNameIDs(projects), "project")
}

func projectPath(org, id string) string {
	return "/v2/organisation/" + org + "/projects/" + id
}

func runProjectsShow(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects show", resp,
		fmt.Sprintf("Project %s", resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsCreate(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	var resp map[string]any
	if err := sess.Client.PostJSON(context.Background(), "/v2/projects",
		map[string]string{"name": args[0]}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects create", resp,
		fmt.Sprintf("Created project %s", args[0]),
		[]output.Breadcrumb{{Action: "activate", Cmd: "urlbox projects select " + args[0]}})
	return writeEnvelope(cmd, env)
}

func runProjectsRename(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(), projectPath(org, resolved.ID),
		map[string]string{"name": args[1]}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects rename", resp,
		fmt.Sprintf("Renamed %s to %s", resolved.ID, args[1]), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsSetEnabled(cmd *cobra.Command, args []string, enabled bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(), projectPath(org, resolved.ID),
		map[string]bool{"enabled": enabled}, &resp); err != nil {
		return asCLIError(err)
	}
	verb := "Enabled"
	if !enabled {
		verb = "Disabled"
	}
	env := output.NewEnvelope("projects "+map[bool]string{true: "enable", false: "disable"}[enabled],
		resp, fmt.Sprintf("%s %s", verb, resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDelete(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	if !yes {
		name := resolved.Name
		if name == "" {
			name = resolved.ID
		}
		if err := prompt.TypeToConfirm(fmt.Sprintf("Type %q to confirm deletion:", name), name); err != nil {
			if errors.Is(err, prompt.ErrNotInteractive) {
				return output.NewCLIError(output.ErrUsage,
					"deletion needs confirmation",
					"Re-run with --yes to confirm non-interactively.")
			}
			return output.NewCLIError(output.ErrUsage, err.Error(), "")
		}
	}
	if err := sess.Client.DeleteJSON(context.Background(), projectPath(org, resolved.ID), nil); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects delete",
		map[string]any{"deleted": resolved.ID},
		fmt.Sprintf("Deleted project %s", resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDefaultsShow(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &resp); err != nil {
		return asCLIError(err)
	}
	defaults := map[string]any{}
	if project, ok := resp["project"].(map[string]any); ok {
		if d, ok := project["defaultOptions"].(map[string]any); ok {
			defaults = d
		}
	}
	env := output.NewEnvelope("projects defaults show",
		map[string]any{"project": resolved.ID, "defaults": defaults},
		fmt.Sprintf("%d default options on %s", len(defaults), resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDefaultsSet(cmd *cobra.Command, args []string, jsonBody string, merge bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	if jsonBody == "" {
		return output.NewCLIError(output.ErrUsage, "missing --json",
			`Pass the defaults as a JSON object: --json '{"format":"png"}'.`)
	}
	var options map[string]any
	if err := json.Unmarshal([]byte(jsonBody), &options); err != nil {
		return output.NewCLIError(output.ErrUsage, "--json is not a valid JSON object: "+err.Error(),
			`Example: --json '{"format":"png","full_page":true}'.`)
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	final := options
	if merge {
		var current map[string]any
		if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &current); err != nil {
			return asCLIError(err)
		}
		merged := map[string]any{}
		if project, ok := current["project"].(map[string]any); ok {
			if d, ok := project["defaultOptions"].(map[string]any); ok {
				for k, v := range d {
					merged[k] = v
				}
			}
		}
		for k, v := range options {
			merged[k] = v
		}
		final = merged
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(),
		projectPath(org, resolved.ID)+"/render-defaults",
		map[string]any{"options": final}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects defaults set", resp,
		fmt.Sprintf("Set %d default options on %s", len(final), resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDefaultsRemove(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(cmd, sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(),
		projectPath(org, resolved.ID)+"/render-defaults",
		map[string]any{"options": nil}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects defaults remove", resp,
		fmt.Sprintf("Removed default options from %s", resolved.ID), nil)
	return writeEnvelope(cmd, env)
}
```

Add the imports `encoding/json` and `github.com/urlbox/urlbox-cli/internal/prompt` to `internal/cmd/projects.go`.

- [ ] **Step 4: Cross-check against source**

Read `SRC/internal/cmd/projects.go:224-1003`. Align any endpoint/body divergence to the source (they are pinned above from that file; this step is verification, not discovery). The `--merge` read path deliberately uses the org-scoped show endpoint (source line 925).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -race -v`
Expected: PASS — the new CRUD tests plus every earlier task's tests plus the Task 2 compatibility suite.

- [ ] **Step 6: Run `make surface-snapshot` then `make ci`** — expected green.

- [ ] **Step 7: Mark task complete — NO commit.**

---

### Task 16: Config keys, in-repo docs, final surface, agent-layer verification

**Files:**
- Modify: `internal/cmd/config.go` (the `config get` read switch and the `config set` write switch — locate both with `grep -n '"api_secret"' internal/cmd/config.go`)
- Test: `internal/cmd/config_session_keys_test.go` (create)
- Modify: `skills/SKILL.md`, `README.md`, `npm/README.md`
- Create: `docs/superpowers/verification/2026-08-12-plan1-agent-layer.md`

**Interfaces:**
- Consumes: `maskSecret` (auth.go:313 — stays in place; Plan 2's auth sweep must relocate it before deleting auth.go), Task 1 fields.

- [ ] **Step 1: Write the failing test**

Create `internal/cmd/config_session_keys_test.go`:

```go
package cmd

import (
	"bytes"
	"testing"
)

func TestConfigGetSessionTokenMasked(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "get", "session_token", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("sess_tok_compat_123456")) {
		t.Fatalf("session token leaked unmasked: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("sess")) {
		t.Fatalf("masked value missing: %s", stdout.String())
	}
}

func TestConfigGetSessionTokenReveal(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "get", "session_token", "--reveal", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("sess_tok_compat_123456")) {
		t.Fatalf("--reveal must show the token: %s", stdout.String())
	}
}

func TestConfigGetActiveOrgAndProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	for key, want := range map[string]string{"active_org": "org_compat", "active_project": "proj_compat"} {
		var stdout, stderr bytes.Buffer
		code := Execute([]string{"config", "get", key, "--output-format", "json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s exit %d", key, code)
		}
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("%s: %s", key, stdout.String())
		}
	}
}

func TestConfigSetActiveProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "set", "active_project", "proj_other"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr.String())
	}
	if p := readProfileMap(t, dir); p["active_project"] != "proj_other" {
		t.Fatalf("profile: %#v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cmd/ -run TestConfigGetSession -v`
Expected: FAIL — unknown config key `session_token` (whatever exact error the existing unknown-key path produces).

- [ ] **Step 3: Extend the config key switches**

In `internal/cmd/config.go`: add `"session_token"` (masked with `maskSecret` unless the existing `--reveal` flag on `config get` is set — mirror the `api_secret` case exactly), `"active_org"`, and `"active_project"` (plain string cases) to BOTH the `config get` read switch and the `config set` write switch (the one feeding `writeProfileValue` / the equivalent). Also extend the valid-keys list in the command's help/error text. `config set session_token` validates through `config.ValidateSecretValue`, exactly as `api_secret` does.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestConfig -v`
Expected: PASS — the new tests plus all pre-existing config command tests.

- [ ] **Step 5: In-repo docs sweep (additive only — auth references are Plan 2's)**

1. `skills/SKILL.md`: add the new commands to the command inventory, matching the file's existing list format, with one line each: `login` (browser sign-in; agents: use URLBOX_API_SECRET instead), `logout`, `whoami`/`me`, `orgs list|select`, `projects list|select|show|create|rename|enable|disable|delete|defaults`, `usage`.
2. `README.md`: in the command list, add `urlbox login`, `urlbox whoami`, `urlbox orgs list`, `urlbox projects list`, `urlbox usage` example lines, following the existing bullet format. Do NOT touch existing `urlbox auth` text (Plan 2).
3. `npm/README.md`: mirror the README additions.

- [ ] **Step 6: Final surface + gates**

Run: `make surface-snapshot && make ci && go test ./... -race`
Expected: all green; `SURFACE.txt` diff shows ONLY additions (login, logout, whoami, orgs, projects, usage + their flags).

- [ ] **Step 7: Agent-layer verification (record actual output)**

Create `docs/superpowers/verification/2026-08-12-plan1-agent-layer.md`. Build the binary (`go build -o bin/urlbox ./cmd/urlbox`). For EACH of the two config states (legacy: profile with api_key+api_secret only; post-login: plus session_token/active_org/active_project — construct both in a scratch `XDG_CONFIG_HOME`), run every pre-existing command exactly as listed and paste the actual stdout/stderr/exit-code into the doc:

```
bin/urlbox version
bin/urlbox commands --output-format json
bin/urlbox schema render --output-format json | head -5
bin/urlbox render https://example.com --dry-run --output-format json
bin/urlbox screenshot https://example.com --dry-run --output-format json
bin/urlbox pdf https://example.com --dry-run --output-format json
bin/urlbox render https://example.com --curl
bin/urlbox link https://example.com
bin/urlbox config path
bin/urlbox config get api_secret
bin/urlbox config profile list
bin/urlbox doctor || true
bin/urlbox dashboard --output-format json
```

plus the new commands in their logged-out state (`login` interrupted with Ctrl-C after the code prints, `whoami`, `usage`, `orgs list`, `projects list` — each must produce the auth error envelope, exit 3). Any behavioural difference between the two states for a pre-existing command is a STOP-the-line bug. This doc is the input to the human-layer checklist (Plan 2's release gate).

- [ ] **Step 8: Mark task complete — NO commit.** Plan 1 ends here; Plan 2 (credential resources + auth sweep) starts from this uncommitted state.

---

## Self-Review

**1. Spec coverage (slices 1–3):** config fields → Task 1; compatibility net → Task 2 (+16 Step 7); session client + 401 mapping → Task 3; five transplants → Tasks 4 (poll), 5 (name-or-id), 6 (org/project matrix + id translation), 8 (render credential; masking + payload-mapping transplants belong to Plan 2's credential resources, where their behaviour lives); picker → Task 7; login flow steps 1–8 → Task 9; logout → Task 10; whoami → Task 11; orgs → Task 12; project context → Task 13; usage → Task 14; projects CRUD + defaults → Task 15; config keys/docs/surface/agent-verification → Task 16. Envelope/error/surface/help conventions are embedded per-task. Gaps: none for slices 1–3; masking + payload-mapping transplants explicitly deferred to Plan 2 with their consuming commands.

**2. Placeholder scan:** no TBDs; every code step carries complete code; the two "cross-check against source" steps (12.4, 15.4) are verification steps with pinned expected behaviour, not deferred design. Task 15's active-org test note resolves its own ordering dependency (`writeCompatConfigNoOrg` fallback).

**3. Type consistency:** `nameID`/`pickFunc`/`errNotInteractivePick`/`sessionState`/`asCLIError` defined once (Tasks 5/6/9) and consumed with identical signatures in Tasks 8–15; `deviceauth.Exchange{AccessToken, RFCCode, Err}` matches Task 9's exchange closure; `config.ProfileName(flag, env, overlay, cfg)` matches Task 9's `sessionHost` usage; `writeEnvelope(cmd, env)` / `writeEnvelopeWithQuietData(cmd, env, scalar)` used exactly as defined in `internal/cmd/config.go:614-650`; `apitest.ScriptedResponse{Status, Body}` literals match `internal/api/apitest/server.go:46`. One deliberate adaptation: source's `resolveNameOrID` returned `error`, here it returns `*output.CLIError` (house closed-code requirement) — all call sites in Tasks 13/15 use the CLIError form consistently.
