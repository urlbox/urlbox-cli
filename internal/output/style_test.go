package output_test

import (
	"testing"

	"github.com/urlbox/cli/internal/output"
)

func TestStyles_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	styles := output.NewStyles()

	// When NO_COLOR is set, styled text should equal unstyled text
	styled := styles.Success.Render("hello")
	if styled != "hello" {
		t.Errorf("expected NO_COLOR to strip styles, got %q", styled)
	}
}

func TestStyles_HasStyles(t *testing.T) {
	// Unset NO_COLOR to ensure styles are applied
	t.Setenv("NO_COLOR", "")

	styles := output.NewStyles()

	// Verify all styles exist and don't panic
	_ = styles.Success.Render("hello")
	_ = styles.Error.Render("hello")
	_ = styles.Warning.Render("hello")
	_ = styles.Muted.Render("hello")
	_ = styles.Bold.Render("hello")
}

func TestStyles_NoColor_AllStyles(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	styles := output.NewStyles()

	// Every style should strip ANSI when NO_COLOR is set
	for name, style := range map[string]interface{ Render(...string) string }{
		"Error":   styles.Error,
		"Warning": styles.Warning,
		"Muted":   styles.Muted,
		"Bold":    styles.Bold,
	} {
		got := style.Render("test")
		if got != "test" {
			t.Errorf("%s style should strip formatting with NO_COLOR, got %q", name, got)
		}
	}
}
