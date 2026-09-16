# CLI Render Reporting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Post-render report hint line, `urlbox report <renderId>` (session-authed create against `/v2/organisation/{org}/render-reports`), and `urlbox support` (browser opener for https://urlbox.com/contact).

**Architecture:** The api package captures the render id that sync responses only carry in the `x-urlbox-request-id` header (and error bodies carry as `requestId`), exposing it in-memory only — envelopes never change shape. The render command prints a gated muted stderr hint line naming `urlbox report <id>`. `report` is a session command modelled on `proxies create`; `support` is a clone of `dashboard`.

**Tech Stack:** Go, cobra, charmbracelet/huh (prompts), lipgloss (styles), httptest via `internal/api/apitest`.

**Spec:** `docs/superpowers/specs/2026-09-11-render-report-command-design.md` — binding. Re-verify mono contracts at build time (Arnold has an in-flight render-reports worktree).

## Global Constraints

- Zero code comments except linter-forced exported doc comments.
- TDD failing-first; `make ci` (fmt-check, lint, test, unit, build, surface-check) green per task; `make surface-snapshot` + commit SURFACE.txt with any surface change.
- stdout = data, stderr = human. Every interactive affordance has a flag equivalent; non-TTY never hangs.
- Production-shape fixtures ONLY — wire shapes below were captured live 2026-09-11; do not invent fields.
- All user-facing copy in this plan (hint line, prompt titles, category labels, summaries, hints, help text) is **proposed** — Arnold approves or rewrites at task review before merge. No em-dashes in customer-facing prose; positioning is "website screenshot API".
- Commits per task (squash to one before PR), author `Arnold Cubici-Jones <108676317+AJCJ1@users.noreply.github.com>`, subjects <72 chars, imperative. NEVER push without Arnold's word. NEVER open a PR without Arnold's explicit ask.
- The engine test-watcher note does not apply here; `make test` runs plain `go test`.

## Pinned wire shapes (captured live 2026-09-11 — fixture sources)

```text
Sync success 200: body {"renderUrl":"https://renders.urlbox.com/…png","size":17752,…}  (NO renderId)
                  header x-urlbox-request-id: 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps
Sync failure 400: body {"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}
                  (no x-urlbox-request-id header)
Async 201:        body {"status":"created","renderId":"01a09072-9ec6-76ca-bcc7-bbf5cc35a9e4_pa","statusUrl":"https://api.urlbox.com/v1/render/…"}
Report create DTO (from mono origin/main zod contract packages/domain/src/render-report/render-report.schemas.ts):
  {"id":"rpt_x1","renderId":"01a0…_ps","category":"bot-detection","comment":"…","rects":[],
   "state":"open","outputUrl":null,"outputFormat":null,"resolutionMessage":null,
   "resolutionExampleOptions":null,"resolutionExampleUrl":null,"hasUnseenResolution":false,
   "createdAt":"2026-09-11T12:00:00.000Z"}
Report 404: oRPC error {"defined":false,"code":"NOT_FOUND","status":404,"message":"Render not found"}
Report 409: {"defined":false,"code":"CONFLICT","status":409,"message":"This report is being looked at by the team and can no longer be edited."}
Categories: bot-detection, login-required, missing-content, cookie-banner-or-popup, render-failed, other
```

Note the oRPC error shape has `message` at top level — `extractAPIError` (`internal/api/http_client.go:299`) already handles the flat `{"message": …}` shape.

---

### Task 0: Branch setup

**Files:** none (git only)

Preconditions Arnold resolves before execution starts (surface, don't decide):
the main checkout `~/Code/work/urlbox-cli` sits on `feat/account-management`
with a modified `README.md` and stray untracked files (`.idea/`, `check.png`,
`hello.png`). Switching to a branch off `origin/main` will fail on the README.

- [ ] **Step 1: Fetch and branch**

```bash
cd ~/Code/work/urlbox-cli
git fetch origin
git switch -c feat/render-report origin/main
```

Expected: new branch at `c9d96d6` (or newer origin/main tip).

- [ ] **Step 2: Commit the docs**

```bash
git add docs/superpowers/HANDOFF-2026-09-11-render-report-command.md \
        docs/superpowers/specs/2026-09-11-render-report-command-design.md \
        docs/superpowers/plans/2026-09-11-render-report-command.md
git commit -m "docs: render-report spec and plan"
```

- [ ] **Step 3: Baseline green**

Run: `make ci`
Expected: PASS on the untouched tree.

---

### Task 1: Capture the render id in the api package

**Files:**
- Modify: `internal/api/types.go` (Response struct, line 34)
- Modify: `internal/api/http_client.go` (`do` at line 81, add `extractRequestID` near `extractAPIError` at line 299)
- Create: `internal/api/render_id_test.go`

**Interfaces:**
- Produces: `api.Response.RenderID string` — set from the `x-urlbox-request-id` response header on any 2xx render-family response; zero value when the header is absent.
- Produces: `api.RenderIDError` — `type RenderIDError struct { Err *output.CLIError; RenderID string }` with `Error() string` and `Unwrap() error`; returned by `HTTPClient.do` instead of the bare `*output.CLIError` when a ≥400 response body carries a non-empty `requestId`. `errors.As(err, &cliErr)` still resolves the inner `*output.CLIError` via Unwrap, so `Execute` (root.go:53) and the render command's hint refinement (render.go:405) keep working unchanged.

- [ ] **Step 1: Write the failing tests**

Create `internal/api/render_id_test.go` (package `api_test`, mirroring `http_client_test.go`):

```go
package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestRender_SyncSuccess_CapturesRenderIDHeader(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{"application/json"},
			"X-Urlbox-Request-Id": []string{"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps"},
		},
		Body: `{"renderUrl":"https://renders.urlbox.com/x.png","size":17752}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.RenderID != "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps" {
		t.Fatalf("RenderID=%q", resp.RenderID)
	}
	if _, ok := resp.Data["renderId"]; ok {
		t.Fatalf("renderId must NOT be injected into Data: %v", resp.Data)
	}
}

func TestRender_SyncSuccess_NoHeader_RenderIDEmpty(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"renderUrl":"https://renders.urlbox.com/x.png","size":1}`))
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.RenderID != "" {
		t.Fatalf("RenderID=%q, want empty", resp.RenderID)
	}
}

func TestRender_Failure_WrapsRenderIDError(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	c.Retry = api.NoRetryConfig()
	_, err := c.Render(context.Background(), map[string]any{"url": "https://x.invalid"})
	var ridErr *api.RenderIDError
	if !errors.As(err, &ridErr) {
		t.Fatalf("want RenderIDError, got %T: %v", err, err)
	}
	if ridErr.RenderID != "01a09082-25cf-74bd-9188-022c4f5bf25f_ps" {
		t.Fatalf("RenderID=%q", ridErr.RenderID)
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("errors.As must still reach the CLIError through Unwrap")
	}
	if cliErr.Code != output.ErrValidation {
		t.Fatalf("code=%q, want validation (InvalidURLError)", cliErr.Code)
	}
}

func TestRender_Failure_NoRequestID_PlainCLIError(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusUnauthorized,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid token","code":"ApiKeyInvalid"}}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	c.Retry = api.NoRetryConfig()
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var ridErr *api.RenderIDError
	if errors.As(err, &ridErr) {
		t.Fatalf("no requestId in body must yield a plain CLIError, got RenderIDError")
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != output.ErrAuth {
		t.Fatalf("want auth CLIError, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/api/ -run 'RenderID|CapturesRenderIDHeader' -v`
Expected: FAIL — `resp.RenderID undefined` / `api.RenderIDError` undefined (compile errors count as the failing state).

- [ ] **Step 3: Implement**

`internal/api/types.go` — add the field to `Response` (after `Hint`, line 42):

```go
	// RenderID is the render id the API surfaced OUTSIDE the body — the
	// x-urlbox-request-id response header. Sync render bodies carry no id,
	// so this is the only place a sync render's id exists. In-memory only:
	// json:"-" keeps envelopes unchanged.
	RenderID string `json:"-"`
```

`internal/api/http_client.go` — in `do`, replace the ≥400 branch (line 133-135):

```go
	if resp.StatusCode >= 400 {
		cliErr := mapStatusToCLIError(resp, respBody)
		if rid := extractRequestID(respBody); rid != "" {
			return nil, &RenderIDError{Err: cliErr, RenderID: rid}
		}
		return nil, cliErr
	}
```

and set the field when building the success Response (line 171):

```go
	return &Response{OK: true, Data: data, RenderID: resp.Header.Get("x-urlbox-request-id")}, nil
```

Add next to `extractAPIError` (line 299):

```go
// extractRequestID reads the requestId field Urlbox error bodies carry
// alongside the error object. Returns "" for non-JSON or absent field.
func extractRequestID(body []byte) string {
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	rid, _ := parsed["requestId"].(string)
	return rid
}

// RenderIDError decorates a CLIError from a failed render call with the
// render id the error body carried, so the render command can offer
// `urlbox report <id>` for the failed render. Unwrap keeps errors.As
// resolution to *output.CLIError intact everywhere else.
type RenderIDError struct {
	Err      *output.CLIError
	RenderID string
}

func (e *RenderIDError) Error() string { return e.Err.Error() }

func (e *RenderIDError) Unwrap() error { return e.Err }
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/api/ -v`
Expected: PASS (all api tests, not just the new ones — the changed error return must not break existing render/status error tests).

- [ ] **Step 5: `make ci`, commit**

```bash
make ci
git add internal/api/types.go internal/api/http_client.go internal/api/render_id_test.go
git commit -m "feat(api): capture render id from header and error bodies"
```

---

### Task 2: Post-render hint line

**Files:**
- Create: `internal/cmd/report_hint.go`
- Create: `internal/cmd/report_hint_test.go`
- Modify: `internal/cmd/render.go` (error branch line 402-412, envelope write line 445-453)

**Interfaces:**
- Consumes: `api.Response.RenderID`, `api.RenderIDError` (Task 1); `isStderrTTY` (root.go:26); `output.ResolveFormat`, `output.NewStylesForWriter`.
- Produces: `reportHintLine(renderID string) string`; `printReportHint(cmd *cobra.Command, renderID string)` (gated, no-op when gates fail); `renderIDForHint(resp *api.Response) string`; `appendReportHint(cmd *cobra.Command, err error) error` (mutates the wrapped CLIError's Hint when gates pass, always returns err unchanged).

- [ ] **Step 1: Write the failing tests**

Create `internal/cmd/report_hint_test.go`. The harness idiom is `storage_test.go`: temp XDG config, `apitest` server, full `Execute`. Render needs only the api_secret, so `writeCompatConfig(t, dir, false)` suffices; text mode is forced with `--output-format text` and stderr TTY with `SetStderrTTYForTest(true)`.

```go
package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const syncSuccessBody = `{"renderUrl":"https://renders.urlbox.com/x.png","size":17752}`

func syncSuccessWithHeader() apitest.ScriptedResponse {
	return apitest.ScriptedResponse{
		Status: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{"application/json"},
			"X-Urlbox-Request-Id": []string{"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps"},
		},
		Body: syncSuccessBody,
	}
}

func runRenderForHint(t *testing.T, script apitest.ScriptedResponse, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(script)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestRenderHint_SyncSuccess_TextTTY_PrintsHintToStderr(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "urlbox report 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps") {
		t.Fatalf("stderr missing hint: %q", stderr)
	}
	if strings.Contains(stdout, "urlbox report") {
		t.Fatalf("hint leaked to stdout: %q", stdout)
	}
}

func TestRenderHint_SyncSuccess_NonTTYStderr_NoHint(t *testing.T) {
	SetStderrTTYForTest(false)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("hint must be absent off-TTY: %q", stderr)
	}
}

func TestRenderHint_JSONMode_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") || strings.Contains(stdout, "urlbox report") {
		t.Fatalf("hint must be absent in json mode\nstdout=%q\nstderr=%q", stdout, stderr)
	}
	if strings.Contains(stdout, "renderId") {
		t.Fatalf("sync json envelope must not gain a renderId field: %q", stdout)
	}
}

func TestRenderHint_QuietMode_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "quiet")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stdout+stderr, "urlbox report") {
		t.Fatalf("hint must be absent in quiet mode")
	}
}

func TestRenderHint_SyncSuccess_NoHeader_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t, apitest.SuccessJSON(syncSuccessBody),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("no id captured, no hint: %q", stderr)
	}
}

func TestRenderHint_Async_UsesBodyRenderID(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t,
		apitest.SuccessJSON(`{"status":"created","renderId":"01a09072-9ec6-76ca-bcc7-bbf5cc35a9e4_pa","statusUrl":"https://api.urlbox.com/v1/render/x"}`),
		"render", "https://example.com", "--async", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr, "urlbox report 01a09072-9ec6-76ca-bcc7-bbf5cc35a9e4_pa") {
		t.Fatalf("async hint missing: %q", stderr)
	}
}

func TestRenderHint_Failure_TextTTY_HintUnderError(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	}
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://x.invalid", "--output-format", "text", "--no-retry")
	if code != 2 {
		t.Fatalf("exit %d, want 2 (validation)\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "urlbox report 01a09082-25cf-74bd-9188-022c4f5bf25f_ps") {
		t.Fatalf("failure hint missing from stderr: %q", stderr)
	}
}

func TestRenderHint_Failure_JSONMode_NoHintInEnvelope(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	}
	stdout, _, code := runRenderForHint(t, fail,
		"render", "https://x.invalid", "--output-format", "json", "--no-retry")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if strings.Contains(stdout, "urlbox report") {
		t.Fatalf("json error envelope must not carry the report hint: %q", stdout)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run RenderHint -v`
Expected: FAIL — hint assertions unmet (success cases print no hint; failure case hint absent).

- [ ] **Step 3: Implement**

Create `internal/cmd/report_hint.go`:

```go
package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// reportHintLine is the post-render affordance: one ready-to-run command
// carrying the render id. Copy is Arnold's; change only with his sign-off.
func reportHintLine(renderID string) string {
	return "Something wrong with this render? Report it: urlbox report " + renderID
}

// renderIDForHint picks the id the hint should name: async bodies carry
// renderId; sync bodies carry nothing, so the header-captured Response
// field is the fallback.
func renderIDForHint(resp *api.Response) string {
	if id, ok := resp.Data["renderId"].(string); ok && id != "" {
		return id
	}
	return resp.RenderID
}

// hintGatesPass centralises the affordance rulings: text output only,
// stderr TTY only. json/quiet stay byte-identical for agents.
func hintGatesPass(cmd *cobra.Command) bool {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	if output.ResolveFormat(formatFlag, cmd.OutOrStdout()) != output.FormatText {
		return false
	}
	return isStderrTTY(cmd.ErrOrStderr())
}

// printReportHint writes the muted hint line to stderr after a render
// result. No-op when any gate fails or no id was captured.
func printReportHint(cmd *cobra.Command, renderID string) {
	if renderID == "" || !hintGatesPass(cmd) {
		return
	}
	styles := output.NewStylesForWriter(cmd.ErrOrStderr())
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), styles.Muted.Render("  "+reportHintLine(renderID)))
}

// appendReportHint rides the failed render's existing Hint (printed under
// the Error: line in text mode) so the report affordance lands below the
// error rather than above it. The mutation only happens when the text+TTY
// gates pass, so json/quiet error envelopes never carry it.
func appendReportHint(cmd *cobra.Command, err error) error {
	var ridErr *api.RenderIDError
	if !errors.As(err, &ridErr) || ridErr.RenderID == "" || !hintGatesPass(cmd) {
		return err
	}
	if ridErr.Err.Hint != "" {
		ridErr.Err.Hint += "\n"
	}
	ridErr.Err.Hint += reportHintLine(ridErr.RenderID)
	return err
}
```

Modify `internal/cmd/render.go`. The error branch (line 402-412) gets one added line before `return err`:

```go
	if err != nil {
		var cli *output.CLIError
		if errors.As(err, &cli) {
			if cli.Code == output.ErrTimeout || cli.Code == output.ErrNetwork {
				cli.Hint = networkHint(errors.New(cli.Message), true, f.timeout)
			}
		}
		return appendReportHint(cmd, err)
	}
```

The success tail (line 445-453) becomes:

```go
	env := output.NewEnvelope(
		"render",
		resp.Data,
		summariseRenderResp(resp),
		breadcrumbsForResp(resp, f),
	)
	env.Warnings = warnings
	werr := writeRenderEnvelope(cmd, env)
	printReportHint(cmd, renderIDForHint(resp))
	return werr
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/cmd/ -run RenderHint -v` then `go test ./internal/cmd/`
Expected: PASS, including every pre-existing render/status test (the envelope and json/quiet bytes are unchanged).

- [ ] **Step 5: `make ci`, commit**

```bash
make ci
git add internal/cmd/report_hint.go internal/cmd/report_hint_test.go internal/cmd/render.go
git commit -m "feat(render): report hint line on every render response"
```

---

### Task 3: prompt.TextInput

**Files:**
- Modify: `internal/prompt/prompt.go`
- Create: `internal/prompt/textinput_test.go`

**Interfaces:**
- Produces: `prompt.TextInput(title string, validate func(string) error) (string, error)` — huh single-line input on stderr, trimmed result; `validate` (nil-able) runs inside huh so the user retries in place; returns `prompt.ErrNotInteractive` off-TTY.

- [ ] **Step 1: Write the failing test**

Create `internal/prompt/textinput_test.go` (test binaries have no TTY stdin, so the reachable path is the guard):

```go
package prompt

import (
	"errors"
	"testing"
)

func TestTextInput_NonInteractive_ReturnsErrNotInteractive(t *testing.T) {
	_, err := TextInput("Add a short description:", nil)
	if !errors.Is(err, ErrNotInteractive) {
		t.Fatalf("want ErrNotInteractive, got %v", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/prompt/ -run TextInput -v`
Expected: FAIL — `TextInput` undefined.

- [ ] **Step 3: Implement**

Append to `internal/prompt/prompt.go` (same shape as `TypeToConfirm`, line 76):

```go
// TextInput draws a single-line text input to stderr and returns the
// trimmed value. validate (optional) runs inside the form so the user can
// correct in place. It returns ErrNotInteractive when stdin is not a
// terminal.
func TextInput(title string, validate func(string) error) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // file descriptors fit in int on every platform Go supports
		return "", ErrNotInteractive
	}
	var typed string
	input := huh.NewInput().Title(title).Value(&typed)
	if validate != nil {
		input = input.Validate(validate)
	}
	if err := input.WithTheme(theme()).Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(typed), nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/prompt/ -v`
Expected: PASS.

- [ ] **Step 5: `make ci`, commit**

```bash
make ci
git add internal/prompt/prompt.go internal/prompt/textinput_test.go
git commit -m "feat(prompt): TextInput single-line prompt"
```

---

### Task 4: `urlbox report` — flag path, errors, output

**Files:**
- Create: `internal/cmd/report.go`
- Create: `internal/cmd/report_test.go`
- Modify: `internal/cmd/root.go` (registration, after `newRenderCmd()` line 239)
- Modify: `SURFACE.txt` (via `make surface-snapshot`)

**Interfaces:**
- Consumes: `loadSession`, `requireActiveOrg`, `attachSessionRetryFlags` (session_helpers.go); `asCLIError` (login_resolve.go:155); `writeEnvelopeWithQuietData` (config.go:638); `valueOrEmpty`; `interactiveText` (projects.go:538).
- Produces: `newReportCmd() *cobra.Command`; `reportCategories` (ordered slice of `{value, label}` pairs — Task 5's picker reuses it); `renderReportsPath(org string) string` returning `"/v2/organisation/" + org + "/render-reports"`; `runReport` split so Task 5 only fills the interactive branch stub `resolveReportInputs`.

- [ ] **Step 1: Write the failing tests**

Create `internal/cmd/report_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const reportDTOJSON = `{"id":"rpt_x1","renderId":"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps","category":"bot-detection","comment":"page shows a captcha","rects":[],"state":"open","outputUrl":null,"outputFormat":null,"resolutionMessage":null,"resolutionExampleOptions":null,"resolutionExampleUrl":null,"hasUnseenResolution":false,"createdAt":"2026-09-11T12:00:00.000Z"}`

func runReport(t *testing.T, scripts []apitest.ScriptedResponse, args ...string) (stdout, stderr string, code int, srv *apitest.Server) {
	t.Helper()
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv = apitest.New(scripts...)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code, srv
}

func TestReport_FlagsPath_PostsAndSummarises(t *testing.T) {
	stdout, stderr, code, srv := runReport(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "bot-detection", "--comment", "page shows a captcha",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/render-reports" {
		t.Fatalf("request: %+v", reqs[0])
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["renderId"] != "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps" ||
		body["category"] != "bot-detection" ||
		body["comment"] != "page shows a captcha" {
		t.Fatalf("body: %v", body)
	}
	if _, present := body["rects"]; present {
		t.Fatalf("rects must not be sent: %v", body)
	}
	if !strings.Contains(stdout, "Report filed for render 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps") {
		t.Fatalf("summary missing: %q", stdout)
	}
}

func TestReport_JSONMode_EmitsDTOEnvelope(t *testing.T) {
	stdout, _, code, _ := runReport(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "other", "--comment", "x",
		"--output-format", "json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stdout)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if env["ok"] != true || env["command"] != "report" {
		t.Fatalf("envelope: %v", env)
	}
	data, _ := env["data"].(map[string]any)
	if data["id"] != "rpt_x1" || data["state"] != "open" {
		t.Fatalf("data: %v", data)
	}
}

func TestReport_QuietMode_EmitsReportID(t *testing.T) {
	stdout, _, code, _ := runReport(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "other", "--comment", "x",
		"--output-format", "quiet")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(stdout) != `"rpt_x1"` {
		t.Fatalf("quiet stdout=%q, want the report id", stdout)
	}
}

func TestReport_InvalidCategory_UsageErrorListsValues(t *testing.T) {
	_, stderr, code, _ := runReport(t, nil,
		"report", "01a0_ps", "--category", "captcha", "--comment", "x",
		"--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (usage)", code)
	}
	for _, want := range []string{"bot-detection", "login-required", "missing-content", "cookie-banner-or-popup", "render-failed", "other"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("error must list %q: %q", want, stderr)
		}
	}
}

func TestReport_EmptyComment_UsageError(t *testing.T) {
	_, _, code, _ := runReport(t, nil,
		"report", "01a0_ps", "--category", "other", "--comment", "   ",
		"--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestReport_MissingFlags_NonInteractive_UsageError(t *testing.T) {
	stdout, _, code, _ := runReport(t, nil,
		"report", "01a0_ps", "--output-format", "json")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (never hangs)", code)
	}
	if !strings.Contains(stdout, `"usage"`) || !strings.Contains(stdout, "--category") {
		t.Fatalf("json error envelope must name the missing flags: %q", stdout)
	}
}

func TestReport_NotLoggedIn_AuthError(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out, errBuf bytes.Buffer
	code := Execute([]string{"report", "01a0_ps", "--category", "other", "--comment", "x", "--output-format", "text"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit %d, want 3 (auth)", code)
	}
	if !strings.Contains(errBuf.String(), "urlbox login") {
		t.Fatalf("auth error must carry the login hint: %q", errBuf.String())
	}
}

func TestReport_RenderNotFound_Exit5WithExpiryHint(t *testing.T) {
	notFound := apitest.ScriptedResponse{
		Status: http.StatusNotFound,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"defined":false,"code":"NOT_FOUND","status":404,"message":"Render not found"}`,
	}
	_, stderr, code, _ := runReport(t, []apitest.ScriptedResponse{notFound},
		"report", "01a0906a-gone_ps", "--category", "other", "--comment", "x",
		"--output-format", "text")
	if code != 5 {
		t.Fatalf("exit %d, want 5", code)
	}
	if !strings.Contains(stderr, "Render not found") {
		t.Fatalf("API message must surface: %q", stderr)
	}
	if !strings.Contains(stderr, "60 days") {
		t.Fatalf("hint must mention render expiry: %q", stderr)
	}
}

func TestReport_LockedReport_Exit7SurfacesAPIMessage(t *testing.T) {
	locked := apitest.ScriptedResponse{
		Status: http.StatusConflict,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"defined":false,"code":"CONFLICT","status":409,"message":"This report is being looked at by the team and can no longer be edited."}`,
	}
	_, stderr, code, _ := runReport(t, []apitest.ScriptedResponse{locked},
		"report", "01a0_ps", "--category", "other", "--comment", "x",
		"--output-format", "text")
	if code != 7 {
		t.Fatalf("exit %d, want 7", code)
	}
	if !strings.Contains(stderr, "being looked at by the team") {
		t.Fatalf("API conflict message must surface: %q", stderr)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run TestReport_ -v`
Expected: FAIL — `unknown command "report"`.

- [ ] **Step 3: Implement**

Create `internal/cmd/report.go`:

```go
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// reportCategory pairs the API enum value with the human label the
// interactive picker shows. Order is the picker order. Labels are
// customer-facing copy — Arnold's.
type reportCategory struct {
	value string
	label string
}

// reportCategories is the closed category set from the mono render-report
// contract (packages/domain/src/render-report/render-report.schemas.ts).
var reportCategories = []reportCategory{
	{value: "bot-detection", label: "Bot detection / blocked"},
	{value: "login-required", label: "Login required"},
	{value: "missing-content", label: "Missing content"},
	{value: "cookie-banner-or-popup", label: "Cookie banner or popup"},
	{value: "render-failed", label: "Render failed"},
	{value: "other", label: "Other"},
}

func categoryValues() []string {
	vals := make([]string, len(reportCategories))
	for i, c := range reportCategories {
		vals[i] = c.value
	}
	return vals
}

func isValidCategory(v string) bool {
	for _, c := range reportCategories {
		if c.value == v {
			return true
		}
	}
	return false
}

const maxReportCommentLen = 2000

// renderReportsPath builds the org-scoped create/list path, matching the
// credkind orgListPath convention.
func renderReportsPath(org string) string {
	return "/v2/organisation/" + org + "/render-reports"
}

func newReportCmd() *cobra.Command {
	var category, comment string
	c := &cobra.Command{
		Use:   "report <renderId>",
		Short: "Report a problem with a render",
		Long: `File a render report with the Urlbox team.

The render id is printed after every render, and async responses carry it
as renderId. Requires a signed-in session (urlbox login) with an active
organisation. Reports are limited to a category plus a short comment; the
team follows up via support.

Without --category/--comment in an interactive terminal, the command
prompts for the missing values. In non-interactive use both flags are
required.

Exit codes:
  0   report filed
  1   usage (missing/invalid category or comment in non-interactive use)
  3   not logged in
  5   render not found (expired or not from your organisation)
  7   the report is already with the team and locked`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, args[0], category, comment)
		},
	}
	c.Flags().StringVar(&category, "category", "", "Problem category: "+strings.Join(categoryValues(), ", "))
	c.Flags().StringVar(&comment, "comment", "", "Short description of what went wrong (required with --category)")
	attachSessionRetryFlags(c)
	return c
}

func runReport(cmd *cobra.Command, renderID, category, comment string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	category, comment, inErr := resolveReportInputs(cmd, category, comment)
	if inErr != nil {
		return inErr
	}
	body := map[string]any{
		"renderId": renderID,
		"category": category,
		"comment":  comment,
	}
	ctx := context.Background()
	var created map[string]any
	if err := sess.Client.PostJSON(ctx, renderReportsPath(org), body, &created); err != nil {
		return refineReportError(err)
	}
	env := output.NewEnvelope("report", created,
		fmt.Sprintf("Report filed for render %s", renderID), nil)
	return writeEnvelopeWithQuietData(cmd, env, valueOrEmpty(created["id"]))
}

// resolveReportInputs validates flag-provided values and fills missing ones
// interactively (text mode + TTY). Task 5 wires the prompts; until then the
// interactive branch behaves like non-interactive.
func resolveReportInputs(cmd *cobra.Command, category, comment string) (string, string, *output.CLIError) {
	if category != "" && !isValidCategory(category) {
		return "", "", output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("unknown --category %q", category),
			"Use one of: "+strings.Join(categoryValues(), ", ")+".")
	}
	comment = strings.TrimSpace(comment)
	if len(comment) > maxReportCommentLen {
		return "", "", output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("--comment is too long (%d chars, max %d)", len(comment), maxReportCommentLen),
			"Shorten the comment; detail can follow via urlbox support.")
	}
	if category != "" && comment != "" {
		return category, comment, nil
	}
	if interactiveText(cmd) {
		return promptReportInputs(cmd, category, comment)
	}
	return "", "", missingReportFlagsError(category, comment)
}

func missingReportFlagsError(category, comment string) *output.CLIError {
	missing := []string{}
	if category == "" {
		missing = append(missing, "--category")
	}
	if comment == "" {
		missing = append(missing, "--comment")
	}
	return output.NewCLIError(output.ErrUsage,
		"missing "+strings.Join(missing, " and "),
		"Pass --category ("+strings.Join(categoryValues(), ", ")+") and --comment, or run in an interactive terminal.")
}

// promptReportInputs is completed in the interactive task; the flag path
// never reaches it with both values set.
func promptReportInputs(cmd *cobra.Command, category, comment string) (string, string, *output.CLIError) {
	return "", "", missingReportFlagsError(category, comment)
}

// refineReportError swaps the generic 404/409 hints (written for render
// lookups) for report-specific recovery text. Everything else passes
// through asCLIError untouched.
func refineReportError(err error) error {
	cli := asCLIError(err)
	switch cli.Code {
	case output.ErrNotFound:
		cli.Hint = "Renders are kept for around 60 days and must belong to your active organisation. Check the id against a recent render."
	case output.ErrConflict:
		cli.Hint = "The team already has this report in review. Use `urlbox support` if you need to add something."
	}
	return cli
}
```

Register in `internal/cmd/root.go` after `newRenderCmd()` (line 239):

```go
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newReportCmd())
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/cmd/ -run TestReport_ -v` then `go test ./internal/cmd/`
Expected: PASS. (`TestReport_MissingFlags_NonInteractive_UsageError` passes already because the interactive stub falls through to the usage error — Task 5 upgrades it.)

- [ ] **Step 5: Surface snapshot, `make ci`, commit**

```bash
make surface-snapshot
make ci
git add internal/cmd/report.go internal/cmd/report_test.go internal/cmd/root.go SURFACE.txt
git commit -m "feat(report): urlbox report command, flag path"
```

Expected new SURFACE.txt entries: `urlbox report <renderId>` plus `--agent --category --comment --jq --max-retries --no-retry --output-format --profile` lines.

---

### Task 5: `urlbox report` — interactive path

**Files:**
- Modify: `internal/cmd/report.go` (replace the `promptReportInputs` stub)
- Modify: `internal/cmd/report_test.go` (add interactive tests)

**Interfaces:**
- Consumes: `promptPick` (session_helpers.go:153), `pickFunc` (login_resolve.go:13), `errNotInteractivePick` (login_resolve.go:15), `prompt.TextInput` (Task 3), `reportCategories` (Task 4).
- Produces: package vars `reportCategoryPick pickFunc` and `reportCommentInput func(string, func(string) error) (string, error)` with `SetReportPromptsForTest(pick pickFunc, input func(string, func(string) error) (string, error))` / `ResetReportPromptsForTest()`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/cmd/report_test.go`:

```go
func TestReport_Interactive_PromptsForCategoryAndComment(t *testing.T) {
	SetReportPromptsForTest(
		func(label string, options []string, active int) (int, error) {
			if len(options) != 6 {
				t.Fatalf("picker options: %v", options)
			}
			return 0, nil
		},
		func(title string, validate func(string) error) (string, error) {
			if validate == nil {
				t.Fatal("comment input must validate non-empty")
			}
			if err := validate(""); err == nil {
				t.Fatal("empty comment must fail validation")
			}
			return "page shows a captcha", nil
		},
	)
	t.Cleanup(ResetReportPromptsForTest)
	stdout, stderr, code, srv := runReport(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	var body map[string]any
	if err := json.Unmarshal(srv.Requests()[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["category"] != "bot-detection" || body["comment"] != "page shows a captcha" {
		t.Fatalf("prompted values not sent: %v", body)
	}
}

func TestReport_Interactive_FlagCategoryOnlyPromptsComment(t *testing.T) {
	picked := false
	SetReportPromptsForTest(
		func(label string, options []string, active int) (int, error) {
			picked = true
			return 0, nil
		},
		func(title string, validate func(string) error) (string, error) {
			return "left half is blank", nil
		},
	)
	t.Cleanup(ResetReportPromptsForTest)
	_, _, code, srv := runReport(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0_ps", "--category", "missing-content",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if picked {
		t.Fatal("category picker must not run when --category was given")
	}
	var body map[string]any
	if err := json.Unmarshal(srv.Requests()[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["category"] != "missing-content" || body["comment"] != "left half is blank" {
		t.Fatalf("body: %v", body)
	}
}

func TestReport_Interactive_NonTTYStdin_FallsBackToUsage(t *testing.T) {
	ResetReportPromptsForTest()
	_, stderr, code, _ := runReport(t, nil,
		"report", "01a0_ps", "--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (prompt.ErrNotInteractive path)", code)
	}
	if !strings.Contains(stderr, "--category") {
		t.Fatalf("usage error must name the flags: %q", stderr)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run TestReport_Interactive -v`
Expected: FAIL — `SetReportPromptsForTest` undefined.

- [ ] **Step 3: Implement**

In `internal/cmd/report.go`, replace the `promptReportInputs` stub and add the injection vars:

```go
var (
	reportCategoryPick pickFunc                                          = promptPick
	reportCommentInput func(string, func(string) error) (string, error) = prompt.TextInput
)

// SetReportPromptsForTest swaps both report prompts. Pair with
// t.Cleanup(ResetReportPromptsForTest).
func SetReportPromptsForTest(pick pickFunc, input func(string, func(string) error) (string, error)) {
	reportCategoryPick = pick
	reportCommentInput = input
}

// ResetReportPromptsForTest restores the production prompts.
func ResetReportPromptsForTest() {
	reportCategoryPick = promptPick
	reportCommentInput = prompt.TextInput
}

func promptReportInputs(cmd *cobra.Command, category, comment string) (string, string, *output.CLIError) {
	if category == "" {
		labels := make([]string, len(reportCategories))
		for i, c := range reportCategories {
			labels[i] = c.label
		}
		idx, err := reportCategoryPick("What went wrong with this render?", labels, -1)
		if err != nil {
			return "", "", missingReportFlagsError(category, comment)
		}
		category = reportCategories[idx].value
	}
	if comment == "" {
		typed, err := reportCommentInput("Add a short description:", func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("a short description is required")
			}
			if len(s) > maxReportCommentLen {
				return fmt.Errorf("keep it under %d characters", maxReportCommentLen)
			}
			return nil
		})
		if err != nil {
			return "", "", missingReportFlagsError(category, comment)
		}
		comment = typed
	}
	return category, comment, nil
}
```

Add `"errors"` and `"github.com/urlbox/urlbox-cli/internal/prompt"` to report.go's imports.

Note: `promptPick` maps `prompt.ErrNotInteractive` to `errNotInteractivePick`; both prompt error paths intentionally collapse to the same usage error the non-interactive branch produces, so a piped stdin can never hang or half-file.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/cmd/ -run TestReport_ -v`
Expected: PASS, including the Task 4 tests unchanged.

- [ ] **Step 5: `make ci`, commit**

```bash
make ci
git add internal/cmd/report.go internal/cmd/report_test.go
git commit -m "feat(report): interactive category and comment prompts"
```

---

### Task 6: `urlbox support`

**Files:**
- Create: `internal/cmd/support.go`
- Create: `internal/cmd/support_test.go`
- Modify: `internal/cmd/root.go` (registration, after `newStorageCmd()` line 244)
- Modify: `SURFACE.txt` (via `make surface-snapshot`)

**Interfaces:**
- Consumes: `browser.Opener` / `browser.NewOSOpener` (internal/browser/opener.go), `detectHeadless` (dashboard.go:54), `writeEnvelope` (config.go:623).
- Produces: `newSupportCmd() *cobra.Command`; test hooks `SetSupportOpenerForTest` / `ResetSupportOpenerForTest` (own opener var, per the dashboard no-cross-contamination note at dashboard.go:20-24).

- [ ] **Step 1: Write the failing tests**

Create `internal/cmd/support_test.go`:

```go
package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSupportOpener struct{ opened []string }

func (f *fakeSupportOpener) Open(url string) error {
	f.opened = append(f.opened, url)
	return nil
}

func TestSupport_TextMode_OpensContactPage(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(ResetHeadlessDetectorForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if len(fake.opened) != 1 || fake.opened[0] != "https://urlbox.com/contact" {
		t.Fatalf("opened: %v", fake.opened)
	}
}

func TestSupport_JSONMode_NoBrowserEmitsEnvelope(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(fake.opened) != 0 {
		t.Fatalf("json mode must not launch a browser: %v", fake.opened)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if env["command"] != "support" || data["url"] != "https://urlbox.com/contact" {
		t.Fatalf("envelope: %v", env)
	}
}

func TestSupport_Headless_PrintsURLToStderr(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	SetHeadlessDetectorForTest(func() bool { return true })
	t.Cleanup(ResetHeadlessDetectorForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(fake.opened) != 0 {
		t.Fatalf("headless must not launch: %v", fake.opened)
	}
	if !strings.Contains(stderr.String(), "https://urlbox.com/contact") {
		t.Fatalf("stderr must carry the URL: %q", stderr.String())
	}
}

func TestSupport_QuietMode_EmitsBareURL(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "quiet"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(stdout.String()) != "https://urlbox.com/contact" {
		t.Fatalf("quiet stdout=%q", stdout.String())
	}
	if len(fake.opened) != 0 {
		t.Fatalf("quiet mode must not launch: %v", fake.opened)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/cmd/ -run TestSupport_ -v`
Expected: FAIL — `unknown command "support"`.

- [ ] **Step 3: Implement**

Create `internal/cmd/support.go` (mirror of dashboard.go with its own opener):

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// supportURL is the Urlbox contact page — the canonical support entry
// point (ruled 2026-09-11; /support has no dedicated page).
const supportURL = "https://urlbox.com/contact"

// supportOpener is the browser opener used by the support command. Own var
// so test injection doesn't cross-contaminate dashboard's.
var supportOpener browser.Opener = browser.NewOSOpener()

// SetSupportOpenerForTest swaps in a fake browser.Opener. Pair with
// t.Cleanup(ResetSupportOpenerForTest).
func SetSupportOpenerForTest(o browser.Opener) { supportOpener = o }

// ResetSupportOpenerForTest restores the production OSOpener.
func ResetSupportOpenerForTest() { supportOpener = browser.NewOSOpener() }

func newSupportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "support",
		Short: "Open the Urlbox support contact page in your browser",
		Long: `Opens https://urlbox.com/contact in your default browser.

On headless environments the URL is printed to stderr instead. The
standard envelope is still emitted on stdout so agents and pipelines can
rely on the same shape regardless of host.

Exit codes:
  0   browser launched, or URL printed in headless mode
  10  the OS browser handler returned an error (URL is in the hint)`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSupport(c)
		},
	}
}

func runSupport(c *cobra.Command) error {
	data := map[string]any{"url": supportURL}
	formatFlag, _ := c.Root().PersistentFlags().GetString("output-format")
	resolvedFormat := output.ResolveFormat(formatFlag, c.OutOrStdout())
	if resolvedFormat == output.FormatJSON || resolvedFormat == output.FormatQuiet {
		return writeSupportEnvelope(c, data,
			"Support URL emitted (no browser launched in machine-readable mode)",
			[]output.Breadcrumb{{Action: "copy", Cmd: supportURL}})
	}
	if isHeadless() {
		_, _ = fmt.Fprintln(c.ErrOrStderr(),
			"Support URL: "+supportURL+"  (open in any browser)")
		return writeSupportEnvelope(c, data,
			"Support URL printed (no graphical session detected)",
			[]output.Breadcrumb{{Action: "copy", Cmd: supportURL}})
	}
	if err := supportOpener.Open(supportURL); err != nil {
		return output.NewCLIError(
			output.ErrServer,
			"Failed to open browser: "+err.Error(),
			"Open this URL manually: "+supportURL,
		)
	}
	return writeSupportEnvelope(c, data,
		"Opened "+supportURL,
		[]output.Breadcrumb{{Action: "browse", Cmd: supportURL}})
}

// writeSupportEnvelope mirrors writeDashboardEnvelope: quiet emits the
// bare URL for pipelines, everything else the standard envelope.
func writeSupportEnvelope(c *cobra.Command, data map[string]any, summary string, breadcrumbs []output.Breadcrumb) error {
	formatFlag, _ := c.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := c.Root().PersistentFlags().GetString("jq")
	stdout := c.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if format == output.FormatQuiet && jqExpr == "" {
		_, err := fmt.Fprintln(stdout, supportURL)
		return err
	}
	env := output.NewEnvelope("support", data, summary, breadcrumbs)
	return writeEnvelope(c, env)
}
```

Register in `internal/cmd/root.go` after `newStorageCmd()` (line 244):

```go
	cmd.AddCommand(newStorageCmd())
	cmd.AddCommand(newSupportCmd())
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/cmd/ -run TestSupport_ -v` then `go test ./internal/cmd/`
Expected: PASS.

- [ ] **Step 5: Surface snapshot, `make ci`, commit**

```bash
make surface-snapshot
make ci
git add internal/cmd/support.go internal/cmd/support_test.go internal/cmd/root.go SURFACE.txt
git commit -m "feat(support): urlbox support opens the contact page"
```

Expected new SURFACE.txt entries: `urlbox support` plus `--agent --jq --output-format --profile`.

---

### Task 7: Docs, skill, changelog, final gate

**Files:**
- Modify: `skills/SKILL.md` (add report + support to the agent skill)
- Modify: `README.md` and `npm/README.md` (command list entries)
- Modify: `CHANGELOG.md` (unreleased entry)

**Interfaces:** none new — documentation of Tasks 4-6 surfaces exactly as shipped.

- [ ] **Step 1: Update skills/SKILL.md**

Two rows in the `## Available commands` table (line 151): `urlbox report` after the `urlbox video <url>` row, `urlbox support` after the `urlbox dashboard` row:

```markdown
| `urlbox report <renderId>`       | Report a bad render to the Urlbox team (category + comment) |
| `urlbox support`                 | Open the Urlbox support contact page in the user's browser |
```

Two sections after the `## dashboard: open the Urlbox dashboard` section (line 417), matching its shape:

```markdown
## report: report a bad render

`urlbox report <renderId>` files a render report with the Urlbox team.
Requires a signed-in session — the HUMAN runs `urlbox login` (browser
device flow) once; agents never sign in themselves and the render
secret (`URLBOX_API_SECRET`) cannot file reports.

Non-interactive use requires both flags; the command never prompts when
stdin is not a terminal:

```sh
urlbox report 01a0906a-…_ps --category bot-detection --comment "page shows a captcha" --output-format json --jq '.data.id'
```

`--category` is a closed set: `bot-detection`, `login-required`,
`missing-content`, `cookie-banner-or-popup`, `render-failed`, `other`.
Where the renderId comes from: async render responses carry
`data.renderId`; sync render JSON carries NO id (use `--async` when you
plan to report). One report per render — re-filing while the report is
open edits it in place; once the team picks it up you get exit 7.

Exit codes: 0 filed; 1 missing/invalid category or comment; 3 not
logged in; 5 render not found (expired ~60 days or another org's);
7 report locked (already in review).

## support: open the support contact page

`urlbox support` opens https://urlbox.com/contact. Same envelope contract
as `dashboard`: json/quiet never launch a browser and always carry
`data.url`; headless prints the URL to stderr.
```

- [ ] **Step 2: Update README.md**

In the `| Command | Does |` table under "Account and context" (README.md:128), add after the `login` / `logout` row block, matching row style:

```markdown
| `report <renderId>` | File a render problem report with the Urlbox team (needs `login`) |
| `support` | Open the support contact page in your browser |
```

And a short section after "### Async renders" (README.md:107):

```markdown
### Reporting a bad render

```sh
urlbox report 01a0906a-…_ps                      # interactive: category picker + comment
urlbox report 01a0906a-…_ps --category bot-detection --comment "page shows a captcha"
```

Every render prints its id in a hint line under the result. `report` files
the problem straight to the Urlbox team (requires `urlbox login`); the team
follows up on your report from the dashboard. `urlbox support` opens the
contact page for anything else.
```

`npm/README.md` needs no change — it is a quick-start that does not
enumerate commands (verify this is still true before skipping).

- [ ] **Step 3: CHANGELOG entry**

Add above the `## v1.2.0 — 2026-08-19` heading (CHANGELOG.md:7), matching house style:

```markdown
## Unreleased

**Report a bad render from the CLI.** Every render response now prints a
muted hint line with the render's id and the ready-to-run report command
(interactive terminals only — json/quiet/piped output is byte-identical).
`urlbox report <renderId>` files the report: `--category` + `--comment`
for agents and scripts, an interactive category picker + comment prompt
for humans. Reports need a signed-in session (`urlbox login`); render
secrets cannot file them. `urlbox support` opens the support contact
page, with the dashboard command's headless and machine-readable
behaviour.
```

Version number stays unset (release numbering is Arnold+Gus).

- [ ] **Step 4: Full gate**

```bash
make ci
make surface-snapshot
git status --short   # SURFACE.txt must be clean (no drift from Task 6)
git add skills/SKILL.md README.md CHANGELOG.md
git commit -m "docs: report and support commands"
```

- [ ] **Step 5: Arnold's manual gate (his hands, not the executor's)**

Hand Arnold this checklist verbatim; do not run it for him:

```bash
urlbox login                      # refresh the expired session first
urlbox render https://example.com                       # expect hint line with a _ps id
urlbox render https://example.com --output-format json  # expect NO hint, NO renderId field
urlbox report <id-from-hint>                            # interactive picker + comment
urlbox report <same-id> --category other --comment "edited from CLI"   # upsert edits in place
urlbox report doesnotexist --category other --comment x # expect exit 5, expiry hint
urlbox report <id> --category other --comment x --output-format json   # DTO envelope
urlbox support                                          # browser lands on /contact
```

Note: the first real `urlbox report` against production opens a Crisp
conversation and pings the team Slack — deliberate, once.

After his pass: squash to one commit on latest origin/main (rebase, force-push with lease) — message proposed to Arnold first. STOP. No PR without his explicit ask.
