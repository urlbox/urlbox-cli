package config

// Profile is one named credential set persisted in Config.Profiles.
type Profile struct {
	APIKey        string `json:"api_key,omitempty"`
	APISecret     string `json:"api_secret,omitempty"`
	APIHost       string `json:"api_host,omitempty"`
	SessionToken  string `json:"session_token,omitempty"`
	ActiveOrg     string `json:"active_org,omitempty"`
	ActiveProject string `json:"active_project,omitempty"`
}

// IsEmpty reports whether the profile has no credentials at all.
//
//nolint:gocritic // value receiver is required: callers invoke IsEmpty on non-addressable composite literals.
func (p Profile) IsEmpty() bool {
	return p.APIKey == "" && p.APISecret == "" && p.APIHost == "" &&
		p.SessionToken == "" && p.ActiveOrg == "" && p.ActiveProject == ""
}
