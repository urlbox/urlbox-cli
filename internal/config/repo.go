package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// RepoOverlay is the parsed contents of the nearest .urlbox/config.json found
// when walking up from the working directory. All fields are optional; missing
// fields fall through to lower-priority sources during Resolve.
type RepoOverlay struct {
	Profile   string `json:"profile,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`
	APIHost   string `json:"api_host,omitempty"`
	Path      string `json:"-"`
}

// LoadRepoOverlay walks from start up to (but not including) boundary looking
// for .urlbox/config.json. Returns (nil, nil) if no overlay is found before
// crossing the boundary or reaching the filesystem root.
//
// Boundary semantics: the search stops once dir == boundary. A .urlbox/config.json
// AT boundary is checked but anything strictly above it is not. Phase 3 callers
// pass $HOME so a stray overlay in /tmp or / never affects production renders.
func LoadRepoOverlay(start, boundary string) (*RepoOverlay, error) {
	dir := filepath.Clean(start)
	stop := filepath.Clean(boundary)
	for {
		candidate := filepath.Join(dir, ".urlbox", "config.json")
		b, err := os.ReadFile(candidate)
		if err == nil {
			var o RepoOverlay
			if err := json.Unmarshal(b, &o); err != nil {
				// Round 8 HH: wrap the path in so callers can produce
				// actionable error messages ("fix the JSON at <path>").
				return nil, fmt.Errorf("%s: %w", candidate, err)
			}
			o.Path = candidate
			return &o, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", candidate, err)
		}
		if dir == stop {
			return nil, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}
