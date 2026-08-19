package cmd

const (
	loginHint = "Run `urlbox login` to sign in."

	// credentialHint covers both onboarding paths. The device flow needs a
	// browser, so anything that only wants a render credential must also
	// point at the headless route — otherwise CI, agents, and any
	// browserless box are told to run a command that cannot work there.
	// `config profile create` is that route: unlike `config set` it works
	// from zero profiles, so it bootstraps a bare machine.
	credentialHint = "Run `urlbox login` to sign in. Headless/CI: set URLBOX_API_SECRET, or store one with `printf %s \"$URLBOX_API_SECRET\" | urlbox config profile create default --api-secret-stdin`. Get your secret from https://urlbox.com/dashboard/projects." //nolint:gosec // user-facing help text naming flags, not a credential
	notLoggedInMsg = "not logged in — run `urlbox login`"
)
