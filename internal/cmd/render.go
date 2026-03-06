package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/jobs"
	"github.com/urlbox/cli/internal/output"
	"github.com/urlbox/cli/internal/schema"
)

type stringValue struct {
	value string
	set   bool
}

func (v *stringValue) String() string { return v.value }
func (v *stringValue) Set(value string) error {
	v.value = value
	v.set = true
	return nil
}

type intValue struct {
	value int
	set   bool
}

func (v *intValue) String() string { return fmt.Sprintf("%d", v.value) }
func (v *intValue) Set(value string) error {
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return err
	}
	v.value = parsed
	v.set = true
	return nil
}

type boolValue struct {
	value bool
	set   bool
}

func (v *boolValue) String() string {
	if v.value {
		return "true"
	}
	return "false"
}
func (v *boolValue) Set(value string) error {
	if value == "" {
		v.value = true
		v.set = true
		return nil
	}

	switch strings.ToLower(value) {
	case "1", "true", "yes", "y":
		v.value = true
	case "0", "false", "no", "n":
		v.value = false
	default:
		return fmt.Errorf("invalid boolean %q", value)
	}

	v.set = true
	return nil
}
func (v *boolValue) IsBoolFlag() bool { return true }

func runRender(args []string) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var jsonInput stringValue
	var formatValue stringValue
	var width intValue
	var height intValue
	var fullPage boolValue
	var delay intValue
	var timeout intValue
	var selector stringValue
	var quality intValue
	var webhookURL stringValue
	var dryRun bool
	var async bool
	var bg bool
	var outputPath string
	var outputFormat string
	var profile string
	var apiHost string

	fs.Var(&jsonInput, "json", "full render payload or - for stdin")
	fs.Var(&formatValue, "format", "render format")
	fs.Var(&width, "width", "viewport width")
	fs.Var(&height, "height", "viewport height")
	fs.Var(&fullPage, "full-page", "capture the full page")
	fs.Var(&delay, "delay", "delay in ms before capture")
	fs.Var(&timeout, "timeout", "timeout in ms")
	fs.Var(&selector, "selector", "CSS selector")
	fs.Var(&quality, "quality", "image quality")
	fs.Var(&webhookURL, "webhook-url", "callback URL for async render completion")
	fs.BoolVar(&dryRun, "dry-run", false, "validate locally without network calls")
	fs.BoolVar(&async, "async", false, "use async render mode")
	fs.BoolVar(&bg, "bg", false, "run in the background")
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputPath, "output", "", "output file path")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")

	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--json":          true,
		"--format":        true,
		"--width":         true,
		"--height":        true,
		"--delay":         true,
		"--timeout":       true,
		"--selector":      true,
		"--quality":       true,
		"--webhook-url":   true,
		"--profile":       true,
		"--api-host":      true,
		"--output":        true,
		"--output-format": true,
	})

	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}

	cfg := config.Load(config.Options{
		Profile:      profile,
		APIHost:      apiHost,
		OutputFormat: outputFormat,
	})

	manifest, err := schema.Load("render")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	properties, ok := manifest["properties"].(map[string]interface{})
	if !ok {
		fmt.Fprintln(os.Stderr, "render schema manifest is invalid")
		return 1
	}

	payload := map[string]interface{}{}

	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			printRenderError(output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
			return 1
		}
		payload = parsed
	}

	if len(positionalArgs) > 0 {
		payload["url"] = positionalArgs[0]
	}

	mergeFlagString(payload, "format", formatValue)
	mergeFlagString(payload, "selector", selector)
	mergeFlagInt(payload, "width", width)
	mergeFlagInt(payload, "height", height)
	mergeFlagInt(payload, "delay", delay)
	mergeFlagInt(payload, "timeout", timeout)
	mergeFlagInt(payload, "quality", quality)
	mergeFlagString(payload, "webhook_url", webhookURL)
	mergeFlagBool(payload, "full_page", fullPage)

	warnings, err := validatePayload(payload, properties)
	if err != nil {
		printRenderError(output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		return 1
	}

	if _, ok := payload["url"]; !ok {
		if _, ok := payload["html"]; !ok {
			printRenderError(output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("either url or html is required"))
			return 1
		}
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)

	result := output.Envelope{
		OK:      true,
		Command: "render",
		Data: map[string]interface{}{
			"dry_run":           dryRun,
			"async":             async || bg,
			"validated_options": payload,
			"warnings":          warnings,
			"api_host":          cfg.APIHost,
		},
	}

	if dryRun {
		if format == "json" {
			_ = output.PrintJSON(result)
			return 0
		}

		_ = output.PrintHuman(format, "Dry run OK", result)
		return 0
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	apiResponse := map[string]interface{}{}
	path := "/v1/render/sync"
	if async || bg {
		path = "/v1/render"
	}

	if err := client.PostJSON(path, payload, &apiResponse); err != nil {
		return printAPIError("render", format, err)
	}

	if async || bg {
		result.Data = apiResponse
		if bg {
			renderID := valueOrEmpty(apiResponse["renderId"])
			job, err := jobs.Add(renderID, valueOrEmpty(payload["url"]), outputPath)
			if err != nil {
				return printAPIError("render", format, err)
			}
			result.Data = map[string]interface{}{
				"job":       job,
				"render_id": renderID,
				"result":    apiResponse,
			}
		}
		if format == "json" {
			_ = output.PrintJSON(result)
			return 0
		}
		renderID := valueOrEmpty(apiResponse["renderId"])
		if bg {
			_ = output.PrintHuman(format, fmt.Sprintf("Queued background render %s", renderID), result)
		} else {
			_ = output.PrintHuman(format, fmt.Sprintf("Queued render %s", renderID), result)
		}
		return 0
	}

	result.Data = apiResponse

	if shouldDownload(format, outputPath) {
		renderURL := firstString(
			apiResponse["renderUrl"],
			apiResponse["markdownUrl"],
			apiResponse["htmlUrl"],
			apiResponse["mhtmlUrl"],
		)
		if renderURL == "" {
			return printAPIError("render", format, fmt.Errorf("sync render response did not include a downloadable URL"))
		}

		bytes, _, err := client.Download(renderURL)
		if err != nil {
			return printAPIError("render", format, err)
		}

		targetPath := outputPath
		if targetPath == "" {
			targetPath = defaultOutputPath(payload)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return printAPIError("render", format, err)
		}
		if err := ioutil.WriteFile(targetPath, bytes, 0644); err != nil {
			return printAPIError("render", format, err)
		}

		result.Data = map[string]interface{}{
			"saved_to": targetPath,
			"result":   apiResponse,
		}
		if format == "json" {
			_ = output.PrintJSON(result)
			return 0
		}
		_ = output.PrintHuman(format, fmt.Sprintf("Saved to %s", targetPath), result)
		return 0
	}

	if format == "json" {
		_ = output.PrintJSON(result)
		return 0
	}

	_ = output.PrintHuman(format, fmt.Sprintf("Rendered %s", valueOrEmpty(apiResponse["renderUrl"])), result)
	return 0
}

func readJSONPayload(raw string) (map[string]interface{}, error) {
	if raw == "-" {
		bytes, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		raw = string(bytes)
	}

	if len(raw) > 1024*1024 {
		return nil, fmt.Errorf("--json input exceeds 1MB limit")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func mergeFlagString(payload map[string]interface{}, key string, value stringValue) {
	if value.set {
		payload[key] = value.value
	}
}

func mergeFlagInt(payload map[string]interface{}, key string, value intValue) {
	if value.set {
		payload[key] = value.value
	}
}

func mergeFlagBool(payload map[string]interface{}, key string, value boolValue) {
	if value.set {
		payload[key] = value.value
	}
}

func validatePayload(payload map[string]interface{}, properties map[string]interface{}) ([]string, error) {
	warnings := []string{}

	for key, value := range payload {
		if strings.ContainsAny(key, "\x00\n\r\t") {
			return nil, fmt.Errorf("invalid control characters in key %q", key)
		}

		if !knownProperty(key, properties) {
			suggestion := closestKey(key, properties)
			if suggestion == "" {
				return nil, fmt.Errorf("unknown option %q", key)
			}

			return nil, fmt.Errorf("unknown option %q; did you mean %q?", key, suggestion)
		}

		if err := rejectControlChars(value); err != nil {
			return nil, err
		}
	}

	if hasKey(payload, "format") && hasKey(payload, "quality") {
		if payload["format"] == "pdf" {
			warnings = append(warnings, "quality is ignored when format=pdf")
		}
	}

	sort.Strings(warnings)
	return warnings, nil
}

func knownProperty(key string, properties map[string]interface{}) bool {
	_, ok := properties[key]
	return ok
}

func hasKey(payload map[string]interface{}, key string) bool {
	_, ok := payload[key]
	return ok
}

func rejectControlChars(value interface{}) error {
	switch typed := value.(type) {
	case string:
		for _, char := range typed {
			if char < 0x20 {
				return fmt.Errorf("control characters are not allowed in string values")
			}
		}
	case []interface{}:
		for _, item := range typed {
			if err := rejectControlChars(item); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for nestedKey, nestedValue := range typed {
			if strings.ContainsAny(nestedKey, "\x00\n\r\t") {
				return fmt.Errorf("invalid control characters in nested key %q", nestedKey)
			}
			if err := rejectControlChars(nestedValue); err != nil {
				return err
			}
		}
	}

	return nil
}

func closestKey(input string, properties map[string]interface{}) string {
	bestKey := ""
	bestDistance := math.MaxInt32

	for key := range properties {
		distance := levenshtein(input, key)
		if distance < bestDistance {
			bestDistance = distance
			bestKey = key
		}
	}

	if bestDistance > 3 {
		return ""
	}

	return bestKey
}

func levenshtein(a string, b string) int {
	if a == b {
		return 0
	}

	if len(a) == 0 {
		return len(b)
	}

	if len(b) == 0 {
		return len(a)
	}

	column := make([]int, len(b)+1)
	for i := 0; i <= len(b); i++ {
		column[i] = i
	}

	for i := 1; i <= len(a); i++ {
		prev := column[0]
		column[0] = i
		for j := 1; j <= len(b); j++ {
			old := column[j]
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			column[j] = min(
				column[j]+1,
				column[j-1]+1,
				prev+cost,
			)
			prev = old
		}
	}

	return column[len(b)]
}

func min(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func printRenderError(format string, err error) {
	envelope := output.Envelope{
		OK:      false,
		Command: "render",
		Error: &output.ErrorBody{
			Code:    "INVALID_INPUT",
			Message: err.Error(),
		},
	}

	if format == "json" {
		_ = output.PrintJSON(envelope)
		return
	}

	_, _ = fmt.Fprintln(os.Stderr, err.Error())
}

func printAPIError(command string, format string, err error) int {
	if apiErr, ok := err.(*api.APIError); ok {
		envelope := output.Envelope{
			OK:      false,
			Command: command,
			Error: &output.ErrorBody{
				Code:    fallback(apiErr.Code, "API_ERROR"),
				Message: apiErr.Message,
				Field:   apiErr.Field,
			},
		}
		if format == "json" {
			_ = output.PrintJSON(envelope)
		} else {
			_, _ = fmt.Fprintln(os.Stderr, envelope.Error.Message)
		}
		return 1
	}

	printRenderError(format, err)
	return 1
}

func fallback(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func shouldDownload(format string, outputPath string) bool {
	return format != "json" || outputPath != ""
}

func defaultOutputPath(payload map[string]interface{}) string {
	extension := "png"
	if format, ok := payload["format"].(string); ok && format != "" {
		extension = format
		if extension == "jpeg" {
			extension = "jpg"
		}
	}
	return "screenshot." + extension
}

func firstString(values ...interface{}) string {
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func valueOrEmpty(value interface{}) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
