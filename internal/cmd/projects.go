package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/prompt"
)

var confirmPrompt = prompt.Confirm

// SetConfirmPromptForTest swaps the yes/no confirm prompt used by the projects
// commands (create's switch offer and disable's guard). Pair with
// t.Cleanup(ResetConfirmPromptForTest).
func SetConfirmPromptForTest(c func(string) (bool, error)) { confirmPrompt = c }

// ResetConfirmPromptForTest restores the production confirm prompt.
func ResetConfirmPromptForTest() { confirmPrompt = prompt.Confirm }

var deleteProjectPick pickFunc = promptPick

// SetDeleteProjectPickForTest swaps the picker used to re-resolve the active
// project after deleting it. Pair with t.Cleanup(ResetDeleteProjectPickForTest).
func SetDeleteProjectPickForTest(p pickFunc) { deleteProjectPick = p }

// ResetDeleteProjectPickForTest restores the production picker.
func ResetDeleteProjectPickForTest() { deleteProjectPick = promptPick }

func newProjectsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "projects",
		Aliases: []string{"project"},
		Short:   "Manage projects and the active project",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the active organisation's projects",
		Args:  cobra.NoArgs,
		RunE:  runProjectsList,
	}
	sel := &cobra.Command{
		Use:   "select [name-or-id]",
		Short: "Set the active project (used by render)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runProjectsSelect,
	}
	var showReveal bool
	show := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsShow(cmd, args, showReveal)
		},
	}
	show.Flags().BoolVar(&showReveal, "reveal", false, "Print the webhook key unmasked (default: masked)")
	var createSelect bool
	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a project in the active organisation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsCreate(cmd, args, createSelect)
		},
	}
	create.Flags().BoolVar(&createSelect, "select", false, "Make the new project the active one and refresh the render credential")
	rename := &cobra.Command{
		Use:   "rename <name-or-id> <new-name>",
		Short: "Rename a project",
		Args:  cobra.ExactArgs(2),
		RunE:  runProjectsRename,
	}
	enable := &cobra.Command{
		Use:   "enable <name-or-id>",
		Short: "Enable a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsSetEnabled(cmd, args, true, true)
		},
	}
	var disableYes bool
	disable := &cobra.Command{
		Use:   "disable <name-or-id>",
		Short: "Disable a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsSetEnabled(cmd, args, false, disableYes)
		},
	}
	disable.Flags().BoolVar(&disableYes, "yes", false, "Confirm disabling the project (stops its renders)")
	var yes bool
	del := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsDelete(cmd, args, yes)
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "Skip the retype-to-confirm prompt")
	defaults := &cobra.Command{
		Use:   "defaults",
		Short: "Manage the project's default render options",
	}
	defaultsShow := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show default render options",
		Args:  cobra.ExactArgs(1),
		RunE:  runProjectsDefaultsShow,
	}
	var defaultsJSON string
	var defaultsMerge bool
	defaultsSet := &cobra.Command{
		Use:   "set <name-or-id> --json <object>",
		Short: "Set default render options",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsDefaultsSet(cmd, args, defaultsJSON, defaultsMerge)
		},
	}
	defaultsSet.Flags().StringVar(&defaultsJSON, "json", "", "Default options as a JSON object")
	defaultsSet.Flags().BoolVar(&defaultsMerge, "merge", false, "Merge into the existing defaults instead of replacing them")
	var defaultsRemoveYes bool
	defaultsRemove := &cobra.Command{
		Use:   "remove <name-or-id>",
		Short: "Remove all default render options",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsDefaultsRemove(cmd, args, defaultsRemoveYes)
		},
	}
	defaultsRemove.Flags().BoolVar(&defaultsRemoveYes, "yes", false, "Skip the retype-to-confirm prompt")
	defaults.AddCommand(defaultsShow, defaultsSet, defaultsRemove)
	c.AddCommand(list, sel, show, create, rename, enable, disable, del, defaults)
	c.AddCommand(
		newProjectsCredSubCmd(storageKind),
		newProjectsCredSubCmd(proxyKind),
		newProjectsCredSubCmd(llmKind),
	)
	attachSessionRetryFlags(c)
	return c
}

func newProjectsCredSubCmd(kind credKind) *cobra.Command { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	group := &cobra.Command{
		Use:   kind.group,
		Short: fmt.Sprintf("Assign or unassign the project's %s", kind.noun),
	}
	assign := &cobra.Command{
		Use:   "assign <project> <" + kind.group + ">",
		Short: fmt.Sprintf("Assign %s %s to a project", kind.article, kind.noun),
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsCredAssign(cmd, args, kind)
		},
	}
	unassign := &cobra.Command{
		Use:   "unassign <project>",
		Short: fmt.Sprintf("Unassign the project's %s", kind.noun),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsCredUnassign(cmd, args, kind)
		},
	}
	group.AddCommand(assign, unassign)
	return group
}

func runProjectsCredAssign(cmd *cobra.Command, args []string, kind credKind) error { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	project, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	ctx := context.Background()
	items, err := fetchList(ctx, sess.Client, kind.orgListPath(org), kind.listKey)
	if err != nil {
		return asCLIError(err)
	}
	cred, credErr := resolveCredArg(items, args[1], kind)
	if credErr != nil {
		return credErr
	}
	var resp map[string]any
	if err := sess.Client.PutJSON(ctx, kind.assignPath(org, project.ID),
		map[string]string{kind.bodyKey: cred.ID}, &resp); err != nil {
		return asCLIError(err)
	}
	credName := cred.Name
	if credName == "" {
		credName = cred.ID
	}
	env := output.NewEnvelope("projects "+kind.group+" assign", resp,
		fmt.Sprintf("Assigned %s to %s", credName, projectLabel(project)), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsCredUnassign(cmd *cobra.Command, args []string, kind credKind) error { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	project, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.DeleteJSON(context.Background(), kind.assignPath(org, project.ID), &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects "+kind.group+" unassign", resp,
		fmt.Sprintf("Unassigned the %s from %s", kind.noun, projectLabel(project)), nil)
	return writeEnvelope(cmd, env)
}

func projectLabel(project nameID) string {
	if project.Name != "" {
		return project.Name
	}
	return project.ID
}

func runProjectsList(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return asCLIError(err)
	}
	rows := make([]map[string]any, len(projects))
	tableRows := make([][]string, len(projects))
	activeName := ""
	activeIndex := -1
	for i, m := range projects {
		id := valueOrEmpty(m["id"])
		active := id != "" && id == sess.Profile.ActiveProject
		if active {
			activeName = valueOrEmpty(m["name"])
			activeIndex = i
		}
		m["active"] = active
		rows[i] = m
		tableRows[i] = []string{
			valueOrEmpty(m["name"]),
			id,
			projectStatusLabel(m),
			valueOrEmpty(m["engineVersion"]),
		}
	}
	summary := fmt.Sprintf("%d projects", len(rows))
	if activeName != "" {
		summary = fmt.Sprintf("%d projects — active: %s", len(rows), activeName)
	}
	env := output.NewEnvelope("projects list", map[string]any{"projects": rows}, summary, nil)
	env.SetTable([]string{"NAME", "ID", "STATUS", "ENGINE"}, tableRows, activeIndex)
	return writeEnvelope(cmd, env)
}

func runProjectsSelect(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	projects, err := fetchList(ctx, sess.Client, "/v2/projects", "projects")
	if err != nil {
		return asCLIError(err)
	}
	rows := toNameIDs(projects)
	if len(rows) == 0 {
		return output.NewCLIError(output.ErrNotFound, "no projects in the active organisation",
			"Create one with `urlbox projects create <name>`.")
	}

	var chosen nameID
	if len(args) == 1 {
		chosen, cliErr = resolveNameOrID(args[0], "proj_", rows, "project")
		if cliErr != nil {
			return cliErr
		}
	} else {
		names := make([]string, len(rows))
		active := -1
		for i, r := range rows {
			names[i] = r.Name
			if r.ID == sess.Profile.ActiveProject {
				active = i
			}
		}
		idx, perr := promptPick("Select the active project (used by render):", names, active)
		if perr != nil {
			if errors.Is(perr, errNotInteractivePick) {
				return output.NewCLIError(output.ErrUsage,
					"selection needs an interactive terminal",
					"Pass the project directly: `urlbox projects select <name-or-id>`.")
			}
			return output.NewCLIError(output.ErrUsage, perr.Error(),
				"Pass the project directly: `urlbox projects select <name-or-id>`.")
		}
		chosen = rows[idx]
	}

	renderStatus, cliErr := activateProject(cmd, sess, chosen)
	if cliErr != nil {
		return cliErr
	}

	data := map[string]any{
		"project": map[string]any{"id": chosen.ID, "name": chosen.Name},
		"render":  map[string]any{"credential": renderStatus},
	}
	env := output.NewEnvelope("projects select", data,
		fmt.Sprintf("Active project: %s", chosen.Name), nil)
	return writeEnvelopeWithQuietData(cmd, env, chosen.ID)
}

func activateProject(cmd *cobra.Command, sess *sessionState, chosen nameID) (string, *output.CLIError) {
	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) { p.ActiveProject = chosen.ID }); cliErr != nil {
		return "none", cliErr
	}
	renderStatus := "none"
	if org := sess.Profile.ActiveOrg; org != "" {
		formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
		interactive := formatFlag != "json" && formatFlag != "quiet"
		if cred, issued, err := ensureRenderCredential(context.Background(), sess.Client, org, chosen.ID, interactive, promptPick); err == nil && cred.secret != "" {
			if updateProfile(sess.ProfileName, func(p *config.Profile) {
				p.APIKey = cred.key
				p.APISecret = cred.secret
			}) == nil {
				if issued {
					renderStatus = "issued"
				} else {
					renderStatus = "ready"
				}
			}
		}
	}
	return renderStatus, nil
}

func projectStatusLabel(project map[string]any) string {
	if enabled, ok := project["enabled"].(bool); ok {
		if enabled {
			return "enabled"
		}
		return "disabled"
	}
	return "unknown"
}

func projectDetailPairs(project map[string]any, reveal bool) [][2]string {
	pairs := [][2]string{
		{"Name", valueOrEmpty(project["name"])},
		{"ID", valueOrEmpty(project["id"])},
		{"Enabled", projectStatusLabel(project)},
		{"Engine", valueOrEmpty(project["engineVersion"])},
	}
	appendIf := func(label, key string) {
		if v := valueOrEmpty(project[key]); v != "" {
			pairs = append(pairs, [2]string{label, v})
		}
	}
	appendIf("Region", "region")
	appendIf("Queue", "renderQueue")
	if key := valueOrEmpty(project["webhookKey"]); key != "" {
		if !reveal {
			key = maskSecret(key)
		}
		pairs = append(pairs, [2]string{"Webhook key", key})
	}
	appendIf("Storage credential", "storageCredentialId")
	appendIf("Proxy", "proxyId")
	appendIf("LLM credential", "llmCredentialId")
	appendIf("Created", "createdAt")
	return pairs
}

func optionsKVPairs(options map[string]any) [][2]string {
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([][2]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, [2]string{k, formatOptionValue(options[k])})
	}
	return pairs
}

func formatOptionValue(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case bool:
		return fmt.Sprintf("%t", vv)
	case float64:
		if vv == float64(int64(vv)) {
			return fmt.Sprintf("%d", int64(vv))
		}
		return fmt.Sprintf("%v", vv)
	default:
		b, err := json.Marshal(vv)
		if err != nil {
			return fmt.Sprintf("%v", vv)
		}
		return string(b)
	}
}

func resolveProjectArg(sess *sessionState, arg string) (nameID, *output.CLIError) {
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, asCLIError(err)
	}
	return resolveNameOrID(arg, "proj_", toNameIDs(projects), "project")
}

func resolveProjectArgWithEnabled(sess *sessionState, arg string) (nameID, *bool, *output.CLIError) {
	if strings.HasPrefix(arg, "proj_") {
		project, resErr := resolveProjectArg(sess, arg)
		return project, nil, resErr
	}
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, nil, asCLIError(err)
	}
	project, resErr := resolveNameOrID(arg, "proj_", toNameIDs(projects), "project")
	if resErr != nil {
		return nameID{}, nil, resErr
	}
	for _, row := range projects {
		if valueOrEmpty(row["id"]) == project.ID {
			if enabled, ok := row["enabled"].(bool); ok {
				return project, &enabled, nil
			}
			break
		}
	}
	return project, nil, nil
}

func projectPath(org, id string) string {
	return "/v2/organisation/" + org + "/projects/" + id
}

func runProjectsShow(cmd *cobra.Command, args []string, reveal bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &resp); err != nil {
		return asCLIError(err)
	}
	name := valueOrEmpty(resp["name"])
	if name == "" {
		name = valueOrEmpty(resp["id"])
	}
	env := output.NewEnvelope("projects show", redactMap(resp, projectSecretFields, reveal),
		fmt.Sprintf("Project %s", name), nil)
	env.SetKV(projectDetailPairs(resp, reveal))
	return writeEnvelope(cmd, env)
}

func runProjectsCreate(cmd *cobra.Command, args []string, selectNew bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	var resp map[string]any
	if err := sess.Client.PostJSON(context.Background(), "/v2/projects",
		map[string]string{"name": args[0]}, &resp); err != nil {
		return asCLIError(err)
	}
	created := createdProjectNameID(resp, args[0])

	switched := false
	if wantSwitchToNewProject(cmd, selectNew, created) {
		if _, aerr := activateProject(cmd, sess, created); aerr != nil {
			return aerr
		}
		switched = true
	}

	if switched {
		env := output.NewEnvelope("projects create", withSelected(resp),
			fmt.Sprintf("Created project %s (now active)", args[0]), nil)
		return writeEnvelope(cmd, env)
	}
	env := output.NewEnvelope("projects create", resp,
		fmt.Sprintf("Created project %s", args[0]),
		[]output.Breadcrumb{{Action: "activate", Cmd: "urlbox projects select " + args[0]}})
	return writeEnvelope(cmd, env)
}

func createdProjectNameID(resp map[string]any, fallbackName string) nameID {
	src := resp
	if nested, ok := resp["project"].(map[string]any); ok {
		src = nested
	}
	name := valueOrEmpty(src["name"])
	if name == "" {
		name = fallbackName
	}
	return nameID{ID: valueOrEmpty(src["id"]), Name: name}
}

func interactiveText(cmd *cobra.Command) bool {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	return output.ResolveFormat(formatFlag, cmd.OutOrStdout()) == output.FormatText
}

func wantSwitchToNewProject(cmd *cobra.Command, selectNew bool, created nameID) bool {
	if created.ID == "" {
		return false
	}
	if selectNew {
		return true
	}
	if !interactiveText(cmd) {
		return false
	}
	ok, err := confirmPrompt("Switch to this project?")
	if err != nil {
		return false
	}
	return ok
}

func withSelected(resp map[string]any) map[string]any {
	out := make(map[string]any, len(resp)+1)
	for k, v := range resp {
		out[k] = v
	}
	out["selected"] = true
	return out
}

func runProjectsRename(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(), projectPath(org, resolved.ID),
		map[string]string{"name": args[1]}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects rename", resp,
		fmt.Sprintf("Renamed %s to %s", resolved.ID, args[1]), nil)
	return writeEnvelope(cmd, env)
}

func disableNeedsInteractiveConfirm(cmd *cobra.Command) bool {
	return interactiveText(cmd)
}

func confirmDisable(cmd *cobra.Command, name string) (bool, error) {
	ok, err := confirmPrompt(fmt.Sprintf("Disable project %s?", name))
	if err != nil {
		if errors.Is(err, prompt.ErrNotInteractive) {
			return false, output.NewCLIError(output.ErrUsage,
				"disabling stops the project's renders",
				"Re-run with --yes to confirm non-interactively.")
		}
		return false, output.NewCLIError(output.ErrUsage, err.Error(),
			"Re-run with --yes to confirm non-interactively.")
	}
	return ok, nil
}

func runProjectsSetEnabled(cmd *cobra.Command, args []string, enabled, yes bool) error {
	if !enabled && !yes && !disableNeedsInteractiveConfirm(cmd) {
		return output.NewCLIError(output.ErrUsage,
			"disabling stops the project's renders",
			"Re-run with --yes to confirm.")
	}
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	if enabled {
		resolved, resErr := resolveProjectArg(sess, args[0])
		if resErr != nil {
			return resErr
		}
		return patchEnabled(cmd, sess, org, resolved, true)
	}
	resolved, currentEnabled, resErr := resolveProjectArgWithEnabled(sess, args[0])
	if resErr != nil {
		return resErr
	}
	name := resolved.Name
	if name == "" {
		name = resolved.ID
	}
	if currentEnabled != nil && !*currentEnabled {
		env := output.NewEnvelope("projects disable",
			map[string]any{"id": resolved.ID, "enabled": false},
			fmt.Sprintf("%s is already disabled", name), nil)
		return writeEnvelope(cmd, env)
	}
	if !yes {
		ok, confirmErr := confirmDisable(cmd, name)
		if confirmErr != nil {
			return confirmErr
		}
		if !ok {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Left %s enabled.\n", name)
			return nil
		}
	}
	return patchEnabled(cmd, sess, org, resolved, false)
}

func patchEnabled(cmd *cobra.Command, sess *sessionState, org string, resolved nameID, enabled bool) error {
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(), projectPath(org, resolved.ID),
		map[string]bool{"enabled": enabled}, &resp); err != nil {
		return asCLIError(err)
	}
	verb := "Enabled"
	command := "projects enable"
	if !enabled {
		verb = "Disabled"
		command = "projects disable"
	}
	env := output.NewEnvelope(command, resp,
		fmt.Sprintf("%s %s", verb, resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDelete(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	if !yes {
		name := resolved.Name
		if name == "" {
			name = resolved.ID
		}
		if err := prompt.TypeToConfirm(fmt.Sprintf("Type %q to confirm deletion:", name), name); err != nil {
			if errors.Is(err, prompt.ErrNotInteractive) {
				return output.NewCLIError(output.ErrUsage,
					"deletion needs confirmation",
					"Re-run with --yes to confirm non-interactively.")
			}
			return output.NewCLIError(output.ErrUsage, err.Error(),
				"Re-run with --yes to confirm non-interactively.")
		}
	}
	if err := sess.Client.DeleteJSON(context.Background(), projectPath(org, resolved.ID), nil); err != nil {
		return asCLIError(err)
	}
	data := map[string]any{"deleted": resolved.ID}
	summary := fmt.Sprintf("Deleted project %s", resolved.ID)
	wasActive := resolved.ID == sess.Profile.ActiveProject
	if wasActive {
		data["was_active"] = true
		summary = fmt.Sprintf("Deleted project %s (was your active project)", resolved.ID)
		if nowActive, cliErr := reResolveActiveAfterDelete(cmd, sess, org); cliErr != nil {
			return cliErr
		} else if nowActive.ID != "" {
			data["now_active"] = map[string]any{"id": nowActive.ID, "name": nowActive.Name}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Now active: %s\n", nowActive.Name)
		} else {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Select one with `urlbox projects select`.")
		}
	}
	env := output.NewEnvelope("projects delete", data, summary, nil)
	return writeEnvelope(cmd, env)
}

func clearActiveProject(sess *sessionState) *output.CLIError {
	return updateProfile(sess.ProfileName, func(p *config.Profile) {
		p.ActiveProject = ""
		p.APIKey = ""
		p.APISecret = ""
	})
}

func reResolveActiveAfterDelete(cmd *cobra.Command, sess *sessionState, org string) (nameID, *output.CLIError) {
	projects, err := fetchList(context.Background(), sess.Client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, clearActiveProject(sess)
	}
	rows := toNameIDs(projects)
	switch {
	case len(rows) == 0:
		return nameID{}, clearActiveProject(sess)
	case len(rows) == 1:
		return activateSurvivor(cmd, sess, org, rows[0])
	case interactiveText(cmd):
		names := make([]string, len(rows))
		for i, r := range rows {
			names[i] = r.Name
		}
		idx, perr := deleteProjectPick("Select the active project (used by render):", names, 0)
		if perr != nil || idx < 0 || idx >= len(rows) {
			return nameID{}, clearActiveProject(sess)
		}
		return activateSurvivor(cmd, sess, org, rows[idx])
	default:
		return nameID{}, clearActiveProject(sess)
	}
}

func activateSurvivor(cmd *cobra.Command, sess *sessionState, org string, survivor nameID) (nameID, *output.CLIError) {
	if cliErr := clearActiveProject(sess); cliErr != nil {
		return nameID{}, cliErr
	}
	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) { p.ActiveProject = survivor.ID }); cliErr != nil {
		return nameID{}, cliErr
	}
	if cred, _, credErr := ensureRenderCredential(context.Background(), sess.Client, org, survivor.ID, interactiveText(cmd), deleteProjectPick); credErr == nil && cred.secret != "" {
		_ = updateProfile(sess.ProfileName, func(p *config.Profile) {
			p.APIKey = cred.key
			p.APISecret = cred.secret
		})
	}
	return survivor, nil
}

func runProjectsDefaultsShow(cmd *cobra.Command, args []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	var resp map[string]any
	if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &resp); err != nil {
		return asCLIError(err)
	}
	defaults := map[string]any{}
	if d, ok := resp["defaultOptions"].(map[string]any); ok {
		defaults = d
	}
	env := output.NewEnvelope("projects defaults show",
		map[string]any{"project": resolved.ID, "defaults": defaults},
		fmt.Sprintf("%d default options on %s", len(defaults), resolved.ID), nil)
	env.SetKV(optionsKVPairs(defaults))
	return writeEnvelope(cmd, env)
}

func runProjectsDefaultsSet(cmd *cobra.Command, args []string, jsonBody string, merge bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	if jsonBody == "" {
		return output.NewCLIError(output.ErrUsage, "missing --json",
			`Pass the defaults as a JSON object: --json '{"format":"png"}'.`)
	}
	var options map[string]any
	if err := json.Unmarshal([]byte(jsonBody), &options); err != nil {
		return output.NewCLIError(output.ErrUsage, "--json is not a valid JSON object: "+err.Error(),
			`Example: --json '{"format":"png","full_page":true}'.`)
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	final := options
	if merge {
		var current map[string]any
		if err := sess.Client.GetJSON(context.Background(), projectPath(org, resolved.ID), &current); err != nil {
			return asCLIError(err)
		}
		merged := map[string]any{}
		if d, ok := current["defaultOptions"].(map[string]any); ok {
			for k, v := range d {
				merged[k] = v
			}
		}
		for k, v := range options {
			merged[k] = v
		}
		final = merged
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(),
		projectPath(org, resolved.ID)+"/render-defaults",
		map[string]any{"options": final}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects defaults set", resp,
		fmt.Sprintf("Set %d default options on %s", len(final), resolved.ID), nil)
	return writeEnvelope(cmd, env)
}

func runProjectsDefaultsRemove(cmd *cobra.Command, args []string, yes bool) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	resolved, resErr := resolveProjectArg(sess, args[0])
	if resErr != nil {
		return resErr
	}
	if !yes {
		name := resolved.Name
		if name == "" {
			name = resolved.ID
		}
		if err := prompt.TypeToConfirm(fmt.Sprintf("Type %q to confirm removing defaults:", name), name); err != nil {
			if errors.Is(err, prompt.ErrNotInteractive) {
				return output.NewCLIError(output.ErrUsage,
					"removing defaults needs confirmation",
					"Re-run with --yes to confirm non-interactively.")
			}
			return output.NewCLIError(output.ErrUsage, err.Error(),
				"Re-run with --yes to confirm non-interactively.")
		}
	}
	var resp map[string]any
	if err := sess.Client.PatchJSON(context.Background(),
		projectPath(org, resolved.ID)+"/render-defaults",
		map[string]any{"options": nil}, &resp); err != nil {
		return asCLIError(err)
	}
	env := output.NewEnvelope("projects defaults remove", resp,
		fmt.Sprintf("Removed default options from %s", resolved.ID), nil)
	return writeEnvelope(cmd, env)
}
