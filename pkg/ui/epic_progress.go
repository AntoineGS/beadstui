package ui

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// buildEpicProgressANSI renders an epic's progress summary line plus one
// status-pill row per child, in natural-numeric order (epicChildrenSorted). It
// supplies the detail-pane Epic Progress block. The epic focus card builds
// its own role-styled rows, sharing epicChildrenSorted and the pill helpers.
//
// selectedIdx highlights one child row with a ▸ cursor; pass -1 for the static
// detail-pane embed. width is the available
// content width — child titles truncate to whatever remains after the fixed
// cursor/pill/id segments (0 disables truncation). Returns "" when the epic has
// no children so callers can skip the section (and its heading) entirely.
//
// Pills reuse RenderStatusBadge / RenderPriorityBadge (styles.go). Closed
// children render role-specific ID and title spans faint so completed work
// recedes (replacing the old markdown strikethrough) while the grey DONE pill
// still conveys status; the colored pills on active children pop. The summary +
// done count match the detail block's prior epicProgress semantics
// (Status.IsClosed). The output contains ANSI SGR sequences and MUST be routed
// through addANSI (renderSection's ANSI track), never addMD — lipgloss cannot
// survive Glamour's chroma code-fence path (bt-x5xc4).
func buildEpicProgressANSI(epic model.Issue, allIssues []model.Issue, selectedIdx, width int) string {
	children := epicChildrenSorted(epic.ID, allIssues)
	if len(children) == 0 {
		return ""
	}

	done := 0
	for _, c := range children {
		if c.Status.IsClosed() {
			done++
		}
	}
	total := len(children)
	pct := done * 100 / total

	// This detail embed has no Theme parameter; use the cached global roles.
	text := ActiveTextStyles
	dimIDStyle, dimTitleStyle := text.Metadata.Faint(true), text.Body.Faint(true)

	lines := []string{
		text.Metadata.Render(fmt.Sprintf("%d / %d children complete (%d%%)", done, total, pct)),
		"",
	}

	for i, child := range children {
		statusPill := RenderStatusBadge(string(child.Status))
		prioPill := RenderPriorityBadge(child.Priority)

		idStyle, titleStyle, cursorStyle := text.Metadata, text.Body, text.Metadata
		if child.Status.IsClosed() {
			idStyle, titleStyle = dimIDStyle, dimTitleStyle
		}
		cursor := "  "
		if i == selectedIdx {
			idStyle, titleStyle, cursorStyle = text.Selected, text.Selected, text.Selected
			cursor = "▸ "
		}
		cursor = cursorStyle.Render(cursor)

		title := child.Title
		if width > 0 {
			// Title budget: width minus the fixed leading segments. Pills carry
			// background SGR but their display width is the label text width, so
			// lipgloss.Width measures the on-screen cells correctly.
			fixed := lipgloss.Width(cursor) + lipgloss.Width(statusPill) + 1 +
				lipgloss.Width(prioPill) + 1 + lipgloss.Width(child.ID) + 3 // " — "
			budget := width - fixed
			if budget < 0 {
				budget = 0
			}
			title = truncateString(child.Title, budget)
		}

		// Style ordinary spans separately: never apply text attributes over
		// nested ANSI pills, which retain their domain-semantic color pairs.
		idAndTitle := idStyle.Render(child.ID)
		if title != "" {
			idAndTitle += titleStyle.Render(" — ") + titleStyle.Render(title)
		}

		gap := titleStyle.Render(" ")
		lines = append(lines, cursor+statusPill+gap+prioPill+gap+idAndTitle)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
