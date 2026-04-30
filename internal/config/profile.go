package config

// Profile is one named credential set persisted in Config.Profiles.
type Profile struct {
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`
	APIHost   string `json:"api_host,omitempty"`
}

// IsEmpty reports whether the profile has no credentials at all.
func (p Profile) IsEmpty() bool {
	return p.APIKey == "" && p.APISecret == "" && p.APIHost == ""
}
