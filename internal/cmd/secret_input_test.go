// internal/cmd/secret_input_test.go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAPISecretInput_NoSourceReturnsEmpty(t *testing.T) {
	s, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "" {
		t.Errorf("got %q, want empty", s)
	}
}

func TestResolveAPISecretInput_DirectFlag_NoTTY_NoWarning(t *testing.T) {
	SetStderrTTYForTest(false)
	t.Cleanup(ResetStderrTTYForTest)

	var stderr bytes.Buffer
	s, err := resolveAPISecretInput(strings.NewReader(""), &stderr, "sec_direct", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "sec_direct" {
		t.Errorf("got %q, want sec_direct", s)
	}
	if stderr.Len() != 0 {
		t.Errorf("non-TTY should suppress history warning; got %q", stderr.String())
	}
}

func TestResolveAPISecretInput_DirectFlag_TTY_WarnsAboutShellHistory(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)

	var stderr bytes.Buffer
	s, err := resolveAPISecretInput(strings.NewReader(""), &stderr, "sec_direct", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "sec_direct" {
		t.Errorf("got %q, want sec_direct", s)
	}
	got := stderr.String()
	if !strings.Contains(got, "warning") || !strings.Contains(got, "shell history") {
		t.Errorf("TTY stderr should contain a shell-history warning; got %q", got)
	}
	if !strings.Contains(got, "--api-secret-stdin") {
		t.Errorf("warning should suggest --api-secret-stdin; got %q", got)
	}
}

func TestResolveAPISecretInput_Stdin_ReadsAndTrimsNewline(t *testing.T) {
	stdin := strings.NewReader("sec_from_stdin\n")
	s, err := resolveAPISecretInput(stdin, &bytes.Buffer{}, "", true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "sec_from_stdin" {
		t.Errorf("got %q, want sec_from_stdin", s)
	}
}

func TestResolveAPISecretInput_Stdin_TrimsCRLF(t *testing.T) {
	stdin := strings.NewReader("sec_crlf\r\n")
	s, err := resolveAPISecretInput(stdin, &bytes.Buffer{}, "", true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "sec_crlf" {
		t.Errorf("got %q, want sec_crlf", s)
	}
}

func TestResolveAPISecretInput_Stdin_EmptyErrors(t *testing.T) {
	_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", true, "")
	if err == nil {
		t.Fatal("empty stdin should error")
	}
	if string(err.Code) != "usage" {
		t.Errorf("code=%q, want usage", err.Code)
	}
	if !strings.Contains(err.Message, "stdin") {
		t.Errorf("message should mention stdin; got %q", err.Message)
	}
}

func TestResolveAPISecretInput_File_ReadsAndTrimsNewline(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("sec_from_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "sec_from_file" {
		t.Errorf("got %q, want sec_from_file", s)
	}
}

func TestResolveAPISecretInput_File_MissingErrors(t *testing.T) {
	_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, "/nonexistent/path/secret.txt")
	if err == nil {
		t.Fatal("missing file should error")
	}
	if string(err.Code) != "usage" {
		t.Errorf("code=%q, want usage", err.Code)
	}
}

func TestResolveAPISecretInput_File_EmptyErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(p, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, p)
	if err == nil {
		t.Fatal("empty file should error")
	}
}

func TestResolveAPISecretInput_MultipleSourcesError(t *testing.T) {
	cases := []struct {
		name   string
		direct string
		stdin  bool
		file   string
	}{
		{"direct + stdin", "sec_x", true, ""},
		{"direct + file", "sec_x", false, "/tmp/x"},
		{"stdin + file", "", true, "/tmp/x"},
		{"all three", "sec_x", true, "/tmp/x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, c.direct, c.stdin, c.file)
			if err == nil {
				t.Fatal("expected mutex error")
			}
			if string(err.Code) != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
			if !strings.Contains(err.Message, "at most one") {
				t.Errorf("message should say 'at most one'; got %q", err.Message)
			}
		})
	}
}

func TestResolveAPISecretInput_Stdin_RejectsTooLarge(t *testing.T) {
	// Build a payload larger than the cap.
	big := strings.Repeat("a", 4097)
	_, err := resolveAPISecretInput(strings.NewReader(big), &bytes.Buffer{}, "", true, "")
	if err == nil {
		t.Fatal("oversized stdin should error")
	}
	if string(err.Code) != "usage" {
		t.Errorf("code=%q, want usage", err.Code)
	}
	if !strings.Contains(err.Message, "too large") {
		t.Errorf("message should say 'too large'; got %q", err.Message)
	}
}

func TestResolveAPISecretInput_File_RejectsTooLarge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	big := bytes.Repeat([]byte("a"), 4097)
	if err := os.WriteFile(p, big, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, p)
	if err == nil {
		t.Fatal("oversized file should error")
	}
	if string(err.Code) != "usage" {
		t.Errorf("code=%q, want usage", err.Code)
	}
	if !strings.Contains(err.Message, "too large") {
		t.Errorf("message should say 'too large'; got %q", err.Message)
	}
}

func TestResolveAPISecretInput_File_MissingHintIsFriendly(t *testing.T) {
	_, err := resolveAPISecretInput(strings.NewReader(""), &bytes.Buffer{}, "", false, "/nonexistent/path/secret.txt")
	if err == nil {
		t.Fatal("missing file should error")
	}
	if !strings.Contains(err.Message, "/nonexistent/path/secret.txt") {
		t.Errorf("message should name the path; got %q", err.Message)
	}
	if !strings.Contains(err.Hint, "Check the path") || !strings.Contains(err.Hint, "permissions") {
		t.Errorf("hint should reference path + permissions; got %q", err.Hint)
	}
}
