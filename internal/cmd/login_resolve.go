package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type pickFunc func(label string, options []string, active int) (int, error)

var errNotInteractivePick = errors.New("not an interactive terminal")

type orgListRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PublicID string `json:"publicId"`
}

type sessionResponse struct {
	User struct {
		Email string `json:"email"`
	} `json:"user"`
	Session struct {
		ActiveOrganizationID       string `json:"activeOrganizationId"`
		ActiveOrganizationPublicID string `json:"activeOrganizationPublicId"`
	} `json:"session"`
}

type resolvedOrg struct {
	publicID string
	name     string
	email    string
}

func matchOrg(orgs []orgListRow, arg string) (orgListRow, bool) {
	for _, o := range orgs {
		if o.PublicID == arg || o.ID == arg || strings.EqualFold(o.Name, arg) {
			return o, true
		}
	}
	return orgListRow{}, false
}

func resolveActiveOrg(ctx context.Context, client api.SessionAPI, orgFlag string, pick pickFunc) (resolvedOrg, *output.CLIError) {
	var orgs []orgListRow
	if err := client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	if len(orgs) == 0 {
		return resolvedOrg{}, output.NewCLIError(output.ErrNotFound,
			"your account has no organisation",
			"Create one in the dashboard at https://urlbox.com/dashboard, then run `urlbox login` again.")
	}
	chosen := orgs[0]
	if orgFlag != "" {
		match, ok := matchOrg(orgs, orgFlag)
		if !ok {
			return resolvedOrg{}, output.NewCLIError(output.ErrNotFound,
				fmt.Sprintf("no organisation matching %q", orgFlag),
				"Run `urlbox orgs list` to see your organisations.")
		}
		chosen = match
	} else if len(orgs) > 1 {
		names := make([]string, len(orgs))
		for i, o := range orgs {
			names[i] = o.Name
		}
		idx, err := pick("Select an organisation:", names, -1)
		if err != nil {
			if errors.Is(err, errNotInteractivePick) {
				return resolvedOrg{}, output.NewCLIError(output.ErrUsage,
					"multiple organisations and no interactive terminal",
					"Pass --org <name-or-id> to choose one non-interactively.")
			}
			return resolvedOrg{}, output.NewCLIError(output.ErrUsage, err.Error(),
				"Pass --org <name-or-id> to choose one non-interactively.")
		}
		chosen = orgs[idx]
	}
	if err := client.PostJSON(ctx, "/v1/auth/organization/set-active",
		map[string]string{"organizationId": chosen.ID}, nil); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	var session sessionResponse
	if err := client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return resolvedOrg{}, asCLIError(err)
	}
	return resolvedOrg{
		publicID: session.Session.ActiveOrganizationPublicID,
		name:     chosen.Name,
		email:    session.User.Email,
	}, nil
}

func resolveActiveProject(ctx context.Context, client api.SessionAPI, projectFlag string, pick pickFunc) (nameID, *output.CLIError) {
	projects, err := fetchList(ctx, client, "/v2/projects", "projects")
	if err != nil {
		return nameID{}, asCLIError(err)
	}
	rows := toNameIDs(projects)
	if len(rows) == 0 {
		return nameID{}, nil
	}
	if projectFlag != "" {
		return resolveNameOrID(projectFlag, "proj_", rows, "project")
	}
	if len(rows) == 1 {
		return rows[0], nil
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	idx, perr := pick("Select the active project (used by render):", names, -1)
	if perr != nil {
		if errors.Is(perr, errNotInteractivePick) {
			return nameID{}, output.NewCLIError(output.ErrUsage,
				"multiple projects and no interactive terminal",
				"Pass --project <name-or-id>, or run `urlbox projects select` later.")
		}
		return nameID{}, output.NewCLIError(output.ErrUsage, perr.Error(),
			"Pass --project <name-or-id>, or run `urlbox projects select` later.")
	}
	return rows[idx], nil
}

func activeOrgName(ctx context.Context, client api.SessionAPI) string {
	var session sessionResponse
	if err := client.GetJSON(ctx, "/v1/auth/get-session", &session); err != nil {
		return "(none)"
	}
	activeID := session.Session.ActiveOrganizationID
	if activeID == "" {
		return "(none)"
	}
	var orgs []orgListRow
	if err := client.GetJSON(ctx, "/v1/auth/organization/list", &orgs); err == nil {
		for _, o := range orgs {
			if o.ID == activeID {
				return o.Name
			}
		}
	}
	if session.Session.ActiveOrganizationPublicID != "" {
		return session.Session.ActiveOrganizationPublicID
	}
	return "(none)"
}

func asCLIError(err error) *output.CLIError {
	var cli *output.CLIError
	if errors.As(err, &cli) {
		return cli
	}
	return output.NewCLIError(output.ErrServer, err.Error(),
		"Run `urlbox doctor` to verify connectivity, then try again.")
}
