// internal/validation/fuzzy_test.go
package validation_test

import (
	"testing"

	"github.com/urlbox/urlbox-cli/internal/validation"
)

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"flaw", "lawn", 2},
		{"full_page", "fullpage", 1},  // case-insensitive: just delete the underscore
		{"width", "widht", 2},         // transposition counted as 2 (insert+delete)
		{"FULL_PAGE", "full_page", 0}, // case-insensitive equality
	}
	for _, c := range cases {
		got := validation.Levenshtein(c.a, c.b)
		if got != c.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClosestMatch_FindsObviousNeighbor(t *testing.T) {
	candidates := []string{"url", "format", "width", "height", "full_page", "block_ads", "user_agent"}

	tests := []struct {
		want   string
		expect string
		ok     bool
	}{
		{"widht", "width", true},          // transposition (distance 2, within ≤2 for len≤7)
		{"hieght", "height", true},        //nolint:misspell // deliberate typo: tests fuzz-correction
		{"fullPage", "full_page", true},   // case-fold plus char swap (distance 2)
		{"full-page", "full_page", true},  // hyphen vs underscore (distance 1)
		{"blockads", "block_ads", true},   // missing underscore (distance 1)
		{"useragent", "user_agent", true}, // missing underscore (longer name, distance 1, within ≤3)
		{"completely_different", "", false},
		{"x", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got, ok := validation.ClosestMatch(tc.want, candidates)
			if ok != tc.ok {
				t.Errorf("ok=%v, want %v (got=%q)", ok, tc.ok, got)
			}
			if got != tc.expect {
				t.Errorf("got=%q, want %q", got, tc.expect)
			}
		})
	}
}

func TestClosestMatch_TieBreakLexicographic(t *testing.T) {
	// "abx" is dist=1 from both "abc" and "aby". With both within threshold,
	// the lexicographically-first wins.
	candidates := []string{"aby", "abc", "abz"}
	got, ok := validation.ClosestMatch("abx", candidates)
	if !ok {
		t.Fatal("expected a match")
	}
	if got != "abc" {
		t.Errorf("got=%q, want %q", got, "abc")
	}
}

func TestClosestMatch_EmptyCandidates(t *testing.T) {
	got, ok := validation.ClosestMatch("anything", nil)
	if ok || got != "" {
		t.Errorf("got=(%q,%v), want (\"\",false)", got, ok)
	}
}
