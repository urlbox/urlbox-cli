package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestScreenshot_DefaultsToPNG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"screenshot", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["format"] != "png" {
		t.Errorf("format=%v, want png", data["format"])
	}
	if data["url"] != "https://example.com" {
		t.Errorf("url=%v", data["url"])
	}
}

func TestScreenshot_UserFormatOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"screenshot", "https://example.com", "--format", "webp", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["data"].(map[string]any)["format"] != "webp" {
		t.Errorf("user --format webp should override alias default; got %v", env["data"].(map[string]any)["format"])
	}
}

func TestScreenshot_ShotAlias(t *testing.T) {
	// The `shot` cobra alias should resolve to the screenshot command.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"shot", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["data"].(map[string]any)["format"] != "png" {
		t.Errorf("shot alias should default to png like screenshot")
	}
}

func TestPDF_DefaultsToPDFAndFullPage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"pdf", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["format"] != "pdf" {
		t.Errorf("format=%v, want pdf", data["format"])
	}
	if data["full_page"] != true {
		t.Errorf("full_page=%v, want true", data["full_page"])
	}
}

func TestPDF_UserCanDisableFullPage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"pdf", "https://example.com", "--full-page=false", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["full_page"] != false {
		t.Errorf("user --full-page=false should override alias default; got %v", data["full_page"])
	}
}

func TestVideo_DefaultsToMP4(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"video", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["data"].(map[string]any)["format"] != "mp4" {
		t.Errorf("format=%v, want mp4", env["data"].(map[string]any)["format"])
	}
}

func TestAliases_AppearInCommandsList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"commands", "--output-format", "json"}, &stdout, &stderr)
	body := stdout.String()
	for _, name := range []string{"screenshot", "pdf", "video"} {
		if !strings.Contains(body, `"`+name+`"`) {
			t.Errorf("commands JSON missing %q in output", name)
		}
	}
}

func TestAliases_AllAcceptStdinJSON(t *testing.T) {
	// Aliases should inherit the full render flag pipeline including --json -.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, alias := range []string{"screenshot", "pdf", "video"} {
		t.Run(alias, func(t *testing.T) {
			cmd.SetStdinForTest(strings.NewReader(`{"url":"https://stdin.example"}`))
			t.Cleanup(cmd.ResetStdinForTest)

			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{alias, "--json", "-", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("alias=%s exit=%d stderr=%s", alias, exit, stderr.String())
			}
			var env map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("not JSON: %v", err)
			}
			if env["data"].(map[string]any)["url"] != "https://stdin.example" {
				t.Errorf("alias=%s did not pick up --json - stdin", alias)
			}
		})
	}
}
