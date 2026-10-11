package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

var (
	keySpace     = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyEsc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	keyBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func runeKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func runKeys(t *testing.T, m Model, msgs ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, msg := range msgs {
		m = runKey(t, m, msg)
	}
	return m
}

// leaderModel binds the test-only sequence <leader>vb to view.board; the
// default keymap has no leader keys yet.
func leaderModel(t *testing.T) Model {
	t.Helper()
	m := actionTestModel(t)
	b := &m.keys.Global.Board
	b.SetKeys(append(b.Keys(), "<leader>vb")...)
	return m
}

func TestLeader_SequenceFiresAction(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace, runeKey('v'), runeKey('b'))
	if m.mode != ViewBoard {
		t.Errorf("mode = %v, want ViewBoard", m.mode)
	}
	if m.seq.prefix != "" {
		t.Errorf("sequence still pending: %+v", m.seq)
	}
}

func TestLeader_EscCancels(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace, runeKey('v'), keyEsc)
	if m.mode != ViewList {
		t.Errorf("mode = %v, want ViewList", m.mode)
	}
	if m.seq.prefix != "" {
		t.Errorf("sequence still pending: %+v", m.seq)
	}
}

func TestLeader_BackspacePopsStep(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace, runeKey('v'), keyBackspace)
	if m.seq.prefix != "<leader>" || len(m.seq.steps) != 0 {
		t.Fatalf("after backspace seq = %+v, want pending at the leader root", m.seq)
	}
	m = runKeys(t, m, runeKey('b'))
	if m.mode != ViewList {
		t.Errorf("mode = %v, want ViewList (SPC b is not bound)", m.mode)
	}
	if !strings.Contains(m.statusMsg, "SPC b is not bound") {
		t.Errorf("status = %q, want SPC b is not bound", m.statusMsg)
	}
}

func TestLeader_BackspaceAtRootCancels(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace, keyBackspace)
	if m.seq.prefix != "" {
		t.Errorf("seq = %+v, want idle", m.seq)
	}
}

func TestLeader_MissDoesNotFallThrough(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace, runeKey('?'))
	if m.activeModal == ModalHelp {
		t.Error("? inside a leader sequence opened help")
	}
	if !strings.Contains(m.statusMsg, "SPC ? is not bound") {
		t.Errorf("status = %q, want SPC ? is not bound", m.statusMsg)
	}
	if m.seq.prefix != "" {
		t.Errorf("seq = %+v, want idle", m.seq)
	}
}

func TestLeader_InactiveWhileFiltering(t *testing.T) {
	m := runKeys(t, leaderModel(t), runeKey('/'))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("setup: filter state = %v, want Filtering", m.list.FilterState())
	}
	m = runKeys(t, m, keySpace)
	if m.seq.prefix != "" {
		t.Errorf("leader started while filtering: %+v", m.seq)
	}
	if got := m.list.FilterInput.Value(); got != " " {
		t.Errorf("filter input = %q, want a typed space", got)
	}
}

func TestLeader_InactiveInModal(t *testing.T) {
	issues := []model.Issue{
		{ID: "bv-1", Title: "One", Status: model.StatusOpen, Labels: []string{"area:tui"}},
		{ID: "bv-2", Title: "Two", Status: model.StatusOpen, Labels: []string{"area:product"}},
	}
	m := newSizedModel(t, issues, 140, 40)
	b := &m.keys.Global.Board
	b.SetKeys(append(b.Keys(), "<leader>vb")...)
	m.labelPicker = NewLabelPickerModel(
		[]string{"area:tui", "area:product"},
		map[string]int{"area:tui": 1, "area:product": 1},
		m.theme,
	)
	m.openModal(ModalLabelPicker)
	m.focused = focusLabelPicker
	before := len(m.labelPicker.SelectedLabels())
	m = runKeys(t, m, keySpace)
	if got := len(m.labelPicker.SelectedLabels()); got == before {
		t.Errorf("space did not toggle the picker (selected %d -> %d)", before, got)
	}
	if m.seq.prefix != "" {
		t.Errorf("leader started in a modal: %+v", m.seq)
	}
}

func TestWhichKey_Debounced(t *testing.T) {
	m := leaderModel(t)
	m.whichKeyDelay = time.Millisecond
	m, cmd, handled := m.routeSequence(keySpace)
	if !handled || cmd == nil {
		t.Fatalf("leader: handled=%v cmd=%v, want handled with a tick", handled, cmd)
	}
	if m.seq.showPanel {
		t.Fatal("panel shown before the debounce")
	}
	tick, ok := cmd().(whichKeyMsg)
	if !ok || tick.gen != m.seq.gen {
		t.Fatalf("tick = %#v, want whichKeyMsg{gen: %d}", tick, m.seq.gen)
	}

	stale, _ := m.Update(whichKeyMsg{gen: tick.gen - 1})
	if stale.(Model).seq.showPanel {
		t.Error("stale tick showed the panel")
	}
	shown, _ := m.Update(tick)
	if !shown.(Model).seq.showPanel {
		t.Error("matching tick did not show the panel")
	}
}

func TestWhichKey_BackspaceReschedules(t *testing.T) {
	m := leaderModel(t)
	m.whichKeyDelay = time.Millisecond
	m, _, _ = m.routeSequence(keySpace)
	m, _, _ = m.routeSequence(runeKey('v'))
	m, cmd, handled := m.routeSequence(keyBackspace)
	if !handled || cmd == nil {
		t.Fatalf("backspace: handled=%v cmd=%v, want handled with a tick", handled, cmd)
	}
	tick, ok := cmd().(whichKeyMsg)
	if !ok || tick.gen != m.seq.gen {
		t.Fatalf("tick = %#v, want whichKeyMsg{gen: %d}", tick, m.seq.gen)
	}
	shown, _ := m.Update(tick)
	if !shown.(Model).seq.showPanel {
		t.Error("tick after backspace did not show the panel")
	}
}

func TestLeader_CtrlCQuitsMidSequence(t *testing.T) {
	m := runKeys(t, leaderModel(t), keySpace)
	if !m.seq.pending() {
		t.Fatal("setup: leader not pending")
	}
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	quit := false
	for _, msg := range runMsgs(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			quit = true
		}
	}
	if !quit {
		t.Error("ctrl+c mid-sequence did not quit")
	}
	if updated.(Model).seq.pending() {
		t.Error("sequence still pending after ctrl+c")
	}
}

func TestLeader_OpenModalResetsSequence(t *testing.T) {
	m := leaderModel(t)
	m.whichKeyDelay = 0
	m = runKeys(t, m, keySpace)
	if !m.seq.pending() || !m.seq.showPanel {
		t.Fatalf("setup: seq = %+v, want pending with panel", m.seq)
	}
	m.openModal(ModalHelp)
	if m.seq.pending() || m.seq.showPanel {
		t.Errorf("seq = %+v after openModal, want idle", m.seq)
	}
}

func TestSeqMaps_History(t *testing.T) {
	m := actionTestModel(t)
	m.mode = ViewHistory
	maps := m.seqMaps()
	if len(maps) != 2 {
		t.Fatalf("seqMaps = %d maps, want Global + history", len(maps))
	}
	if _, ok := maps[1].(keys.HistoryNormalKeys); !ok {
		t.Errorf("history map = %T, want keys.HistoryNormalKeys", maps[1])
	}
	if _, ok := m.viewSpecificKeyMap().(keys.HistoryNormalKeys); !ok {
		t.Errorf("viewSpecificKeyMap = %T, want keys.HistoryNormalKeys", m.viewSpecificKeyMap())
	}
}

func TestWhichKey_ZeroDelayShowsAtOnce(t *testing.T) {
	m := leaderModel(t)
	m.whichKeyDelay = 0
	m, cmd, _ := m.routeSequence(keySpace)
	if cmd != nil || !m.seq.showPanel {
		t.Errorf("cmd=%v showPanel=%v, want no tick and panel shown", cmd, m.seq.showPanel)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "+view") {
		t.Errorf("which-key panel missing the v group:\n%s", view)
	}
	if !strings.Contains(view, "SPC…") {
		t.Errorf("footer missing the pending sequence:\n%s", view)
	}
}

func TestWhichKey_OffNeverTicks(t *testing.T) {
	m := leaderModel(t)
	m.whichKeyOff = true
	m, cmd, handled := m.routeSequence(keySpace)
	if !handled || cmd != nil || m.seq.showPanel {
		t.Errorf("handled=%v cmd=%v showPanel=%v, want handled, no tick, no panel", handled, cmd, m.seq.showPanel)
	}
}

func boardSeqModel(t *testing.T) Model {
	t.Helper()
	m := runKey(t, actionTestModel(t), tokenMsg("view.board"))
	if m.mode != ViewBoard {
		t.Fatalf("setup: mode = %v, want ViewBoard", m.mode)
	}
	return m
}

func TestBoard_GGJumpsToFirstCard(t *testing.T) {
	m := boardSeqModel(t)
	first := m.board.SelectedIssue()
	m = runKeys(t, m, runeKey('j'))
	if m.board.SelectedIssue() == first {
		t.Fatal("setup: j did not move")
	}
	m = runKeys(t, m, runeKey('g'), runeKey('g'))
	if got := m.board.SelectedIssue(); got == nil || got.ID != first.ID {
		t.Errorf("after gg selected = %v, want %s", got, first.ID)
	}
}

func TestBoard_GThenJMovesDown(t *testing.T) {
	m := boardSeqModel(t)
	first := m.board.SelectedIssue()
	m = runKeys(t, m, runeKey('g'), runeKey('j'))
	if got := m.board.SelectedIssue(); got == nil || got.ID == first.ID {
		t.Errorf("g j left selection on %v, want it moved down", got)
	}
	if m.seq.prefix != "" {
		t.Errorf("seq = %+v, want idle", m.seq)
	}
}

func TestLeader_InertWithoutLeaderKeys(t *testing.T) {
	issues := []model.Issue{
		{ID: "parent", Title: "Parent", Status: model.StatusOpen, IssueType: model.TypeEpic},
		{ID: "child", Title: "Child", Status: model.StatusOpen, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{{DependsOnID: "parent", Type: model.DepParentChild}}},
	}
	m := newSizedModel(t, issues, 140, 40)
	m.tree.SetBeadsDir(t.TempDir())
	m = runKey(t, m, tokenMsg("view.tree"))
	if m.mode != ViewTree || m.focused != focusTree {
		t.Fatalf("setup: mode=%v focus=%v, want tree", m.mode, m.focused)
	}
	node := m.tree.SelectedNode()
	if node == nil || len(node.Children) == 0 {
		t.Fatalf("setup: selected node %+v has no children", node)
	}
	was := node.Expanded
	m = runKeys(t, m, keySpace)
	if m.tree.SelectedNode().Expanded == was {
		t.Error("space did not toggle the tree node")
	}
	if m.seq.prefix != "" {
		t.Errorf("seq = %+v, want idle", m.seq)
	}
}
