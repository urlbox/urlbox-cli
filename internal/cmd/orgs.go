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
	sel := &cobra.Command{
		Use:   "select [name-or-id]",
		Short: "Set the active organisation",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runOrgsSelect,
	}
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

func runOrgsSelect(cmd *cobra.Command, args []string) error {
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
	if cliErr := updateProfile(sess.ProfileName, func(p *config.Profile) {
		p.ActiveOrg = publicID
		p.ActiveProject = ""
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

	project, projErr := resolveActiveProject(ctx, sess.Client, "", projectPick)
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
	if projErr == nil && project.ID == "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No projects in this organisation yet — run `urlbox projects select` after creating one.")
	}
	if projErr != nil {
		if isNonInteractiveProjectStep(projErr) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Several projects in this organisation — run `urlbox projects select` to pick one.")
		} else {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "org switched, but no active project set: %v\n", projErr)
		}
	}

	data := map[string]any{
		"org":    map[string]any{"id": publicID, "name": chosen.Name},
		"render": map[string]any{"credential": renderStatus},
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	env := output.NewEnvelope("orgs select", data,
		fmt.Sprintf("Active organisation: %s", chosen.Name), nil)
	return writeEnvelopeWithQuietData(cmd, env, publicID)
}

func isNonInteractiveProjectStep(err error) bool {
	var cli *output.CLIError
	return errors.As(err, &cli) && cli.Code == output.ErrUsage
}
