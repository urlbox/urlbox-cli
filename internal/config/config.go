// Package config handles XDG-aware persistent configuration for the urlbox CLI.
// In Phase 1 it stores a single API key; full profile support comes in Phase 3.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// EnvAPIKey is the env var that overrides any saved API key.
const EnvAPIKey = "URLBOX_API_SECRET" //nolint:gosec // env var name, not a credential

// Config is the persisted CLI configuration.
type Config struct {
	APIKey string `json:"api_key,omitempty"`
}

// Path returns the config file location (XDG-aware).
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "urlbox", "config.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "urlbox", "config.json")
}

// Load reads the config file. Missing file returns an empty config without error.
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
	return &c, nil
}

// Save writes the config with 0600 permissions, creating parent dirs as needed.
func Save(c *Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ") //nolint:gosec // serializing our own config to a 0600 file we own
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// ResolveAPIKey returns the API key, preferring the env var over the file.
func ResolveAPIKey() string {
	if v := os.Getenv(EnvAPIKey); v != "" {
		return v
	}
	c, err := Load()
	if err != nil {
		return ""
	}
	return c.APIKey
}

// APIKeySource describes where the resolved API key came from: "env", "file", or "none".
func APIKeySource() string {
	if os.Getenv(EnvAPIKey) != "" {
		return "env"
	}
	c, err := Load()
	if err == nil && c.APIKey != "" {
		return "file"
	}
	return "none"
}
