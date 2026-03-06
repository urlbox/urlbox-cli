package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Profile struct {
	APIHost   string `json:"api_host,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`
}

type File struct {
	DefaultProfile string             `json:"default_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles,omitempty"`
}

type Runtime struct {
	APIHost      string
	APIKey       string
	APISecret    string
	OutputFormat string
	ProfileName  string
}

type Options struct {
	Profile      string
	APIHost      string
	OutputFormat string
}

func Load(options Options) Runtime {
	cfg, _ := Read()

	profileName := options.Profile
	if profileName == "" {
		profileName = os.Getenv("URLBOX_PROFILE")
	}
	if profileName == "" {
		profileName = cfg.DefaultProfile
	}
	if profileName == "" {
		profileName = "default"
	}

	runtime := Runtime{
		APIHost:      "https://api.urlbox.com",
		OutputFormat: os.Getenv("URLBOX_OUTPUT_FORMAT"),
		ProfileName:  profileName,
	}

	if profile, ok := cfg.Profiles[profileName]; ok {
		if profile.APIHost != "" {
			runtime.APIHost = profile.APIHost
		}
		runtime.APIKey = profile.APIKey
		runtime.APISecret = profile.APISecret
	}

	if value := os.Getenv("URLBOX_API_HOST"); value != "" {
		runtime.APIHost = value
	}
	if value := os.Getenv("URLBOX_API_KEY"); value != "" {
		runtime.APIKey = value
	}
	if value := os.Getenv("URLBOX_API_SECRET"); value != "" {
		runtime.APISecret = value
	}
	if options.APIHost != "" {
		runtime.APIHost = options.APIHost
	}
	if options.OutputFormat != "" {
		runtime.OutputFormat = options.OutputFormat
	}

	return runtime
}

func DefaultPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ".urlbox-config.json"
	}
	return filepath.Join(homeDir, ".config", "urlbox", "config.json")
}

func Read() (File, error) {
	path := DefaultPath()
	bytes, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return File{
			Profiles: map[string]Profile{
				"default": {
					APIHost: "https://api.urlbox.com",
				},
			},
		}, nil
	}
	if err != nil {
		return File{}, err
	}

	var cfg File
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return File{}, err
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if _, ok := cfg.Profiles["default"]; !ok {
		cfg.Profiles["default"] = Profile{APIHost: "https://api.urlbox.com"}
	}
	return cfg, nil
}

func Write(cfg File) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	if _, ok := cfg.Profiles["default"]; !ok {
		cfg.Profiles["default"] = Profile{APIHost: "https://api.urlbox.com"}
	}

	path := DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	bytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, append(bytes, '\n'), 0644)
}

func GetProfile(cfg File, name string) (Profile, bool) {
	profile, ok := cfg.Profiles[name]
	return profile, ok
}

func SetProfile(cfg File, name string, profile Profile) File {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	cfg.Profiles[name] = profile
	return cfg
}

func MustProfile(cfg File, name string) (Profile, error) {
	profile, ok := GetProfile(cfg, name)
	if !ok {
		return Profile{}, fmt.Errorf("profile %q not found", name)
	}
	return profile, nil
}
