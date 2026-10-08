package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

// pluginSyncMsg asks the model to send the current issues to the plugin
// host. Init returns it so plugins get the initial issue set even when no
// background worker ever delivers a snapshot.
type pluginSyncMsg struct{}

// syncPlugins sends the issues to the plugin host when they changed since
// the last sync. It is a no-op without a host. The data hash covers
// updated_at, and bd bumps updated_at on metadata writes (verified
// 2026-10-07), so metadata-only changes are detected too.
func (m *Model) syncPlugins() {
	m.syncPluginsWithHash("")
}

// syncPluginsWithHash is syncPlugins with the data hash already known, e.g.
// from the worker's snapshot. An empty hash is computed here.
func (m *Model) syncPluginsWithHash(hash string) {
	if m.pluginHost == nil {
		return
	}
	if hash == "" {
		hash = analysis.ComputeDataHash(m.data.issues)
	}
	if hash == m.pluginSyncHash {
		return
	}
	m.pluginHost.SyncIssues(m.data.issues)
	m.pluginSyncHash = hash
}

// handlePluginMsg handles the messages the plugin host sends. ok is false
// for any other message.
func (m Model) handlePluginMsg(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case pluginSyncMsg:
		m.syncPlugins()
		return m, nil, true

	case plugin.StateChangedMsg:
		// pending actions are cleared in clearPluginPending
		m.clearPluginPending(msg.Beads)
		if m.pluginFields != nil && m.filter.activeBQLExpr != nil && strings.HasPrefix(m.filter.currentFilter, "bql:") {
			query := strings.TrimPrefix(m.filter.currentFilter, "bql:")
			if queryMentionsPluginField(query, m.pluginFields()) {
				m.reapplyBQLKeepingSelection(query)
				return m, nil, true
			}
		}
		if m.detailPaneVisible() {
			m.updateViewportContent()
		}
		return m, nil, true

	case plugin.ToastMsg:
		switch msg.Toast.Tone {
		case "warn":
			m.setNotice(msg.Toast.Message)
		case "error":
			m.setFailure(msg.Toast.Message)
		default:
			m.setStatus(msg.Toast.Message)
		}
		return m, nil, true

	case plugin.StatusMsg:
		st := msg.Status
		switch {
		case st.State == "failed":
			m.setFailure(fmt.Sprintf("Plugin %s failed: %s", st.Name, st.LastError))
		case st.State == "active" && st.Restarts > 0:
			m.setStatus(fmt.Sprintf("Plugin %s restarted", st.Name))
		}
		return m, nil, true

	case plugin.PromptMsg:
		if msg.Reply != nil {
			msg.Reply(nil)
		}
		return m, nil, true
	}
	return m, nil, false
}

// clearPluginPending ends the pending state of plugin actions on beads whose
// plugin state changed.
func (m *Model) clearPluginPending(_ []plugin.BeadKey) {}

// queryMentionsPluginField reports whether a BQL query names a field of a
// plugin, given the plugins' "<name>." field prefixes.
func queryMentionsPluginField(query string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.Contains(query, p) {
			return true
		}
	}
	return false
}

// detailPaneVisible reports whether the list view currently shows the
// detail viewport.
func (m Model) detailPaneVisible() bool {
	if m.mode != ViewList {
		return false
	}
	switch m.fullscreen {
	case fullscreenDetails:
		return true
	case fullscreenNone:
		return m.isSplitView || m.showDetails
	}
	return false
}

// reapplyBQLKeepingSelection re-runs the active BQL query, keeping the
// selected bead selected when it still matches.
func (m *Model) reapplyBQLKeepingSelection(query string) {
	var selectedID string
	if sel := m.list.SelectedItem(); sel != nil {
		if item, ok := sel.(IssueItem); ok {
			selectedID = item.Issue.ID
		}
	}
	m.applyBQL(m.filter.activeBQLExpr, query)
	if selectedID == "" {
		return
	}
	for i, it := range m.list.VisibleItems() {
		if item, ok := it.(IssueItem); ok && item.Issue.ID == selectedID {
			m.list.Select(i)
			m.updateViewportContent()
			return
		}
	}
}
