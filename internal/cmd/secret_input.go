// internal/cmd/secret_input.go
package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// secretStdin is the io.Reader used to satisfy --api-secret-stdin. Defaults
// to os.Stdin; tests inject a strings.Reader via SetSecretStdinForTest.
// Separated from renderStdin so the --json - and --api-secret-stdin paths
// can be independently scripted in tests (production code rejects the
// dual-stdin conflict at the runRender entry).
var secretStdin io.Reader = os.Stdin

// SetSecretStdinForTest swaps in an alternate reader for --api-secret-stdin.
func SetSecretStdinForTest(r io.Reader) { secretStdin = r }

// ResetSecretStdinForTest restores the os.Stdin default.
func ResetSecretStdinForTest() { secretStdin = os.Stdin }

// resolveAPISecretInput picks the API secret from at most one of three
// command-line sources, in order of safety:
//
//   - --api-secret-stdin (bool)   : read entire stdin until EOF
//   - --api-secret-file <path>    : read the file's contents
//   - --api-secret <value>        : direct argv flag (least safe)
//
// When more than one source is provided, returns ErrUsage. When none is
// provided, returns "" — the caller falls back to env (URLBOX_API_SECRET)
// or saved config. Trailing \r\n is trimmed from stdin/file reads; the
// trimmed result must be non-empty (an empty file or empty stdin is a
// user mistake worth flagging).
//
// When direct (--api-secret) is non-empty AND stderr is a TTY, a warning
// is emitted about shell-history preservation and `ps` visibility — the
// 2026-05-08 incident class. Suppressed in non-TTY (CI / agents) to keep
// pipeline output clean.
func resolveAPISecretInput(stdin io.Reader, stderr io.Writer, direct string, fromStdin bool, fromFile string) (string, *output.CLIError) {
	chosen := 0
	if direct != "" {
		chosen++
	}
	if fromStdin {
		chosen++
	}
	if fromFile != "" {
		chosen++
	}
	if chosen > 1 {
		return "", output.NewCLIError(
			output.ErrUsage,
			"pass at most one of --api-secret, --api-secret-stdin, --api-secret-file",
			"Pick a single secret source. Prefer --api-secret-stdin (CI) or --api-secret-file <path> over --api-secret <value>.",
		)
	}

	switch {
	case direct != "":
		if isStderrTTY(stderr) {
			_, _ = fmt.Fprintln(stderr, "warning: --api-secret on the command line is preserved in shell history and visible to `ps`. Prefer URLBOX_API_SECRET, --api-secret-stdin, or --api-secret-file.")
		}
		return direct, nil
	case fromStdin:
		b, err := io.ReadAll(stdin)
		if err != nil {
			return "", output.NewCLIError(output.ErrUsage, "failed to read --api-secret-stdin", err.Error())
		}
		s := strings.TrimRight(string(b), "\r\n")
		if s == "" {
			return "", output.NewCLIError(
				output.ErrUsage,
				"--api-secret-stdin received no secret on stdin",
				"Pipe the secret on stdin, e.g. `printf %s \"$URLBOX_API_SECRET\" | urlbox auth --api-secret-stdin`.",
			)
		}
		return s, nil
	case fromFile != "":
		b, err := os.ReadFile(fromFile) //nolint:gosec // user-provided path is expected; this is the intended secret-input mechanism
		if err != nil {
			return "", output.NewCLIError(
				output.ErrUsage,
				"failed to read --api-secret-file",
				err.Error(),
			)
		}
		s := strings.TrimRight(string(b), "\r\n")
		if s == "" {
			return "", output.NewCLIError(
				output.ErrUsage,
				"--api-secret-file is empty",
				"The file at "+fromFile+" contained no secret after trimming trailing newlines.",
			)
		}
		return s, nil
	}
	return "", nil
}
