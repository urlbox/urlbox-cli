package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func retryable500() apitest.ScriptedResponse {
	return apitest.ScriptedResponse{Status: 500}
}

func TestSessionNoRetryStopsAfterOneAttempt(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(retryable500(), retryable500(), retryable500(), retryable500())
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--no-retry", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit on server error, got 0\n%s", stdout.String())
	}
	if reqs := srv.Requests(); len(reqs) != 1 {
		t.Fatalf("--no-retry must make exactly 1 attempt, got %d", len(reqs))
	}
}

func TestSessionMaxRetriesOneMakesTwoAttempts(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(retryable500(), retryable500(), retryable500(), retryable500())
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--max-retries", "1", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit on server error, got 0\n%s", stdout.String())
	}
	if reqs := srv.Requests(); len(reqs) != 2 {
		t.Fatalf("--max-retries 1 must make exactly 2 attempts, got %d", len(reqs))
	}
}

func TestSessionDefaultRetriesUseFullBudget(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	scripts := make([]apitest.ScriptedResponse, 6)
	for i := range scripts {
		scripts[i] = retryable500()
	}
	srv := apitest.New(scripts...)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--max-retries", "2", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit on server error, got 0\n%s", stdout.String())
	}
	if reqs := srv.Requests(); len(reqs) != 3 {
		t.Fatalf("--max-retries 2 must make exactly 3 attempts, got %d", len(reqs))
	}
}

func TestSessionRetryFlagsInheritedBySubcommandHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	Execute([]string{"orgs", "list", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	for _, want := range []string{"--no-retry", "--max-retries"} {
		if !strings.Contains(help, want) {
			t.Fatalf("orgs list --help missing inherited %s\n%s", want, help)
		}
	}
}
