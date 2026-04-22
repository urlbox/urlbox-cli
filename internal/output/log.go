package output

import (
	"io"
	"os"

	"github.com/charmbracelet/log"
)

// NewLogger creates a stderr logger for human messages.
// All human-facing output goes to stderr; stdout is reserved for structured data.
func NewLogger(verbose bool) *log.Logger {
	level := log.InfoLevel
	if verbose {
		level = log.DebugLevel
	}

	logger := log.NewWithOptions(os.Stderr, log.Options{
		Level:           level,
		ReportTimestamp: false,
	})

	return logger
}

// NewLoggerWithWriter creates a logger writing to a specific writer (for testing).
func NewLoggerWithWriter(w io.Writer, verbose bool) *log.Logger {
	level := log.InfoLevel
	if verbose {
		level = log.DebugLevel
	}

	return log.NewWithOptions(w, log.Options{
		Level:           level,
		ReportTimestamp: false,
	})
}
