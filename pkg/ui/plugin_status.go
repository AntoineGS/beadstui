package ui

import (
	"fmt"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// PluginKeyConflict is a plugin action key that is not bound in some or all
// plugin views: bt binds it there, or an earlier action claimed it.
type PluginKeyConflict struct {
	Plugin string `json:"plugin"`
	Action string `json:"action"`
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// PluginKeyConflicts reports the key conflicts of the actions in statuses,
// against bt's default keymaps. Actions are taken in config then manifest
// order; the first to declare a key wins it.
func PluginKeyConflicts(statuses []plugin.Status) []PluginKeyConflict {
	return pluginKeyConflictsIn(keys.NewAppKeys(), statuses)
}

func pluginKeyConflictsIn(ks keys.AppKeys, statuses []plugin.Status) []PluginKeyConflict {
	var out []PluginKeyConflict
	claimed := map[string]plugin.Action{}
	for _, st := range statuses {
		for _, a := range st.Actions {
			if a.Key == "" {
				continue
			}
			if a.Key == pluginMenuKey {
				out = append(out, PluginKeyConflict{Plugin: a.Plugin, Action: a.ID, Key: a.Key,
					Reason: "bt opens the plugin action menu with it"})
				continue
			}
			var views []string
			for _, v := range pluginViews {
				if btBindsKeyIn(ks, v, a.Key) {
					views = append(views, v)
				}
			}
			if len(views) > 0 {
				out = append(out, PluginKeyConflict{Plugin: a.Plugin, Action: a.ID, Key: a.Key,
					Reason: "bt binds it in " + strings.Join(views, ", ")})
			}
			if len(views) == len(pluginViews) {
				continue
			}
			if first, ok := claimed[a.Key]; ok {
				out = append(out, PluginKeyConflict{Plugin: a.Plugin, Action: a.ID, Key: a.Key,
					Reason: fmt.Sprintf("taken by %s/%s", first.Plugin, first.ID)})
				continue
			}
			claimed[a.Key] = a
		}
	}
	return out
}

// pluginStatusTitle is the title of the plugin status popup and the action
// menu row that opens it.
const pluginStatusTitle = "Plugin status"

// openPluginStatus opens the plugin status popup.
func (m *Model) openPluginStatus() {
	m.showPluginPrompt(&pluginPrompt{kind: pluginPromptStatus, title: pluginStatusTitle})
}

// pluginStatusLines describes every configured plugin: a summary line, then
// its last error and key conflicts indented below it. Plugin text is
// sanitized.
func (m Model) pluginStatusLines() []string {
	if m.pluginStatuses == nil {
		return nil
	}
	statuses := m.pluginStatuses()
	if len(statuses) == 0 {
		return []string{"No plugins configured"}
	}
	conflicts := map[string][]PluginKeyConflict{}
	for _, c := range pluginKeyConflictsIn(m.keys, statuses) {
		conflicts[c.Plugin] = append(conflicts[c.Plugin], c)
	}
	var lines []string
	for _, st := range statuses {
		lines = append(lines, formatPluginStatus(st))
		if st.LastError != "" {
			lines = append(lines, "  error: "+pluginText(st.LastError, false))
		}
		for _, c := range conflicts[st.Name] {
			lines = append(lines, fmt.Sprintf("  key %s (%s): %s", pluginText(c.Key, false), pluginText(c.Action, false), c.Reason))
		}
	}
	return lines
}

// formatPluginStatus is the summary line of one plugin: name, state,
// version and restarts.
func formatPluginStatus(st plugin.Status) string {
	line := st.Name + "  " + st.State
	if st.Version != "" {
		line += "  v" + pluginText(st.Version, false)
	}
	if st.Restarts > 0 {
		line += fmt.Sprintf("  restarts %d", st.Restarts)
	}
	return line
}

// pluginStatusLayout returns the status popup's lines and options, and the
// largest scroll offset that still fills the body.
func (m Model) pluginStatusLayout(title string) (lines []string, opts PopupOpts, maxScroll int) {
	lines = m.pluginStatusLines()
	opts = PopupOpts{
		Title: title, Theme: m.theme, Available: &PopupSize{m.width, max(0, m.height-1)},
		Footer: []string{"↑/↓ scroll  esc close", "esc"},
	}
	l := MeasurePopup(lines, opts)
	return lines, opts, max(0, len(lines)-max(1, l.BodyHeight))
}

// scrollPluginStatus moves the status popup's scroll offset by delta.
func (m Model) scrollPluginStatus(p *pluginPrompt, delta int) {
	_, _, maxScroll := m.pluginStatusLayout(p.title)
	p.cursor = max(0, min(p.cursor+delta, maxScroll))
}

// renderPluginStatus returns the plugin status popup, scrolled to the
// prompt's offset. Statuses are read on every render, so the popup follows
// plugins as they start, fail and restart.
func (m Model) renderPluginStatus(p *pluginPrompt) string {
	lines, opts, maxScroll := m.pluginStatusLayout(p.title)
	off := min(p.cursor, maxScroll)
	if maxScroll > 0 {
		opts.RightLabel = fmt.Sprintf("%d/%d", off+1, len(lines))
		opts.Height = MeasurePopup(lines, opts).Height
	}
	return RenderPopup(lines[off:], opts)
}
