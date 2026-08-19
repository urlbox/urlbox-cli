package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newProxiesCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "proxies",
		Aliases: []string{"proxy"},
		Short:   "Manage org proxy pools",
		Long: `Manage the active organisation's proxy pools.

Proxy pools are owned by the organisation and assigned to projects.
Create one once, then assign it to any project's renders.

Proxy URLs routinely embed credentials, so the password portion is masked in
both text and JSON output — pass --reveal for full values. The host and port
stay legible either way.

Examples:
  urlbox proxies list
  urlbox proxies show eu --reveal
  urlbox proxies create --name eu --url http://user:pass@host:8080 --assign-to my-project
  urlbox proxies update eu --url http://user:pass@host:8080
  urlbox proxies delete eu`,
	}
	var listReveal bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List the organisation's proxy pools",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxiesList(cmd, args, listReveal)
		},
	}
	list.Flags().BoolVar(&listReveal, "reveal", false, "Print proxy URLs unmasked (default: passwords masked)")
	var showReveal bool
	show := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one proxy pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxiesShow(cmd, args, showReveal)
		},
	}
	show.Flags().BoolVar(&showReveal, "reveal", false, "Print proxy URLs unmasked (default: passwords masked)")
	var (
		createName     string
		createURLs     []string
		createAssignTo string
	)
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a proxy pool",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxiesCreate(cmd, args, createName, createURLs, createAssignTo)
		},
	}
	create.Flags().StringVar(&createName, "name", "", "Pool name")
	create.Flags().StringArrayVar(&createURLs, "url", nil, "Proxy URL (repeatable)")
	create.Flags().StringVar(&createAssignTo, "assign-to", "", "Assign to this project after create")
	var (
		updateName string
		updateURLs []string
	)
	update := &cobra.Command{
		Use:   "update <name-or-id>",
		Short: "Update a proxy pool (name and/or the whole URL list)",
		Long: `Update a proxy pool.

The server replaces the pool's proxy list with exactly what you send: passing any --url replaces the whole list; omitting --url keeps the existing list.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxiesUpdate(cmd, args, updateName, updateURLs)
		},
	}
	update.Flags().StringVar(&updateName, "name", "", "Pool name")
	update.Flags().StringArrayVar(&updateURLs, "url", nil, "Proxy URL (repeatable; any --url replaces the whole list)")
	var deleteYes bool
	del := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a proxy pool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProxiesDelete(cmd, args, deleteYes)
		},
	}
	del.Flags().BoolVar(&deleteYes, "yes", false, "Skip the retype-to-confirm prompt")
	c.AddCommand(list, show, create, update, del)
	attachSessionRetryFlags(c)
	return c
}

func proxyListRows(pools []map[string]any) [][]string {
	rows := make([][]string, len(pools))
	for i, p := range pools {
		entries, _ := p["proxies"].([]any)
		rows[i] = []string{
			valueOrEmpty(p["id"]), valueOrEmpty(p["name"]),
			strconv.Itoa(len(entries)), assignedCount(p),
		}
	}
	return rows
}

func proxyDetailPairs(pool map[string]any, reveal bool) [][2]string {
	pairs := [][2]string{
		{"ID", valueOrEmpty(pool["id"])},
		{"Name", valueOrEmpty(pool["name"])},
	}
	entries, _ := pool["proxies"].([]any)
	for _, e := range entries {
		entry, _ := e.(map[string]any)
		if entry == nil {
			continue
		}
		label := valueOrEmpty(entry["name"])
		if label == "" {
			label = valueOrEmpty(entry["id"])
		}
		pairs = append(pairs, [2]string{label, maskProxyURL(valueOrEmpty(entry["url"]), reveal)})
	}
	pairs = append(pairs,
		[2]string{"Assigned projects", assignedCount(pool)},
		[2]string{"Created", valueOrEmpty(pool["createdAt"])},
	)
	return pairs
}

func runProxiesList(cmd *cobra.Command, _ []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, proxyKind.orgListPath(org), proxyKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("proxies list",
		map[string]any{"proxies": redactProxyPools(items, reveal)},
		fmt.Sprintf("%d proxy pools", len(items)), nil)
	env.SetTable([]string{"ID", "NAME", "URLS", "ASSIGNED"}, proxyListRows(items), -1)
	return writeEnvelopeWithQuietData(cmd, env, strconv.Itoa(len(items)))
}

func runProxiesShow(cmd *cobra.Command, args []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, proxyKind.orgListPath(org), proxyKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], proxyKind)
	if resErr != nil {
		return resErr
	}
	var detail map[string]any
	if err := sess.Client.GetJSON(ctx, proxyKind.resourcePath(org, resolved.ID), &detail); err != nil {
		return asCLIError(err)
	}
	name := valueOrEmpty(detail["name"])
	if name == "" {
		name = valueOrEmpty(detail["id"])
	}
	env := output.NewEnvelope("proxies show", redactProxyPool(detail, reveal),
		fmt.Sprintf("Proxy pool %s", name), nil)
	env.SetKV(proxyDetailPairs(detail, reveal))
	return writeEnvelopeWithQuietData(cmd, env, valueOrEmpty(detail["id"]))
}

func runProxiesCreate(cmd *cobra.Command, args []string, name string, urls []string, assignTo string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolvedName, nameErr := createName(cmd, args, name)
	if nameErr != nil {
		return nameErr
	}
	body, bodyErr := buildProxyCreateBody(resolvedName, urls)
	if bodyErr != nil {
		return bodyErr
	}
	ctx := context.Background()
	var created map[string]any
	if err := sess.Client.PostJSON(ctx, proxyKind.orgListPath(org), body, &created); err != nil {
		return asCLIError(err)
	}
	createdID := valueOrEmpty(created["id"])
	createdName := valueOrEmpty(created["name"])
	if createdName == "" {
		createdName = createdID
	}
	outcome := maybeAssignAfterCreate(ctx, sess.Client, org, proxyKind, createdID, assignTo, interactiveText(cmd))
	return reportCredCreate(cmd, "proxies create", proxyKind, created, createdName, createdID, outcome)
}

func runProxiesUpdate(cmd *cobra.Command, args []string, name string, urls []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	nameSet := cmd.Flags().Changed("name")
	if !nameSet && len(urls) == 0 {
		return output.NewCLIError(output.ErrUsage,
			"nothing to update — pass --name and/or --url",
			"Pass --name and/or --url (any --url replaces the whole list).")
	}
	ctx := context.Background()
	resolved, resErr := resolveProxyArg(ctx, sess, org, args[0])
	if resErr != nil {
		return resErr
	}
	var existing map[string]any
	if err := sess.Client.GetJSON(ctx, proxyKind.resourcePath(org, resolved.ID), &existing); err != nil {
		return asCLIError(err)
	}
	body := mergeProxyUpdate(existing, name, urls, map[string]bool{"name": nameSet})
	var updated map[string]any
	if err := sess.Client.PatchJSON(ctx, proxyKind.resourcePath(org, resolved.ID), body, &updated); err != nil {
		return asCLIError(err)
	}
	label := valueOrEmpty(updated["name"])
	if label == "" {
		label = resolved.ID
	}
	env := output.NewEnvelope("proxies update", updated,
		fmt.Sprintf("Updated proxy pool %s", label), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func runProxiesDelete(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, proxyKind.orgListPath(org), proxyKind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	resolved, resErr := resolveCredArg(items, args[0], proxyKind)
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
	if err := sess.Client.DeleteJSON(ctx, proxyKind.resourcePath(org, resolved.ID), nil); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("proxies delete",
		map[string]any{"deleted": resolved.ID},
		fmt.Sprintf("Deleted proxy pool %s", name), nil)
	return writeEnvelopeWithQuietData(cmd, env, resolved.ID)
}

func resolveProxyArg(ctx context.Context, sess *sessionState, org, arg string) (nameID, *output.CLIError) {
	var items []map[string]any
	if !strings.HasPrefix(arg, proxyKind.prefix) {
		fetched, err := fetchList(ctx, sess.Client, proxyKind.orgListPath(org), proxyKind.listKey)
		if err != nil {
			return nameID{}, asCLIError(err)
		}
		items = fetched
	}
	return resolveCredArg(items, arg, proxyKind)
}
