package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

type fakeInvoke struct {
	action plugin.Action
	id     string
	view   string
}

// fakePluginActions offers the same actions on every bead and records
// invocations.
type fakePluginActions struct {
	actions []plugin.Action
	result  plugin.ActionResult
	calls   []fakeInvoke
}

func (f *fakePluginActions) Actions(*model.Issue) []plugin.Action { return f.actions }

func (f *fakePluginActions) Invoke(_ context.Context, a plugin.Action, issue *model.Issue, view string) plugin.ActionResult {
	f.calls = append(f.calls, fakeInvoke{action: a, id: issue.ID, view: view})
	return f.result
}

var (
	dispatchAction = plugin.Action{Plugin: "example", ID: "dispatch", Label: "Dispatch", Key: "D"}
	stopAction     = plugin.Action{Plugin: "example", ID: "stop", Label: "Stop"}
)

func newPluginActionModel(t *testing.T, fake *fakePluginActions) Model {
	t.Helper()
	m := newSizedModel(t, pluginTestIssues(), 120, 40)
	m.pluginActions = fake
	return m
}

func keyRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func sendMsg(m Model, msg tea.Msg) (Model, tea.Cmd) {
	u, cmd := m.Update(msg)
	return u.(Model), cmd
}

// actionResults drains cmd and returns the plugin action results it produced.
func actionResults(cmd tea.Cmd) []pluginActionResultMsg {
	var out []pluginActionResultMsg
	for _, msg := range drainCmdMsgs(cmd) {
		if r, ok := msg.(pluginActionResultMsg); ok {
			out = append(out, r)
		}
	}
	return out
}

func TestPluginActionKeyInvokes(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)

	m, cmd := sendMsg(m, keyRune('D'))
	pw, ok := m.pendingWrites[id]
	if !ok || pw.Kind != writePluginAction || pw.Field != "Dispatch" {
		t.Fatalf("pending after D = %+v (%v), want a Dispatch plugin action", pw, ok)
	}
	if !m.writeSpinnerActive {
		t.Fatal("spinner not started")
	}
	results := actionResults(cmd)
	if len(fake.calls) != 1 {
		t.Fatalf("invocations = %d, want 1", len(fake.calls))
	}
	if c := fake.calls[0]; c.action.ID != "dispatch" || c.id != id || c.view != "list" {
		t.Fatalf("invocation = %+v", c)
	}
	if len(results) != 1 || results[0].ID != id {
		t.Fatalf("results = %+v", results)
	}

	m, _ = sendMsg(m, results[0])
	if _, ok := m.pendingWrites[id]; ok {
		t.Fatal("result did not clear the pending action")
	}
	if m.statusMsg != "Dispatch done" || m.statusSeverity != SeveritySuccess {
		t.Fatalf("status = %q (%v), want \"Dispatch done\"", m.statusMsg, m.statusSeverity)
	}
}

func TestPluginActionRefusedWhilePending(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)
	m.pendingWrites[id] = pendingWrite{Kind: writePluginAction, Field: "Dispatch", StartedAt: time.Now()}

	m, cmd := sendMsg(m, keyRune('D'))
	if len(actionResults(cmd)) != 0 || len(fake.calls) != 0 {
		t.Fatal("action invoked on a bead with a pending action")
	}
	if want := "Dispatch already running on " + id; m.statusMsg != want {
		t.Fatalf("status = %q, want %q", m.statusMsg, want)
	}
}

func TestPluginActionKeyIgnoredWhileTyping(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)

	m, _ = sendMsg(m, keyRune('/'))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("filter state = %v, want filtering", m.list.FilterState())
	}
	m, _ = sendMsg(m, keyRune('D'))
	if len(m.pendingWrites) != 0 {
		t.Fatal("D while filtering started an action")
	}

	m = newPluginActionModel(t, fake)
	m, _ = sendMsg(m, keyRune(':'))
	if m.activeModal != ModalBQLQuery {
		t.Fatalf("activeModal = %v, want BQL", m.activeModal)
	}
	m, _ = sendMsg(m, keyRune('D'))
	if len(m.pendingWrites) != 0 {
		t.Fatal("D in the BQL modal started an action")
	}
}

func TestPluginActionKeyLosesToBtBinding(t *testing.T) {
	jAction := plugin.Action{Plugin: "example", ID: "jump", Label: "Jump", Key: "j"}
	fake := &fakePluginActions{actions: []plugin.Action{jAction}}
	m := newPluginActionModel(t, fake)
	first := selectedID(m)

	m, cmd := sendMsg(m, keyRune('j'))
	if len(m.pendingWrites) != 0 || len(actionResults(cmd)) != 0 || len(fake.calls) != 0 {
		t.Fatal("plugin action bound to j was invoked")
	}
	if selectedID(m) == first {
		t.Fatal("j did not move the list cursor")
	}

	m, _ = sendMsg(m, keyRune('P'))
	if m.activeModal != ModalPluginPrompt || m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptMenu {
		t.Fatalf("P did not open the action menu (modal %v)", m.activeModal)
	}
	if len(m.pluginPrompt.options) != 1 || m.pluginPrompt.options[0].label != "Jump" {
		t.Fatalf("menu options = %+v", m.pluginPrompt.options)
	}
}

func TestPluginActionMenuInvokes(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction, stopAction}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)

	m, _ = sendMsg(m, keyRune('P'))
	if m.activeModal != ModalPluginPrompt || m.focused != focusPluginPrompt {
		t.Fatalf("P: modal %v focus %v", m.activeModal, m.focused)
	}
	if out := m.renderPluginPrompt(); !strings.Contains(out, "Dispatch") || !strings.Contains(out, "Stop") {
		t.Fatalf("menu render missing actions:\n%s", out)
	}
	m, _ = sendMsg(m, keyRune('j'))
	m, cmd := sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.activeModal != ModalNone || m.focused != focusList || m.pluginPrompt != nil {
		t.Fatalf("menu not closed: modal %v focus %v", m.activeModal, m.focused)
	}
	actionResults(cmd)
	if len(fake.calls) != 1 || fake.calls[0].action.ID != "stop" || fake.calls[0].id != id {
		t.Fatalf("invocations = %+v, want stop on %s", fake.calls, id)
	}
}

func TestPluginActionMenuEmpty(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	id := selectedID(m)
	m, _ = sendMsg(m, keyRune('P'))
	if m.activeModal != ModalNone {
		t.Fatalf("empty menu opened a modal %v", m.activeModal)
	}
	if want := "No plugin actions for " + id; m.statusMsg != want {
		t.Fatalf("status = %q, want %q", m.statusMsg, want)
	}
}

func TestPluginActionMenuEscCloses(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{actions: []plugin.Action{dispatchAction}})
	m, _ = sendMsg(m, keyRune('P'))
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.activeModal != ModalNone || m.focused != focusList || len(m.pendingWrites) != 0 {
		t.Fatalf("esc: modal %v focus %v pending %v", m.activeModal, m.focused, m.pendingWrites)
	}
}

func TestPluginActionDismissQuitsOnlyInPopup(t *testing.T) {
	msg := pluginActionResultMsg{ID: "proj-1", Action: dispatchAction, Result: plugin.ActionResult{Dismiss: true}}

	m := newPluginActionModel(t, &fakePluginActions{})
	_, cmd := sendMsg(m, msg)
	for _, out := range drainCmdMsgs(cmd) {
		if _, ok := out.(tea.QuitMsg); ok {
			t.Fatal("dismiss quit outside popup mode")
		}
	}

	m.SetPopupMode(true)
	_, cmd = sendMsg(m, msg)
	if cmd == nil {
		t.Fatal("dismiss in popup mode returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("dismiss in popup mode did not quit")
	}
}

func TestPluginActionUnknownResult(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}, result: plugin.ActionResult{Unknown: true}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)

	m, cmd := sendMsg(m, keyRune('D'))
	results := actionResults(cmd)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	m, _ = sendMsg(m, results[0])
	if _, ok := m.pendingWrites[id]; ok {
		t.Fatal("unknown result left the action pending")
	}
	if !strings.Contains(m.statusMsg, "result unknown") || m.statusSeverity != SeverityFailure {
		t.Fatalf("status = %q (%v), want a result-unknown failure", m.statusMsg, m.statusSeverity)
	}
}

func TestPluginActionNotRunningResult(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}, result: plugin.ActionResult{NotRunning: true}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)

	m, cmd := sendMsg(m, keyRune('D'))
	results := actionResults(cmd)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	m, _ = sendMsg(m, results[0])
	if _, ok := m.pendingWrites[id]; ok {
		t.Fatal("not-running result left the action pending")
	}
	if want := "example is not running; action not sent"; m.statusMsg != want || m.statusSeverity != SeverityNotice {
		t.Fatalf("status = %q (%v), want notice %q", m.statusMsg, m.statusSeverity, want)
	}
}

func TestPluginActionResultToastTone(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	m, _ = sendMsg(m, pluginActionResultMsg{ID: "proj-1", Action: dispatchAction,
		Result: plugin.ActionResult{Toast: &plugin.Toast{Message: "not ready", Tone: "warn"}}})
	if m.statusMsg != "not ready" || m.statusSeverity != SeverityNotice {
		t.Fatalf("status = %q (%v), want a notice", m.statusMsg, m.statusSeverity)
	}
}

func TestPluginActionStaleResultKeepsNewerPending(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	newer := pendingWrite{Kind: writePluginAction, Field: "Dispatch", StartedAt: time.Now()}
	m.pendingWrites["proj-1"] = newer
	m, _ = sendMsg(m, pluginActionResultMsg{ID: "proj-1", Action: dispatchAction, started: newer.StartedAt.Add(-time.Minute)})
	if _, ok := m.pendingWrites["proj-1"]; !ok {
		t.Fatal("result of an earlier action cleared a newer pending action")
	}
}

func TestPluginActionInBoardShowsSpinner(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)
	m.mode = ViewBoard
	m.focused = focusBoard
	m.refreshBoardAndGraphForCurrentFilter()
	iss := m.board.SelectedIssue()
	if iss == nil {
		t.Fatal("board has no selected bead")
	}

	m, cmd := sendMsg(m, keyRune('D'))
	if m.statusMsg != "" {
		t.Fatalf("status = %q, want no notice", m.statusMsg)
	}
	frame := claimSpinnerFrame(m.writeSpinnerIdx)
	if card := m.board.renderCard(*iss, 40, true, 0, 0); !strings.Contains(card, frame) {
		t.Fatalf("pending card missing spinner %q:\n%s", frame, card)
	}
	results := actionResults(cmd)
	if len(fake.calls) != 1 || fake.calls[0].view != "board" {
		t.Fatalf("invocations = %+v, want one from the board", fake.calls)
	}
	m, _ = sendMsg(m, results[0])
	if card := m.board.renderCard(*iss, 40, true, 0, 0); strings.Contains(card, frame) {
		t.Fatalf("card keeps spinner after the result:\n%s", card)
	}
}

func TestPendingSpinnerInTreeAndEpics(t *testing.T) {
	issue := slotTestIssue()
	ids := map[string]bool{issue.ID: true}
	frame := claimSpinnerFrame(3)

	tr := NewTreeModel(DefaultTheme())
	tr.SetSize(120, 20)
	tr.SetPending(ids, frame)
	if out := tr.renderNode(&IssueTreeNode{Issue: &issue}, false); !strings.Contains(out, frame) {
		t.Fatalf("pending tree node missing spinner: %q", out)
	}
	tr.SetPending(nil, frame)
	if out := tr.renderNode(&IssueTreeNode{Issue: &issue}, false); strings.Contains(out, frame) {
		t.Fatalf("settled tree node keeps spinner: %q", out)
	}

	var e EpicsTreeModel
	e.SetTheme(DefaultTheme())
	e.SetSize(100, 20)
	e.SetPending(ids, frame)
	row := epicTreeRow{kind: rowChild, issue: &issue, lastKid: []bool{false, true}}
	if out := e.renderChildRow(row, false); !strings.Contains(out, frame) {
		t.Fatalf("pending epics child row missing spinner: %q", out)
	}
}

func TestPendingSpinnerKeepsRowWidths(t *testing.T) {
	issue := slotTestIssue()
	ids := map[string]bool{issue.ID: true}
	frame := claimSpinnerFrame(0)
	for width := 16; width <= 60; width++ {
		plain := NewBoardModel([]model.Issue{issue}, DefaultTheme())
		pending := NewBoardModel([]model.Issue{issue}, DefaultTheme())
		pending.SetPending(ids, frame)
		want := plain.renderCard(issue, width, false, 0, 0)
		got := pending.renderCard(issue, width, false, 0, 0)
		if strings.Count(got, "\n") != strings.Count(want, "\n") || lipgloss.Width(got) != lipgloss.Width(want) {
			t.Errorf("card width %d: pending %dx%d, plain %dx%d", width,
				lipgloss.Width(got), strings.Count(got, "\n")+1, lipgloss.Width(want), strings.Count(want, "\n")+1)
		}
	}
	// A title longer than any row, so the spinner can only take title cells.
	issue.Title = strings.Repeat("long title ", 20)
	for _, width := range []int{40, 80, 120} {
		plain := NewTreeModel(DefaultTheme())
		plain.SetSize(width, 20)
		pending := NewTreeModel(DefaultTheme())
		pending.SetSize(width, 20)
		pending.SetPending(ids, frame)
		node := &IssueTreeNode{Issue: &issue}
		if got, want := lipgloss.Width(pending.renderNode(node, false)), lipgloss.Width(plain.renderNode(node, false)); got != want {
			t.Errorf("tree width %d: pending row %d cells, plain %d", width, got, want)
		}
	}
	for _, width := range []int{40, 100} {
		var e EpicsTreeModel
		e.SetTheme(DefaultTheme())
		e.SetSize(width, 20)
		e.SetPending(ids, frame)
		row := epicTreeRow{kind: rowChild, issue: &issue, lastKid: []bool{false, true}}
		if w := lipgloss.Width(e.renderChildRow(row, false)); w > width {
			t.Errorf("epics child row width %d exceeds %d", w, width)
		}
	}
}

func TestStateChangeStopsPluginSpinner(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	m.pendingWrites["proj-1"] = pendingWrite{Kind: writePluginAction, Field: "Dispatch", StartedAt: time.Now()}
	m.pendingWrites["proj-2"] = pendingWrite{Kind: writeClaim, StartedAt: time.Now()}
	m, _ = sendMsg(m, plugin.StateChangedMsg{Beads: []plugin.BeadKey{{DB: "proj", ID: "proj-1"}, {DB: "proj", ID: "proj-2"}}})
	if ids := m.pendingWriteIDs(); ids["proj-1"] {
		t.Fatal("state change did not stop the plugin action's spinner")
	}
	if ids := m.pendingWriteIDs(); !ids["proj-2"] {
		t.Fatal("state change stopped a pending claim's spinner")
	}
}

func TestStateChangeKeepsPluginActionGuard(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)

	m, cmd := sendMsg(m, keyRune('D'))
	results := actionResults(cmd)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	// A plugin's replace push or crash names every bead it had state on.
	m, _ = sendMsg(m, plugin.StateChangedMsg{Beads: []plugin.BeadKey{{DB: "proj", ID: id}}})
	m, cmd = sendMsg(m, keyRune('D'))
	if len(actionResults(cmd)) != 0 || len(fake.calls) != 1 {
		t.Fatalf("second D after a state change dispatched again (calls %d)", len(fake.calls))
	}
	if want := "Dispatch already running on " + id; m.statusMsg != want {
		t.Fatalf("status = %q, want %q", m.statusMsg, want)
	}

	m, _ = sendMsg(m, results[0])
	m, cmd = sendMsg(m, keyRune('D'))
	if len(actionResults(cmd)) != 1 || len(fake.calls) != 2 {
		t.Fatalf("D after the result was refused (calls %d)", len(fake.calls))
	}
}

func confirmPrompt(reply func(any)) plugin.PromptMsg {
	return plugin.PromptMsg{Plugin: "example", Confirm: &plugin.ConfirmParams{Title: "Dispatch", Message: "Start a session?"}, Reply: reply}
}

func selectPrompt(reply func(any)) plugin.PromptMsg {
	return plugin.PromptMsg{Plugin: "example", Select: &plugin.SelectParams{Title: "Agent", Options: []plugin.SelectOption{
		{Value: "a", Label: "Alpha"},
		{Value: "b", Label: "Beta", Description: "second"},
	}}, Reply: reply}
}

// recorder records the answers a prompt receives.
type recorder struct{ answers []any }

func (r *recorder) reply(a any) { r.answers = append(r.answers, a) }

func TestPluginConfirmPrompt(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	m.mode = ViewBoard
	m.focused = focusBoard
	m.refreshBoardAndGraphForCurrentFilter()

	var r recorder
	m, _ = sendMsg(m, confirmPrompt(r.reply))
	if m.activeModal != ModalPluginPrompt || m.focused != focusPluginPrompt {
		t.Fatalf("confirm prompt: modal %v focus %v", m.activeModal, m.focused)
	}
	if out := m.renderPluginPrompt(); !strings.Contains(out, "Start a session?") {
		t.Fatalf("confirm render:\n%s", out)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Start a session?") {
		t.Fatal("confirm prompt not overlaid in View")
	}
	m, _ = sendMsg(m, keyRune('y'))
	if len(r.answers) != 1 || r.answers[0] != true {
		t.Fatalf("answers = %v, want [true]", r.answers)
	}
	if m.activeModal != ModalNone || m.focused != focusBoard {
		t.Fatalf("after y: modal %v focus %v, want board focus", m.activeModal, m.focused)
	}

	r = recorder{}
	m, _ = sendMsg(m, confirmPrompt(r.reply))
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(r.answers) != 1 || r.answers[0] != false {
		t.Fatalf("answers = %v, want [false]", r.answers)
	}
}

func TestPluginConfirmPromptCustomLabels(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	msg := confirmPrompt(func(any) {})
	msg.Confirm.Confirm = "Launch"
	msg.Confirm.Cancel = "Keep waiting"
	m, _ = sendMsg(m, msg)
	out := m.renderPluginPrompt()
	if !strings.Contains(out, "Launch") || !strings.Contains(out, "Keep waiting") {
		t.Fatalf("custom labels missing:\n%s", out)
	}
}

func TestPluginSelectPrompt(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})

	var r recorder
	m, _ = sendMsg(m, selectPrompt(r.reply))
	if out := m.renderPluginPrompt(); !strings.Contains(out, "Beta") || !strings.Contains(out, "second") {
		t.Fatalf("select render:\n%s", out)
	}
	m, _ = sendMsg(m, keyRune('j'))
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(r.answers) != 1 || r.answers[0] != "b" {
		t.Fatalf("answers = %v, want [b]", r.answers)
	}
	if m.activeModal != ModalNone || m.focused != focusList {
		t.Fatalf("after enter: modal %v focus %v", m.activeModal, m.focused)
	}

	r = recorder{}
	m, _ = sendMsg(m, selectPrompt(r.reply))
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(r.answers) != 1 || r.answers[0] != nil {
		t.Fatalf("answers = %v, want [nil]", r.answers)
	}
}

func TestPluginPromptWhileOtherOpen(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	var first, second recorder
	m, _ = sendMsg(m, confirmPrompt(first.reply))
	m, _ = sendMsg(m, selectPrompt(second.reply))
	if len(second.answers) != 1 || second.answers[0] != nil {
		t.Fatalf("second prompt answers = %v, want [nil]", second.answers)
	}
	if len(first.answers) != 0 || m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptConfirm {
		t.Fatal("second prompt disturbed the first")
	}
}

func TestPluginPromptRefusedWhileTyping(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(m Model) Model
	}{
		{"list filter", func(m Model) Model {
			m, _ = sendMsg(m, keyRune('/'))
			if m.list.FilterState() != list.Filtering {
				t.Fatalf("filter state = %v, want filtering", m.list.FilterState())
			}
			return m
		}},
		{"board search", func(m Model) Model {
			m, _ = sendMsg(m, keyRune('b'))
			m, _ = sendMsg(m, keyRune('/'))
			if m.mode != ViewBoard || !m.board.IsSearchMode() {
				t.Fatalf("mode %v search %v, want board search", m.mode, m.board.IsSearchMode())
			}
			return m
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.setup(newPluginActionModel(t, &fakePluginActions{}))
			var r recorder
			m, _ = sendMsg(m, confirmPrompt(r.reply))
			if len(r.answers) != 1 || r.answers[0] != nil {
				t.Fatalf("answers = %v, want [nil]", r.answers)
			}
			if m.activeModal != ModalNone || m.pluginPrompt != nil {
				t.Fatalf("prompt opened while typing: modal %v", m.activeModal)
			}
			// The typed key still reaches the input.
			m, _ = sendMsg(m, keyRune('y'))
			if len(r.answers) != 1 {
				t.Fatalf("y answered the refused prompt: %v", r.answers)
			}
		})
	}
}

func TestPluginPromptCtrlCQuits(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	var r recorder
	m, _ = sendMsg(m, confirmPrompt(r.reply))
	_, cmd := sendMsg(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if len(r.answers) != 1 || r.answers[0] != nil {
		t.Fatalf("answers = %v, want [nil]", r.answers)
	}
	if cmd == nil {
		t.Fatal("ctrl+c returned no cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestPluginPromptClosesWhenStale(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	var r recorder
	done := make(chan struct{})
	msg := confirmPrompt(r.reply)
	msg.Done = done
	m, cmd := sendMsg(m, msg)
	if cmd == nil {
		t.Fatal("prompt with Done returned no watcher")
	}
	close(done)
	stale := cmd()
	m, _ = sendMsg(m, stale)
	if m.activeModal != ModalNone || m.pluginPrompt != nil || m.focused != focusList {
		t.Fatalf("stale prompt still shown: modal %v focus %v", m.activeModal, m.focused)
	}
	if len(r.answers) != 0 {
		t.Fatalf("stale prompt was answered: %v", r.answers)
	}

	// A stale message for a prompt no longer shown leaves the current one.
	m, _ = sendMsg(m, confirmPrompt(r.reply))
	m, _ = sendMsg(m, stale)
	if m.activeModal != ModalPluginPrompt {
		t.Fatal("stale message closed a different prompt")
	}
}

func TestPluginResultClosesEndedPrompt(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	done := make(chan struct{})
	msg := confirmPrompt(func(any) {})
	msg.Done = done
	m, _ = sendMsg(m, msg)
	close(done)
	m, _ = sendMsg(m, pluginActionResultMsg{ID: "proj-1", Action: dispatchAction})
	if m.activeModal != ModalNone || m.pluginPrompt != nil {
		t.Fatal("result of the plugin's action left its ended prompt open")
	}
}

func TestPluginActionNotExpiredWhileInvokeMayRun(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginActionModel(t, fake)
	id := selectedID(m)
	m.pendingWrites[id] = pendingWrite{Kind: writePluginAction, Field: "Dispatch", StartedAt: time.Now().Add(-50 * time.Second)}
	m.writeSpinnerActive = true

	m, _ = sendMsg(m, writeSpinnerTickMsg{})
	if _, ok := m.pendingWrites[id]; !ok {
		t.Fatal("spinner tick expired a 50s-old plugin action")
	}
	m.settlePendingWrites()
	if _, ok := m.pendingWrites[id]; !ok {
		t.Fatal("data reload expired a 50s-old plugin action")
	}

	m, cmd := sendMsg(m, keyRune('D'))
	if len(actionResults(cmd)) != 0 || len(fake.calls) != 0 {
		t.Fatal("second D at 50s dispatched again")
	}
	if want := "Dispatch already running on " + id; m.statusMsg != want {
		t.Fatalf("status = %q, want %q", m.statusMsg, want)
	}
}

func TestPluginActionSpinnerExpiry(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	old := time.Now().Add(-66 * time.Second)
	m.pendingWrites["proj-1"] = pendingWrite{Kind: writePluginAction, Field: "Dispatch", StartedAt: old}
	m.pendingWrites["proj-2"] = pendingWrite{Kind: writeClaim, StartedAt: old}
	m.writeSpinnerActive = true

	m, _ = sendMsg(m, writeSpinnerTickMsg{})
	if _, ok := m.pendingWrites["proj-1"]; ok {
		t.Fatal("expired plugin action still pending")
	}
	if _, ok := m.pendingWrites["proj-2"]; !ok {
		t.Fatal("spinner tick expired a claim (claims settle on reload)")
	}
	if want := "Dispatch on proj-1: no result after 65s"; m.statusMsg != want || m.statusSeverity != SeverityFailure {
		t.Fatalf("status = %q (%v), want %q", m.statusMsg, m.statusSeverity, want)
	}
}

func TestPluginPromptEndLeavesOtherModal(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(stale tea.Msg) tea.Msg
	}{
		{"stale", func(stale tea.Msg) tea.Msg { return stale }},
		{"result", func(tea.Msg) tea.Msg { return pluginActionResultMsg{ID: "proj-1", Action: dispatchAction} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newPluginActionModel(t, &fakePluginActions{})
			done := make(chan struct{})
			msg := confirmPrompt(func(any) {})
			msg.Done = done
			m, cmd := sendMsg(m, msg)
			// An asynchronous handler (e.g. the AGENTS.md check) opens its
			// modal over the prompt.
			m.openModal(ModalAgentPrompt)
			m.focused = focusAgentPrompt
			close(done)
			m, _ = sendMsg(m, tc.end(cmd()))
			if m.activeModal != ModalAgentPrompt || m.focused != focusAgentPrompt {
				t.Fatalf("ended prompt closed another modal: modal %v focus %v", m.activeModal, m.focused)
			}
		})
	}
}

func TestPluginTextSanitized(t *testing.T) {
	const dirty = "\x1b]0;evil\x07\x1b[31mDis\x07patch\x1b[0m"

	fake := &fakePluginActions{actions: []plugin.Action{{Plugin: "example", ID: "dispatch", Label: dirty, Key: "D"}}}
	m := newPluginActionModel(t, fake)
	m, _ = sendMsg(m, keyRune('P'))
	if m.pluginPrompt == nil || m.pluginPrompt.options[0].label != "Dispatch" {
		t.Fatalf("menu label not sanitized: %+v", m.pluginPrompt)
	}

	m = newPluginActionModel(t, fake)
	m, _ = sendMsg(m, plugin.ToastMsg{Plugin: "example", Toast: plugin.Toast{Message: dirty + " done"}})
	if m.statusMsg != "Dispatch done" {
		t.Fatalf("toast = %q, want %q", m.statusMsg, "Dispatch done")
	}

	m, _ = sendMsg(m, plugin.PromptMsg{Plugin: "example", Confirm: &plugin.ConfirmParams{
		Title: dirty, Message: "line one\n" + dirty, Confirm: dirty, Cancel: dirty,
	}, Reply: func(any) {}})
	p := m.pluginPrompt
	if p.title != "Dispatch" || p.message != "line one\nDispatch" || p.confirmLabel != "Dispatch" || p.cancelLabel != "Dispatch" {
		t.Fatalf("confirm prompt not sanitized: %+v", p)
	}
	if out := m.renderPluginPrompt(); strings.Contains(out, "\x07") || strings.Contains(out, "evil") {
		t.Fatalf("confirm render carries plugin control text: %q", out)
	}

	m = newPluginActionModel(t, fake)
	m, _ = sendMsg(m, plugin.PromptMsg{Plugin: "example", Select: &plugin.SelectParams{
		Title: dirty, Options: []plugin.SelectOption{{Value: "a", Label: dirty, Description: dirty}},
	}, Reply: func(any) {}})
	if o := m.pluginPrompt.options[0]; o.label != "Dispatch" || o.description != "Dispatch" || o.value != "a" {
		t.Fatalf("select option not sanitized: %+v", o)
	}
}

func TestPluginSelectLongOptionFitsWidth(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	m, _ = sendMsg(m, plugin.PromptMsg{Plugin: "example", Select: &plugin.SelectParams{
		Title: "Agent", Options: []plugin.SelectOption{{Value: "a", Label: strings.Repeat("x", 300), Description: strings.Repeat("y", 300)}},
	}, Reply: func(any) {}})
	var row string
	for _, line := range strings.Split(ansi.Strip(m.renderPluginPrompt()), "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Fatalf("line width %d exceeds terminal width %d: %q", w, m.width, line)
		}
		if strings.Contains(line, "xxx") {
			row = line
		}
	}
	// The panel clips an over-wide row at its border, hiding the description
	// and the side padding; a truncated option keeps both.
	if !strings.Contains(row, "...") || !strings.Contains(row, "yyy") || !strings.HasSuffix(row, "  │") {
		t.Fatalf("option row not truncated to the panel: %q", row)
	}
}

func TestSelectedIssuePerView(t *testing.T) {
	m := newPluginActionModel(t, &fakePluginActions{})
	if iss := m.selectedIssue(); iss == nil || iss.ID != selectedID(m) {
		t.Fatalf("list selectedIssue = %v", iss)
	}

	m.mode = ViewBoard
	m.focused = focusBoard
	m.refreshBoardAndGraphForCurrentFilter()
	if iss := m.selectedIssue(); iss == nil || iss.ID != m.board.SelectedIssue().ID {
		t.Fatalf("board selectedIssue = %v", iss)
	}

	m.mode = ViewTree
	m.focused = focusTree
	m.tree.Build(m.data.issues)
	if iss := m.selectedIssue(); iss == nil || iss.ID != m.tree.SelectedIssue().ID {
		t.Fatalf("tree selectedIssue = %v", iss)
	}

	e := epicsTestModel(epicsFixture())
	if r, _ := e.epicsTree.cursorRow(); r.kind != rowProjectHeader {
		t.Fatalf("epics cursor starts on %v, want a header", r.kind)
	}
	if iss := e.selectedIssue(); iss != nil {
		t.Fatalf("epics header selectedIssue = %v, want nil", iss.ID)
	}
	e.epicsTree.cursor = 1
	e.epicsTree.expandCursor()
	e.epicsTree.cursor = 2
	r, _ := e.epicsTree.cursorRow()
	if r.kind != rowChild {
		t.Fatalf("epics row 2 kind = %v, want child", r.kind)
	}
	if iss := e.selectedIssue(); iss == nil || iss.ID != r.issue.ID {
		t.Fatalf("epics child selectedIssue = %v, want %s", iss, r.issue.ID)
	}

	m.mode = ViewGraph
	m.focused = focusGraph
	if iss := m.selectedIssue(); iss != nil {
		t.Fatalf("graph selectedIssue = %v, want nil", iss.ID)
	}
}
