// internal/output/jq.go
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
)

// ApplyJQ runs a jq expression over a JSON envelope and returns the result.
// When quiet is true, the expression runs against `.data`; otherwise the full envelope.
// Multiple results are emitted newline-delimited.
func ApplyJQ(envelopeBytes []byte, expr string, quiet bool) ([]byte, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid jq expression: %w", err)
	}

	var input any
	if err := json.Unmarshal(envelopeBytes, &input); err != nil {
		return nil, fmt.Errorf("invalid envelope JSON: %w", err)
	}
	if quiet {
		if m, ok := input.(map[string]any); ok {
			input = m["data"]
		}
	}

	iter := q.Run(input)
	var buf bytes.Buffer
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if errVal, isErr := v.(error); isErr {
			return nil, errVal
		}
		out, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		buf.Write(out)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// WriteEnvelopeWithJQ marshals env, runs the jq expression, and writes the result.
// quiet=true filters against `.data`; otherwise the full envelope.
// Returns a CLIError with ErrUsage code on invalid jq input.
func WriteEnvelopeWithJQ(w io.Writer, env *Envelope, jqExpr string, quiet bool) error {
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	out, err := ApplyJQ(raw, jqExpr, quiet)
	if err != nil {
		return NewCLIError(ErrUsage, err.Error(), "Check the --jq expression (uses gojq syntax)")
	}
	_, err = w.Write(out)
	return err
}
