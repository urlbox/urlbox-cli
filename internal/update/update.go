// Package update answers "is there a newer urlbox release?" for the
// post-command notice, `urlbox upgrade`, and `urlbox doctor`.
//
// The latest release comes from the GitHub releases API — the same source
// every installer (npm, Homebrew, Scoop, install.sh) downloads from. The
// answer is cached in the config directory for 24h so the check costs at
// most one request per day.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// LatestReleaseURL is the GitHub API endpoint for the newest non-prerelease.
const LatestReleaseURL = "https://api.github.com/repos/urlbox/urlbox-cli/releases/latest"

// CheckInterval is how long a cached answer stays fresh.
const CheckInterval = 24 * time.Hour

// State is the cached result of the last check.
type State struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest_version,omitempty"`
}

// Stale reports whether the cached answer should be refreshed. A timestamp
// in the future (clock skew, a restored backup) counts as stale.
func (s State) Stale(now time.Time) bool {
	if s.CheckedAt.IsZero() || s.CheckedAt.After(now) {
		return true
	}
	return now.Sub(s.CheckedAt) >= CheckInterval
}

// StatePath is the cache file, next to config.json.
func StatePath() string {
	return filepath.Join(filepath.Dir(config.Path()), "update-check.json")
}

// LoadState reads the cache. A missing or unreadable file is a zero State,
// which is always stale.
func LoadState(path string) State {
	var s State
	b, err := os.ReadFile(path) //nolint:gosec // path is the CLI's own cache file
	if err != nil {
		return State{}
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes the cache (0600, parent dir created if needed).
func SaveState(path string, s State) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return config.SafeWriteUserFile(path, b, config.SafeWriteOptions{Force: true})
}

// FetchLatest returns the newest release's version, without a leading "v".
func FetchLatest(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", api.BuildUserAgent(version.Version))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release check returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("release check: %w", err)
	}
	if body.TagName == "" {
		return "", errors.New("release check: no tag_name in response")
	}
	return strings.TrimPrefix(body.TagName, "v"), nil
}

// IsNewer reports whether latest is a newer release than current. Anything
// unparseable (a "dev" build, an empty answer) is never newer, and neither
// is a prerelease: users are only pointed at stable releases.
func IsNewer(current, latest string) bool {
	cur, ok := parse(current)
	if !ok {
		return false
	}
	lat, ok := parse(latest)
	if !ok || lat.pre != "" {
		return false
	}
	for i := range cur.nums {
		if lat.nums[i] != cur.nums[i] {
			return lat.nums[i] > cur.nums[i]
		}
	}
	// Same x.y.z: a stable release beats the current prerelease of it.
	return cur.pre != ""
}

// IsRelease reports whether v is a comparable release version. Local
// builds report "dev" and are never checked.
func IsRelease(v string) bool {
	_, ok := parse(v)
	return ok
}

// Notice is the one-line message shown after a command.
func Notice(current, latest string) string {
	return fmt.Sprintf("A new version of urlbox is available: %s → %s. Run `urlbox upgrade` to update.",
		strings.TrimPrefix(current, "v"), latest)
}

type semver struct {
	nums [3]int
	pre  string
}

func parse(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		s.nums[i] = n
	}
	if gitDescribe.MatchString(pre) {
		return semver{}, false // a local build past the tag, not a release
	}
	s.pre = pre
	return s, true
}

// gitDescribe matches the suffix `git describe --dirty` adds after a tag:
// "-dirty", "-14-g3a164cc" or "-1-gc9d96d6-dirty".
var gitDescribe = regexp.MustCompile(`^(\d+-g[0-9a-f]+(-dirty)?|dirty)$`)
