package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// renderWhichKey renders the which-key panel for the pending sequence: a
// full-width bordered strip listing the next keys, groups first (bt-i4k).
// Returns "" when no panel is due.
func (m Model) renderWhichKey() string {
	if !m.seq.pending() || !m.seq.showPanel || m.seq.node == nil || m.width < 8 {
		return ""
	}
	node := m.seq.node

	title := keys.FormatSeq(m.seq.prefix, m.seq.steps, m.leaderLabel())
	if node.Label != "" {
		title += "  " + strings.TrimPrefix(node.Label, "+")
	}

	keyStyle := ActiveTextStyles.Heading
	descStyle := ActiveTextStyles.Body
	groupStyle := ActiveTextStyles.Metadata
	type cell struct{ plain, styled string }
	cells := make([]cell, 0, len(node.Children))
	colW := 0
	for _, c := range node.Children {
		desc, style := c.Desc, descStyle
		if c.IsGroup() {
			desc, style = c.Label, groupStyle
		}
		plain := c.Key + "  " + desc
		cells = append(cells, cell{plain, keyStyle.Render(c.Key) + "  " + style.Render(desc)})
		colW = max(colW, lipgloss.Width(plain)+2)
	}

	inner := m.width - 4 // border plus one cell of padding each side
	cols := max(1, inner/max(1, colW))
	maxRows := max(1, m.height/3)
	rows := (len(cells) + cols - 1) / cols
	if rows > maxRows {
		rows = maxRows
		keep := rows*cols - 1
		cells = append(cells[:keep], cell{"…", groupStyle.Render("…")})
	}

	lines := make([]string, rows)
	for i, c := range cells {
		r := i % rows
		pad := ""
		if i/rows < cols-1 {
			pad = strings.Repeat(" ", max(0, colW-lipgloss.Width(c.plain)))
		}
		lines[r] += c.styled + pad
	}
	for i := range lines {
		lines[i] = " " + lines[i]
	}

	return RenderTitledPanel(strings.Join(lines, "\n"), PanelOpts{
		Title:       title,
		Width:       m.width,
		BorderColor: m.theme.Border,
	})
}
