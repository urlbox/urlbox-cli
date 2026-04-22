package output_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/cli/internal/output"
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
