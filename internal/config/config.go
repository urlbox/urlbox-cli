// Package config handles XDG-aware persistent configuration for the urlbox CLI.
// Multi-profile storage with transparent migration of Phase 1 files on first save.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is the persisted CLI configuration. Multi-profile.
type Config struct {
	DefaultProfile string             `json:"default_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles,omitempty"`

	// LegacyAPIKey holds the top-level api_key field from Phase 1 single-key
	// files. Populated only by Load when reading a legacy file. Never written
	// back: Save strips it so the first persist after upgrade migrates the
	// file to the new shape. Phase 1 stored the *secret* in this field
	// (mislabel — see CHANGELOG v0.6.0); Load migrates the value into
	// Profiles["default"].APISecret.
	LegacyAPIKey string `json:"api_key,omitempty"`
}

// Path returns the config file location (XDG-aware).
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "urlbox", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "urlbox", "config.json")
}

// Load reads the config file. Missing file returns an empty config without
// error. A Phase 1 flat-shape file (`{"api_key": "..."}`) is migrated in
// memory: LegacyAPIKey is set, and Profiles["default"].APISecret +
// DefaultProfile = "default" are populated. The flat api_key field always
// held the *secret* in Phase 1 (the field name was a mislabel).
func Load() (*Config, error) {
	b, err := os.ReadFile(Path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.LegacyAPIKey != "" && len(c.Profiles) == 0 {
		c.Profiles = map[string]Profile{"default": {APISecret: c.LegacyAPIKey}}
		c.DefaultProfile = "default"
	}
	return &c, nil
}

// Save writes the config atomically with 0600 permissions. The temp file is
// created in the same directory so os.Rename stays on a single filesystem.
// LegacyAPIKey is never written back: a migrated config drops the legacy
// top-level api_key on first save.
func Save(c *Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	out := Config{
		DefaultProfile: c.DefaultProfile,
		Profiles:       c.Profiles,
	}
	b, err := json.MarshalIndent(out, "", "  ") //nolint:gosec // serializing our own config to a 0600 file we own
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), "config.json.tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, p)
}
