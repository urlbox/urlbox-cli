package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
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
	endpoint := host + "/v1/account"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return Check{Name: "auth", Status: "fail", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "urlbox-cli/"+version.Version+" "+runtime.GOOS+"/"+runtime.GOARCH)

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Check{Name: "auth", Status: "fail", Message: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return Check{
			Name:    "auth",
			Status:  "fail",
			Message: "credentials rejected",
			Hint:    "Re-run `urlbox auth --api-key <key>` with a valid key",
		}
	case resp.StatusCode >= 500:
		return Check{
			Name:    "auth",
			Status:  "warn",
			Message: fmt.Sprintf("API returned %d", resp.StatusCode),
		}
	default:
		return Check{Name: "auth", Status: "ok", Message: "credentials valid"}
	}
}
