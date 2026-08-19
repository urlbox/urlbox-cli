package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

type storageFlags struct {
	name, provider, bucket, region, endpoint, key, secret, cdnHost string
	accountName, containerName, sasToken                           string
	privateBucket, objectLock                                      bool
	set                                                            map[string]bool
}

type llmFlags struct {
	name, provider, apiKey, model, baseURL string
	set                                    map[string]bool
}

var s3Providers = map[string]bool{
	"aws_s3":               true,
	"google_cloud_storage": true,
	"cloudflare_r2":        true,
	"backblaze_b2":         true,
	"digitalocean_spaces":  true,
	"wasabi":               true,
	"custom":               true,
	"minio":                true,
}

var s3ProvidersNeedingEndpoint = map[string]bool{
	"google_cloud_storage": true,
	"cloudflare_r2":        true,
	"backblaze_b2":         true,
	"digitalocean_spaces":  true,
	"wasabi":               true,
	"custom":               true,
	"minio":                true,
}

func parseJSONBody(jsonBody string) (map[string]any, *output.CLIError) {
	body := map[string]any{}
	if jsonBody == "" {
		return body, nil
	}
	if err := json.Unmarshal([]byte(jsonBody), &body); err != nil {
		return nil, output.NewCLIError(output.ErrUsage, "--json is not a valid JSON object: "+err.Error(),
			`Example: --json '{"key":"value"}'.`)
	}
	return body, nil
}

func buildStorageBody(jsonBody string, f storageFlags, requireCreate bool) (map[string]any, *output.CLIError) { //nolint:gocritic // storageFlags is the flag-value struct passed by value from the command layer
	body, cliErr := parseJSONBody(jsonBody)
	if cliErr != nil {
		return nil, cliErr
	}
	if f.set["name"] {
		body["name"] = f.name
	}
	if f.set["bucket"] {
		body["bucket"] = f.bucket
	}
	if f.set["region"] {
		body["region"] = f.region
	}
	if f.set["endpoint"] {
		body["endpoint"] = f.endpoint
	}
	if f.set["key"] {
		body["key"] = f.key
	}
	if f.set["secret"] {
		body["secret"] = f.secret
	}
	if f.set["cdn-host"] {
		body["cdnHost"] = f.cdnHost
	}
	if f.set["account-name"] {
		body["accountName"] = f.accountName
	}
	if f.set["container-name"] {
		body["containerName"] = f.containerName
	}
	if f.set["sas-token"] {
		body["sasToken"] = f.sasToken
	}
	if f.set["private-bucket"] {
		body["privateBucket"] = f.privateBucket
	}
	if f.set["object-lock"] {
		body["objectLock"] = f.objectLock
	}
	if f.set["provider"] {
		switch {
		case f.provider == "azure":
			body["type"] = "azure"
			delete(body, "provider")
		case s3Providers[f.provider]:
			body["type"] = "s3"
			body["provider"] = f.provider
		default:
			return nil, output.NewCLIError(output.ErrUsage,
				fmt.Sprintf("unknown --provider %q (one of: aws_s3, google_cloud_storage, cloudflare_r2, backblaze_b2, digitalocean_spaces, wasabi, custom, azure)", f.provider),
				"Pass a supported --provider value.")
		}
	}
	if requireCreate {
		if cliErr := validateStorageCreate(body); cliErr != nil {
			return nil, cliErr
		}
	}
	return body, nil
}

func validateStorageCreate(body map[string]any) *output.CLIError {
	usage := func(msg string) *output.CLIError {
		return output.NewCLIError(output.ErrUsage, msg, "Pass the flags the message names.")
	}
	if _, ok := body["name"]; !ok {
		return usage("--name is required")
	}
	credType := valueOrEmpty(body["type"])
	provider := valueOrEmpty(body["provider"])
	switch credType {
	case "azure":
		if valueOrEmpty(body["accountName"]) == "" ||
			valueOrEmpty(body["containerName"]) == "" ||
			valueOrEmpty(body["sasToken"]) == "" {
			return usage("azure requires --account-name, --container-name, and --sas-token")
		}
		return nil
	case "s3":
		if valueOrEmpty(body["bucket"]) == "" ||
			valueOrEmpty(body["key"]) == "" ||
			valueOrEmpty(body["secret"]) == "" ||
			valueOrEmpty(body["region"]) == "" {
			return usage("storage requires --bucket, --key, --secret, and --region")
		}
		if s3ProvidersNeedingEndpoint[provider] && valueOrEmpty(body["endpoint"]) == "" {
			return usage("--endpoint is required for non-AWS providers")
		}
		if provider == "cloudflare_r2" && valueOrEmpty(body["cdnHost"]) == "" {
			return usage("--cdn-host is required for this provider")
		}
		return nil
	default:
		return usage("--provider is required (one of: aws_s3, google_cloud_storage, cloudflare_r2, backblaze_b2, digitalocean_spaces, wasabi, custom, azure)")
	}
}

func buildProxyCreateBody(name string, urls []string) (map[string]any, *output.CLIError) {
	if name == "" {
		return nil, output.NewCLIError(output.ErrUsage, "--name is required", "Pass --name for the proxy pool.")
	}
	if len(urls) == 0 {
		return nil, output.NewCLIError(output.ErrUsage, "at least one --url is required", "Pass one or more --url flags.")
	}
	proxies := make([]map[string]any, len(urls))
	for i, u := range urls {
		proxies[i] = map[string]any{"url": u}
	}
	return map[string]any{"name": name, "proxies": proxies}, nil
}

func mergeProxyUpdate(existing map[string]any, name string, urls []string, set map[string]bool) map[string]any {
	finalName := valueOrEmpty(existing["name"])
	if set["name"] {
		finalName = name
	}
	var proxies []map[string]any
	if len(urls) > 0 {
		for _, u := range urls {
			proxies = append(proxies, map[string]any{"url": u})
		}
	} else {
		entries, _ := existing["proxies"].([]any)
		for _, e := range entries {
			entry, _ := e.(map[string]any)
			if entry == nil {
				continue
			}
			item := map[string]any{"url": valueOrEmpty(entry["url"])}
			if n := valueOrEmpty(entry["name"]); n != "" {
				item["name"] = n
			}
			proxies = append(proxies, item)
		}
	}
	return map[string]any{"name": finalName, "proxies": proxies}
}

func buildLlmBody(jsonBody string, f llmFlags, requireCreate bool) (map[string]any, *output.CLIError) { //nolint:gocritic // llmFlags is the flag-value struct passed by value from the command layer
	body, cliErr := parseJSONBody(jsonBody)
	if cliErr != nil {
		return nil, cliErr
	}
	if f.set["name"] {
		body["name"] = f.name
	}
	if f.set["provider"] {
		body["provider"] = f.provider
	}
	if f.set["api-key"] {
		body["apiKey"] = f.apiKey
	}
	if f.set["model"] {
		body["model"] = f.model
	}
	if f.set["base-url"] {
		body["baseUrl"] = f.baseURL
	}
	if requireCreate {
		if _, ok := body["name"]; !ok {
			return nil, output.NewCLIError(output.ErrUsage, "--name is required", "Pass --name for the credential.")
		}
		if _, ok := body["provider"]; !ok {
			return nil, output.NewCLIError(output.ErrUsage, "--provider is required", "Pass --provider for the credential.")
		}
		if cliErr := validateLlmCreate(body); cliErr != nil {
			return nil, cliErr
		}
	} else if _, ok := body["provider"]; ok {
		return nil, output.NewCLIError(output.ErrUsage,
			"provider is immutable after create — create a new credential instead",
			"Create a new credential with the desired provider.")
	}
	return body, nil
}

func validateLlmCreate(body map[string]any) *output.CLIError {
	usage := func(msg string) *output.CLIError {
		return output.NewCLIError(output.ErrUsage, msg, "Pass the fields the message names.")
	}
	missing := func(fields ...string) []string {
		var out []string
		for _, f := range fields {
			if valueOrEmpty(body[f]) == "" {
				out = append(out, f)
			}
		}
		return out
	}
	provider := valueOrEmpty(body["provider"])
	switch provider {
	case "amazon-bedrock":
		if m := missing("awsRegion", "awsAccessKeyId", "awsSecretAccessKey"); len(m) > 0 {
			return usage("amazon-bedrock requires " + strings.Join(m, ", ") + " — pass them via --json")
		}
	case "google-vertex":
		if m := missing("gcpProject", "gcpLocation", "gcpServiceAccountJson"); len(m) > 0 {
			return usage("google-vertex requires " + strings.Join(m, ", ") + " — pass them via --json")
		}
	case "azure":
		if len(missing("apiKey")) > 0 {
			return usage("--api-key is required")
		}
		if len(missing("azureResourceName")) > 0 && len(missing("baseUrl")) > 0 {
			return usage("azure requires --base-url or azureResourceName (via --json)")
		}
	default:
		if len(missing("apiKey")) > 0 {
			return usage("--api-key is required")
		}
	}
	return nil
}
