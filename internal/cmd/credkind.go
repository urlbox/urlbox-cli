package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func createName(cmd *cobra.Command, args []string, flagName string) (string, *output.CLIError) {
	var positional string
	if len(args) == 1 {
		positional = args[0]
	}
	if positional != "" && flagName != "" && positional != flagName {
		return "", output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("name given twice and they differ: positional %q vs --name %q", positional, flagName),
			"Pass the name once — as the positional argument or --name, not both.")
	}
	if positional != "" {
		return positional, nil
	}
	return flagName, nil
}

func assignedCount(m map[string]any) string {
	items, _ := m["assignedProjectIds"].([]any)
	return strconv.Itoa(len(items))
}

type credKind struct {
	noun        string
	article     string
	group       string
	pathPart    string
	bodyKey     string
	prefix      string
	listPath    string
	listKey     string
	listCmd     string
	matchFields []string
}

var (
	storageKind = credKind{noun: "storage credential", article: "a", group: "storage", pathPart: "storage-credential", bodyKey: "storageCredentialId", prefix: "store_", listPath: "storage-credentials", listKey: "storageCredentials", listCmd: "urlbox storage list", matchFields: []string{"bucket", "containerName"}}
	proxyKind   = credKind{noun: "proxy pool", article: "a", group: "proxy", pathPart: "proxy", bodyKey: "proxyId", prefix: "pool_", listPath: "proxies", listKey: "proxies", listCmd: "urlbox proxies list"}
	llmKind     = credKind{noun: "LLM credential", article: "an", group: "llm", pathPart: "llm-credential", bodyKey: "llmCredentialId", prefix: "llm_", listPath: "llm-credentials", listKey: "llmCredentials", listCmd: "urlbox llm list"}
)

func (k credKind) orgListPath(org string) string { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	return "/v2/organisation/" + org + "/" + k.listPath
}

func (k credKind) resourcePath(org, id string) string { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	return k.orgListPath(org) + "/" + id
}

func (k credKind) assignPath(org, project string) string { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	return "/v2/organisation/" + org + "/projects/" + project + "/" + k.pathPart
}

func resolveCredArg(items []map[string]any, arg string, kind credKind) (nameID, *output.CLIError) { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	if strings.HasPrefix(arg, kind.prefix) {
		for _, m := range items {
			if valueOrEmpty(m["id"]) == arg {
				return nameID{ID: arg, Name: valueOrEmpty(m["name"])}, nil
			}
		}
		return nameID{ID: arg}, nil
	}
	fields := append([]string{"name"}, kind.matchFields...)
	var matches []nameID
	for _, m := range items {
		for _, f := range fields {
			v := valueOrEmpty(m[f])
			if v != "" && strings.EqualFold(v, arg) {
				matches = append(matches, nameID{ID: valueOrEmpty(m["id"]), Name: valueOrEmpty(m["name"])})
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nameID{}, output.NewCLIError(
			output.ErrNotFound,
			fmt.Sprintf("no %s matching %q", kind.noun, arg),
			fmt.Sprintf("List them with `%s`, then pass a name or id.", kind.listCmd),
		)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nameID{}, output.NewCLIError(
			output.ErrValidation,
			fmt.Sprintf("%q matches multiple %ss", arg, kind.noun),
			"Use one of the ids instead: "+strings.Join(ids, ", "),
		)
	}
}

type assignOutcome struct {
	Attempted bool
	Project   nameID
	Err       error
}

func maybeAssignAfterCreate(ctx context.Context, client api.SessionAPI, org string, kind credKind, createdID, assignTo string, interactive bool) assignOutcome { //nolint:gocritic // credKind is a value descriptor passed by value throughout
	assign := func(project nameID) assignOutcome {
		if err := client.PutJSON(ctx, kind.assignPath(org, project.ID), map[string]string{kind.bodyKey: createdID}, nil); err != nil {
			return assignOutcome{Attempted: true, Project: project, Err: err}
		}
		return assignOutcome{Attempted: true, Project: project}
	}
	if assignTo != "" {
		projects, err := fetchList(ctx, client, "/v2/projects", "projects")
		if err != nil {
			return assignOutcome{Attempted: true, Err: err}
		}
		project, cliErr := resolveNameOrID(assignTo, "proj_", toNameIDs(projects), "project")
		if cliErr != nil {
			return assignOutcome{Attempted: true, Err: cliErr}
		}
		return assign(project)
	}
	if !interactive {
		return assignOutcome{}
	}
	projects, err := fetchList(ctx, client, "/v2/projects", "projects")
	if err != nil || len(projects) == 0 {
		return assignOutcome{}
	}
	rows := toNameIDs(projects)
	options := make([]string, 0, len(rows)+1)
	options = append(options, "Don't assign")
	for _, p := range rows {
		options = append(options, p.Name)
	}
	idx, err := promptPick("Assign to a project?", options, 0)
	if err != nil || idx == 0 {
		return assignOutcome{}
	}
	return assign(rows[idx-1])
}
