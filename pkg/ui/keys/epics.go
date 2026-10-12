package keys

import "charm.land/bubbles/v2/key"

// EpicsKeys are the bindings available when the epics tree is focused.
// handleEpicsKeys dispatches against these via key.Matches; help surfaces
// consume them via ShortHelp / FullHelp.
//
// Up/Down field names and Help.Key strings match ListNormalKeys/TreeKeys for
// the universal-nav consistency test (arrows-primary, vim-alternate per
// ADR-004 Decision 1). Expand/Collapse are deliberately NOT named
// Right/Left/Enter so they sit outside the universal-nav consistency contract
// (their key sets are tree-specific: enter is the context-sensitive Open).
//
// The exit binding is named Exit (not Back) to keep it outside the
// universal-nav consistency check.
type EpicsKeys struct {
	// Navigation
	Up         key.Binding `action:"nav.up"`
	Down       key.Binding `action:"nav.down"`
	JumpTop    key.Binding `action:"nav.top"`
	JumpBottom key.Binding `action:"nav.bottom"`
	PageDown   key.Binding `action:"nav.page_down"`
	PageUp     key.Binding `action:"nav.page_up"`

	// Tree
	Expand      key.Binding `action:"epics.expand"`       // expand epic/lane (and focus subtree)
	Collapse    key.Binding `action:"epics.collapse"`     // collapse epic/lane (or jump to parent)
	CollapseAll key.Binding `action:"epics.collapse_all"` // collapse every epic back to the lane overview

	// Actions
	Open        key.Binding `action:"epics.open"` // enter: expand epic/header, or drill into a child
	CycleStatus key.Binding `action:"epics.cycle_status"`
	Card        key.Binding `action:"epics.card"` // open the single-epic focus card (zoom)
	FieldEdit   key.Binding `action:"bead.edit"`  // e: edit the selected bead's fields
	Exit        key.Binding `action:"epics.exit"`
}

// NewEpicsKeys returns the default epics-tree keymap.
func NewEpicsKeys() EpicsKeys {
	return EpicsKeys{
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
			key.WithKeys("G", "end"),
			key.WithHelp("G/end", "jump to bottom"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("⌃d/pgdn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("⌃u/pgup", "page up"),
		),
		Expand: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("z"),
			key.WithHelp("z", "collapse all"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "expand / drill"),
		),
		CycleStatus: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "active / all / completed"),
		),
		Card: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "zoom card"),
		),
		FieldEdit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit field"),
		),
		Exit: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back to list"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
// Item nav first as the primary orientation; expand/drill/zoom/exit are the
// resolution actions.
func (k EpicsKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Expand, k.Collapse, k.Open, k.CycleStatus, k.Card, k.Exit}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
// Columns: Navigate / Tree / Actions.
func (k EpicsKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.JumpTop, k.JumpBottom, k.PageDown, k.PageUp},
		{k.Expand, k.Collapse, k.CollapseAll},
		{k.Open, k.CycleStatus, k.Card, k.FieldEdit, k.Exit},
	}
}
