// internal/validation/fuzzy.go
package validation

import (
	"sort"
	"strings"
)

// Levenshtein returns the case-insensitive edit distance between a and b.
// Insert, delete, and substitute each cost 1; transpositions cost 2.
func Levenshtein(a, b string) int {
	la, lb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	if len(la) == 0 {
		return len(lb)
	}
	if len(lb) == 0 {
		return len(la)
	}

	prev := make([]int, len(lb)+1)
	curr := make([]int, len(lb)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(la); i++ {
		curr[0] = i
		for j := 1; j <= len(lb); j++ {
			cost := 1
			if la[i-1] == lb[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(lb)]
}

// ClosestMatch returns the best candidate for want within an edit-distance
// threshold (≤2 for len(want) ≤7, else ≤3). Ties broken lexicographically.
// Returns ("", false) if no candidate is within threshold.
func ClosestMatch(want string, candidates []string) (string, bool) {
	if len(candidates) == 0 {
		return "", false
	}
	threshold := 2
	if len(want) > 7 {
		threshold = 3
	}

	type scored struct {
		name string
		dist int
	}
	var hits []scored
	for _, c := range candidates {
		d := Levenshtein(want, c)
		if d <= threshold {
			hits = append(hits, scored{name: c, dist: d})
		}
	}
	if len(hits) == 0 {
		return "", false
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	return hits[0].name, true
}
