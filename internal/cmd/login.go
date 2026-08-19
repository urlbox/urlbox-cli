package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/deviceauth"
	"github.com/urlbox/urlbox-cli/internal/output"
)

var loginClock clock.Clock = clock.New()

// SetLoginClockForTest swaps the package-level loginClock so the device-poll
// loop runs in synthetic time. Pair with t.Cleanup(ResetLoginClockForTest).
func SetLoginClockForTest(c clock.Clock) { loginClock = c }

// ResetLoginClockForTest restores the real wall clock.
func ResetLoginClockForTest() { loginClock = clock.New() }

var loginOpener browser.Opener = browser.NewOSOpener()

// SetLoginOpenerForTest swaps in a fake browser.Opener for the login command.
// Pair with t.Cleanup(ResetLoginOpenerForTest).
func SetLoginOpenerForTest(o browser.Opener) { loginOpener = o }

// ResetLoginOpenerForTest restores the production OSOpener.
func ResetLoginOpenerForTest() { loginOpener = browser.NewOSOpener() }

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri_complete"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`
}

type loginFlags struct {
	org     string
	project string
}

func newLoginCmd() *cobra.Command {
	f := &loginFlags{}
	c := &cobra.Command{
		Use:   "login",
		Short: "Sign in via your browser (device flow)",
		Long: `Sign in to Urlbox via your browser.

Prints a short code and opens the approval page; once you approve, the CLI
stores a session for management commands, sets your active organisation and
project, and fetches the active project's render credential so render
commands work immediately.

CI and headless environments should set URLBOX_API_SECRET instead — the
device flow needs a browser.

Examples:
  urlbox login
  urlbox login --org acme --project production
  urlbox login --output-format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLogin(cmd, f)
		},
	}
	c.Flags().StringVar(&f.org, "org", "", "Organisation to make active (name or id) — skips the picker")
	c.Flags().StringVar(&f.project, "project", "", "Project to make active (name or id) — skips the picker")
	attachSessionRetryFlags(c)
	return c
}

func renderCredentialLabel(status string) string {
	switch status {
	case "issued":
		return "ready (new credential issued)"
	case "ready":
		return "ready"
	case "error":
		return "error (see messages above)"
	default:
		return "none"
	}
}

func runLogin(cmd *cobra.Command, f *loginFlags) error {
	ctx := context.Background()
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return cliErr
	}
	stderr := cmd.ErrOrStderr()
	anon := newSessionClient(cmd, host, "")

	var code deviceCodeResponse
	if err := anon.PostJSON(ctx, "/v1/auth/device/code", map[string]string{"client_id": "urlbox-cli"}, &code); err != nil {
		return asCLIError(err)
	}

	_, _ = fmt.Fprintf(stderr, "Your code: %s\n", code.UserCode)
	_, _ = fmt.Fprintf(stderr, "Open this URL to continue: %s\n", code.VerificationURI)
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	format := output.ResolveFormat(formatFlag, cmd.OutOrStdout())
	interactive := format != output.FormatJSON && format != output.FormatQuiet
	if interactive {
		_ = loginOpener.Open(code.VerificationURI)
	}
	_, _ = fmt.Fprintln(stderr, "Waiting for approval…")

	exchange := func() deviceauth.Exchange {
		status, data, err := anon.DoRaw(ctx, "POST", "/v1/auth/device/token", map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": code.DeviceCode,
			"client_id":   "urlbox-cli",
		})
		if err != nil {
			return deviceauth.Exchange{Err: err}
		}
		if status < 400 {
			return deviceauth.Exchange{AccessToken: valueOrEmpty(data["access_token"])}
		}
		return deviceauth.Exchange{RFCCode: valueOrEmpty(data["error"])}
	}
	token, pollErr := deviceauth.Poll(loginClock, code.Interval, code.ExpiresIn, exchange)
	if pollErr != nil {
		return pollErr
	}

	if cliErr := updateProfile(profileName, func(p *config.Profile) { p.SessionToken = token }); cliErr != nil {
		return cliErr
	}

	authed := newSessionClient(cmd, host, token)
	org, orgErr := resolveActiveOrg(ctx, authed, f.org, promptPick)
	if orgErr != nil {
		return orgErr
	}
	if org.publicID != "" {
		if cliErr := updateProfile(profileName, func(p *config.Profile) { p.ActiveOrg = org.publicID }); cliErr != nil {
			return cliErr
		}
	}

	project, _, projErr := resolveActiveProject(ctx, authed, f.project, promptPick)
	if projErr != nil {
		return projErr
	}
	renderStatus := "none"
	if project.ID != "" {
		if cliErr := updateProfile(profileName, func(p *config.Profile) { p.ActiveProject = project.ID }); cliErr != nil {
			return cliErr
		}
		cred, issued, err := ensureRenderCredential(ctx, authed, org.publicID, project.ID, interactive, promptPick)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(stderr, "Logged in, but could not fetch the render credential: %v\n", err)
			renderStatus = "error"
		case cred.secret != "":
			if cliErr := updateProfile(profileName, func(p *config.Profile) {
				p.APIKey = cred.key
				p.APISecret = cred.secret
			}); cliErr != nil {
				_, _ = fmt.Fprintf(stderr, "Logged in, but could not save the render credential: %v\n", cliErr)
				renderStatus = "error"
			} else if issued {
				renderStatus = "issued"
			} else {
				renderStatus = "ready"
			}
		}
	} else {
		_, _ = fmt.Fprintln(stderr, "No projects in this organisation yet.")
	}

	data := map[string]any{
		"email":   org.email,
		"org":     map[string]any{"id": org.publicID, "name": org.name},
		"project": nil,
		"render":  map[string]any{"credential": renderStatus},
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	summary := fmt.Sprintf("Logged in as %s — org %s", org.email, org.name)
	breadcrumbs := []output.Breadcrumb{{
		Action: "render",
		Cmd:    "urlbox screenshot https://example.com --output hello.png",
	}}
	env := output.NewEnvelope("login", data, summary, breadcrumbs)
	pairs := identityKVPairs(org.email, org.name, org.publicID, project)
	pairs = append(pairs, [2]string{"Render", renderCredentialLabel(renderStatus)})
	env.SetKV(pairs)
	return writeEnvelopeWithQuietData(cmd, env, org.email)
}
