package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

// pluginActionExpiry is the safety net for a pending plugin action. The
// host's result is authoritative and arrives within its 60s invoke timeout,
// so this only fires if a result is lost.
const pluginActionExpiry = 65 * time.Second

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
	for _, a := range m.pluginActionsFor(issue) {
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
	case msg.Result.NotRunning:
		m.setNotice(fmt.Sprintf("%s is not running; action not sent", msg.Action.Plugin))
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

// pluginActionsFor lists the plugin actions on issue with labels safe to
// draw.
func (m Model) pluginActionsFor(issue *model.Issue) []plugin.Action {
	var out []plugin.Action
	for _, a := range m.pluginActions.Actions(issue) {
		a.Label = pluginText(a.Label, false)
		out = append(out, a)
	}
	return out
}

// showPluginToast shows a plugin toast with the severity of its tone.
func (m *Model) showPluginToast(t plugin.Toast) {
	msg := pluginText(t.Message, false)
	switch t.Tone {
	case "warn":
		m.setNotice(msg)
	case "error":
		m.setFailure(msg)
	default:
		m.setStatus(msg)
	}
}

// pluginText makes plugin-supplied text safe to draw: escape sequences are
// stripped and other control characters dropped. Newlines are kept when
// multiline; otherwise they and tabs become spaces.
func pluginText(s string, multiline bool) string {
	s = ansi.Strip(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' && multiline:
			b.WriteRune(r)
		case r == '\n' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clearPluginPending stops the spinner of plugin actions on beads whose
// plugin state changed. The entry stays, refusing a second action on the
// bead, until the action's result arrives or it expires.
func (m *Model) clearPluginPending(keys []plugin.BeadKey) {
	changed := false
	for _, k := range keys {
		if pw, ok := m.pendingWrites[k.ID]; ok && pw.Kind == writePluginAction && !pw.SpinnerCleared {
			pw.SpinnerCleared = true
			m.pendingWrites[k.ID] = pw
			changed = true
		}
	}
	if changed {
		m.updateListDelegate()
	}
}

// expirePluginActions drops plugin actions pending longer than
// pluginActionExpiry. Unlike bd writes they never settle on a reload, so the
// spinner tick checks them.
func (m *Model) expirePluginActions(now time.Time) {
	changed := false
	for id, pw := range m.pendingWrites {
		if pw.Kind != writePluginAction || now.Sub(pw.StartedAt) < pluginActionExpiry {
			continue
		}
		delete(m.pendingWrites, id)
		changed = true
		m.setFailure(fmt.Sprintf("%s on %s: no result after %ds", pw.label(), id, int(pluginActionExpiry.Seconds())))
	}
	if changed {
		m.updateListDelegate()
	}
}
