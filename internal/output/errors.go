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
)

var validErrorCodes = map[ErrorCode]bool{
	ErrUsage: true, ErrValidation: true, ErrAuth: true,
	ErrForbidden: true, ErrNotFound: true, ErrRateLimit: true,
	ErrConflict: true, ErrServer: true, ErrNetwork: true,
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
}

// IsValidErrorCode reports whether code is in the closed set.
func IsValidErrorCode(code ErrorCode) bool {
	return validErrorCodes[code]
}

// CLIError carries a machine-readable code and optional agent hint.
type CLIError struct {
	Code    ErrorCode
	Message string
	Hint    string
}

func (e *CLIError) Error() string { return e.Message }

// ExitCode returns the process exit code for this error.
func (e *CLIError) ExitCode() int {
	if code, ok := exitCodeMap[e.Code]; ok {
		return code
	}
	return 10
}

// NewCLIError creates a new CLIError.
func NewCLIError(code ErrorCode, message, hint string) *CLIError {
	return &CLIError{Code: code, Message: message, Hint: hint}
}
