package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// renderEpicCard renders the tier-2 epic focus card (ModalEpicCard): the epic's
// children as status pills, with a cursor on
// the drillable child. Composited via OverlayCenterDimBackdrop in View() per
// docs/design/tui-modal-compositing.md, so this returns just the titled panel.
//
// The child rows are windowed around the cursor (with ↑/↓ "N more" indicators)
// so a large epic still fits a scrunched terminal - the user routinely runs
// 14-30 row windows. The shared child ordering and status/priority pill helpers
// retain domain semantics; ordinary spans use this model's resolved text roles.
func (m Model) renderEpicCard() string {
	epic, ok := m.data.issueMap[m.epicCardID]
	if !ok || epic == nil {
		return ""
	}

	opts := PopupOpts{Title: "Epic " + epic.ID, Theme: m.theme, Available: &PopupSize{m.width, max(0, m.height-1)}, Width: 70}
	children := epicChildrenSorted(epic.ID, m.data.issues)
	if len(children) == 0 {
		return RenderPopup([]string{m.theme.Text.Metadata.Render("No children.")}, opts)
	}
	done := 0
	entries := make([]PopupMenuEntry, len(children))
	for i, child := range children {
		selected := i == m.epicCardCursor
		idStyle, titleStyle := m.theme.Text.Metadata, m.theme.Text.Body
		if child.Status.IsClosed() {
			done++
			idStyle, titleStyle = idStyle.Faint(true), titleStyle.Faint(true)
		}
		gap := titleStyle.Render(" ")
		label := RenderStatusBadge(string(child.Status)) + gap + RenderPriorityBadge(child.Priority) + gap + idStyle.Render(child.ID) + titleStyle.Render(" — ") + titleStyle.Render(child.Title)
		entries[i] = PopupMenuEntry{Label: label, Selected: selected}
	}
	shape := make([]string, len(entries)+4)
	opts.MinBodyRows = 5
	natural := MeasurePopup(shape, opts)
	opts.Height = min(natural.Height, max(8, m.height*8/10))
	l := MeasurePopup(shape, opts)
	if l.Compact || l.Height == 0 {
		return RenderPopup(shape, opts)
	}
	start, end := popupMenuWindowRange(entries, m.epicCardCursor, l.BodyHeight-4)
	muted := m.theme.Text.Metadata
	content := []string{muted.Render(fmt.Sprintf("%d / %d children complete (%d%%)", done, len(children), done*100/len(children))), "", ""}
	if start > 0 {
		content[2] = muted.Render(fmt.Sprintf("↑ %d more", start))
	}
	content = append(content, RenderPopupMenu(entries[start:end], MeasurePopupMenu(entries, PopupMenuOpts{}), m.theme, l.BodyWidth)...)
	for len(content) < l.BodyHeight-1 {
		content = append(content, "")
	}
	below := ""
	if end < len(entries) {
		below = muted.Render(fmt.Sprintf("↓ %d more", len(entries)-end))
	}
	content = append(content, below)
	opts.MinBodyRows = l.BodyHeight
	return RenderPopup(content, opts)
}

// handleEpicCardKeys handles keyboard input while the epic focus card is open
// (activeModal == ModalEpicCard). j/k move the child cursor; enter drills into
// the selected child (jump + focus detail, the alerts-modal mechanism); esc
// closes the card and restores the underlying surface. All other keys are
// swallowed, per the modal contract. bt-gfxhz.3.
func (m Model) handleEpicCardKeys(msg tea.KeyMsg) Model {
	k := m.keys.EpicCard
	children := epicChildrenSorted(m.epicCardID, m.data.issues)
	switch {
	case key.Matches(msg, k.Down):
		if m.epicCardCursor < len(children)-1 {
			m.epicCardCursor++
		}
	case key.Matches(msg, k.Up):
		if m.epicCardCursor > 0 {
			m.epicCardCursor--
		}
	case key.Matches(msg, k.Open):
		if len(children) > 0 && m.epicCardCursor < len(children) {
			childID := children[m.epicCardCursor].ID
			m.closeModal()
			if m.selectIssueByID(childID) {
				m.focusDetailAfterJump()
			} else {
				m.setStatus("Child " + childID + " not in current view")
			}
		}
	case key.Matches(msg, k.Exit):
		m.closeModal()
	}
	return m
}
