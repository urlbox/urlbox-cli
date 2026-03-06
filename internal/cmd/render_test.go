package cmd

import (
	"bytes"
	"io"
	"os"
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
