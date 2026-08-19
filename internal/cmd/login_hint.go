package cmd

const (
	loginHint = "Run `urlbox login` to sign in."

	// credentialHint covers both onboarding paths. The device flow needs a
	// browser, so anything that only wants a render credential must also
	// point at the headless route — otherwise CI, agents, and any
	// browserless box are told to run a command that cannot work there.
	credentialHint = "Run `urlbox login` to sign in, or for headless/CI use `urlbox auth --api-secret <secret>` (also --api-secret-stdin / --api-secret-file) or set URLBOX_API_SECRET. Get your secret from https://urlbox.com/dashboard/projects." //nolint:gosec // user-facing help text naming flags, not a credential
	notLoggedInMsg = "not logged in — run `urlbox login`"
)
