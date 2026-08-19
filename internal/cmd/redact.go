package cmd

// Credential material is masked in the JSON envelope by default, not just in
// the text views. stdout on a pipe resolves to JSON without any flag, so JSON
// is what agents, CI logs and `| tee` transcripts capture — the places a
// long-lived storage secret or proxy password is hardest to recall once
// written. `--reveal` is the single switch that unhides both surfaces, matching
// `config get api_secret --reveal`.
//
// The masked copy is always a fresh map: the text KV builders take the raw
// response and apply their own masking, so redacting in place would double-mask
// and change what a human sees.

// storageSecretFields are the storage-credential response fields that carry
// credential material. Mirrors what storageDetailPairs masks.
var storageSecretFields = []string{"key", "secret", "sasToken"}

// llmSecretFields are the LLM-credential response fields that carry credential
// material, across every provider shape. Mirrors what llmDetailPairs masks.
var llmSecretFields = []string{
	"apiKey",
	"awsAccessKeyId",
	"awsSecretAccessKey",
	"awsSessionToken",
	"gcpServiceAccountJson",
}

// projectSecretFields are the project response fields that carry credential
// material. Mirrors what projectDetailPairs masks.
var projectSecretFields = []string{"webhookKey"}

// redactMap returns a copy of m with every named field masked. Fields that are
// absent, empty, or not strings are left exactly as they came off the wire, so
// a null stays null rather than becoming "***".
func redactMap(m map[string]any, fields []string, reveal bool) map[string]any {
	if reveal || m == nil {
		return m
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, f := range fields {
		if s := valueOrEmpty(out[f]); s != "" {
			out[f] = maskSecret(s)
		}
	}
	return out
}

// redactMaps applies redactMap across a list, returning a new slice.
func redactMaps(items []map[string]any, fields []string, reveal bool) []map[string]any {
	if reveal {
		return items
	}
	out := make([]map[string]any, len(items))
	for i, m := range items {
		out[i] = redactMap(m, fields, reveal)
	}
	return out
}

// redactProxyPool returns a copy of a proxy pool with the password component of
// every entry URL masked. Only the password is touched — scheme, host and port
// stay legible so an operator can still tell pools apart.
func redactProxyPool(pool map[string]any, reveal bool) map[string]any {
	if reveal || pool == nil {
		return pool
	}
	out := make(map[string]any, len(pool))
	for k, v := range pool {
		out[k] = v
	}
	entries, _ := pool["proxies"].([]any)
	if entries == nil {
		return out
	}
	masked := make([]any, 0, len(entries))
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			masked = append(masked, e)
			continue
		}
		copied := make(map[string]any, len(entry))
		for k, v := range entry {
			copied[k] = v
		}
		if raw := valueOrEmpty(entry["url"]); raw != "" {
			copied["url"] = maskProxyURL(raw, false)
		}
		masked = append(masked, copied)
	}
	out["proxies"] = masked
	return out
}

// redactProxyPools applies redactProxyPool across a list, returning a new slice.
func redactProxyPools(pools []map[string]any, reveal bool) []map[string]any {
	if reveal {
		return pools
	}
	out := make([]map[string]any, len(pools))
	for i, p := range pools {
		out[i] = redactProxyPool(p, reveal)
	}
	return out
}
