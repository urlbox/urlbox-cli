package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

var orgsProjectPick pickFunc = promptPick

// SetOrgsProjectPickForTest swaps the picker used by the post-switch project
// step in `orgs select`. Pair with t.Cleanup(ResetOrgsProjectPickForTest).
func SetOrgsProjectPickForTest(p pickFunc) { orgsProjectPick = p }

// ResetOrgsProjectPickForTest restores the production picker.
func ResetOrgsProjectPickForTest() { orgsProjectPick = promptPick }

func newOrgsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "orgs",
		Aliases: []string{"org"},
		Short:   "Manage the active organisation",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List your organisations",
		Args:  cobra.NoArgs,
		RunE:  runOrgsList,
	}
	var selProject string
	sel := &cobra.Command{
		Use:   "select [name-or-id]",
		Short: "Set the active organisation",
		Long: `Set the active organisation.

Switching organisations clears the stored render credential: it belongs to a
project in the organisation you are leaving, so keeping it would let ` + "`render`" + `
bill the previous organisation. The CLI picks the new organisation's project up
again automatically when there is exactly one; when there are several, pass
--project to finish the switch in a single step instead of following up with
` + "`urlbox projects select`" + `.

Examples:
  urlbox orgs select acme
  urlbox orgs select acme --project production
  urlbox orgs select --output-format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOrgsSelect(cmd, args, selProject)
		},
	}
	sel.Flags().StringVar(&selProject, "project", "", "Project to make active after the switch (name or id) — skips the picker")
	c.AddCommand(list, sel)
	attachSessionRetryFlags(c)
	return c
}

func runOrgsList(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var orgs []orgListRow
	if err := sess.Client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return asCLIError(err)
	}
	var session sessionResponse
	_ = sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session)
	activeID := session.Session.ActiveOrganizationID

	rows := make([]map[string]any, len(orgs))
	tableRows := make([][]string, len(orgs))
	activeName := ""
	activeIndex := -1
	for i, o := range orgs {
		active := o.ID != "" && o.ID == activeID
		if active {
			activeName = o.Name
			activeIndex = i
		}
		rows[i] = map[string]any{"id": o.PublicID, "name": o.Name, "active": active}
		tableRows[i] = []string{o.Name, o.PublicID}
	}
	summary := fmt.Sprintf("%d organisations", len(orgs))
	if activeName != "" {
		summary = fmt.Sprintf("%d organisations — active: %s", len(orgs), activeName)
	}
	env := output.NewEnvelope("orgs list", map[string]any{"organisations": rows}, summary, nil)
	env.SetTable([]string{"NAME", "ID"}, tableRows, activeIndex)
	return writeEnvelope(cmd, env)
}

func runOrgsSelect(cmd *cobra.Command, args []string, projectFlag string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var orgs []orgListRow
	if err := sess.Client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return asCLIError(err)
	}
	if len(orgs) == 0 {
		return output.NewCLIError(output.ErrNotFound, "no organisations",
			"Create one in the dashboard at https://urlbox.com/dashboard.")
	}

	var chosen orgListRow
	if len(args) == 1 {
		match, ok := matchOrg(orgs, args[0])
		if !ok {
			return output.NewCLIError(output.ErrNotFound,
				fmt.Sprintf("no organisation matching %q", args[0]),
				"Run `urlbox orgs list` to see your organisations.")
		}
		chosen = match
	} else {
		names := make([]string, len(orgs))
		active := -1
		var session sessionResponse
		_ = sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session)
		for i, o := range orgs {
			names[i] = o.Name
			if o.ID == session.Session.ActiveOrganizationID {
				active = i
			}
		}
		idx, err := promptPick("Select the active organisation:", names, active)
		if err != nil {
			if errors.Is(err, errNotInteractivePick) {
				return output.NewCLIError(output.ErrUsage,
					"selection needs an interactive terminal",
					"Pass the organisation directly: `urlbox orgs select <name-or-id>`.")
			}
			return output.NewCLIError(output.ErrUsage, err.Error(),
				"Pass the organisation directly: `urlbox orgs select <name-or-id>`.")
		}
		chosen = orgs[idx]
	}

	if err := sess.Client.PostJSON(ctx, "/v1/auth/organization/set-active",
		map[string]string{"organizationId": chosen.ID}, nil); err != nil {
		return asCLIError(err)
	}
	var session sessionResponse
	if err := sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return asCLIError(err)
	}
	publicID := session.Session.ActiveOrganizationPublicID
	// The render credential is project-scoped and the project belongs to the
	// organisation being left, so both halves go — a surviving api_key would
	// still name the previous org.
	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) {
		p.ActiveOrg = publicID
		p.ActiveProject = ""
		p.APIKey = ""
		p.APISecret = ""
	}); cliErr != nil {
		return cliErr
	}

	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	interactive := formatFlag != "json" && formatFlag != "quiet"
	projectPick := orgsProjectPick
	if !interactive {
		projectPick = func(_ string, _ []string, _ int) (int, error) { return -1, errNotInteractivePick }
	}

	project, projectCount, projErr := resolveActiveProject(ctx, sess.Client, projectFlag, projectPick)
	// An explicit --project that cannot be resolved is a real failure, not a
	// half-finished switch: the caller named something that isn't there.
	if projErr != nil && projectFlag != "" {
		return projErr
	}
	renderStatus := "none"
	if projErr == nil && project.ID != "" {
		if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) { p.ActiveProject = project.ID }); cliErr == nil {
			if cred, issued, err := ensureRenderCredential(ctx, sess.Client, publicID, project.ID, interactive, projectPick); err == nil && cred.secret != "" {
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
	}
	// The switch can land without an active project (none exist yet, or several
	// and nothing to pick with). That state is expected, but it leaves `render`
	// without a credential, so it has to travel in the envelope — an agent reads
	// stdout and never sees a stderr line.
	summary := fmt.Sprintf("Active organisation: %s", chosen.Name)
	var breadcrumbs []output.Breadcrumb
	switch {
	case projErr == nil && project.ID == "":
		summary += " — no projects yet"
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No projects in this organisation yet — create one, then run `urlbox projects select`.")
		breadcrumbs = []output.Breadcrumb{{Action: "create project", Cmd: "urlbox projects create <name>"}}
	case projErr != nil && isNonInteractiveProjectStep(projErr):
		summary += fmt.Sprintf(" — %d projects; pick one next", projectCount)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"%d projects in this organisation — run `urlbox orgs select %s --project <name>` or `urlbox projects select` to finish.\n",
			projectCount, chosen.Name)
		breadcrumbs = []output.Breadcrumb{{Action: "pick project", Cmd: "urlbox projects select"}}
	case projErr != nil:
		summary += " — no active project"
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "org switched, but no active project set: %v\n", projErr)
		breadcrumbs = []output.Breadcrumb{{Action: "pick project", Cmd: "urlbox projects select"}}
	}

	data := map[string]any{
		"org":    map[string]any{"id": publicID, "name": chosen.Name},
		"render": map[string]any{"credential": renderStatus},
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	env := output.NewEnvelope("orgs select", data, summary, breadcrumbs)
	return writeEnvelopeWithQuietData(cmd, env, publicID)
}

func isNonInteractiveProjectStep(err error) bool {
	var cli *output.CLIError
	return errors.As(err, &cli) && cli.Code == output.ErrUsage
}
