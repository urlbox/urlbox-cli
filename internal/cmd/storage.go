package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/prompt"
)

func newStorageCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "storage",
		Short: "Manage org storage credentials",
		Long: `Manage the active organisation's storage credentials.

Storage credentials are owned by the organisation and assigned to projects.
Create one once, then assign it to any project's renders.

Secrets are masked on display — pass --reveal for full values (JSON output
always includes them in full).

Examples:
  urlbox storage list
  urlbox storage show prod-bucket --reveal
  urlbox storage create --name prod --provider aws_s3 --bucket b --region us-east-1 --key k --secret s --assign-to my-project
  urlbox storage update prod --region eu-west-1
  urlbox storage delete prod`,
	}
	var listReveal bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List the organisation's storage credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStorageList(cmd, args, listReveal)
		},
	}
	list.Flags().BoolVar(&listReveal, "reveal", false, "Print secrets unmasked (default: masked)")
	var showReveal bool
	show := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one storage credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStorageShow(cmd, args, showReveal)
		},
	}
	show.Flags().BoolVar(&showReveal, "reveal", false, "Print secrets unmasked (default: masked)")
	var (
		createFlags    storageFlags
		createJSON     string
		createAssignTo string
	)
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a storage credential",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStorageCreate(cmd, args, createJSON, createFlags, createAssignTo)
		},
	}
	bindStorageFlags(create, &createFlags)
	create.Flags().StringVar(&createJSON, "json", "", "Full payload as a JSON object (typed flags win)")
	create.Flags().StringVar(&createAssignTo, "assign-to", "", "Assign to this project after create")
	var (
		updateFlags storageFlags
		updateJSON  string
	)
	update := &cobra.Command{
		Use:   "update <name-or-id>",
		Short: "Update a storage credential (only the flags you pass)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStorageUpdate(cmd, args, updateJSON, updateFlags)
		},
	}
	bindStorageFlags(update, &updateFlags)
	update.Flags().StringVar(&updateJSON, "json", "", "Fields to update as a JSON object (typed flags win)")
	var deleteYes bool
	del := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a storage credential",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStorageDelete(cmd, args, deleteYes)
		},
	}
	del.Flags().BoolVar(&deleteYes, "yes", false, "Skip the retype-to-confirm prompt")
	c.AddCommand(list, show, create, update, del)
	attachSessionRetryFlags(c)
	return c
}

func bindStorageFlags(cmd *cobra.Command, f *storageFlags) {
	cmd.Flags().StringVar(&f.name, "name", "", "Credential name")
	cmd.Flags().StringVar(&f.provider, "provider", "", "Storage provider (aws_s3|google_cloud_storage|cloudflare_r2|backblaze_b2|digitalocean_spaces|wasabi|custom|azure)")
	cmd.Flags().StringVar(&f.bucket, "bucket", "", "Bucket name")
	cmd.Flags().StringVar(&f.region, "region", "", "Bucket region")
	cmd.Flags().StringVar(&f.endpoint, "endpoint", "", "Custom endpoint")
	cmd.Flags().StringVar(&f.key, "key", "", "Access key")
	cmd.Flags().StringVar(&f.secret, "secret", "", "Secret key")
	cmd.Flags().StringVar(&f.cdnHost, "cdn-host", "", "CDN host")
	cmd.Flags().StringVar(&f.accountName, "account-name", "", "Azure account name")
	cmd.Flags().StringVar(&f.containerName, "container-name", "", "Azure container name")
	cmd.Flags().StringVar(&f.sasToken, "sas-token", "", "Azure SAS token")
	cmd.Flags().BoolVar(&f.privateBucket, "private-bucket", false, "Bucket is private")
	cmd.Flags().BoolVar(&f.objectLock, "object-lock", false, "Bucket has object lock")
}

func storageFlagsChanged(cmd *cobra.Command, f *storageFlags) {
	f.set = map[string]bool{}
	for _, name := range []string{
		"name", "provider", "bucket", "region", "endpoint", "key", "secret",
		"cdn-host", "account-name", "container-name", "sas-token",
		"private-bucket", "object-lock",
	} {
		if cmd.Flags().Changed(name) {
			f.set[name] = true
		}
	}
}

var storageProviderLabels = map[string]string{
	"aws_s3":               "AWS S3",
	"google_cloud_storage": "Google Cloud",
	"cloudflare_r2":        "Cloudflare R2",
	"backblaze_b2":         "Backblaze B2",
	"digitalocean_spaces":  "DigitalOcean",
	"wasabi":               "Wasabi",
	"custom":               "Custom",
	"minio":                "MinIO",
	"azure":                "Azure",
}

func storageProviderLabel(c map[string]any) string {
	provider := valueOrEmpty(c["provider"])
	if valueOrEmpty(c["type"]) == "azure" {
		provider = "azure"
	}
	if label, ok := storageProviderLabels[provider]; ok {
		return label
	}
	return provider
}

func storageEndpointCell(c map[string]any) string {
	if endpoint := valueOrEmpty(c["endpoint"]); endpoint != "" {
		return endpoint
	}
	if valueOrEmpty(c["provider"]) == "aws_s3" {
		return "AWS S3 (default)"
	}
	return ""
}

func storageListRows(creds []map[string]any) [][]string {
	rows := make([][]string, len(creds))
	for i, c := range creds {
		bucket := valueOrEmpty(c["bucket"])
		if bucket == "" {
			bucket = valueOrEmpty(c["containerName"])
		}
		key := valueOrEmpty(c["key"])
		if key != "" {
			key = maskSecret(key)
		}
		rows[i] = []string{
			bucket, valueOrEmpty(c["id"]), storageProviderLabel(c),
			storageEndpointCell(c), key, assignedCount(c),
		}
	}
	return rows
}

func storageDetailPairs(c map[string]any, reveal bool) [][2]string {
	pairs := [][2]string{
		{"ID", valueOrEmpty(c["id"])},
		{"Name", valueOrEmpty(c["name"])},
	}
	if provider := storageProviderLabel(c); provider != "" {
		pairs = append(pairs, [2]string{"Provider", provider})
	}
	if bucket := valueOrEmpty(c["bucket"]); bucket != "" {
		pairs = append(pairs, [2]string{"Bucket", bucket})
	}
	if region := valueOrEmpty(c["region"]); region != "" {
		pairs = append(pairs, [2]string{"Region", region})
	}
	if endpoint := valueOrEmpty(c["endpoint"]); endpoint != "" {
		pairs = append(pairs, [2]string{"Endpoint", endpoint})
	}
	if account := valueOrEmpty(c["accountName"]); account != "" {
		pairs = append(pairs, [2]string{"Azure account", account})
	}
	if container := valueOrEmpty(c["containerName"]); container != "" {
		pairs = append(pairs, [2]string{"Azure container", container})
	}
	visibility := "public"
	if private, _ := c["privateBucket"].(bool); private {
		visibility = "private"
	}
	pairs = append(pairs, [2]string{"Visibility", visibility})
	if locked, _ := c["objectLock"].(bool); locked {
		pairs = append(pairs, [2]string{"Object lock", "yes"})
	}
	if cdn := valueOrEmpty(c["cdnHost"]); cdn != "" {
		pairs = append(pairs, [2]string{"CDN", cdn})
	}
	if key := valueOrEmpty(c["key"]); key != "" {
		pairs = append(pairs, [2]string{"Key", revealOrMask(key, reveal)})
	}
	if secret := valueOrEmpty(c["secret"]); secret != "" {
		pairs = append(pairs, [2]string{"Secret", revealOrMask(secret, reveal)})
	}
	if sas := valueOrEmpty(c["sasToken"]); sas != "" {
		pairs = append(pairs, [2]string{"SAS token", revealOrMask(sas, reveal)})
	}
	pairs = append(pairs,
		[2]string{"Assigned projects", assignedCount(c)},
		[2]string{"Created", valueOrEmpty(c["createdAt"])},
	)
	return pairs
}

func revealOrMask(value string, reveal bool) string {
	if reveal {
		return value
	}
	return maskSecret(value)
}

func runStorageList(cmd *cobra.Command, _ []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, storageKind.orgListPath(org), storageKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("storage list",
		map[string]any{"storageCredentials": redactMaps(items, storageSecretFields, reveal)},
		fmt.Sprintf("%d storage credentials", len(items)), nil)
	env.SetTable([]string{"BUCKET", "ID", "PROVIDER", "ENDPOINT", "KEY", "ASSIGNED"}, storageListRows(items), -1)
	return writeEnvelopeWithQuietData(cmd, env, strconv.Itoa(len(items)))
}

func runStorageShow(cmd *cobra.Command, args []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, storageKind.orgListPath(org), storageKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], storageKind)
	if resErr != nil {
		return resErr
	}
	var detail map[string]any
	if err := sess.Client.GetJSON(ctx, storageKind.resourcePath(org, resolved.ID), &detail); err != nil {
		return asCLIError(err)
	}
	name := valueOrEmpty(detail["name"])
	if name == "" {
		name = valueOrEmpty(detail["id"])
	}
	env := output.NewEnvelope("storage show", redactMap(detail, storageSecretFields, reveal),
		fmt.Sprintf("Storage credential %s", name), nil)
	env.SetKV(storageDetailPairs(detail, reveal))
	return writeEnvelopeWithQuietData(cmd, env, valueOrEmpty(detail["id"]))
}

func runStorageCreate(cmd *cobra.Command, args []string, jsonBody string, flags storageFlags, assignTo string) error { //nolint:gocritic // storageFlags is the flag-value struct passed by value from the command layer
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	storageFlagsChanged(cmd, &flags)
	resolvedName, nameErr := createName(cmd, args, flags.name)
	if nameErr != nil {
		return nameErr
	}
	if resolvedName != "" {
		flags.name = resolvedName
		flags.set["name"] = true
	}
	body, bodyErr := buildStorageBody(jsonBody, flags, true)
	if bodyErr != nil {
		return bodyErr
	}
	ctx := context.Background()
	var created map[string]any
	if err := sess.Client.PostJSON(ctx, storageKind.orgListPath(org), body, &created); err != nil {
		return asCLIError(err)
	}
	createdID := valueOrEmpty(created["id"])
	name := valueOrEmpty(created["name"])
	if name == "" {
		name = createdID
	}
	outcome := maybeAssignAfterCreate(ctx, sess.Client, org, storageKind, createdID, assignTo, interactiveText(cmd))
	return reportCredCreate(cmd, "storage create", storageKind, created, name, createdID, outcome)
}

func reportCredCreate(cmd *cobra.Command, command string, kind credKind, created map[string]any, name, createdID string, outcome assignOutcome) error { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	if outcome.Err != nil {
		return output.NewCLIError(output.ErrServer,
			fmt.Sprintf("created %s but could not assign it: %v", createdID, outcome.Err),
			fmt.Sprintf("Assign it later with `urlbox projects %s assign <project> %s`.", kind.group, createdID))
	}
	var assigned any
	summary := fmt.Sprintf("Created %s %s", kind.noun, name)
	if outcome.Attempted {
		assigned = map[string]any{"project": map[string]any{"id": outcome.Project.ID, "name": outcome.Project.Name}}
		summary = fmt.Sprintf("Created %s %s (assigned to %s)", kind.noun, name, outcome.Project.Name)
	}
	env := output.NewEnvelope(command,
		map[string]any{"credential": created, "assigned": assigned}, summary, nil)
	return writeEnvelopeWithQuietData(cmd, env, createdID)
}

func runStorageUpdate(cmd *cobra.Command, args []string, jsonBody string, flags storageFlags) error { //nolint:gocritic // storageFlags is the flag-value struct passed by value from the command layer
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	storageFlagsChanged(cmd, &flags)
	body, bodyErr := buildStorageBody(jsonBody, flags, false)
	if bodyErr != nil {
		return bodyErr
	}
	if len(body) == 0 {
		return output.NewCLIError(output.ErrUsage,
			"nothing to update — pass at least one field flag or --json",
			"Pass a field flag (e.g. --region) or --json.")
	}
	ctx := context.Background()
	resolved, resErr := resolveStorageArg(ctx, sess, org, args[0])
	if resErr != nil {
		return resErr
	}
	var updated map[string]any
	if err := sess.Client.PatchJSON(ctx, storageKind.resourcePath(org, resolved.ID), body, &updated); err != nil {
		return asCLIError(err)
	}
	name := resolved.Name
	if name == "" {
		name = resolved.ID
	}
	env := output.NewEnvelope("storage update", updated,
		fmt.Sprintf("Updated storage credential %s", name), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func runStorageDelete(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, storageKind.orgListPath(org), storageKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], storageKind)
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
	if err := sess.Client.DeleteJSON(ctx, storageKind.resourcePath(org, resolved.ID), nil); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("storage delete",
		map[string]any{"deleted": resolved.ID},
		fmt.Sprintf("Deleted storage credential %s", name), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func resolveStorageArg(ctx context.Context, sess *sessionState, org, arg string) (nameID, *output.CLIError) {
	var items []map[string]any
	if !strings.HasPrefix(arg, storageKind.prefix) {
		fetched, err := fetchList(ctx, sess.Client, storageKind.orgListPath(org), storageKind.listKey)
		if err != nil {
			return nameID{}, asCLIError(err)
		}
		items = fetched
	}
	return resolveCredArg(items, arg, storageKind)
}

func confirmDeletion(name string) *output.CLIError {
	if err := prompt.TypeToConfirm(fmt.Sprintf("Type %q to confirm deletion:", name), name); err != nil {
		if errors.Is(err, prompt.ErrNotInteractive) {
			return output.NewCLIError(output.ErrUsage,
				"deletion needs confirmation",
				"Re-run with --yes to confirm non-interactively.")
		}
		return output.NewCLIError(output.ErrUsage, err.Error(),
			"Re-run with --yes to confirm non-interactively.")
	}
	return nil
}
