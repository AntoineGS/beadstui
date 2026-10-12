package keys

import "charm.land/bubbles/v2/key"

// FlowMatrixKeys are the bindings available when the flow-matrix view is
// focused. Universal nav (j/k) is declared here, not in GlobalKeys, because
// the flow-matrix cursor operates on its own label list and drilldown list
// independently of the main list. See ADR-004 Decision 1.
//
// Convention (per ADR-004 Decision 1, set by ListKeys in bt-ift6.2):
// arrows-primary, vim-keys-as-alternate. WithKeys lists arrow first, vim
// second; help text shows arrow first ("↑/k", not "k/↑").
//
// The exit binding uses field name Close, not Back, to keep it outside the
// universal-nav consistency test. See history.go ExitHistory/ExitFileTree for
// the same pattern.
type FlowMatrixKeys struct {
	Up          key.Binding `action:"nav.up"`
	Down        key.Binding `action:"nav.down"`
	JumpTop     key.Binding `action:"nav.top"`
	JumpBottom  key.Binding `action:"nav.bottom"`
	PageDown    key.Binding `action:"nav.page_down"`
	PageUp      key.Binding `action:"nav.page_up"`
	TogglePanel key.Binding `action:"flow.toggle_panel"`
	Enter       key.Binding `action:"flow.enter"`
	Close       key.Binding `action:"flow.close"`
}

// NewFlowMatrixKeys returns the default flow-matrix keymap.
func NewFlowMatrixKeys() FlowMatrixKeys {
	return FlowMatrixKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		JumpTop: key.NewBinding(
			key.WithKeys("home", "gg"),
			key.WithHelp("gg/home", "jump to top"),
		),
		JumpBottom: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("end/G", "jump to bottom"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("⌃d/pgdn", "half page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("⌃u/pgup", "half page up"),
		),
		TogglePanel: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "toggle panel"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "open drilldown / jump to issue"),
		),
		Close: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close flow matrix"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
// Order: most-orienting actions first.
func (k FlowMatrixKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
// Columns: Move, Act, Exit.
func (k FlowMatrixKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.JumpTop, k.JumpBottom, k.PageDown, k.PageUp},
		{k.Enter, k.TogglePanel},
		{k.Close},
	}
}
