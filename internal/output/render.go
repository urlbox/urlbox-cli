package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

type textView struct {
	headers     []string
	rows        [][]string
	activeIndex int
	kv          [][2]string
}

func (v *textView) render(w io.Writer, styles *Styles) {
	if v == nil {
		return
	}
	if v.kv != nil {
		RenderKV(w, styles, v.kv)
		return
	}
	RenderList(w, styles, v.headers, v.rows, v.activeIndex)
}

// RenderList writes a styled table for any "list" command. headers are the
// column titles; rows are the cell values (one []string per row, same length as
// headers). activeIndex marks the active row (the active org or project) with a
// leading ● marker plus emphasis, or -1 for none. The marker column keeps the
// active row identifiable once colour is stripped (NO_COLOR / piped output).
func RenderList(w io.Writer, styles *Styles, headers []string, rows [][]string, activeIndex int) {
	fullHeaders := headers
	fullRows := rows
	if activeIndex >= 0 {
		fullHeaders = append([]string{""}, headers...)
		fullRows = make([][]string, len(rows))
		for i, r := range rows {
			marker := " "
			if i == activeIndex {
				marker = "●"
			}
			fullRows[i] = append([]string{marker}, r...)
		}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(styles.Border).
		Headers(fullHeaders...).
		Rows(fullRows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			switch row {
			case table.HeaderRow:
				return styles.Header.Padding(0, 1)
			case activeIndex:
				return styles.Active.Padding(0, 1)
			default:
				return lipgloss.NewStyle().Padding(0, 1)
			}
		})
	_, _ = fmt.Fprintln(w, t)
}

// RenderKV writes a bordered two-column table for detail output (whoami, login
// summaries, project detail). It draws the same visual language as RenderList:
// a NormalBorder table with the shared border colour, a label column and a value
// column, no header row. Labels are uppercased and take the RenderList header
// styling so tables and detail views read as one product. An empty pairs writes
// nothing.
func RenderKV(w io.Writer, styles *Styles, pairs [][2]string) {
	if len(pairs) == 0 {
		return
	}
	rows := make([][]string, len(pairs))
	for i, p := range pairs {
		rows[i] = []string{strings.ToUpper(p[0]), p[1]}
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(styles.Border).
		Rows(rows...).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == 0 {
				return styles.Header.Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	_, _ = fmt.Fprintln(w, t)
}
