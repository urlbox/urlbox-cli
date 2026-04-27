package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestNewLoggerWithWriter_InfoLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, false)

	logger.Info("hello info")
	logger.Debug("hello debug")

	out := buf.String()
	if !strings.Contains(out, "hello info") {
		t.Errorf("expected info message in output, got %q", out)
	}
	if strings.Contains(out, "hello debug") {
		t.Errorf("expected debug message to be suppressed at info level, got %q", out)
	}
}

func TestNewLoggerWithWriter_DebugLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, true)

	logger.Debug("hello debug")

	out := buf.String()
	if !strings.Contains(out, "hello debug") {
		t.Errorf("expected debug message in verbose output, got %q", out)
	}
}

func TestNewLoggerWithWriter_WritesToProvidedWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, false)

	logger.Info("test message")

	if buf.Len() == 0 {
		t.Error("expected logger to write to provided writer, got nothing")
	}
}

func TestNewLoggerWithWriter_WarnLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, false)

	logger.Warn("warning message")

	out := buf.String()
	if !strings.Contains(out, "warning message") {
		t.Errorf("expected warn message in output, got %q", out)
	}
}

func TestNewLoggerWithWriter_ErrorLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, false)

	logger.Error("error message")

	out := buf.String()
	if !strings.Contains(out, "error message") {
		t.Errorf("expected error message in output, got %q", out)
	}
}

func TestNewLoggerWithWriter_VerboseShowsAllLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := output.NewLoggerWithWriter(buf, true)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")
	logger.Error("error msg")

	out := buf.String()
	for _, expected := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected %q in verbose output, got %q", expected, out)
		}
	}
}

func TestNewLogger_ReturnsNonNil(t *testing.T) {
	logger := output.NewLogger(false)
	if logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestNewLogger_Verbose(t *testing.T) {
	logger := output.NewLogger(true)
	if logger == nil {
		t.Error("expected non-nil logger in verbose mode")
	}
}
