package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// httpTimeout caps each individual HTTP check (api_reachable and the
// render_credential live probe).
// Set to 10s rather than the original 5s to absorb cold-container
// startup costs — Round 5 CI-1 reproed a false-fail on the first
// invocation in a fresh container because DNS+TCP+TLS to api.urlbox.com
// blew through the 5s budget even though warm-cache curl returned in
// ~350ms. The outer doctor context is sized to fit all checks at this
// per-check limit.
const httpTimeout = 10 * time.Second

// Check is one diagnostic result.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "ok" | "warn" | "fail"
	Message string `json:"message,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check installation, configuration, network, and credentials",
		Long: `Runs a series of self-checks: version, install method, config file,
session, active organisation and project, render credential, DNS
resolution, and API reachability.
Exits non-zero if any check fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Sized to fit session + DNS + api_reachable + the
			// render_credential probe (each httpTimeout = 10s) plus a
			// little headroom. Round 5 CI-1 bumped the per-check timeout
			// to absorb cold-start latency.
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			// Round 6 Z class-fix: doctor previously called
			// config.ResolveAPISecret() directly, which always looks at
			// the default profile and ignores --profile / URLBOX_PROFILE.
			// Now it goes through config.Resolve — the same path
			// render/status/link use — so unknown profile names error
			// instead of falling back to default, and `--profile work`
			// actually checks the work profile's credentials.
			profileFlag, _ := cmd.Root().PersistentFlags().GetString("profile")
			cfg, _ := config.Load() // missing config is reported by checkConfigFile
			overlay, ovErr := loadRepoOverlay()
			if ovErr != nil {
				return ovErr
			}
			resolved, rerr := config.Resolve(config.ResolveOptions{
				FlagProfile:  profileFlag,
				EnvAPISecret: os.Getenv(config.EnvAPISecret),
				EnvProfile:   os.Getenv(config.EnvProfile),
				EnvAPIHost:   os.Getenv(config.EnvAPIHost),
				RepoOverlay:  overlay,
				Config:       cfg,
			})
			if rerr != nil {
				var cli *output.CLIError
				if errors.As(rerr, &cli) {
					return cli
				}
				return output.NewCLIError(
					output.ErrUsage,
					"failed to resolve profile",
					rerr.Error(),
				)
			}

			profile := config.Profile{}
			if cfg != nil {
				profile = cfg.Profiles[resolved.Profile]
			}

			checks := runDoctorChecks(ctx, resolved, &profile)
			anyFail := false
			for _, c := range checks {
				if c.Status == "fail" {
					anyFail = true
				}
			}

			summary := "All checks passed"
			if anyFail {
				summary = "Some checks failed — see hints for next steps"
			}
			// Overall status string for quiet-mode scalar output and JSON
			// consumers who want a single field to switch on.
			overall := "ok"
			if anyFail {
				overall = "fail"
			}
			env := output.NewEnvelope(
				"doctor",
				map[string]any{"checks": checks, "status": overall},
				summary,
				[]output.Breadcrumb{
					{Action: "login", Cmd: "urlbox login"},
				},
			)
			// Reflect failure state on the envelope's `ok` field so JSON
			// consumers see `ok: false` alongside the failed checks.
			if anyFail {
				env.OK = false
			}
			env.SetTable([]string{"", "CHECK", "MESSAGE", "HINT"}, doctorCheckRows(checks), -1)

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)

			var writeErr error
			switch {
			case jqExpr != "":
				writeErr = output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			case format == output.FormatQuiet:
				// Round 8 JJ: quiet mode used to print the whole checks
				// tree (broke the "single scalar" contract). Print the
				// overall status string instead — agents can pipe it.
				_, writeErr = fmt.Fprintln(stdout, overall)
			default:
				formatter := output.NewFormatter(format, styles)
				writeErr = formatter.WriteSuccess(stdout, env)
			}
			if writeErr != nil {
				return writeErr
			}

			if anyFail {
				// Round 8 JJ: pick the exit code based on which checks
				// failed, rather than always returning ErrServer (10).
				// The contract:
				//   3  (auth)    — credential / api_secret problem
				//   11 (network) — DNS / unreachable
				//   10 (server)  — last resort / defensive default
				return &output.CLIError{
					Code:    doctorExitCode(checks),
					Message: summary,
					Silent:  true,
				}
			}
			return nil
		},
	}
}

func doctorCheckRows(checks []Check) [][]string {
	rows := make([][]string, len(checks))
	for i, c := range checks {
		rows[i] = []string{checkStatusGlyph(c.Status), c.Name, c.Message, c.Hint}
	}
	return rows
}

func checkStatusGlyph(status string) string {
	switch status {
	case "ok":
		return "✓"
	case "warn":
		return "!"
	default:
		return "✗"
	}
}

// doctorExitCode maps the worst failing check to the closed-set exit
// code so agents reading the exit value can branch on the failure
// category. Priority: auth-related credential issues first (3), then
// network/DNS reachability (11), then everything else as server (10).
func doctorExitCode(checks []Check) output.ErrorCode {
	var hasAuth, hasNetwork, hasOther bool
	for _, c := range checks {
		if c.Status != "fail" {
			continue
		}
		switch c.Name {
		case "session", "render_credential":
			hasAuth = true
		case "dns", "api_reachable":
			hasNetwork = true
		default:
			hasOther = true
		}
	}
	switch {
	case hasAuth:
		return output.ErrAuth
	case hasNetwork:
		return output.ErrNetwork
	case hasOther:
		return output.ErrServer
	}
	return output.ErrServer // unreachable when anyFail, defensive default
}

func runDoctorChecks(ctx context.Context, resolved *config.Resolved, profile *config.Profile) []Check {
	host := api.ResolveAPIHost()
	if resolved != nil && resolved.APIHost != "" {
		host = resolved.APIHost
	}
	return []Check{
		checkVersion(),
		checkInstallMethod(),
		checkConfigFile(),
		checkSession(ctx, host, profile),
		checkActiveOrg(profile),
		checkActiveProject(profile),
		checkRenderCredential(ctx, host, resolved),
		checkDNS(ctx, host),
		checkAPIReachable(ctx, host),
	}
}

func checkVersion() Check {
	return Check{Name: "version", Status: "ok", Message: version.Version}
}

func checkInstallMethod() Check {
	execPath, err := os.Executable()
	if err != nil {
		return Check{Name: "install_method", Status: "warn", Message: "could not determine binary path"}
	}
	method := DetectInstallMethod(execPath)
	if method == "unknown" {
		return Check{
			Name:    "install_method",
			Status:  "warn",
			Message: "unknown",
			Hint:    "Install via brew, scoop, npm, or curl install.sh for upgrade support",
		}
	}
	return Check{Name: "install_method", Status: "ok", Message: method}
}

func checkConfigFile() Check {
	p := config.Path()
	if _, err := os.Stat(p); err == nil {
		return Check{Name: "config_file", Status: "ok", Message: p}
	}
	return Check{
		Name:    "config_file",
		Status:  "warn",
		Message: "missing",
		Hint:    loginHint,
	}
}

func checkSession(ctx context.Context, host string, profile *config.Profile) Check {
	if profile.SessionToken == "" {
		return Check{
			Name:    "session",
			Status:  "fail",
			Message: notLoggedInMsg,
			Hint:    loginHint,
		}
	}
	client := api.NewSessionClient(host, profile.SessionToken)
	var session sessionResponse
	if err := client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return Check{
			Name:    "session",
			Status:  "fail",
			Message: "session token rejected",
			Hint:    loginHint,
		}
	}
	if session.User.Email == "" {
		return Check{
			Name:    "session",
			Status:  "fail",
			Message: "session expired",
			Hint:    loginHint,
		}
	}
	return Check{Name: "session", Status: "ok", Message: "signed in as " + session.User.Email}
}

func checkActiveOrg(profile *config.Profile) Check {
	if profile.ActiveOrg == "" {
		return Check{
			Name:    "active_org",
			Status:  "fail",
			Message: "no active organisation",
			Hint:    "Select one with `urlbox orgs select`.",
		}
	}
	return Check{Name: "active_org", Status: "ok", Message: profile.ActiveOrg}
}

func checkActiveProject(profile *config.Profile) Check {
	if profile.ActiveProject == "" {
		return Check{
			Name:    "active_project",
			Status:  "fail",
			Message: "no active project",
			Hint:    "Select one with `urlbox projects select`.",
		}
	}
	return Check{Name: "active_project", Status: "ok", Message: profile.ActiveProject}
}

func checkRenderCredential(ctx context.Context, host string, resolved *config.Resolved) Check {
	if resolved == nil || resolved.APISecret == "" {
		return Check{
			Name:    "render_credential",
			Status:  "fail",
			Message: "no render credential",
			Hint:    loginHint + " CI and headless environments can set URLBOX_API_SECRET instead.",
		}
	}
	src := resolved.Source.APISecret
	if src == "profile" {
		src = "file"
	}

	endpoint := host + "/v1/user/me"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return Check{Name: "render_credential", Status: "fail", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+resolved.APISecret)
	req.Header.Set("User-Agent", api.BuildUserAgent(version.Version))

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "render_credential", Status: "fail", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return Check{Name: "render_credential", Status: "ok", Message: "valid (" + src + ")"}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Check{
			Name:    "render_credential",
			Status:  "fail",
			Message: "credential invalid",
			Hint:    loginHint,
		}
	case resp.StatusCode >= 500:
		return Check{
			Name:    "render_credential",
			Status:  "warn",
			Message: fmt.Sprintf("API returned %d", resp.StatusCode),
		}
	default:
		msg := fmt.Sprintf("API returned %d", resp.StatusCode)
		if apiMsg := readAPIErrorMessage(resp); apiMsg != "" {
			msg = fmt.Sprintf("API returned %d: %s", resp.StatusCode, apiMsg)
		}
		return Check{
			Name:    "render_credential",
			Status:  "fail",
			Message: msg,
			Hint:    loginHint + " Or check `urlbox config get api_secret --reveal` against the dashboard.",
		}
	}
}

func checkDNS(ctx context.Context, host string) Check {
	u, err := url.Parse(host)
	if err != nil || u.Host == "" {
		return Check{Name: "dns", Status: "warn", Message: "no host to check"}
	}
	if _, err := net.DefaultResolver.LookupHost(ctx, u.Hostname()); err != nil {
		return Check{
			Name:    "dns",
			Status:  "fail",
			Message: err.Error(),
			Hint:    "Check network / DNS resolver",
		}
	}
	return Check{Name: "dns", Status: "ok", Message: u.Hostname() + " resolves"}
}

func checkAPIReachable(ctx context.Context, host string) Check {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host+"/", http.NoBody)
	if err != nil {
		return Check{Name: "api_reachable", Status: "fail", Message: err.Error()}
	}
	req.Header.Set("User-Agent", api.BuildUserAgent(version.Version))
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "api_reachable", Status: "fail", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	return Check{
		Name:    "api_reachable",
		Status:  "ok",
		Message: fmt.Sprintf("HTTP %d from %s", resp.StatusCode, host),
	}
}

// readAPIErrorMessage best-effort extracts `error.message` from the Urlbox
// API's standard error envelope: {"error":{"code":"...","message":"..."}}.
// Returns "" on any parse / shape failure — callers must fall back to a
// generic message.
func readAPIErrorMessage(resp *http.Response) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if err := dec.Decode(&body); err != nil {
		return ""
	}
	return body.Error.Message
}
