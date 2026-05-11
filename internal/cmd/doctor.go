package cmd

import (
	"context"
	"encoding/json"
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

const httpTimeout = 5 * time.Second

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
API key, DNS resolution, API reachability, and credential validity.
Exits non-zero if any check fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			checks := runDoctorChecks(ctx)
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
			env := output.NewEnvelope(
				"doctor",
				map[string]any{"checks": checks},
				summary,
				[]output.Breadcrumb{
					{Action: "auth", Cmd: "urlbox auth --api-secret <secret>"},
				},
			)
			// Reflect failure state on the envelope's `ok` field so JSON
			// consumers see `ok: false` alongside the failed checks. Exit
			// code stays 10 via the silent CLIError below.
			if anyFail {
				env.OK = false
			}

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)

			var writeErr error
			if jqExpr != "" {
				writeErr = output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			} else {
				formatter := output.NewFormatter(format, styles)
				writeErr = formatter.WriteSuccess(stdout, env)
			}
			if writeErr != nil {
				return writeErr
			}

			if anyFail {
				return &output.CLIError{
					Code:    output.ErrServer,
					Message: summary,
					Silent:  true,
				}
			}
			return nil
		},
	}
}

func runDoctorChecks(ctx context.Context) []Check {
	host := api.ResolveAPIHost()
	return []Check{
		checkVersion(),
		checkInstallMethod(),
		checkConfigFile(),
		checkAPISecret(),
		checkDNS(ctx, host),
		checkAPIReachable(ctx, host),
		checkAuth(ctx, host),
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
		Hint:    "Run `urlbox auth --api-secret <secret>` to create",
	}
}

func checkAPISecret() Check {
	src := config.APISecretSource()
	if src == "none" {
		return Check{
			Name:    "api_secret",
			Status:  "fail",
			Message: "no API secret found",
			Hint:    "Set URLBOX_API_SECRET or run `urlbox auth --api-secret <secret>`",
		}
	}
	return Check{Name: "api_secret", Status: "ok", Message: "configured (" + src + ")"}
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

func checkAuth(ctx context.Context, host string) Check {
	key := config.ResolveAPISecret()
	if key == "" {
		return Check{Name: "auth", Status: "warn", Message: "skipped (no API secret)"}
	}
	endpoint := host + "/v1/user/me"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return Check{Name: "auth", Status: "fail", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", api.BuildUserAgent(version.Version))

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "auth", Status: "fail", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	// Auth status is determined by the HTTP class:
	//   - 2xx → credentials accepted
	//   - 401/403 → explicit credential rejection
	//   - other 4xx (e.g. 400 "Api Key does not exist", 404, 429) → fail too
	//   - 5xx → warn; we can't tell whether creds are valid when the API is sick
	//
	// Round 4 H2: before this, only 401/403/5xx fell out of "ok". A real
	// production 400 with body `{"error":{"code":"ApiKeyNotFound",...}}`
	// silently reported "credentials valid", which let CI green-light a
	// broken secret.
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return Check{Name: "auth", Status: "ok", Message: "credentials valid"}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Check{
			Name:    "auth",
			Status:  "fail",
			Message: "credentials rejected",
			Hint:    "Re-run `urlbox auth --api-secret <secret>` with a valid secret",
		}
	case resp.StatusCode >= 500:
		return Check{
			Name:    "auth",
			Status:  "warn",
			Message: fmt.Sprintf("API returned %d", resp.StatusCode),
		}
	default:
		// Non-2xx, non-401/403, non-5xx — typically a 400 ApiKeyNotFound
		// or 404. Treat as a credential failure; surface the API's
		// error message if it parses as the standard envelope.
		msg := fmt.Sprintf("API returned %d", resp.StatusCode)
		if apiMsg := readAPIErrorMessage(resp); apiMsg != "" {
			msg = fmt.Sprintf("API returned %d: %s", resp.StatusCode, apiMsg)
		}
		return Check{
			Name:    "auth",
			Status:  "fail",
			Message: msg,
			Hint:    "Re-run `urlbox auth --api-secret <secret>` with a valid secret, or check `urlbox config get api_secret --reveal` against the dashboard.",
		}
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
