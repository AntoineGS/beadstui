package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// renderEpicCard renders the tier-2 epic focus card (ModalEpicCard): the epic's
// children as status pills (the shared buildEpicProgressANSI), with a cursor on
// the drillable child. Composited via OverlayCenterDimBackdrop in View() per
// docs/design/tui-modal-compositing.md, so this returns just the titled panel.
//
// The child rows are windowed around the cursor (with ↑/↓ "N more" indicators)
// so a large epic still fits a scrunched terminal - the user routinely runs
// 14-30 row windows. buildEpicProgressANSI stays the single source of truth for
// the row styling; the windowing lives here (bt-gfxhz.3).
func (m Model) renderEpicCard() string {
	epic, ok := m.data.issueMap[m.epicCardID]
	if !ok || epic == nil {
		return ""
	}

	opts := PopupOpts{Title: "Epic " + epic.ID, Theme: m.theme, Available: &PopupSize{m.width, max(0, m.height-1)}, Width: 70, Footer: []string{"j/k move · enter drill · esc close", "j/k enter esc"}}
	body := buildEpicProgressANSI(*epic, m.data.issues, -1, 0)
	if body == "" {
		return RenderPopup([]string{lipgloss.NewStyle().Foreground(m.theme.Muted).Render("No children.")}, opts)
	}
	lines := strings.Split(body, "\n")
	entries := make([]PopupMenuEntry, len(lines)-2)
	for i, row := range lines[2:] {
		entries[i] = PopupMenuEntry{Label: ansi.TruncateLeft(row, 2, ""), Selected: i == m.epicCardCursor}
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
	content := []string{lines[0], "", ""}
	muted := lipgloss.NewStyle().Foreground(m.theme.Muted)
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
