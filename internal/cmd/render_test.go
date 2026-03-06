package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeRenderArgs(t *testing.T) {
	normalized, positional := normalizeArgs([]string{
		"https://example.com",
		"--format",
		"png",
		"--width",
		"1920",
		"--full-page",
		"--dry-run",
	}, map[string]bool{
		"--format": true,
		"--width":  true,
	})

	if len(positional) != 1 || positional[0] != "https://example.com" {
		t.Fatalf("unexpected positional args: %#v", positional)
	}

	expected := []string{"--format", "png", "--width", "1920", "--full-page", "--dry-run"}
	if len(normalized) != len(expected) {
		t.Fatalf("unexpected normalized args length: got %d want %d", len(normalized), len(expected))
	}

	for index, value := range expected {
		if normalized[index] != value {
			t.Fatalf("unexpected normalized arg at %d: got %q want %q", index, normalized[index], value)
		}
	}
}

func TestValidatePayloadSuggestsKnownField(t *testing.T) {
	_, err := validatePayload(
		map[string]interface{}{
			"url":    "https://example.com",
			"fromat": "png",
		},
		map[string]interface{}{
			"url":    map[string]interface{}{"type": "string"},
			"format": map[string]interface{}{"type": "string"},
		},
	)

	if err == nil {
		t.Fatal("expected validation error")
	}

	if got, want := err.Error(), "unknown option \"fromat\"; did you mean \"format\"?"; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
}

func TestDecodeBatchEntriesExpandsMatrix(t *testing.T) {
	entries, err := decodeBatchEntries([]byte(`{
		"urls": ["https://a.com", "https://b.com"],
		"matrix": {
			"format": ["png", "pdf"],
			"width": [1280, 1920]
		},
		"options": {
			"full_page": true
		}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(entries), 8; got != want {
		t.Fatalf("unexpected entry count: got %d want %d", got, want)
	}

	if entries[0]["full_page"] != true {
		t.Fatalf("expected full_page option to be copied into entries")
	}
}

func TestDecodeBatchEntriesAcceptsNDJSON(t *testing.T) {
	entries, err := decodeBatchEntries([]byte("{\"url\":\"https://a.com\",\"format\":\"png\"}\n{\"url\":\"https://b.com\",\"format\":\"pdf\"}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(entries), 2; got != want {
		t.Fatalf("unexpected entry count: got %d want %d", got, want)
	}

	if got, want := entries[1]["format"], "pdf"; got != want {
		t.Fatalf("unexpected format: got %#v want %#v", got, want)
	}
}

func TestRenderDryRunAcceptsWebhookURLFlag(t *testing.T) {
	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runRender([]string{
			"https://example.com",
			"--async",
			"--webhook-url", "https://hooks.example.com/urlbox",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"webhook_url\": \"https://hooks.example.com/urlbox\"") {
		t.Fatalf("expected webhook_url in output, got:\n%s", output)
	}
}

func TestBatchDryRunAcceptsWebhookURLFlag(t *testing.T) {
	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runBatch([]string{
			"--json", `[{"url":"https://example.com"}]`,
			"--async",
			"--webhook-url", "https://hooks.example.com/urlbox",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"webhook_url\": \"https://hooks.example.com/urlbox\"") {
		t.Fatalf("expected webhook_url in output, got:\n%s", output)
	}
}

func TestRenderDryRunAcceptsInlineHTMLFlag(t *testing.T) {
	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runRender([]string{
			"--html", "<html><body><h1>Hello</h1></body></html>",
			"--format", "png",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	payload := decodeRenderJSONOutput(t, output)
	if got, want := payload["html"], "<html><body><h1>Hello</h1></body></html>"; got != want {
		t.Fatalf("unexpected html: got %#v want %#v", got, want)
	}
}

func TestRenderDryRunAcceptsHTMLFileFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<html><body>File</body></html>"), 0644); err != nil {
		t.Fatalf("write html file: %v", err)
	}

	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runRender([]string{
			"--html-file", path,
			"--format", "pdf",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	payload := decodeRenderJSONOutput(t, output)
	if got, want := payload["html"], "<html><body>File</body></html>"; got != want {
		t.Fatalf("unexpected html: got %#v want %#v", got, want)
	}
}

func TestRenderDryRunAcceptsHTMLFromStdin(t *testing.T) {
	output, exitCode := captureStdoutAndStderrWithStdin(t, "<html><body>stdin</body></html>", func() int {
		return runRender([]string{
			"--html", "-",
			"--format", "png",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	payload := decodeRenderJSONOutput(t, output)
	if got, want := payload["html"], "<html><body>stdin</body></html>"; got != want {
		t.Fatalf("unexpected html: got %#v want %#v", got, want)
	}
}

func TestBatchDryRunAcceptsInlineHTMLFlag(t *testing.T) {
	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runBatch([]string{
			"--html", "<html><body>Batch</body></html>",
			"--format", "pdf",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"count\": 1") {
		t.Fatalf("expected single batch entry, got:\n%s", output)
	}
	if !strings.Contains(output, "\"html\": \"\\u003chtml\\u003e\\u003cbody\\u003eBatch\\u003c/body\\u003e\\u003c/html\\u003e\"") {
		t.Fatalf("expected html in batch output, got:\n%s", output)
	}
}

func TestBatchDryRunAcceptsHTMLFromStdin(t *testing.T) {
	output, exitCode := captureStdoutAndStderrWithStdin(t, "<html><body>Batch stdin</body></html>", func() int {
		return runBatch([]string{
			"--html", "-",
			"--format", "png",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"html\": \"\\u003chtml\\u003e\\u003cbody\\u003eBatch stdin\\u003c/body\\u003e\\u003c/html\\u003e\"") {
		t.Fatalf("expected html in batch output, got:\n%s", output)
	}
}

func TestBatchDryRunAcceptsHTMLFileFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<html><body>Batch file</body></html>"), 0644); err != nil {
		t.Fatalf("write html file: %v", err)
	}

	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runBatch([]string{
			"--html-file", path,
			"--format", "svg",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"html\": \"\\u003chtml\\u003e\\u003cbody\\u003eBatch file\\u003c/body\\u003e\\u003c/html\\u003e\"") {
		t.Fatalf("expected html in batch output, got:\n%s", output)
	}
}

func TestRenderRejectsURLAndHTMLTogether(t *testing.T) {
	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runRender([]string{
			"https://example.com",
			"--html", "<html></html>",
			"--dry-run",
			"--output-format", "json",
		})
	})

	if exitCode == 0 {
		t.Fatalf("expected failure when url and html are both provided")
	}
	if !strings.Contains(output, "url positional argument cannot be combined with --html or --html-file") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func captureStdoutAndStderr(t *testing.T, fn func() int) (string, int) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer reader.Close()

	os.Stdout = writer
	os.Stderr = writer

	done := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		done <- buffer.String()
	}()

	exitCode := fn()

	_ = writer.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return <-done, exitCode
}

func captureStdoutAndStderrWithStdin(t *testing.T, stdin string, fn func() int) (string, int) {
	t.Helper()

	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if _, err := writer.WriteString(stdin); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = writer.Close()
	os.Stdin = reader
	defer func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	}()

	return captureStdoutAndStderr(t, fn)
}

func decodeRenderJSONOutput(t *testing.T, raw string) map[string]interface{} {
	t.Helper()

	var envelope struct {
		Data struct {
			ValidatedOptions map[string]interface{} `json:"validated_options"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("decode output: %v\nraw:\n%s", err, raw)
	}

	return envelope.Data.ValidatedOptions
}
