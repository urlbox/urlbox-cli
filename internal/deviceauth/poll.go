// Package deviceauth drives the OAuth device-authorization polling loop the
// login command uses to exchange a device code for a session token.
package deviceauth

import (
	"time"

	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// Exchange is one device-token exchange result. AccessToken is set on success;
// RFCCode carries the RFC 8628 error string (authorization_pending, slow_down,
// access_denied, expired_token); Err carries a transport-level failure.
type Exchange struct {
	AccessToken string
	RFCCode     string
	Err         error
}

// Poll runs the device-authorization polling loop against exchange until it
// returns a token, the flow is denied or expired, or expiresIn elapses on clk.
// interval floors at 5s; slow_down widens it by 5s. Transport errors from
// exchange are ignored and polling continues until the deadline.
func Poll(clk clock.Clock, interval, expiresIn int, exchange func() Exchange) (string, *output.CLIError) {
	if interval <= 0 {
		interval = 5
	}
	deadline := clk.Now().Add(time.Duration(expiresIn) * time.Second)
	for clk.Now().Before(deadline) {
		clk.Sleep(time.Duration(interval) * time.Second)
		e := exchange()
		if e.Err == nil && e.AccessToken != "" {
			return e.AccessToken, nil
		}
		switch e.RFCCode {
		case "authorization_pending", "":
			continue
		case "slow_down":
			interval += 5
			continue
		case "access_denied":
			return "", output.NewCLIError(output.ErrAuth, "Login denied.", "Approve the request in your browser, then run `urlbox login` again.")
		case "expired_token":
			return "", output.NewCLIError(output.ErrAuth, "Code expired — run `urlbox login` again.", "Device codes are short-lived; restart the login to get a fresh code.")
		}
	}
	return "", output.NewCLIError(output.ErrAuth, "Code expired — run `urlbox login` again.", "Device codes are short-lived; restart the login to get a fresh code.")
}
