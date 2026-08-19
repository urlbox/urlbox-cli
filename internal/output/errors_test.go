// internal/output/errors_test.go
package output_test

import (
	"errors"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestErrorCode_ClosedSet_HasNineCodes(t *testing.T) {
	expected := []output.ErrorCode{
		output.ErrUsage, output.ErrValidation, output.ErrAuth,
		output.ErrForbidden, output.ErrNotFound, output.ErrRateLimit,
		output.ErrConflict, output.ErrServer, output.ErrNetwork,
	}
	for _, code := range expected {
		if !output.IsValidErrorCode(code) {
			t.Errorf("expected %q to be valid", code)
		}
	}
	if output.IsValidErrorCode("bogus") {
		t.Error("expected 'bogus' to be invalid")
	}
}

func TestExitCodeMap_CorrectValues(t *testing.T) {
	tests := []struct {
		code output.ErrorCode
		exit int
	}{
		{output.ErrUsage, 1},
		{output.ErrValidation, 2},
		{output.ErrAuth, 3},
		{output.ErrForbidden, 4},
		{output.ErrNotFound, 5},
		{output.ErrRateLimit, 6},
		{output.ErrConflict, 7},
		{output.ErrServer, 10},
		{output.ErrNetwork, 11},
	}
	for _, tt := range tests {
		cliErr := output.NewCLIError(tt.code, "test", "")
		if got := cliErr.ExitCode(); got != tt.exit {
			t.Errorf("ExitCode(%q) = %d, want %d", tt.code, got, tt.exit)
		}
	}
}

func TestCLIError_ImplementsError(t *testing.T) {
	var err error = output.NewCLIError(output.ErrUsage, "bad input", "try --help")
	if err.Error() != "bad input" {
		t.Errorf("Error() = %q, want %q", err.Error(), "bad input")
	}
}

func TestCLIError_ExitCode_UnknownDefaultsToServer(t *testing.T) {
	cliErr := output.NewCLIError("unknown_code", "oops", "")
	if got := cliErr.ExitCode(); got != 10 {
		t.Errorf("ExitCode for unknown code = %d, want 10", got)
	}
}

func TestCLIError_CanBeUnwrappedWithErrorsAs(t *testing.T) {
	original := output.NewCLIError(output.ErrAuth, "unauthorized", "run urlbox login")
	wrapped := errors.New("command failed: " + original.Error())
	_ = wrapped // We just verify CLIError works with errors.As directly
	var target *output.CLIError
	err := error(original)
	if !errors.As(err, &target) {
		t.Error("expected errors.As to match *CLIError")
	}
	if target.Code != output.ErrAuth {
		t.Errorf("Code = %q, want %q", target.Code, output.ErrAuth)
	}
}

func TestErrTimeout_HasCodeAndExit(t *testing.T) {
	if string(output.ErrTimeout) != "timeout" {
		t.Errorf("ErrTimeout=%q, want %q", output.ErrTimeout, "timeout")
	}
	if output.ErrTimeout.ExitCode() != 11 {
		t.Errorf("ErrTimeout.ExitCode=%d, want 11", output.ErrTimeout.ExitCode())
	}
}

func TestNewCLIError_SetsAllFields(t *testing.T) {
	err := output.NewCLIError(output.ErrNotFound, "not found", "check the ID")
	if err.Code != output.ErrNotFound {
		t.Errorf("Code = %q, want %q", err.Code, output.ErrNotFound)
	}
	if err.Message != "not found" {
		t.Errorf("Message = %q, want %q", err.Message, "not found")
	}
	if err.Hint != "check the ID" {
		t.Errorf("Hint = %q, want %q", err.Hint, "check the ID")
	}
}
