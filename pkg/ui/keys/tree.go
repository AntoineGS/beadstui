package keys

import "charm.land/bubbles/v2/key"

// TreeKeys are the bindings available when the hierarchical tree view is
// focused. Universal nav (j/k/h/l) is declared here, not in GlobalKeys,
// because the tree's h/l semantics (collapse-to-parent / expand-to-child)
// are tree-specific. See ADR-004 Decision 1.
//
// Convention (per ADR-004 Decision 1, set by ListKeys in bt-ift6.2):
// arrows-primary, vim-keys-as-alternate. WithKeys lists arrow first, vim
// second; help text shows arrow first ("↑/k", not "k/↑").
type TreeKeys struct {
	Up          key.Binding `action:"nav.up"`
	Down        key.Binding `action:"nav.down"`
	Collapse    key.Binding `action:"tree.collapse"`
	Expand      key.Binding `action:"tree.expand"`
	Toggle      key.Binding `action:"tree.toggle"`
	JumpTop     key.Binding `action:"nav.top"`
	JumpBottom  key.Binding `action:"nav.bottom"`
	ExpandAll   key.Binding `action:"tree.expand_all"`
	CollapseAll key.Binding `action:"tree.collapse_all"`
	PageDown    key.Binding `action:"nav.page_down"`
	PageUp      key.Binding `action:"nav.page_up"`
	SyncDetail  key.Binding `action:"tree.sync_detail"`
	Back        key.Binding `action:"tree.back"`
	FieldEdit   key.Binding `action:"bead.edit"`
}

// NewTreeKeys returns the default tree keymap.
func NewTreeKeys() TreeKeys {
	return TreeKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse / jump to parent"),
		),
		Expand: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand / move to child"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "toggle expand"),
		),
		JumpTop: key.NewBinding(
			key.WithKeys("home", "gg"),
			key.WithHelp("gg/home", "jump to top"),
		),
		JumpBottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G/end", "jump to bottom"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "expand all"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "collapse all"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d", "pgdown"),
			key.WithHelp("⌃d", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u", "pgup"),
			key.WithHelp("⌃u", "page up"),
		),
		SyncDetail: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "sync to detail pane"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back to list"),
		),
		FieldEdit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit field"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
// Order matters: most ergonomic / most-used first.
func (k TreeKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Back}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Move, Operate, Page, Exit.
func (k TreeKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Collapse, k.Expand, k.JumpTop, k.JumpBottom},
		{k.Toggle, k.ExpandAll, k.CollapseAll, k.SyncDetail, k.FieldEdit},
		{k.PageDown, k.PageUp},
		{k.Back},
	}
}
