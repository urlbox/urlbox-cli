// internal/output/tty.go
package output

import (
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// Format represents an output format.
type Format string

// Output format constants.
const (
	FormatJSON  Format = "json"
	FormatText  Format = "text"
	FormatQuiet Format = "quiet"
)

// IsTTY reports whether w is a terminal.
func IsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// ResolveFormat picks the output format from an explicit value or TTY detection.
// If explicit is non-empty, it is used directly. Otherwise, text is returned for
// TTY stdout and json for non-TTY (piped) stdout.
func ResolveFormat(explicit string, stdout io.Writer) Format {
	if explicit != "" {
		return Format(explicit)
	}
	if IsTTY(stdout) {
		return FormatText
	}
	return FormatJSON
}
