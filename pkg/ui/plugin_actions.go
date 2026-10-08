package ui

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

// pluginActionSource lists and runs plugin actions. SetPluginHost sets it to
// the plugin host; tests use a fake.
type pluginActionSource interface {
	Actions(issue *model.Issue) []plugin.Action
	Invoke(ctx context.Context, a plugin.Action, issue *model.Issue, view string) plugin.ActionResult
}

// pluginMenuKey opens the plugin action menu.
const pluginMenuKey = "P"

// listPagingKeys are the bubbles list's own paging and jump keys, which the
// list consumes even though no bt map binds them.
var listPagingKeys = key.NewBinding(key.WithKeys("b", "u", "d", "f", "g", "G"))

// pluginActionResultMsg carries the outcome of a plugin action back onto the
// event loop. started identifies the pending entry the action created.
type pluginActionResultMsg struct {
	ID      string
	Action  plugin.Action
	Result  plugin.ActionResult
	started time.Time
}

// selectedIssue returns the bead under the cursor in the list, board, tree or
// epics view, or nil (no selection, an epics project header, another view).
func (m Model) selectedIssue() *model.Issue {
	switch m.mode {
	case ViewList:
		if m.focused != focusList && m.focused != focusDetail {
			return nil
		}
		if id := m.selectedIssueID(); id != "" {
			return m.data.issueMap[id]
		}
	case ViewBoard:
		return m.board.SelectedIssue()
	case ViewTree:
		return m.tree.SelectedIssue()
	case ViewEpics:
		if r, ok := m.epicsTree.cursorRow(); ok && r.kind != rowProjectHeader {
			return r.issue
		}
	}
	return nil
}

// pluginView names the view plugin action keys apply to ("list", "board",
// "tree" or "epics"), or "" when the current state takes no plugin keys.
func (m Model) pluginView() string {
	switch {
	case m.mode == ViewList && (m.focused == focusList || m.focused == focusDetail) && m.list.FilterState() != list.Filtering:
		return "list"
	case m.mode == ViewBoard && m.focused == focusBoard && !m.board.IsSearchMode():
		return "board"
	case m.mode == ViewTree && m.focused == focusTree:
		return "tree"
	case m.mode == ViewEpics && m.focused == focusEpics:
		return "epics"
	}
	return ""
}

// btBindsKey reports whether bt itself binds msg in view: a global binding,
// one of the view's bindings, or a key the view's handler consumes anyway.
func (m Model) btBindsKey(msg tea.KeyPressMsg, view string) bool {
	maps := []help.KeyMap{m.keys.Global}
	if km := m.viewSpecificKeyMap(); km != nil {
		maps = append(maps, km)
	}
	for _, km := range maps {
		for _, group := range km.FullHelp() {
			if key.Matches(msg, group...) {
				return true
			}
		}
		if key.Matches(msg, km.ShortHelp()...) {
			return true
		}
	}
	switch view {
	case "list":
		return key.Matches(msg, listPagingKeys)
	case "board":
		// handleBoardKeys reuses the list's status filter keys.
		ln := m.keys.ListNormal
		return key.Matches(msg, ln.FilterOpen, ln.FilterClosed, ln.FilterReady, ln.CycleStatusFilter)
	}
	return false
}

// tryPluginActionKey handles a key press that bt does not bind in the list,
// board, tree or epics view: P opens the plugin action menu, and a plugin
// action's key invokes it on the selected bead. ok is false when the key is
// not a plugin key.
func (m Model) tryPluginActionKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.pluginActions == nil || m.activeModal != ModalNone {
		return m, nil, false
	}
	view := m.pluginView()
	if view == "" || m.btBindsKey(msg, view) {
		return m, nil, false
	}
	k := msg.String()
	if k == pluginMenuKey {
		m2, cmd := m.openPluginMenu()
		return m2, cmd, true
	}
	issue := m.selectedIssue()
	if issue == nil {
		return m, nil, false
	}
	for _, a := range m.pluginActions.Actions(issue) {
		if a.Key != "" && a.Key == k {
			m2, cmd := m.invokePluginAction(a, issue)
			return m2, cmd, true
		}
	}
	return m, nil, false
}

// invokePluginAction marks issue pending and runs a off the UI goroutine. A
// bead with a pending write or action is refused.
func (m Model) invokePluginAction(a plugin.Action, issue *model.Issue) (Model, tea.Cmd) {
	id := issue.ID
	if pw, busy := m.pendingWrites[id]; busy {
		m.setNotice(fmt.Sprintf("%s already running on %s", pw.label(), id))
		return m, nil
	}
	started := time.Now()
	m.pendingWrites[id] = pendingWrite{Kind: writePluginAction, Field: a.Label, StartedAt: started}
	m.updateListDelegate()
	view := m.pluginView()
	if view != "list" {
		m.setNotice(fmt.Sprintf("%s %s…", a.Label, id))
	}

	src := m.pluginActions
	issueCopy := *issue
	cmds := []tea.Cmd{func() tea.Msg {
		res := src.Invoke(context.Background(), a, &issueCopy, view)
		return pluginActionResultMsg{ID: id, Action: a, Result: res, started: started}
	}}
	if !m.writeSpinnerActive {
		m.writeSpinnerActive = true
		cmds = append(cmds, writeSpinnerTickCmd())
	}
	return m, tea.Batch(cmds...)
}

// handlePluginActionResult ends a plugin action: clears its pending entry,
// closes the plugin's prompt if it is no longer awaited, and reports the
// result. A dismiss result quits bt in popup mode.
func (m Model) handlePluginActionResult(msg pluginActionResultMsg) (Model, tea.Cmd) {
	if pw, ok := m.pendingWrites[msg.ID]; ok && pw.Kind == writePluginAction && pw.StartedAt.Equal(msg.started) {
		delete(m.pendingWrites, msg.ID)
		m.updateListDelegate()
	}
	// A prompt from this plugin may still serve another of its actions; close
	// it only once it is no longer awaited. Otherwise its Done watcher does.
	if p := m.pluginPrompt; p != nil && p.kind != pluginPromptMenu && p.plugin == msg.Action.Plugin && promptEnded(p.done) {
		m.closePluginPrompt()
	}

	label := msg.Action.Label
	switch {
	case msg.Result.Unknown:
		m.setFailure(fmt.Sprintf("%s on %s: result unknown", label, msg.ID))
	case msg.Result.Toast != nil:
		m.showPluginToast(*msg.Result.Toast)
	default:
		m.setStatus(label + " done")
	}
	if msg.Result.Dismiss && m.popupMode {
		return m, tea.Quit
	}
	return m, nil
}

// showPluginToast shows a plugin toast with the severity of its tone.
func (m *Model) showPluginToast(t plugin.Toast) {
	switch t.Tone {
	case "warn":
		m.setNotice(t.Message)
	case "error":
		m.setFailure(t.Message)
	default:
		m.setStatus(t.Message)
	}
}

// clearPluginPending ends the pending state of plugin actions on beads whose
// plugin state changed.
func (m *Model) clearPluginPending(keys []plugin.BeadKey) {
	changed := false
	for _, k := range keys {
		if pw, ok := m.pendingWrites[k.ID]; ok && pw.Kind == writePluginAction {
			delete(m.pendingWrites, k.ID)
			changed = true
		}
	}
	if changed {
		m.updateListDelegate()
	}
}

// expirePluginActions drops plugin actions pending longer than
// writeSettleTimeout. Unlike bd writes they never settle on a reload, so the
// spinner tick checks them.
func (m *Model) expirePluginActions(now time.Time) {
	changed := false
	for id, pw := range m.pendingWrites {
		if pw.Kind != writePluginAction || now.Sub(pw.StartedAt) < writeSettleTimeout {
			continue
		}
		delete(m.pendingWrites, id)
		changed = true
		m.setFailure(fmt.Sprintf("%s on %s: no confirmation after %s", pw.label(), id, writeSettleTimeout))
	}
	if changed {
		m.updateListDelegate()
	}
}
