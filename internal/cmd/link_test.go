// internal/cmd/link_test.go
package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

// PIN: HMAC-SHA256(sec_test_AAA, "url=https%3A%2F%2Fexample.com").
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'url=https%3A%2F%2Fexample.com', hashlib.sha256).hexdigest())"
const f1WantToken = "15d1630f9e28c983166c692788fbf019289ad601c814619944cc63e292c57b82"

// PIN: HMAC-SHA256(sec_test_AAA, "full_page=true&url=https%3A%2F%2Fexample.com&width=1920").
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'full_page=true&url=https%3A%2F%2Fexample.com&width=1920', hashlib.sha256).hexdigest())"
const f2WantToken = "b0bf6c25f0accf3680fcc6879e6e05a1ec9005e863374bcb3bdbd5a579e9cf32"

// PIN: HMAC-SHA256(sec_test_AAA, "url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3Dhello+world%26x%3D1").
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3Dhello+world%26x%3D1', hashlib.sha256).hexdigest())"
const f3WantToken = "aab07c3663ead3a6fbd42ec92a38c6f3c41e0cfa997d93a4849aad818cc50e22"

// PIN: F5 — array option. JSON cookies=["k1=v1","k2=v2"] sorts then expands
// into repeated keys. Canonical query string:
//
//	cookies=k1%3Dv1&cookies=k2%3Dv2&url=https%3A%2F%2Fexample.com
//
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'cookies=k1%3Dv1&cookies=k2%3Dv2&url=https%3A%2F%2Fexample.com', hashlib.sha256).hexdigest())"
const f5WantToken = "771f96b68cfb8177ca2b5bbdc776b2ff1aa872a7a25a5464d8a99120055898a9"

// PIN: F-null — null JSON value serializes as an empty query value (not
// dropped). Canonical query string:
//
//	header=&url=https%3A%2F%2Fexample.com
//
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'header=&url=https%3A%2F%2Fexample.com', hashlib.sha256).hexdigest())"
const fNullWantToken = "6310feff93de5d054e5e4362f41a16d2c4357e88296083b1311d0e58132c7251"

// PIN: F-nested — nested object is JSON-encoded (compact) then URL-encoded
// as a single value. Canonical query string:
//
//	meta=%7B%22foo%22%3A%22bar%22%7D&url=https%3A%2F%2Fexample.com
//
// Re-derive with:
//
//	python3 -c "import hmac,hashlib; print(hmac.new(b'sec_test_AAA', b'meta=%7B%22foo%22%3A%22bar%22%7D&url=https%3A%2F%2Fexample.com', hashlib.sha256).hexdigest())"
const fNestedWantToken = "592b27782a5a09b8e0c2926b230878b16ae5f37bece2761f763c73074e34accb"

func TestLink_Simple_PinnedSignatureAndURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--url", "https://example.com",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Fatalf("ok != true: %v", env["ok"])
	}
	if env["command"] != "link" {
		t.Errorf("command != 'link': %v", env["command"])
	}
	data := env["data"].(map[string]any)
	wantURL := "https://api.urlbox.com/v1/pub_test_KEY/" + f1WantToken + "/png?url=https%3A%2F%2Fexample.com"
	if data["url"] != wantURL {
		t.Errorf("url mismatch:\n got=%s\nwant=%s", data["url"], wantURL)
	}
	if data["token"] != f1WantToken {
		t.Errorf("token mismatch:\n got=%s\nwant=%s", data["token"], f1WantToken)
	}
}

func TestLink_SpecialCharsInURL_PinnedEncoding(t *testing.T) {
	// Fixture F3: ?q=hello world&x=1 inside the URL value.
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--url", "https://example.com/path?q=hello world&x=1",
			"--format", "pdf",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data := env["data"].(map[string]any)
	gotURL := data["url"].(string)
	// PIN exact: scheme + host + /v1/<key>/<token>/<format>?<encoded query>
	wantPrefix := "https://api.urlbox.com/v1/pub_test_KEY/"
	wantSuffix := "/pdf?url=https%3A%2F%2Fexample.com%2Fpath%3Fq%3Dhello+world%26x%3D1"
	if !strings.HasPrefix(gotURL, wantPrefix) {
		t.Errorf("prefix mismatch: %s", gotURL)
	}
	if !strings.HasSuffix(gotURL, wantSuffix) {
		t.Errorf("suffix mismatch: %s", gotURL)
	}
	// Token is between prefix and suffix.
	tok := strings.TrimSuffix(strings.TrimPrefix(gotURL, wantPrefix), wantSuffix)
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64 (sha256 hex)", len(tok))
	}
	// Defence-in-depth: we also know the exact digest for F3.
	if tok != f3WantToken {
		t.Errorf("token = %s, want %s", tok, f3WantToken)
	}
}

func TestLink_FromJSON_DeterministicQueryOrder(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--json", `{"url":"https://example.com","width":1920,"full_page":true}`,
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data := env["data"].(map[string]any)
	gotURL := data["url"].(string)
	// PIN deterministic order: full_page, url, width (alphabetical).
	wantSuffix := "/png?full_page=true&url=https%3A%2F%2Fexample.com&width=1920"
	if !strings.HasSuffix(gotURL, wantSuffix) {
		t.Errorf("query order not deterministic; got: %s", gotURL)
	}
	// Defence-in-depth: pin the exact digest for F2.
	if data["token"] != f2WantToken {
		t.Errorf("token = %v, want %s", data["token"], f2WantToken)
	}
}

func TestLink_MissingURL_UsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != false {
		t.Errorf("ok != false: %v", env["ok"])
	}
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(strings.ToLower(errMsg), "url") {
		t.Errorf("error should mention 'url'; got: %q", errMsg)
	}
	if !strings.Contains(env["hint"].(string), "--url") {
		t.Errorf("hint should mention --url; got: %s", env["hint"])
	}
}

func TestLink_OutputContainsBreadcrumb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--url", "https://example.com",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	bcs, ok := env["breadcrumbs"].([]any)
	if !ok || len(bcs) == 0 {
		t.Fatalf("breadcrumbs missing or empty: %v", env["breadcrumbs"])
	}
	bc := bcs[0].(map[string]any)
	if bc["action"] != "render" {
		t.Errorf("breadcrumb[0].action != 'render': %v", bc["action"])
	}
	if !strings.Contains(bc["cmd"].(string), "urlbox render --json") {
		t.Errorf("breadcrumb[0].cmd should suggest 'urlbox render --json'; got: %v", bc["cmd"])
	}
}

func TestLink_QuietMode_RawURLOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--url", "https://example.com",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "quiet",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	// Quiet: a single line, the bare signed URL (no JSON quotes, no envelope).
	if !strings.HasPrefix(got, "https://api.urlbox.com/v1/pub_test_KEY/") {
		t.Errorf("quiet output unexpected: %s", got)
	}
	if strings.Contains(got, "{") {
		t.Errorf("quiet leaked envelope JSON: %s", got)
	}
	if strings.Contains(got, `"`) {
		t.Errorf("quiet output should be bare URL, not JSON-quoted: %s", got)
	}
}

func TestLink_DoesNotEmitSecretInOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute(
		[]string{
			"link",
			"--url", "https://example.com",
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_super_secret_DO_NOT_LEAK",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if strings.Contains(stdout.String(), "sec_super_secret_DO_NOT_LEAK") {
		t.Fatal("secret leaked to stdout")
	}
	if strings.Contains(stderr.String(), "sec_super_secret_DO_NOT_LEAK") {
		t.Fatal("secret leaked to stderr")
	}
}

func TestLink_ArrayValue_RepeatedKey_PinnedDigest(t *testing.T) {
	// PIN: documents that array options expand to repeated keys, sorted
	// alphabetically by value. See f5WantToken comment for the exact
	// canonical query string.
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--json", `{"url":"https://example.com","cookies":["k1=v1","k2=v2"]}`,
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data := env["data"].(map[string]any)
	wantURL := "https://api.urlbox.com/v1/pub_test_KEY/" + f5WantToken + "/png?cookies=k1%3Dv1&cookies=k2%3Dv2&url=https%3A%2F%2Fexample.com"
	if data["url"] != wantURL {
		t.Errorf("url mismatch:\n got=%s\nwant=%s", data["url"], wantURL)
	}
}

func TestLink_NullValue_PinnedDigest(t *testing.T) {
	// PIN: documents that null serializes as an empty value (header=).
	// See fNullWantToken comment for the exact canonical query string.
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--json", `{"url":"https://example.com","header":null}`,
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data := env["data"].(map[string]any)
	if !strings.Contains(data["url"].(string), fNullWantToken) {
		t.Errorf("expected token %s in url; got %s", fNullWantToken, data["url"])
	}
}

func TestLink_NestedObject_PinnedDigest(t *testing.T) {
	// PIN: documents that nested objects JSON-encode then URL-encode.
	// See fNestedWantToken comment for the exact canonical query string.
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{
			"link",
			"--json", `{"url":"https://example.com","meta":{"foo":"bar"}}`,
			"--api-key", "pub_test_KEY",
			"--api-secret", "sec_test_AAA",
			"--output-format", "json",
		},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data := env["data"].(map[string]any)
	if !strings.Contains(data["url"].(string), fNestedWantToken) {
		t.Errorf("expected token %s in url; got %s", fNestedWantToken, data["url"])
	}
}
