// internal/output/format.go
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// Formatter writes envelopes to a writer in a specific format.
type Formatter interface {
	WriteSuccess(w io.Writer, env *Envelope) error
	WriteError(w io.Writer, env *ErrorEnvelope) error
}

// NewFormatter creates a Formatter for the given format.
func NewFormatter(f Format, styles *Styles) Formatter {
	switch f {
	case FormatText:
		return &TextFormatter{styles: *styles}
	case FormatQuiet:
		return &QuietFormatter{}
	default:
		return &JSONFormatter{}
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// JSONFormatter writes the full envelope as indented JSON.
type JSONFormatter struct{}

// WriteSuccess writes a success envelope as indented JSON.
func (f *JSONFormatter) WriteSuccess(w io.Writer, env *Envelope) error {
	return writeJSON(w, env)
}

// WriteError writes an error envelope as indented JSON.
func (f *JSONFormatter) WriteError(w io.Writer, env *ErrorEnvelope) error {
	return writeJSON(w, env)
}

// TextFormatter writes human-readable styled output.
type TextFormatter struct {
	styles Styles
}

// WriteSuccess writes human-readable output. When the envelope is ok and carries
// a text view (SetTable/SetKV), only the view is rendered — the view is the
// content and the summary would just be noise above it. Otherwise the summary
// line is written first, glyph-coded by env.OK (✓ for ok, ✗ for not-ok — doctor
// sets ok=false on failing checks), so a failing view keeps its ✗ headline and a
// viewless success keeps its summary. Structured consumers reach for
// --output-format json, where summary always rides in the envelope.
func (f *TextFormatter) WriteSuccess(w io.Writer, env *Envelope) error {
	if env.Summary != "" && (!env.OK || env.view == nil) {
		if env.OK {
			_, _ = fmt.Fprintln(w, f.styles.Success.Render("✓ "+env.Summary))
		} else {
			_, _ = fmt.Fprintln(w, f.styles.Error.Render("✗ "+env.Summary))
		}
	}
	env.view.render(w, &f.styles)
	return nil
}

// WriteError writes a human-readable error message with optional hint.
func (f *TextFormatter) WriteError(w io.Writer, env *ErrorEnvelope) error {
	_, _ = fmt.Fprintln(w, f.styles.Error.Render("Error: "+env.Error))
	if env.Hint != "" {
		_, _ = fmt.Fprintln(w, f.styles.Muted.Render("Hint: "+env.Hint))
	}
	return nil
}

// QuietFormatter writes only the data portion or error message.
type QuietFormatter struct{}

// WriteSuccess writes only the data portion as JSON.
func (f *QuietFormatter) WriteSuccess(w io.Writer, env *Envelope) error {
	if env.Data == nil {
		return nil
	}
	return writeJSON(w, env.Data)
}

// WriteError writes only the error message.
func (f *QuietFormatter) WriteError(w io.Writer, env *ErrorEnvelope) error {
	_, err := fmt.Fprintln(w, env.Error)
	return err
}
