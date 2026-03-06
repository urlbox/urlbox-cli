package cmd

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/jobs"
	"github.com/urlbox/cli/internal/output"
	"github.com/urlbox/cli/internal/schema"
)

func runBatch(args []string) int {
	fs := flag.NewFlagSet("batch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var jsonInput stringValue
	var filePath string
	var urlsPath string
	var formatValue stringValue
	var width intValue
	var height intValue
	var fullPage boolValue
	var delay intValue
	var timeout intValue
	var quality intValue
	var selector stringValue
	var webhookURL stringValue
	var htmlInput stringValue
	var htmlFile string
	var dryRun bool
	var async bool
	var bg bool
	var concurrency int
	var outputFormat string
	var profile string
	var apiHost string

	fs.Var(&jsonInput, "json", "batch JSON payload or - for stdin")
	fs.StringVar(&filePath, "file", "", "JSON or NDJSON file with batch entries, or - for stdin")
	fs.StringVar(&urlsPath, "urls", "", "newline-delimited urls file, or - for stdin")
	fs.Var(&formatValue, "format", "render format")
	fs.Var(&width, "width", "viewport width")
	fs.Var(&height, "height", "viewport height")
	fs.Var(&fullPage, "full-page", "capture the full page")
	fs.Var(&delay, "delay", "delay in ms before capture")
	fs.Var(&timeout, "timeout", "timeout in ms")
	fs.Var(&quality, "quality", "image quality")
	fs.Var(&selector, "selector", "css selector")
	fs.Var(&webhookURL, "webhook-url", "callback URL for async render completion")
	fs.Var(&htmlInput, "html", "raw HTML input for a single batch entry, or - for stdin")
	fs.StringVar(&htmlFile, "html-file", "", "path to an HTML file for a single batch entry, or - for stdin")
	fs.BoolVar(&dryRun, "dry-run", false, "validate only")
	fs.BoolVar(&async, "async", false, "use async render mode")
	fs.BoolVar(&bg, "bg", false, "track async renders in the local job registry")
	fs.IntVar(&concurrency, "concurrency", 1, "number of concurrent renders")
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human, json, or ndjson")

	normalizedArgs, _ := normalizeArgs(args, map[string]bool{
		"--json":          true,
		"--file":          true,
		"--urls":          true,
		"--format":        true,
		"--width":         true,
		"--height":        true,
		"--delay":         true,
		"--timeout":       true,
		"--quality":       true,
		"--selector":      true,
		"--webhook-url":   true,
		"--html":          true,
		"--html-file":     true,
		"--concurrency":   true,
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if concurrency < 1 {
		return printAPIError("batch", output.ResolveFormat(outputFormat, ""), fmt.Errorf("--concurrency must be at least 1"))
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	entries, err := loadBatchEntries(jsonInput, filePath, urlsPath, htmlInput, htmlFile)
	if err != nil {
		return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}
	if len(entries) == 0 {
		return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("batch input did not include any entries"))
	}

	for index := range entries {
		mergeFlagString(entries[index], "format", formatValue)
		mergeFlagString(entries[index], "selector", selector)
		mergeFlagInt(entries[index], "width", width)
		mergeFlagInt(entries[index], "height", height)
		mergeFlagInt(entries[index], "delay", delay)
		mergeFlagInt(entries[index], "timeout", timeout)
		mergeFlagInt(entries[index], "quality", quality)
		mergeFlagString(entries[index], "webhook_url", webhookURL)
		mergeFlagBool(entries[index], "full_page", fullPage)
	}

	manifest, err := schema.Load("render")
	if err != nil {
		return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}
	properties, ok := manifest["properties"].(map[string]interface{})
	if !ok {
		return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("render schema manifest is invalid"))
	}

	validationWarnings := make([]map[string]interface{}, 0)
	for index, entry := range entries {
		if _, ok := entry["url"]; !ok {
			if _, ok := entry["html"]; !ok {
				return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("entry %d is missing url or html", index))
			}
		}
		warnings, err := validatePayload(entry, properties)
		if err != nil {
			return printAPIError("batch", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("entry %d: %w", index, err))
		}
		if len(warnings) > 0 {
			validationWarnings = append(validationWarnings, map[string]interface{}{
				"index":    index,
				"warnings": warnings,
			})
		}
	}

	if dryRun {
		envelope := output.Envelope{
			OK:      true,
			Command: "batch",
			Data: map[string]interface{}{
				"dry_run":  true,
				"count":    len(entries),
				"entries":  entries,
				"warnings": validationWarnings,
			},
		}
		format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
		if format == "json" {
			_ = output.PrintJSON(envelope)
			return 0
		}
		_ = output.PrintHuman(format, fmt.Sprintf("Validated %d batch entries", len(entries)), envelope)
		return 0
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if outputFormat == "ndjson" {
		format = "ndjson"
	}

	results, failed := executeBatch(client, entries, batchOptions{
		async:        async || bg,
		bg:           bg,
		outputFormat: format,
		concurrency:  concurrency,
	})

	if format == "ndjson" {
		if failed > 0 {
			return 1
		}
		return 0
	}

	envelope := output.Envelope{
		OK:      failed == 0,
		Command: "batch",
		Data: map[string]interface{}{
			"count":   len(results),
			"failed":  failed,
			"results": results,
			"mode":    batchMode(async || bg, bg),
		},
	}
	if format == "json" {
		_ = output.PrintJSON(envelope)
	} else {
		message := fmt.Sprintf("Completed %d batch entries", len(results))
		if failed > 0 {
			message = fmt.Sprintf("Completed %d batch entries with %d failure(s)", len(results), failed)
		}
		_ = output.PrintHuman(format, message, envelope)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

type batchOptions struct {
	async        bool
	bg           bool
	outputFormat string
	concurrency  int
}

type batchResult struct {
	index  int
	result map[string]interface{}
	failed bool
}

func executeBatch(client *api.Client, entries []map[string]interface{}, opts batchOptions) ([]map[string]interface{}, int) {
	results := make([]map[string]interface{}, len(entries))
	jobsInput := make(chan int)
	resultsOutput := make(chan batchResult)

	var wg sync.WaitGroup
	workerCount := opts.concurrency
	if workerCount > len(entries) {
		workerCount = len(entries)
	}

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobsInput {
				result, failed := executeBatchEntry(client, entries[index], index, opts)
				resultsOutput <- batchResult{index: index, result: result, failed: failed}
			}
		}()
	}

	go func() {
		for index := range entries {
			jobsInput <- index
		}
		close(jobsInput)
		wg.Wait()
		close(resultsOutput)
	}()

	failed := 0
	for batchResult := range resultsOutput {
		results[batchResult.index] = batchResult.result
		if batchResult.failed {
			failed++
		}
		if opts.outputFormat == "ndjson" {
			bytes, _ := json.Marshal(batchResult.result)
			_, _ = fmt.Fprintln(os.Stdout, string(bytes))
		}
	}

	return results, failed
}

func executeBatchEntry(client *api.Client, entry map[string]interface{}, index int, opts batchOptions) (map[string]interface{}, bool) {
	path := "/v1/render/sync"
	status := "complete"
	if opts.async {
		path = "/v1/render"
		status = "queued"
	}

	var response map[string]interface{}
	err := client.PostJSON(path, entry, &response)
	result := map[string]interface{}{
		"index": index,
		"url":   entry["url"],
		"mode":  batchMode(opts.async, opts.bg),
	}
	if err != nil {
		result["status"] = "error"
		result["error"] = err.Error()
		return result, true
	}

	result["status"] = status
	for key, value := range response {
		result[key] = value
	}

	if opts.bg {
		renderID := valueOrEmpty(response["renderId"])
		job, jobErr := jobs.Add(renderID, valueOrEmpty(entry["url"]), "")
		if jobErr != nil {
			result["status"] = "error"
			result["error"] = jobErr.Error()
			return result, true
		}
		result["job"] = job
		result["render_id"] = renderID
	}

	return result, false
}

func batchMode(async bool, bg bool) string {
	if bg {
		return "background"
	}
	if async {
		return "async"
	}
	return "sync"
}

func loadBatchEntries(jsonInput stringValue, filePath string, urlsPath string, htmlInput stringValue, htmlFile string) ([]map[string]interface{}, error) {
	sources := 0
	if jsonInput.set {
		sources++
	}
	if filePath != "" {
		sources++
	}
	if urlsPath != "" {
		sources++
	}
	if htmlInput.set {
		sources++
	}
	if htmlFile != "" {
		sources++
	}
	if sources > 1 {
		return nil, fmt.Errorf("batch input sources are mutually exclusive; use only one of --json, --file, --urls, --html, or --html-file")
	}

	switch {
	case jsonInput.set:
		if jsonInput.value == "-" {
			bytes, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return nil, err
			}
			return decodeBatchEntries(bytes)
		}
		return decodeBatchEntries([]byte(jsonInput.value))
	case filePath != "":
		bytes, err := readBatchSource(filePath)
		if err != nil {
			return nil, err
		}
		return decodeBatchEntries(bytes)
	case urlsPath != "":
		return decodeURLLines(urlsPath)
	case htmlInput.set:
		html, err := readHTMLInput(htmlInput.value)
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{{"html": html}}, nil
	case htmlFile != "":
		var (
			html string
			err  error
		)
		if htmlFile == "-" {
			html, err = readHTMLInput("-")
		} else {
			html, err = readHTMLFile(htmlFile)
		}
		if err != nil {
			return nil, err
		}
		return []map[string]interface{}{{"html": html}}, nil
	default:
		return nil, fmt.Errorf("batch requires one of --json, --file, --urls, --html, or --html-file")
	}
}

func readBatchSource(path string) ([]byte, error) {
	if path == "-" {
		return ioutil.ReadAll(os.Stdin)
	}
	return ioutil.ReadFile(path)
}

func decodeURLLines(path string) ([]map[string]interface{}, error) {
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}

	var entries []map[string]interface{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, map[string]interface{}{"url": line})
	}
	return entries, scanner.Err()
}

func decodeBatchEntries(raw []byte) ([]map[string]interface{}, error) {
	var entries []map[string]interface{}
	if err := json.Unmarshal(raw, &entries); err == nil {
		return entries, nil
	}

	var single map[string]interface{}
	if err := json.Unmarshal(raw, &single); err == nil {
		return decodeBatchObject(single)
	}

	return decodeBatchNDJSON(raw)
}

func decodeBatchObject(single map[string]interface{}) ([]map[string]interface{}, error) {
	if urls, ok := single["urls"].([]interface{}); ok {
		matrixEntries, err := expandBatchObject(urls, single["options"], single["matrix"])
		if err != nil {
			return nil, err
		}
		return matrixEntries, nil
	}

	if entry, ok := single["entry"].(map[string]interface{}); ok {
		return []map[string]interface{}{entry}, nil
	}

	if _, hasURL := single["url"]; hasURL {
		return []map[string]interface{}{single}, nil
	}
	if _, hasHTML := single["html"]; hasHTML {
		return []map[string]interface{}{single}, nil
	}

	return nil, fmt.Errorf("invalid batch object input")
}

func decodeBatchNDJSON(raw []byte) ([]map[string]interface{}, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	entries := make([]map[string]interface{}, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("invalid ndjson line %q: %w", line, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("invalid batch input")
	}
	return entries, nil
}

func expandBatchObject(urls []interface{}, options interface{}, matrix interface{}) ([]map[string]interface{}, error) {
	baseOptions := map[string]interface{}{}
	if parsedOptions, ok := options.(map[string]interface{}); ok {
		baseOptions = parsedOptions
	}

	matrixValues := map[string][]interface{}{}
	if parsedMatrix, ok := matrix.(map[string]interface{}); ok {
		for key, value := range parsedMatrix {
			items, ok := value.([]interface{})
			if !ok || len(items) == 0 {
				return nil, fmt.Errorf("batch matrix %q must be a non-empty array", key)
			}
			matrixValues[key] = items
		}
	}

	combinations := []map[string]interface{}{{}}
	for key, values := range matrixValues {
		next := make([]map[string]interface{}, 0, len(combinations)*len(values))
		for _, combination := range combinations {
			for _, value := range values {
				entry := mergeMaps(combination, map[string]interface{}{key: value})
				next = append(next, entry)
			}
		}
		combinations = next
	}
	if len(combinations) == 0 {
		combinations = []map[string]interface{}{{}}
	}

	entries := make([]map[string]interface{}, 0, len(urls)*len(combinations))
	for _, item := range urls {
		switch typed := item.(type) {
		case string:
			for _, combination := range combinations {
				entry := mergeMaps(baseOptions, combination)
				entry["url"] = typed
				entries = append(entries, entry)
			}
		case map[string]interface{}:
			for _, combination := range combinations {
				entry := mergeMaps(baseOptions, typed)
				entry = mergeMaps(entry, combination)
				entries = append(entries, entry)
			}
		default:
			return nil, fmt.Errorf("batch urls entries must be strings or objects")
		}
	}
	return entries, nil
}
