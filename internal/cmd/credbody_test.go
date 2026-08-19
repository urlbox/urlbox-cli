package cmd

import (
	"strings"
	"testing"
)

func TestBuildStorageBodyProviderDrivesType(t *testing.T) {
	f := storageFlags{
		name: "s3 cred", provider: "cloudflare_r2", bucket: "b", region: "auto", key: "k", secret: "s", endpoint: "https://x.r2.cloudflarestorage.com", cdnHost: "cdn.example.com",
		set: map[string]bool{"name": true, "provider": true, "bucket": true, "region": true, "key": true, "secret": true, "endpoint": true, "cdn-host": true},
	}
	body, cliErr := buildStorageBody("", f, true)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if body["type"] != "s3" || body["provider"] != "cloudflare_r2" {
		t.Fatalf("type/provider: %v %v", body["type"], body["provider"])
	}
}

func TestBuildStorageBodyAzureDropsProvider(t *testing.T) {
	f := storageFlags{
		name: "az", provider: "azure", accountName: "acct", containerName: "cont", sasToken: "sv=…",
		set: map[string]bool{"name": true, "provider": true, "account-name": true, "container-name": true, "sas-token": true},
	}
	body, cliErr := buildStorageBody("", f, true)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if body["type"] != "azure" {
		t.Fatalf("type: %v", body["type"])
	}
	if _, ok := body["provider"]; ok {
		t.Fatalf("azure body must not carry provider")
	}
}

func TestBuildStorageBodyS3MissingFieldsErrors(t *testing.T) {
	f := storageFlags{name: "x", provider: "aws_s3", set: map[string]bool{"name": true, "provider": true}}
	_, cliErr := buildStorageBody("", f, true)
	if cliErr == nil || !strings.Contains(cliErr.Message, "--bucket") {
		t.Fatalf("want required-fields error, got %v", cliErr)
	}
}

func TestBuildStorageBodyUpdateSendsOnlyChanged(t *testing.T) {
	f := storageFlags{region: "eu-west-1", set: map[string]bool{"region": true}}
	body, cliErr := buildStorageBody("", f, false)
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if len(body) != 1 || body["region"] != "eu-west-1" {
		t.Fatalf("partial body wrong: %v", body)
	}
}

func TestMergeProxyUpdateCarriesExistingForward(t *testing.T) {
	existing := map[string]any{"name": "eu pool", "proxies": []any{
		map[string]any{"id": "proxy_1", "name": "one", "url": "http://u:p@a:1"},
	}}
	body := mergeProxyUpdate(existing, "", nil, map[string]bool{})
	if body["name"] != "eu pool" {
		t.Fatalf("name not carried: %v", body["name"])
	}
	proxies := body["proxies"].([]map[string]any)
	if len(proxies) != 1 || proxies[0]["url"] != "http://u:p@a:1" || proxies[0]["name"] != "one" {
		t.Fatalf("existing entries not carried: %v", proxies)
	}
}

func TestMergeProxyUpdateReplacesWholeListWhenURLsGiven(t *testing.T) {
	existing := map[string]any{"name": "eu pool", "proxies": []any{map[string]any{"url": "http://old:1"}}}
	body := mergeProxyUpdate(existing, "", []string{"http://new:1", "http://new:2"}, map[string]bool{})
	proxies := body["proxies"].([]map[string]any)
	if len(proxies) != 2 || proxies[0]["url"] != "http://new:1" {
		t.Fatalf("list not replaced: %v", proxies)
	}
}

func TestBuildProxyCreateBodyRequiresNameAndURL(t *testing.T) {
	body, cliErr := buildProxyCreateBody("eu", []string{"http://a:1", "http://b:2"})
	if cliErr != nil {
		t.Fatalf("unexpected: %v", cliErr)
	}
	if body["name"] != "eu" {
		t.Fatalf("name: %v", body["name"])
	}
	proxies := body["proxies"].([]map[string]any)
	if len(proxies) != 2 || proxies[0]["url"] != "http://a:1" {
		t.Fatalf("proxies: %v", proxies)
	}
	if _, cliErr := buildProxyCreateBody("", []string{"http://a:1"}); cliErr == nil || !strings.Contains(cliErr.Message, "--name") {
		t.Fatalf("want name-required error, got %v", cliErr)
	}
	if _, cliErr := buildProxyCreateBody("eu", nil); cliErr == nil || !strings.Contains(cliErr.Message, "--url") {
		t.Fatalf("want url-required error, got %v", cliErr)
	}
}

func TestBuildLlmBodyUpdateRejectsProvider(t *testing.T) {
	f := llmFlags{provider: "openai", set: map[string]bool{"provider": true}}
	_, cliErr := buildLlmBody("", f, false)
	if cliErr == nil || !strings.Contains(cliErr.Message, "immutable") {
		t.Fatalf("want provider-immutable error, got %v", cliErr)
	}
}

func TestBuildLlmBodyBedrockRequiresAwsFields(t *testing.T) {
	f := llmFlags{name: "b", provider: "amazon-bedrock", set: map[string]bool{"name": true, "provider": true}}
	_, cliErr := buildLlmBody("", f, true)
	if cliErr == nil || !strings.Contains(cliErr.Message, "awsRegion") {
		t.Fatalf("want bedrock required-fields error, got %v", cliErr)
	}
}
