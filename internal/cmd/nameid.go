package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

type nameID struct {
	ID   string
	Name string
}

func valueOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

func resolveNameOrID(arg, prefix string, rows []nameID, kind string) (nameID, *output.CLIError) {
	if strings.HasPrefix(arg, prefix) {
		for _, r := range rows {
			if r.ID == arg {
				return r, nil
			}
		}
		return nameID{ID: arg}, nil
	}
	var matches []nameID
	for _, r := range rows {
		if strings.EqualFold(r.Name, arg) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nameID{}, output.NewCLIError(
			output.ErrNotFound,
			fmt.Sprintf("no %s matching %q", kind, arg),
			fmt.Sprintf("List them with `urlbox %ss list`, then pass a name or id.", kind),
		)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nameID{}, output.NewCLIError(
			output.ErrValidation,
			fmt.Sprintf("%q matches multiple %ss", arg, kind),
			"Use one of the ids instead: "+strings.Join(ids, ", "),
		)
	}
}

func toNameIDs(items []map[string]any) []nameID {
	rows := make([]nameID, len(items))
	for i, m := range items {
		rows[i] = nameID{ID: valueOrEmpty(m["id"]), Name: valueOrEmpty(m["name"])}
	}
	return rows
}

func fetchList(ctx context.Context, client api.SessionAPI, path, key string) ([]map[string]any, error) {
	var resp map[string]any
	if err := client.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	items, _ := resp[key].([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
