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
	// Warnings carries non-fatal advisories agents should surface but
	// shouldn't fail on (e.g. "unknown option 'fromat' — did you mean
	// 'format'?"). v1.0.4 Class 3 — pre-1.0.4 these were emitted as
	// plain stderr text alongside the JSON envelope on stdout, breaking
	// agents that read either stream alone. Now they ride inside the
	// envelope for json/quiet modes; text mode still prints them inline
	// to stderr where humans expect them.
	Warnings []string `json:"warnings,omitempty"`
}

// ErrorEnvelope is the error response shape.
type ErrorEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Error   string `json:"error"`
	Code    string `json:"code"`
	Hint    string `json:"hint,omitempty"`
	// Warnings — see Envelope.Warnings.
	Warnings []string `json:"warnings,omitempty"`
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
