// internal/output/envelope.go
package output

// Breadcrumb suggests a follow-up command for agents.
type Breadcrumb struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}

// Envelope is the success response shape.
type Envelope struct {
	OK          bool         `json:"ok"`
	Command     string       `json:"command"`
	Data        any          `json:"data"`
	Summary     string       `json:"summary,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
}

// ErrorEnvelope is the error response shape.
type ErrorEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Error   string `json:"error"`
	Code    string `json:"code"`
	Hint    string `json:"hint,omitempty"`
}

// NewEnvelope creates a success envelope.
func NewEnvelope(command string, data any, summary string, breadcrumbs []Breadcrumb) *Envelope {
	return &Envelope{
		OK:          true,
		Command:     command,
		Data:        data,
		Summary:     summary,
		Breadcrumbs: breadcrumbs,
	}
}

// NewErrorEnvelope creates an error envelope from a CLIError.
func NewErrorEnvelope(command string, cliErr *CLIError) *ErrorEnvelope {
	return &ErrorEnvelope{
		OK:      false,
		Command: command,
		Error:   cliErr.Message,
		Code:    string(cliErr.Code),
		Hint:    cliErr.Hint,
	}
}
