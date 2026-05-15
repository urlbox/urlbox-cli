// internal/output/envelope_test.go
package output_test

import (
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestNewEnvelope_AllFields(t *testing.T) {
	env := output.NewEnvelope("render", map[string]string{"id": "abc"}, "Rendered OK", []output.Breadcrumb{
		{Action: "status", Cmd: "urlbox status abc"},
	})
	if !env.OK {
		t.Error("expected OK to be true")
	}
	if env.Command != "render" {
		t.Errorf("Command = %q, want %q", env.Command, "render")
	}
	if env.Summary != "Rendered OK" {
		t.Errorf("Summary = %q, want %q", env.Summary, "Rendered OK")
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Action != "status" {
		t.Errorf("Breadcrumbs = %+v, want [{status urlbox status abc}]", env.Breadcrumbs)
	}
}

func TestNewEnvelope_OmitsEmptyOptionals(t *testing.T) {
	env := output.NewEnvelope("test", "data", "", nil)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	s := string(b)
	if containsKey(s, "summary") {
		t.Error("expected empty summary to be omitted from JSON")
	}
	if containsKey(s, "breadcrumbs") {
		t.Error("expected nil breadcrumbs to be omitted from JSON")
	}
}

func TestNewErrorEnvelope_AllFields(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrAuth, "unauthorized", "run urlbox auth")
	env := output.NewErrorEnvelope("render", cliErr)
	if env.OK {
		t.Error("expected OK to be false")
	}
	if env.Command != "render" {
		t.Errorf("Command = %q, want %q", env.Command, "render")
	}
	if env.Error != "unauthorized" {
		t.Errorf("Error = %q, want %q", env.Error, "unauthorized")
	}
	if env.Code != "auth" {
		t.Errorf("Code = %q, want %q", env.Code, "auth")
	}
	if env.Hint != "run urlbox auth" {
		t.Errorf("Hint = %q, want %q", env.Hint, "run urlbox auth")
	}
}

func TestNewErrorEnvelope_OmitsEmptyHint(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrServer, "oops", "")
	env := output.NewErrorEnvelope("test", cliErr)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if containsKey(string(b), "hint") {
		t.Error("expected empty hint to be omitted from JSON")
	}
}

func TestEnvelope_JSONRoundTrip(t *testing.T) {
	env := output.NewEnvelope("render", map[string]int{"size": 1024}, "Done", nil)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded output.Envelope
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !decoded.OK || decoded.Command != "render" || decoded.Summary != "Done" {
		t.Errorf("round trip failed: %+v", decoded)
	}
}

func TestErrorEnvelope_JSONRoundTrip(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrUsage, "bad", "fix it")
	env := output.NewErrorEnvelope("render", cliErr)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var decoded output.ErrorEnvelope
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if decoded.OK || decoded.Code != "usage" || decoded.Error != "bad" {
		t.Errorf("round trip failed: %+v", decoded)
	}
}

// containsKey checks if a JSON string contains a given key.
func containsKey(jsonStr, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// ─── v1.0.4 Class 3.1 — Warnings on envelopes ──────────────────────
// Invariant: agent-consumable warnings (fuzzy-typo hints, --json key
// suggestions) travel inside the envelope as a structured field, not
// as plain stderr text alongside the JSON envelope on stdout.

func TestEnvelope_WarningsFieldRoundtrips(t *testing.T) {
	env := output.NewEnvelope("render", map[string]string{"x": "y"}, "ok", nil)
	env.Warnings = []string{`unknown option "fromat" — did you mean "format"?`}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !containsKey(string(b), "warnings") {
		t.Errorf("expected warnings field, got %s", b)
	}
	var back output.Envelope
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(back.Warnings) != 1 || back.Warnings[0] != env.Warnings[0] {
		t.Errorf("warnings round-trip failed: got %#v", back.Warnings)
	}
}

func TestEnvelope_WarningsOmittedWhenEmpty(t *testing.T) {
	env := output.NewEnvelope("render", "ok", "", nil)
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if containsKey(string(b), "warnings") {
		t.Errorf("warnings should be omitempty when empty/nil, got %s", b)
	}
}

func TestErrorEnvelope_WarningsFieldRoundtrips(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrUsage, "bad opt", "fix it")
	env := output.NewErrorEnvelope("render", cliErr)
	env.Warnings = []string{"the --json payload also had unknown key xyz"}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(string(b), "warnings") {
		t.Errorf("expected warnings field on ErrorEnvelope, got %s", b)
	}
	var back output.ErrorEnvelope
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Warnings) != 1 {
		t.Errorf("warnings round-trip failed: %#v", back.Warnings)
	}
}

func TestErrorEnvelope_WarningsOmittedWhenEmpty(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrUsage, "bad", "")
	env := output.NewErrorEnvelope("render", cliErr)
	b, _ := json.Marshal(env)
	if containsKey(string(b), "warnings") {
		t.Errorf("warnings should be omitempty on ErrorEnvelope, got %s", b)
	}
}
