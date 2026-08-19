package cmd

import (
	"context"
	"errors"

	"github.com/urlbox/urlbox-cli/internal/api"
)

type renderCredential struct {
	key    string
	secret string
}

func pickAPICredential(creds []map[string]any) renderCredential {
	for _, c := range creds {
		if revoked, _ := c["revoked"].(bool); revoked {
			continue
		}
		if secret := valueOrEmpty(c["apiSecret"]); secret != "" {
			return renderCredential{key: valueOrEmpty(c["apiKey"]), secret: secret}
		}
	}
	return renderCredential{}
}

func apiCredentialsPath(org, project string) string {
	return "/v2/organisation/" + org + "/projects/" + project + "/api-credentials"
}

func fetchRenderCredential(ctx context.Context, client api.SessionAPI, org, project string) (renderCredential, error) {
	creds, err := fetchList(ctx, client, apiCredentialsPath(org, project), "apiCredentials")
	if err != nil {
		return renderCredential{}, err
	}
	return pickAPICredential(creds), nil
}

func ensureRenderCredential(ctx context.Context, client api.SessionAPI, org, project string, interactive bool, pick pickFunc) (cred renderCredential, issued bool, err error) {
	cred, err = fetchRenderCredential(ctx, client, org, project)
	if err != nil || cred.secret != "" {
		return cred, false, err
	}
	if interactive {
		idx, perr := pick("No render credential on this project — issue one?", []string{"Issue a new credential", "Skip"}, 0)
		if perr != nil && !errors.Is(perr, errNotInteractivePick) {
			return renderCredential{}, false, nil
		}
		if perr == nil && idx == 1 {
			return renderCredential{}, false, nil
		}
	}
	var created map[string]any
	if err := client.PostJSON(ctx, apiCredentialsPath(org, project), map[string]string{}, &created); err != nil {
		return renderCredential{}, false, err
	}
	return renderCredential{key: valueOrEmpty(created["apiKey"]), secret: valueOrEmpty(created["apiSecret"])}, true, nil
}
