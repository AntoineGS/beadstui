package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// restoreFocusFromHelp returns the exact focus captured before the help overlay
// opened. That may be a pane, a text input, or a modal-local focus state.
func (m Model) restoreFocusFromHelp() focus {
	return m.focusBeforeHelp
}

// closeHelp dismisses the help overlay and restores the modal and focus it
// temporarily replaced. Modal data remains in its owning model field.
func (m *Model) closeHelp() {
	if m.activeModal != ModalHelp {
		return
	}
	m.activeModal = m.helpOriginModal
	m.focused = m.restoreFocusFromHelp()
	m.helpOriginModal = ModalNone
	m.helpScroll = 0
}

// handleHelpKeys handles keyboard input when the help overlay is focused.
//
// Dispatches via key.Matches against m.keys.Help.
func (m Model) handleHelpKeys(msg tea.KeyMsg) Model {
	k := m.keys.Help
	switch {
	case key.Matches(msg, k.Down):
		m.helpScroll++
		if max := m.helpScrollMax(); m.helpScroll > max {
			m.helpScroll = max
		}
	case key.Matches(msg, k.Up):
		if m.helpScroll > 0 {
			m.helpScroll--
		}
	case key.Matches(msg, k.PageDown):
		m.helpScroll += 10
		if max := m.helpScrollMax(); m.helpScroll > max {
			m.helpScroll = max
		}
	case key.Matches(msg, k.PageUp):
		m.helpScroll -= 10
		if m.helpScroll < 0 {
			m.helpScroll = 0
		}
	case key.Matches(msg, k.JumpTop):
		m.helpScroll = 0
	case key.Matches(msg, k.JumpBottom):
		m.helpScroll = m.helpScrollMax()
	case key.Matches(msg, k.Close):
		m.closeHelp()
	case key.Matches(msg, k.Tutorial): // Space opens interactive tutorial (bv-0trk, bv-8y31)
		m.closeModal()
		m.helpOriginModal = ModalNone
		m.helpScroll = 0
		m.openModal(ModalTutorial)
		m.tutorialModel.SetSize(m.width, m.height)
		m.focused = focusTutorial
	default:
		// Any other key dismisses help and restores previous focus
		m.closeHelp()
	}
	return m
}
