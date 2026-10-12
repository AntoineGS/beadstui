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
		{k.Up, k.Down, k.JumpTop, k.JumpBottom},
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

// AttentionKeys are the bindings for the attention view. PickLabel matches the
// digits 1-9; the handler reads the digit from the key text, so dispatching
// PickLabel by token is a no-op.
type AttentionKeys struct {
	Close     key.Binding `action:"attention.close"`
	PickLabel key.Binding `action:"attention.pick_label"`
}

// NewAttentionKeys returns the default attention view keymap.
func NewAttentionKeys() AttentionKeys {
	return AttentionKeys{
		Close:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close attention")),
		PickLabel: key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "filter by label")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k AttentionKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.PickLabel, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k AttentionKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.PickLabel, k.Close}}
}
