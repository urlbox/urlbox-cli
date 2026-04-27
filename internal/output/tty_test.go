// internal/output/tty_test.go
package output_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestIsTTY_BytesBuffer_ReturnsFalse(t *testing.T) {
	buf := &bytes.Buffer{}
	if output.IsTTY(buf) {
		t.Error("expected bytes.Buffer to not be a TTY")
	}
}

func TestIsTTY_DevNull_ReturnsFalse(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("cannot open /dev/null")
	}
	defer f.Close()
	if output.IsTTY(f) {
		t.Error("expected /dev/null to not be a TTY")
	}
}

func TestFormat_Constants(t *testing.T) {
	if output.FormatJSON != "json" {
		t.Errorf("FormatJSON = %q, want %q", output.FormatJSON, "json")
	}
	if output.FormatText != "text" {
		t.Errorf("FormatText = %q, want %q", output.FormatText, "text")
	}
	if output.FormatQuiet != "quiet" {
		t.Errorf("FormatQuiet = %q, want %q", output.FormatQuiet, "quiet")
	}
}

func TestResolveFormat_ExplicitOverridesTTY(t *testing.T) {
	got := output.ResolveFormat("quiet", &bytes.Buffer{})
	if got != output.FormatQuiet {
		t.Errorf("ResolveFormat(quiet) = %q, want %q", got, output.FormatQuiet)
	}
}

func TestResolveFormat_NoFlag_NonTTY_DefaultsToJSON(t *testing.T) {
	got := output.ResolveFormat("", &bytes.Buffer{})
	if got != output.FormatJSON {
		t.Errorf("ResolveFormat('', buffer) = %q, want %q", got, output.FormatJSON)
	}
}

func TestResolveFormat_NoFlag_DevNull_DefaultsToJSON(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("cannot open /dev/null")
	}
	defer f.Close()
	got := output.ResolveFormat("", f)
	if got != output.FormatJSON {
		t.Errorf("ResolveFormat('', /dev/null) = %q, want %q", got, output.FormatJSON)
	}
}
