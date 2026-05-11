// internal/cmd/secret_input.go
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// unwrapFileError strips the leading `op /path:` prefix from an os
// syscall error so we can name the path ourselves without duplicating
// it. Falls back to err.Error() for non-PathError types.
func unwrapFileError(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

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

// maxSecretBytes caps the size of secrets read via --api-secret-stdin or
// --api-secret-file. Real Urlbox API secrets are ~40 chars; 4 KiB is two
// orders of magnitude beyond any real value, small enough that an
// accidental `--api-secret-file /dev/zero` fails fast instead of OOMing
// the process.
const maxSecretBytes = 4096

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
// directExplicit distinguishes `--api-secret ""` (passed explicitly with
// an empty value) from `--api-secret` not being passed at all. cobra
// collapses both to direct == "" at the Go level — only Flags().Changed()
// can tell them apart. Explicit empty is a usage error (Round 4 M3) so
// the user isn't silently fed env/profile when they were trying to test
// "what happens with no auth?".
//
// When direct (--api-secret) is non-empty AND stderr is a TTY, a warning
// is emitted about shell-history preservation and `ps` visibility — the
// 2026-05-08 incident class. Suppressed in non-TTY (CI / agents) to keep
// pipeline output clean.
func resolveAPISecretInput(stdin io.Reader, stderr io.Writer, direct string, directExplicit, fromStdin bool, fromFile string) (string, *output.CLIError) {
	if directExplicit && direct == "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			`--api-secret cannot be empty`,
			"Either pass a non-empty value, or omit --api-secret entirely (the CLI will fall back to URLBOX_API_SECRET or the saved profile).",
		)
	}
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
		// Read one byte past the cap so we can distinguish "exactly at cap"
		// from "exceeds cap" without an extra Read call.
		b, err := io.ReadAll(io.LimitReader(stdin, maxSecretBytes+1))
		if err != nil {
			return "", output.NewCLIError(output.ErrUsage, "failed to read --api-secret-stdin", err.Error())
		}
		if len(b) > maxSecretBytes {
			return "", output.NewCLIError(
				output.ErrUsage,
				fmt.Sprintf("--api-secret-stdin input too large (>%d bytes)", maxSecretBytes),
				"API secrets are ~40 chars. If you piped a file by mistake, use --api-secret-file <path> instead.",
			)
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
		f, err := os.Open(fromFile) //nolint:gosec // user-provided path is expected; this is the intended secret-input mechanism
		if err != nil {
			return "", output.NewCLIError(
				output.ErrUsage,
				fmt.Sprintf("failed to read --api-secret-file %s: %s", fromFile, unwrapFileError(err)),
				"Check the path and permissions. The file must be readable by the current user and contain the API secret on a single line.",
			)
		}
		defer func() { _ = f.Close() }()
		b, err := io.ReadAll(io.LimitReader(f, maxSecretBytes+1))
		if err != nil {
			return "", output.NewCLIError(
				output.ErrUsage,
				fmt.Sprintf("failed to read --api-secret-file %s: %s", fromFile, unwrapFileError(err)),
				"Check the path and permissions.",
			)
		}
		if len(b) > maxSecretBytes {
			return "", output.NewCLIError(
				output.ErrUsage,
				fmt.Sprintf("--api-secret-file %s is too large (>%d bytes)", fromFile, maxSecretBytes),
				"API secrets are ~40 chars. Pointing --api-secret-file at a non-secret file is usually a mistake.",
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
