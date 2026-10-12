package keys

import "charm.land/bubbles/v2/key"

// LabelDashboardKeys are the bindings for the label health dashboard view.
// The dashboard model owns Up/Down/JumpTop/JumpBottom/Apply; the Model owns
// Health, Drilldown and Close.
type LabelDashboardKeys struct {
	Up         key.Binding `action:"nav.up"`
	Down       key.Binding `action:"nav.down"`
	JumpTop    key.Binding `action:"nav.top"`
	JumpBottom key.Binding `action:"nav.bottom"`
	PageDown   key.Binding `action:"nav.page_down"`
	PageUp     key.Binding `action:"nav.page_up"`
	Apply      key.Binding `action:"label_dashboard.apply"`
	Health     key.Binding `action:"label_dashboard.health"`
	Drilldown  key.Binding `action:"label_dashboard.drilldown"`
	Close      key.Binding `action:"label_dashboard.close"`
}

// NewLabelDashboardKeys returns the default label dashboard keymap.
func NewLabelDashboardKeys() LabelDashboardKeys {
	return LabelDashboardKeys{
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "move up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "move down")),
		JumpTop:    key.NewBinding(key.WithKeys("home", "gg"), key.WithHelp("gg/home", "jump to top")),
		JumpBottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("end/G", "jump to bottom")),
		PageDown:   key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("⌃d/pgdn", "page down")),
		PageUp:     key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("⌃u/pgup", "page up")),
		Apply:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "filter list by label")),
		Health:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "health detail")),
		Drilldown:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drilldown")),
		Close:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close dashboard")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k LabelDashboardKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Apply, k.Drilldown, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k LabelDashboardKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.JumpTop, k.JumpBottom, k.PageDown, k.PageUp},
		{k.Apply, k.Health, k.Drilldown, k.Close},
	}
}

// LabelHealthKeys are the bindings for the label health detail modal.
type LabelHealthKeys struct {
	Close     key.Binding `action:"label_health.close"`
	Drilldown key.Binding `action:"label_health.drilldown"`
}

// NewLabelHealthKeys returns the default label health detail keymap.
func NewLabelHealthKeys() LabelHealthKeys {
	return LabelHealthKeys{
		Close:     key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc", "close")),
		Drilldown: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drilldown")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k LabelHealthKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Drilldown, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k LabelHealthKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Drilldown, k.Close}}
}

// LabelDrilldownKeys are the bindings for the label drilldown modal.
type LabelDrilldownKeys struct {
	Apply key.Binding `action:"label_drilldown.apply"`
	Graph key.Binding `action:"label_drilldown.graph"`
	Close key.Binding `action:"label_drilldown.close"`
}

// NewLabelDrilldownKeys returns the default label drilldown keymap.
func NewLabelDrilldownKeys() LabelDrilldownKeys {
	return LabelDrilldownKeys{
		Apply: key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "filter list by label")),
		Graph: key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "graph analysis")),
		Close: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k LabelDrilldownKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Apply, k.Graph, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k LabelDrilldownKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Apply, k.Graph, k.Close}}
}

// LabelGraphKeys are the bindings for the label graph analysis modal.
type LabelGraphKeys struct {
	Close key.Binding `action:"label_graph.close"`
}

// NewLabelGraphKeys returns the default label graph analysis keymap.
func NewLabelGraphKeys() LabelGraphKeys {
	return LabelGraphKeys{
		Close: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k LabelGraphKeys) ShortHelp() []key.Binding { return []key.Binding{k.Close} }

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k LabelGraphKeys) FullHelp() [][]key.Binding { return [][]key.Binding{{k.Close}} }

// AttentionKeys are the bindings for the attention view. Each digit has its
// own action ID so remapped and synthetic bindings retain their ordinal.
type AttentionKeys struct {
	Close      key.Binding `action:"attention.close"`
	PickLabel1 key.Binding `action:"attention.pick_label_1"`
	PickLabel2 key.Binding `action:"attention.pick_label_2"`
	PickLabel3 key.Binding `action:"attention.pick_label_3"`
	PickLabel4 key.Binding `action:"attention.pick_label_4"`
	PickLabel5 key.Binding `action:"attention.pick_label_5"`
	PickLabel6 key.Binding `action:"attention.pick_label_6"`
	PickLabel7 key.Binding `action:"attention.pick_label_7"`
	PickLabel8 key.Binding `action:"attention.pick_label_8"`
	PickLabel9 key.Binding `action:"attention.pick_label_9"`
}

// NewAttentionKeys returns the default attention view keymap.
func NewAttentionKeys() AttentionKeys {
	return AttentionKeys{
		Close:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close attention")),
		PickLabel1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1-9", "filter by label")),
		PickLabel2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "filter by label")),
		PickLabel3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "filter by label")),
		PickLabel4: key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "filter by label")),
		PickLabel5: key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "filter by label")),
		PickLabel6: key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "filter by label")),
		PickLabel7: key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "filter by label")),
		PickLabel8: key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "filter by label")),
		PickLabel9: key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "filter by label")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k AttentionKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.PickLabel1, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k AttentionKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.PickLabel1, k.PickLabel2, k.PickLabel3, k.PickLabel4, k.PickLabel5,
		k.PickLabel6, k.PickLabel7, k.PickLabel8, k.PickLabel9, k.Close}}
}
