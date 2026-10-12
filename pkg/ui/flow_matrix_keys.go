package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// handleFlowMatrixKeys handles keyboard input when flow matrix view is focused.
//
// Dispatches via key.Matches against m.keys.FlowMatrix per ADR-004 Decision 1.
// Converted from switch msg.String() in bt-ift6.8.
func (m Model) handleFlowMatrixKeys(msg tea.KeyMsg) Model {
	k := m.keys.FlowMatrix
	switch {
	case key.Matches(msg, k.Close):
		// If in drilldown mode, close drilldown first
		if m.flowMatrix.showDrilldown {
			if m.flowMatrix.Inspecting() {
				m.flowMatrix.EndInspection()
			} else {
				m.flowMatrix.showDrilldown = false
			}
			return m
		}
		// Close flow matrix view
		m.mode = ViewList
		m.focused = focusList
	case key.Matches(msg, k.Down):
		m.flowMatrix.MoveDown()
	case key.Matches(msg, k.Up):
		m.flowMatrix.MoveUp()
	case key.Matches(msg, k.TogglePanel):
		m.flowMatrix.TogglePanel()
	case key.Matches(msg, k.Enter):
		// Inspect the selected endpoint in place, preserving flow and list state.
		if m.flowMatrix.showDrilldown {
			if selectedIssue := m.flowMatrix.SelectedDrilldownIssue(); selectedIssue != nil {
				m.updateViewportContent()
				m.viewport.GotoTop()
				m.flowMatrix.BeginInspection(m.viewport.View())
			}
		} else {
			// Open drilldown for selected label
			m.flowMatrix.OpenDrilldown()
		}
	case key.Matches(msg, k.JumpBottom):
		m.flowMatrix.GoToEnd()
	case key.Matches(msg, k.JumpTop):
		m.flowMatrix.GoToStart()
	case key.Matches(msg, k.PageDown):
		m.flowMatrix.PageDown()
	case key.Matches(msg, k.PageUp):
		m.flowMatrix.PageUp()
	}
	return m
}
