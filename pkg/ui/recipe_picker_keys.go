package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// handleRecipePickerKeys handles keyboard input when recipe picker is focused.
//
// Dispatches via key.Matches against m.keys.RecipePicker per bt-ift6.9.
func (m Model) handleRecipePickerKeys(msg tea.KeyMsg) Model {
	kk := m.keys.RecipePicker
	switch {
	case key.Matches(msg, kk.Up):
		m.recipePicker.MoveUp()

	case key.Matches(msg, kk.Down):
		m.recipePicker.MoveDown()

	case key.Matches(msg, kk.Cancel):
		m.closeModal()
		m.focused = focusList

	case key.Matches(msg, kk.Apply):
		// Apply selected recipe
		if selected := m.recipePicker.SelectedRecipe(); selected != nil {
			m.setActiveRecipe(selected)
			m.applyRecipe(selected)
		}
		m.closeModal()
		m.focused = focusList
	}
	return m
}
