package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newLlmCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "llm",
		Short: "Manage org LLM credentials",
		Long: `Manage the active organisation's LLM credentials.

LLM credentials are owned by the organisation and assigned to projects.
Create one once, then assign it to any project's renders.

Secrets are masked on display — pass --reveal for full values (JSON output
always includes them in full).

Examples:
  urlbox llm list
  urlbox llm show openai-prod --reveal
  urlbox llm create --name openai --provider openai --api-key sk-… --assign-to my-project
  urlbox llm update openai --model gpt-5-mini
  urlbox llm test openai
  urlbox llm models openai
  urlbox llm delete openai`,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the organisation's LLM credentials",
		Args:  cobra.NoArgs,
		RunE:  runLlmList,
	}
	var showReveal bool
	show := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one LLM credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLlmShow(cmd, args, showReveal)
		},
	}
	show.Flags().BoolVar(&showReveal, "reveal", false, "Print secrets unmasked (default: masked)")
	var (
		createFlags    llmFlags
		createJSON     string
		createAssignTo string
	)
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an LLM credential",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLlmCreate(cmd, args, createJSON, createFlags, createAssignTo)
		},
	}
	bindLlmFlags(create, &createFlags)
	create.Flags().StringVar(&createJSON, "json", "", "Full payload as a JSON object (typed flags win)")
	create.Flags().StringVar(&createAssignTo, "assign-to", "", "Assign to this project after create")
	var (
		updateFlags llmFlags
		updateJSON  string
	)
	update := &cobra.Command{
		Use:   "update <name-or-id>",
		Short: "Update an LLM credential (only the flags you pass)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLlmUpdate(cmd, args, updateJSON, updateFlags)
		},
	}
	bindLlmFlags(update, &updateFlags)
	update.Flags().StringVar(&updateJSON, "json", "", "Fields to update as a JSON object (typed flags win)")
	var deleteYes bool
	del := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete an LLM credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLlmDelete(cmd, args, deleteYes)
		},
	}
	del.Flags().BoolVar(&deleteYes, "yes", false, "Skip the retype-to-confirm prompt")
	test := &cobra.Command{
		Use:   "test <name-or-id>",
		Short: "Test the stored credential's connection",
		Args:  cobra.ExactArgs(1),
		RunE:  runLlmTest,
	}
	models := &cobra.Command{
		Use:   "models <name-or-id>",
		Short: "List the provider's model ids",
		Args:  cobra.ExactArgs(1),
		RunE:  runLlmModels,
	}
	c.AddCommand(list, show, create, update, del, test, models)
	attachSessionRetryFlags(c)
	return c
}

func bindLlmFlags(cmd *cobra.Command, f *llmFlags) {
	cmd.Flags().StringVar(&f.name, "name", "", "Credential name")
	cmd.Flags().StringVar(&f.provider, "provider", "", "LLM provider (openai|anthropic|azure|amazon-bedrock|google-vertex|…)")
	cmd.Flags().StringVar(&f.apiKey, "api-key", "", "Provider API key")
	cmd.Flags().StringVar(&f.model, "model", "", "Default model")
	cmd.Flags().StringVar(&f.baseURL, "base-url", "", "Custom base URL")
}

func llmFlagsChanged(cmd *cobra.Command, f *llmFlags) {
	f.set = map[string]bool{}
	for _, name := range []string{"name", "provider", "api-key", "model", "base-url"} {
		if cmd.Flags().Changed(name) {
			f.set[name] = true
		}
	}
}

func llmListRows(creds []map[string]any) [][]string {
	rows := make([][]string, len(creds))
	for i, c := range creds {
		rows[i] = []string{
			valueOrEmpty(c["id"]), valueOrEmpty(c["name"]),
			valueOrEmpty(c["provider"]), valueOrEmpty(c["model"]), assignedCount(c),
		}
	}
	return rows
}

func llmDetailPairs(c map[string]any, reveal bool) [][2]string {
	pairs := [][2]string{
		{"ID", valueOrEmpty(c["id"])},
		{"Name", valueOrEmpty(c["name"])},
		{"Provider", valueOrEmpty(c["provider"])},
	}
	if model := valueOrEmpty(c["model"]); model != "" {
		pairs = append(pairs, [2]string{"Model", model})
	}
	if baseURL := valueOrEmpty(c["baseUrl"]); baseURL != "" {
		pairs = append(pairs, [2]string{"Base URL", baseURL})
	}
	secretFields := []struct{ label, key string }{
		{"API key", "apiKey"},
		{"AWS access key id", "awsAccessKeyId"},
		{"AWS secret access key", "awsSecretAccessKey"},
		{"AWS session token", "awsSessionToken"},
		{"GCP service account", "gcpServiceAccountJson"},
	}
	for _, s := range secretFields {
		if v := valueOrEmpty(c[s.key]); v != "" {
			pairs = append(pairs, [2]string{s.label, revealOrMask(v, reveal)})
		}
	}
	pairs = append(pairs,
		[2]string{"Assigned projects", assignedCount(c)},
		[2]string{"Created", valueOrEmpty(c["createdAt"])},
	)
	return pairs
}

func llmTestMessage(result map[string]any) (string, bool) {
	if ok, _ := result["ok"].(bool); ok {
		return "Connection OK", true
	}
	if reason := valueOrEmpty(result["error"]); reason != "" {
		return "Connection failed: " + reason, false
	}
	return "Connection failed", false
}

func runLlmList(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, llmKind.orgListPath(org), llmKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("llm list",
		map[string]any{"llmCredentials": items},
		fmt.Sprintf("%d LLM credentials", len(items)), nil)
	env.SetTable([]string{"ID", "NAME", "PROVIDER", "MODEL", "ASSIGNED"}, llmListRows(items), -1)
	return writeEnvelopeWithQuietData(cmd, env, strconv.Itoa(len(items)))
}

func runLlmShow(cmd *cobra.Command, args []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, llmKind.orgListPath(org), llmKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], llmKind)
	if resErr != nil {
		return resErr
	}
	var detail map[string]any
	if err := sess.Client.GetJSON(ctx, llmKind.resourcePath(org, resolved.ID), &detail); err != nil {
		return asCLIError(err)
	}
	name := valueOrEmpty(detail["name"])
	if name == "" {
		name = valueOrEmpty(detail["id"])
	}
	env := output.NewEnvelope("llm show", detail,
		fmt.Sprintf("LLM credential %s", name), nil)
	env.SetKV(llmDetailPairs(detail, reveal))
	return writeEnvelopeWithQuietData(cmd, env, valueOrEmpty(detail["id"]))
}

func runLlmCreate(cmd *cobra.Command, args []string, jsonBody string, flags llmFlags, assignTo string) error { //nolint:gocritic // llmFlags is the flag-value struct passed by value from the command layer
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	llmFlagsChanged(cmd, &flags)
	resolvedName, nameErr := createName(cmd, args, flags.name)
	if nameErr != nil {
		return nameErr
	}
	if resolvedName != "" {
		flags.name = resolvedName
		flags.set["name"] = true
	}
	body, bodyErr := buildLlmBody(jsonBody, flags, true)
	if bodyErr != nil {
		return bodyErr
	}
	ctx := context.Background()
	var created map[string]any
	if err := sess.Client.PostJSON(ctx, llmKind.orgListPath(org), body, &created); err != nil {
		return asCLIError(err)
	}
	createdID := valueOrEmpty(created["id"])
	name := valueOrEmpty(created["name"])
	if name == "" {
		name = createdID
	}
	outcome := maybeAssignAfterCreate(ctx, sess.Client, org, llmKind, createdID, assignTo, interactiveText(cmd))
	return reportCredCreate(cmd, "llm create", llmKind, created, name, createdID, outcome)
}

func runLlmUpdate(cmd *cobra.Command, args []string, jsonBody string, flags llmFlags) error { //nolint:gocritic // llmFlags is the flag-value struct passed by value from the command layer
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	llmFlagsChanged(cmd, &flags)
	body, bodyErr := buildLlmBody(jsonBody, flags, false)
	if bodyErr != nil {
		return bodyErr
	}
	if len(body) == 0 {
		return output.NewCLIError(output.ErrUsage,
			"nothing to update — pass at least one field flag or --json",
			"Pass a field flag (e.g. --model) or --json.")
	}
	ctx := context.Background()
	resolved, resErr := resolveLlmArg(ctx, sess, org, args[0])
	if resErr != nil {
		return resErr
	}
	var updated map[string]any
	if err := sess.Client.PatchJSON(ctx, llmKind.resourcePath(org, resolved.ID), body, &updated); err != nil {
		return asCLIError(err)
	}
	name := resolved.Name
	if name == "" {
		name = resolved.ID
	}
	env := output.NewEnvelope("llm update", updated,
		fmt.Sprintf("Updated LLM credential %s", name), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func runLlmDelete(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, llmKind.orgListPath(org), llmKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], llmKind)
	if resErr != nil {
		return resErr
	}
	name := resolved.Name
	if name == "" {
		name = resolved.ID
	}
	if !yes {
		if err := confirmDeletion(name); err != nil {
			return err
		}
	}
	if err := sess.Client.DeleteJSON(ctx, llmKind.resourcePath(org, resolved.ID), nil); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("llm delete",
		map[string]any{"deleted": resolved.ID},
		fmt.Sprintf("Deleted LLM credential %s", name), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func runLlmTest(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	resolved, resErr := resolveLlmArg(ctx, sess, org, args[0])
	if resErr != nil {
		return resErr
	}
	var result map[string]any
	if err := sess.Client.PostJSON(ctx, llmKind.resourcePath(org, resolved.ID)+"/test", map[string]any{}, &result); err != nil {
		return asCLIError(err)
	}
	message, ok := llmTestMessage(result)
	env := &output.Envelope{OK: ok, Command: "llm test", Data: result, Summary: message}
	if err := writeEnvelope(cmd, env); err != nil {
		return err
	}
	if ok {
		return nil
	}
	return &output.CLIError{Code: output.ErrUsage, Message: message, Silent: true}
}

func runLlmModels(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	resolved, resErr := resolveLlmArg(ctx, sess, org, args[0])
	if resErr != nil {
		return resErr
	}
	var result map[string]any
	if err := sess.Client.PostJSON(ctx, llmKind.resourcePath(org, resolved.ID)+"/models", map[string]any{}, &result); err != nil {
		return asCLIError(err)
	}
	models, _ := result["models"].([]any)
	rows := make([][]string, len(models))
	for i, m := range models {
		rows[i] = []string{valueOrEmpty(m)}
	}
	env := output.NewEnvelope("llm models", result,
		fmt.Sprintf("%d models", len(models)), nil)
	env.SetTable([]string{"MODEL"}, rows, -1)
	return writeEnvelope(cmd, env)
}

func resolveLlmArg(ctx context.Context, sess *sessionState, org, arg string) (nameID, *output.CLIError) {
	var items []map[string]any
	if !strings.HasPrefix(arg, llmKind.prefix) {
		fetched, err := fetchList(ctx, sess.Client, llmKind.orgListPath(org), llmKind.listKey)
		if err != nil {
			return nameID{}, asCLIError(err)
		}
		items = fetched
	}
	return resolveCredArg(items, arg, llmKind)
}
