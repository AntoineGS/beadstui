package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// handleMemoriesKeys handles keyboard input while the Memories view has
// focus. Single entry point reached from two call sites (mirrors
// handleHistoryKeys): the early "in-view typing sub-state" guard in
// model_update_input.go (while search is active, so letters that collide
// with global hotkeys are typed instead of switching views) and the
// bottom focus-specific dispatch switch (once search is inactive).
//
// Dispatches via m.keys.MemoriesSearch while search is active (every other
// key is typed into the search input) and m.keys.MemoriesNormal otherwise.
func (m Model) handleMemoriesKeys(msg tea.KeyMsg) Model {
	if m.memories.IsSearchActive() {
		switch {
		case key.Matches(msg, m.keys.MemoriesSearch.Cancel):
			m.memories.CancelSearch()
		case key.Matches(msg, m.keys.MemoriesSearch.Confirm):
			m.memories.ConfirmSearch()
		default:
			m.memories.UpdateSearchInput(msg)
		}
		return m
	}

	k := m.keys.MemoriesNormal
	switch {
	case key.Matches(msg, k.Search):
		m.memories.StartSearch()
	case key.Matches(msg, k.Down):
		if m.memories.detailFocused && m.memories.isSplitWidth() {
			m.memories.ScrollDetailDown()
		} else {
			m.memories.MoveDown()
		}
	case key.Matches(msg, k.Up):
		if m.memories.detailFocused && m.memories.isSplitWidth() {
			m.memories.ScrollDetailUp()
		} else {
			m.memories.MoveUp()
		}
	case key.Matches(msg, k.FocusToggle):
		m.memories.ToggleDetailFocus()
	case key.Matches(msg, k.Open):
		m.memories.FocusDetail()
	case key.Matches(msg, k.BackToList):
		// Single-pane detail -> back to master (split mode already has both
		// panes visible; tab handles focus there).
		if !m.memories.isSplitWidth() {
			m.memories.detailFocused = false
		}
	}
	return m
}
