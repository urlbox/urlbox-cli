// Package cmd_test owns the Fizzy-item-1 regression guard: every
// NewCLIError call in the production code (non-test) must pass a
// non-empty hint. The Fizzy CLI's failure mode was dropping recovery
// hints on errors precisely when an agent needs them. We refuse to
// repeat that.
package cmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// emptyHintRE matches `NewCLIError(..., "")` only when the empty string
// is the FINAL arg at the end of the call AND the call ends the line
// (allowing trailing whitespace and an optional // comment).
//
// Non-greedy `.*?` is required because 6 of the 7 known sites have
// `err.Error()` (with a `)`) inside the message arg — `[^)]*` would
// fail to match those. Non-greedy matching prevents the pattern from
// spanning across multiple `NewCLIError` calls on one line, since the
// engine consumes as few characters as possible while still satisfying
// the `,\s*""\s*\)\s*(?://.*)?$` tail.
//
// The end-of-line anchor `$` prevents false positives when an empty
// string literal appears inside the message arg (e.g.
// `fmt.Sprintf("got %q", "")`) — in that case the `""` is followed
// by more args, not by `)` at end of line.
//
// Multi-line NewCLIError(...) calls are not detected — the convention
// for this codebase is that empty hints, when re-added during refactors,
// would land on a single line; multi-line refactors rely on PR review
// to spot the gap.
var emptyHintRE = regexp.MustCompile(`NewCLIError\(.*?,\s*""\s*\)\s*(?://.*)?$`)

func TestNoEmptyCLIErrorHints(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}

	var hits []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "bin" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !strings.HasPrefix(rel, "internal/") && !strings.HasPrefix(rel, "cmd/") {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if emptyHintRE.MatchString(line) {
				hits = append(hits, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(hits) > 0 {
		t.Errorf("Fizzy-item-1 regression: %d NewCLIError call(s) pass an empty hint:\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}

// ghostCommandSubstrings names commands/flags that have never existed
// in the shipped CLI, yet have appeared in error hints suggesting a user
// run them. Each hard wall for first-time users; each one a one-character
// fix once discovered.
//
//   - "urlbox config show" — no such subcommand. Closest real: `config get`,
//     `config path`, `config profile list`.
//   - "urlbox auth" — the auth command was removed in favour of `urlbox
//     login`; no production hint may point users at it.
var ghostCommandSubstrings = []string{
	"urlbox config show",
	"urlbox auth",
}

// TestNoGhostCommandsInHints walks production .go files and fails when
// any line contains a substring naming a command/flag that doesn't exist.
// Caught Round 1 review C1/C2 (config.go:306,359 and render.go:487,520).
func TestNoGhostCommandsInHints(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}

	var hits []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "bin" || base == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !strings.HasPrefix(rel, "internal/") && !strings.HasPrefix(rel, "cmd/") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, sub := range ghostCommandSubstrings {
				if strings.Contains(line, sub) {
					hits = append(hits, fmt.Sprintf("%s:%d: ghost command %q in: %s", rel, i+1, sub, strings.TrimSpace(line)))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(hits) > 0 {
		t.Errorf("ghost-command regression: %d hint(s) reference non-existent commands:\n%s",
			len(hits), strings.Join(hits, "\n"))
	}
}

// TestNoEmptyCLIErrorHints_RegexBehavior pins the regex semantics so a
// future tweak that re-introduces a false positive or a false negative
// fails loudly instead of silently weakening the guard.
func TestNoEmptyCLIErrorHints_RegexBehavior(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty hint, EOL", `output.NewCLIError(ErrUsage, "msg", "")`, true},
		{"empty hint, trailing comment OK", `output.NewCLIError(ErrUsage, "msg", "")  // trailing`, true},
		{"empty hint with err.Error() in message", `return output.NewCLIError(ErrUsage, err.Error(), "")`, true},
		{"non-empty hint", `output.NewCLIError(ErrUsage, "msg", "hint")`, false},
		{"empty literal inside message arg, non-empty hint", `output.NewCLIError(ErrUsage, fmt.Sprintf("got %q", ""), "hint")`, false},
		{"empty literal inside message arg, empty hint (correctly caught)", `output.NewCLIError(ErrUsage, fmt.Sprintf("got %q", ""), "")`, true},
	}
	for _, c := range cases {
		got := emptyHintRE.MatchString(c.input)
		if got != c.want {
			t.Errorf("%s: re.MatchString(%q) = %v, want %v", c.name, c.input, got, c.want)
		}
	}
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", fmt.Errorf("could not find go.mod ancestor of %s: %w", wd, os.ErrNotExist)
		}
		wd = parent
	}
}
