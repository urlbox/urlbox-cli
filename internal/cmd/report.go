package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/prompt"
)

// reportCategory pairs the API enum value with the human label the
// interactive picker shows. Order is the picker order.
type reportCategory struct {
	value string
	label string
}

// reportCategories is the closed category set the render-reports API
// accepts, in picker order.
var reportCategories = []reportCategory{
	{value: "bot-detection", label: "Bot detection / blocked"},
	{value: "login-required", label: "Login required"},
	{value: "missing-content", label: "Missing content"},
	{value: "cookie-banner-or-popup", label: "Cookie banner or popup"},
	{value: "render-failed", label: "Render failed"},
	{value: "other", label: "Other"},
}

func categoryValues() []string {
	vals := make([]string, len(reportCategories))
	for i, c := range reportCategories {
		vals[i] = c.value
	}
	return vals
}

func isValidCategory(v string) bool {
	for _, c := range reportCategories {
		if c.value == v {
			return true
		}
	}
	return false
}

const maxReportCommentLen = 2000

// renderReportsPath builds the org-scoped create/list path, matching the
// credkind orgListPath convention.
func renderReportsPath(org string) string {
	return "/v2/organisation/" + org + "/render-reports"
}

func newReportCmd() *cobra.Command {
	var category, comment string
	c := &cobra.Command{
		Use:   "report <renderId>",
		Short: "Report a problem with a render",
		Long: `File a render report with the Urlbox team.

The render id is printed after every render, and async responses carry it
as renderId. Requires a signed-in session (urlbox login) with an active
organisation. Reports are limited to a category plus a short comment; the
team follows up via support.

Without --category/--comment in an interactive terminal, the command
prompts for the missing values. In non-interactive use both flags are
required.

Exit codes:
  0   report filed
  1   usage (missing/invalid category or comment in non-interactive use)
  3   not logged in
  5   render not found (expired or not from your organisation)
  7   the report is already with the team and locked`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, args[0], category, comment)
		},
	}
	c.Flags().StringVar(&category, "category", "", "Problem category: "+strings.Join(categoryValues(), ", "))
	c.Flags().StringVar(&comment, "comment", "", "Short description of what went wrong (required with --category)")
	attachSessionRetryFlags(c)
	return c
}

func runReport(cmd *cobra.Command, renderID, category, comment string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	org, orgErr := requireActiveOrg(sess)
	if orgErr != nil {
		return orgErr
	}
	category, comment, inErr := resolveReportInputs(cmd, category, comment)
	if inErr != nil {
		return inErr
	}
	body := map[string]any{
		"renderId": renderID,
		"category": category,
		"comment":  comment,
	}
	ctx := context.Background()
	var created map[string]any
	if err := sess.Client.PostJSON(ctx, renderReportsPath(org), body, &created); err != nil {
		return refineReportError(err)
	}
	env := output.NewEnvelope("report", created,
		fmt.Sprintf("Report saved for render %s", renderID), nil)
	return writeEnvelopeWithQuietData(cmd, env, valueOrEmpty(created["id"]))
}

// resolveReportInputs validates flag-provided values and fills missing ones
// interactively (text mode + TTY); non-interactive runs get a usage error
// naming the missing flags.
func resolveReportInputs(cmd *cobra.Command, category, comment string) (resolvedCategory, resolvedComment string, cliErr *output.CLIError) {
	if category != "" && !isValidCategory(category) {
		return "", "", output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("unknown --category %q", category),
			"Use one of: "+strings.Join(categoryValues(), ", ")+".")
	}
	comment = strings.TrimSpace(comment)
	if len(comment) > maxReportCommentLen {
		return "", "", output.NewCLIError(output.ErrUsage,
			fmt.Sprintf("--comment is too long (%d chars, max %d)", len(comment), maxReportCommentLen),
			"Shorten the comment; detail can follow via urlbox support.")
	}
	if category != "" && comment != "" {
		return category, comment, nil
	}
	if interactiveText(cmd) {
		return promptReportInputs(cmd, category, comment)
	}
	return "", "", missingReportFlagsError(category, comment)
}

func missingReportFlagsError(category, comment string) *output.CLIError {
	missing := []string{}
	if category == "" {
		missing = append(missing, "--category")
	}
	if comment == "" {
		missing = append(missing, "--comment")
	}
	return output.NewCLIError(output.ErrUsage,
		"missing "+strings.Join(missing, " and "),
		"Pass --category ("+strings.Join(categoryValues(), ", ")+") and --comment, or run in an interactive terminal.")
}

var (
	reportCategoryPick pickFunc                                         = promptPick
	reportCommentInput func(string, func(string) error) (string, error) = prompt.TextInput
)

// SetReportPromptsForTest swaps both report prompts. Pair with
// t.Cleanup(ResetReportPromptsForTest).
func SetReportPromptsForTest(pick pickFunc, input func(string, func(string) error) (string, error)) {
	reportCategoryPick = pick
	reportCommentInput = input
}

// ResetReportPromptsForTest restores the production prompts.
func ResetReportPromptsForTest() {
	reportCategoryPick = promptPick
	reportCommentInput = prompt.TextInput
}

func promptReportInputs(cmd *cobra.Command, category, comment string) (resolvedCategory, resolvedComment string, cliErr *output.CLIError) {
	if category == "" {
		labels := make([]string, len(reportCategories))
		for i, c := range reportCategories {
			labels[i] = c.label
		}
		idx, err := reportCategoryPick("What went wrong with this render?", labels, -1)
		if err != nil {
			return "", "", missingReportFlagsError(category, comment)
		}
		category = reportCategories[idx].value
	}
	if comment == "" {
		typed, err := reportCommentInput("Add a short description:", func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("a short description is required")
			}
			if len(s) > maxReportCommentLen {
				return fmt.Errorf("keep it under %d characters", maxReportCommentLen)
			}
			return nil
		})
		if err != nil {
			return "", "", missingReportFlagsError(category, comment)
		}
		comment = typed
	}
	return category, comment, nil
}

// refineReportError swaps the generic 404/409 hints (written for render
// lookups) for report-specific recovery text. Everything else passes
// through asCLIError untouched.
func refineReportError(err error) error {
	cli := asCLIError(err)
	switch cli.Code {
	case output.ErrNotFound:
		cli.Hint = "Renders are kept for around 60 days and must belong to your active organisation. Check the id against a recent render."
	case output.ErrConflict:
		cli.Hint = "The team already has this report in review. Use `urlbox support` if you need to add something."
	}
	return cli
}
