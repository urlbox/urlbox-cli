package output_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestRenderList_NoColor_ContainsRowsAndMarker(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	buf := &bytes.Buffer{}
	styles := output.NewStylesForWriter(buf)

	output.RenderList(buf, styles,
		[]string{"NAME", "ID"},
		[][]string{{"Acme", "org_a"}, {"Globex", "org_b"}},
		1,
	)

	out := buf.String()
	for _, want := range []string{"NAME", "ID", "Acme", "org_a", "Globex", "org_b", "●"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderList output missing %q, got:\n%s", want, out)
		}
	}
	markerLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Globex") {
			markerLine = line
		}
	}
	if !strings.Contains(markerLine, "●") {
		t.Errorf("active marker ● should be on the Globex row, got line %q", markerLine)
	}
	if acmeLine := lineContaining(out, "Acme"); strings.Contains(acmeLine, "●") {
		t.Errorf("marker must not appear on the inactive Acme row, got %q", acmeLine)
	}
}

func TestRenderList_NegativeActive_NoMarkerColumn(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	buf := &bytes.Buffer{}
	styles := output.NewStylesForWriter(buf)

	output.RenderList(buf, styles,
		[]string{"NAME", "ID"},
		[][]string{{"Acme", "org_a"}},
		-1,
	)

	out := buf.String()
	if strings.Contains(out, "●") {
		t.Errorf("no row is active, marker ● should not appear, got:\n%s", out)
	}
	if !strings.Contains(out, "Acme") {
		t.Errorf("expected row content, got:\n%s", out)
	}
}

func TestRenderKV_NoColor_BorderedUppercaseLabels(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	buf := &bytes.Buffer{}
	styles := output.NewStylesForWriter(buf)

	output.RenderKV(buf, styles, [][2]string{
		{"Signed in", "user@example.com"},
		{"Org", "Acme (org_a)"},
		{"Project", "(none)"},
	})

	out := buf.String()
	for _, want := range []string{
		"SIGNED IN", "user@example.com",
		"ORG", "Acme (org_a)",
		"PROJECT", "(none)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderKV output missing %q, got:\n%s", want, out)
		}
	}
	for _, box := range []string{"┌", "┐", "└", "┘", "│", "─", "┬", "┴"} {
		if !strings.Contains(out, box) {
			t.Errorf("RenderKV should draw a bordered table, missing %q, got:\n%s", box, out)
		}
	}
	if strings.Contains(out, "Signed in") {
		t.Errorf("labels must be uppercased, still saw mixed-case %q:\n%s", "Signed in", out)
	}
}

func TestRenderKV_StyledLabelPathExercised(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	buf := &bytes.Buffer{}
	styles := output.NewStylesForWriter(buf)
	styles.Header = styles.Header.Transform(strings.ToLower)

	output.RenderKV(buf, styles, [][2]string{{"Name", "Value"}})

	out := stripANSI(buf.String())
	if !strings.Contains(out, "name") {
		t.Errorf("label cell must go through styles.Header (its transform ran), got:\n%q", out)
	}
	if strings.Contains(out, "value") {
		t.Errorf("value cell must not take the header style (transform must not touch it), got:\n%q", out)
	}
	if !strings.Contains(out, "Value") {
		t.Errorf("value must survive intact, got:\n%q", out)
	}
}

func TestRenderKV_Empty_NoOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	buf := &bytes.Buffer{}
	styles := output.NewStylesForWriter(buf)

	output.RenderKV(buf, styles, nil)
	if buf.String() != "" {
		t.Errorf("empty KV should write nothing, got %q", buf.String())
	}
}

func lineContaining(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
