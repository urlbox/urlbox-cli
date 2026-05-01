// internal/output/errors.go
package output

// ErrorCode is a closed set of error classification codes.
type ErrorCode string

// Error codes for the CLI. Each maps to a specific exit code.
const (
	ErrUsage      ErrorCode = "usage"
	ErrValidation ErrorCode = "validation"
	ErrAuth       ErrorCode = "auth"
	ErrForbidden  ErrorCode = "forbidden"
	ErrNotFound   ErrorCode = "not_found"
	ErrRateLimit  ErrorCode = "rate_limit"
	ErrConflict   ErrorCode = "conflict"
	ErrServer     ErrorCode = "server"
	ErrNetwork    ErrorCode = "network"
	ErrTimeout    ErrorCode = "timeout"
)

var validErrorCodes = map[ErrorCode]bool{
	ErrUsage: true, ErrValidation: true, ErrAuth: true,
	ErrForbidden: true, ErrNotFound: true, ErrRateLimit: true,
	ErrConflict: true, ErrServer: true, ErrNetwork: true,
	ErrTimeout: true,
}

var exitCodeMap = map[ErrorCode]int{
	ErrUsage:      1,
	ErrValidation: 2,
	ErrAuth:       3,
	ErrForbidden:  4,
	ErrNotFound:   5,
	ErrRateLimit:  6,
	ErrConflict:   7,
	ErrServer:     10,
	ErrNetwork:    11,
	ErrTimeout:    11,
}

// IsValidErrorCode reports whether code is in the closed set.
func IsValidErrorCode(code ErrorCode) bool {
	return validErrorCodes[code]
}

// ExitCode returns the process exit code for this error code.
// Unknown codes default to 10 (server-class).
func (c ErrorCode) ExitCode() int {
	if code, ok := exitCodeMap[c]; ok {
		return code
	}
	return 10
}

// CLIError carries a machine-readable code and optional agent hint.
//
// Set Silent to true when the command has already written its own envelope to
// stdout (e.g., doctor's check report) and only the exit code should reflect
// failure. The Execute layer skips writing an error envelope in that case.
type CLIError struct {
	Code    ErrorCode
	Message string
	Hint    string
	Silent  bool
}

func (e *CLIError) Error() string { return e.Message }

// ExitCode returns the process exit code for this error.
func (e *CLIError) ExitCode() int {
	return e.Code.ExitCode()
}

// NewCLIError creates a new CLIError.
func NewCLIError(code ErrorCode, message, hint string) *CLIError {
	return &CLIError{Code: code, Message: message, Hint: hint}
}
