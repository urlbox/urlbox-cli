// internal/cmd/secret_validate.go
//
// Round 8 FF: the validation logic lives in package config now so that
// config.Resolve can apply the same rule to URLBOX_API_SECRET env
// values. This file keeps the cmd-side name (validateSecretValue) as a
// thin forward, so existing call sites are unchanged.
package cmd

import (
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func validateSecretValue(raw string) (string, *output.CLIError) {
	return config.ValidateSecretValue(raw)
}
