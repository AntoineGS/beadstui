package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// actionCmd dispatches action id as a synthetic key press carrying its token,
// so it runs the full Update path like the physical key would (bt-i4k).
func actionCmd(id string) tea.Cmd {
	return func() tea.Msg { return tea.KeyPressMsg{Text: keys.Token(id)} }
}
