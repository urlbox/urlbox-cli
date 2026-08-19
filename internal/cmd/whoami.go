package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newWhoamiCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"me"},
		Short:   "Show the signed-in user and active context",
		Long: `Show who you are signed in as, plus the active organisation and project.

Examples:
  urlbox whoami
  urlbox whoami --output-format json`,
		Args: cobra.NoArgs,
		RunE: runWhoami,
	}
	attachSessionRetryFlags(c)
	return c
}

func runWhoami(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	ctx := context.Background()
	var session sessionResponse
	if err := sess.Client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return asCLIError(err)
	}
	if session.User.Email == "" {
		return output.NewCLIError(output.ErrAuth, notLoggedInMsg, loginHint)
	}

	project := resolveWhoamiProject(ctx, sess)
	orgName := activeOrgName(ctx, sess.Client)

	data := map[string]any{
		"email": session.User.Email,
		"org": map[string]any{
			"id":   session.Session.ActiveOrganizationPublicID,
			"name": orgName,
		},
		"project": nil,
	}
	if project.ID != "" {
		data["project"] = map[string]any{"id": project.ID, "name": project.Name}
	}
	summary := fmt.Sprintf("Signed in as %s — org %s", session.User.Email, orgName)
	env := output.NewEnvelope("whoami", data, summary, nil)
	env.SetKV(identityKVPairs(session.User.Email, orgName, session.Session.ActiveOrganizationPublicID, project))
	return writeEnvelopeWithQuietData(cmd, env, session.User.Email)
}

func identityKVPairs(email, orgName, orgID string, project nameID) [][2]string {
	org := orgName
	if orgID != "" {
		if org == "" {
			org = orgID
		} else {
			org += " (" + orgID + ")"
		}
	}
	proj := "(none)"
	if project.ID != "" {
		if project.Name == "" {
			proj = project.ID
		} else {
			proj = project.Name + " (" + project.ID + ")"
		}
	}
	return [][2]string{{"Signed in", email}, {"Org", org}, {"Project", proj}}
}

func resolveWhoamiProject(ctx context.Context, sess *sessionState) nameID {
	if sess.Profile.ActiveProject == "" {
		return nameID{}
	}
	projects, err := fetchList(ctx, sess.Client, "/v2/projects", "projects")
	if err != nil {
		return nameID{ID: sess.Profile.ActiveProject}
	}
	for _, r := range toNameIDs(projects) {
		if r.ID == sess.Profile.ActiveProject {
			return r
		}
	}
	return nameID{ID: sess.Profile.ActiveProject}
}
