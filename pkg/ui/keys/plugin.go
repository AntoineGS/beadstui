package keys

import "charm.land/bubbles/v2/key"

// PluginSelectKeys are the bindings for the plugin select prompt and the
// plugin action menu (pkg/ui/plugin_prompt.go). The prompt owns every key
// while open, so these cannot collide with view bindings.
//
// Up/Down field names and Help.Key strings match FieldPickerKeys for the
// universal-nav consistency test. Cancel (not Esc) avoids that check, like
// the other pickers.
type PluginSelectKeys struct {
	Up     key.Binding `action:"nav.up"`
	Down   key.Binding `action:"nav.down"`
	Apply  key.Binding `action:"plugin_select.apply"`
	Cancel key.Binding `action:"plugin_select.cancel"`
	Close  key.Binding `action:"plugin_select.close"`
}

// NewPluginSelectKeys returns the default plugin select keymap.
func NewPluginSelectKeys() PluginSelectKeys {
	return PluginSelectKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Apply: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "select"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Close: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "close status"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k PluginSelectKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Apply, k.Cancel}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
func (k PluginSelectKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Apply, k.Cancel, k.Close},
	}
}
