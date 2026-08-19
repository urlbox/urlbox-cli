package output

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const (
	brandIndigo      = lipgloss.Color("#4f46e5")
	brandIndigoLight = lipgloss.Color("#818cf8")
	brandMuted       = lipgloss.Color("#64748b")
)

// Styles holds the terminal styles used across the CLI.
type Styles struct {
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Muted   lipgloss.Style
	Bold    lipgloss.Style
	Header  lipgloss.Style
	Active  lipgloss.Style
	Border  lipgloss.Style
}

// NewStyles creates a new set of terminal styles.
// Respects NO_COLOR environment variable automatically via Lip Gloss.
func NewStyles() Styles {
	noColor := os.Getenv("NO_COLOR") != ""

	renderer := lipgloss.NewRenderer(os.Stderr)
	if noColor {
		renderer.SetHasDarkBackground(false)
		renderer.SetColorProfile(termenv.Ascii)
	}

	return Styles{
		Success: renderer.NewStyle().Foreground(lipgloss.Color("2")),
		Error:   renderer.NewStyle().Foreground(lipgloss.Color("1")),
		Warning: renderer.NewStyle().Foreground(lipgloss.Color("3")),
		Muted:   renderer.NewStyle().Foreground(lipgloss.Color("8")),
		Bold:    renderer.NewStyle().Bold(true),
		Header:  renderer.NewStyle().Bold(true).Foreground(brandIndigoLight),
		Active:  renderer.NewStyle().Bold(true).Foreground(brandIndigo),
		Border:  renderer.NewStyle().Foreground(brandMuted),
	}
}

// NewStylesForWriter creates styles targeting a specific writer for color detection.
func NewStylesForWriter(w io.Writer) *Styles {
	noColor := os.Getenv("NO_COLOR") != ""

	renderer := lipgloss.NewRenderer(w)
	if noColor {
		renderer.SetHasDarkBackground(false)
		renderer.SetColorProfile(termenv.Ascii)
	}

	return &Styles{
		Success: renderer.NewStyle().Foreground(lipgloss.Color("2")),
		Error:   renderer.NewStyle().Foreground(lipgloss.Color("1")),
		Warning: renderer.NewStyle().Foreground(lipgloss.Color("3")),
		Muted:   renderer.NewStyle().Foreground(lipgloss.Color("8")),
		Bold:    renderer.NewStyle().Bold(true),
		Header:  renderer.NewStyle().Bold(true).Foreground(brandIndigoLight),
		Active:  renderer.NewStyle().Bold(true).Foreground(brandIndigo),
		Border:  renderer.NewStyle().Foreground(brandMuted),
	}
}
