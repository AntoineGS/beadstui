package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/cass"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// clipboardText is what the stubbed clipboard last received.
var clipboardText *string

// stubClipboard swaps the system clipboard for an in-memory one.
func stubClipboard(t *testing.T) *string {
	t.Helper()
	var got string
	orig := writeClipboard
	writeClipboard = func(s string) error { got = s; return nil }
	clipboardText = &got
	t.Cleanup(func() { writeClipboard = orig; clipboardText = nil })
	return &got
}

// pressCmd presses ks without running the commands they produce, except the
// action token a completed sequence emits, and returns the command of the
// action itself. Use it for actions whose commands touch the outside world.
func pressCmd(t *testing.T, m Model, ks ...string) (Model, tea.Cmd) {
	t.Helper()
	m.whichKeyOff = true
	var cmd tea.Cmd
	for _, k := range ks {
		updated, c := m.Update(layoutKey(k))
		m, cmd = updated.(Model), c
	}
	if cmd == nil {
		return m, nil
	}
	kp, ok := cmd().(tea.KeyPressMsg)
	if !ok || !keys.IsToken(kp.Text) {
		t.Fatalf("last key emitted %T, want an action token", kp)
	}
	updated, c := m.Update(kp)
	return updated.(Model), c
}

// Default layout tests (bt-i4k.3): every row of the spec's "Direct keys" and
// "Leader groups" tables (docs/design/2026-10-10-bt-i4k-leader-keymap.md),
// driven by real key presses.

// layoutKey turns a key name into the press a terminal sends.
func layoutKey(s string) tea.KeyPressMsg {
	named := map[string]rune{
		"space": tea.KeySpace, "esc": tea.KeyEscape, "enter": tea.KeyEnter,
		"tab": tea.KeyTab, "home": tea.KeyHome, "end": tea.KeyEnd,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "up": tea.KeyUp,
		"down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"f1": tea.KeyF1, "backspace": tea.KeyBackspace,
	}
	if s == "space" {
		return keySpace
	}
	if c, ok := named[s]; ok {
		return tea.KeyPressMsg{Code: c}
	}
	if strings.HasPrefix(s, "ctrl+") {
		return tea.KeyPressMsg{Code: rune(s[len("ctrl+")]), Mod: tea.ModCtrl}
	}
	return runeKey([]rune(s)[0])
}

// pressAll presses each key through Update, replaying the action tokens the
// commands emit, and reports whether any command quit.
func pressAll(t *testing.T, m Model, ks ...string) (Model, bool) {
	t.Helper()
	quit := false
	for _, k := range ks {
		pending := []tea.KeyPressMsg{layoutKey(k)}
		for round := 0; round < 8 && len(pending) > 0; round++ {
			var next []tea.KeyPressMsg
			for _, p := range pending {
				updated, cmd := m.Update(p)
				m = updated.(Model)
				for _, out := range runMsgs(cmd) {
					switch o := out.(type) {
					case tea.QuitMsg:
						quit = true
					case tea.KeyPressMsg:
						if keys.IsToken(o.Text) {
							next = append(next, o)
						}
					}
				}
			}
			pending = next
		}
	}
	return m, quit
}

func press(t *testing.T, m Model, ks ...string) Model {
	t.Helper()
	m, _ = pressAll(t, m, ks...)
	return m
}

func layoutModel(t *testing.T) Model {
	t.Helper()
	now := time.Now()
	issues := []model.Issue{
		{ID: "bt-e", Title: "Epic", Status: model.StatusOpen, IssueType: model.TypeEpic, CreatedAt: now, Labels: []string{"area:data"},
			Description: strings.Repeat("A long description line.\n", 200)},
		{ID: "bt-c", Title: "Child", Status: model.StatusOpen, IssueType: model.TypeTask, CreatedAt: now,
			Labels:       []string{"area:tui"},
			Dependencies: []*model.Dependency{{IssueID: "bt-c", DependsOnID: "bt-e", Type: model.DepParentChild}}},
		{ID: "bt-1", Title: "One", Status: model.StatusOpen, IssueType: model.TypeTask, CreatedAt: now, Labels: []string{"area:tui", "area:cli"}},
		{ID: "bt-2", Title: "Two", Status: model.StatusOpen, IssueType: model.TypeTask, CreatedAt: now, Labels: []string{"area:cli"}},
	}
	m := newSizedModel(t, issues, 140, 40)
	m.tree.SetBeadsDir(t.TempDir())
	if m.mode != ViewList || m.activeModal != ModalNone || m.hasActiveFilters() {
		t.Fatalf("setup: mode=%v modal=%v filters=%v", m.mode, m.activeModal, m.hasActiveFilters())
	}
	return m
}

// layoutState copies the scalar state the layout checks compare, since
// Model's filter and analysis state sit behind pointers a Model copy shares.
type layoutState struct {
	mode         ViewMode
	modal        ModalType
	focused      focus
	filter       string
	sort         SortMode
	recipe       bool
	wisps, hints bool
	fullscreen   fullscreenPane
	ratio        float64
	index        int
}

func snap(m Model) layoutState {
	return layoutState{
		mode: m.mode, modal: m.activeModal, focused: m.focused,
		filter: m.filter.currentFilter, sort: m.filter.sortMode,
		recipe: m.filter.activeRecipe != nil,
		wisps:  m.showWisps, hints: m.ac.showPriorityHints,
		fullscreen: m.fullscreen, ratio: m.splitPaneRatio, index: m.list.Index(),
	}
}

// inView switches m to the view a toggle token opens.
func inView(t *testing.T, m Model, id string) Model {
	t.Helper()
	return runKey(t, m, tokenMsg(id))
}

func TestLayout_LeaderGroups(t *testing.T) {
	type row struct {
		keys  string // space-separated presses after the leader
		setup func(t *testing.T, m Model) Model
		check func(t *testing.T, before layoutState, after Model)
	}
	mode := func(want ViewMode) func(*testing.T, layoutState, Model) {
		return func(t *testing.T, _ layoutState, m Model) {
			if m.mode != want {
				t.Errorf("mode = %v, want %v", m.mode, want)
			}
		}
	}
	modal := func(want ModalType) func(*testing.T, layoutState, Model) {
		return func(t *testing.T, _ layoutState, m Model) {
			if m.activeModal != want {
				t.Errorf("modal = %v, want %v", m.activeModal, want)
			}
		}
	}
	filter := func(want string) func(*testing.T, layoutState, Model) {
		return func(t *testing.T, _ layoutState, m Model) {
			if m.filter.currentFilter != want {
				t.Errorf("currentFilter = %q, want %q", m.filter.currentFilter, want)
			}
		}
	}
	status := func(want string) func(*testing.T, layoutState, Model) {
		return func(t *testing.T, _ layoutState, m Model) {
			if !strings.Contains(m.statusMsg, want) {
				t.Errorf("status = %q, want it to contain %q", m.statusMsg, want)
			}
		}
	}
	split := func(t *testing.T, m Model) Model {
		m.isSplitView = true
		m.focused = focusList
		return m
	}
	single := func(t *testing.T, m Model) Model {
		m.isSplitView = false
		return m
	}

	rows := []row{
		{"v l", func(t *testing.T, m Model) Model { return inView(t, m, "view.board") }, mode(ViewList)},
		{"v b", nil, mode(ViewBoard)},
		{"v g", nil, mode(ViewGraph)},
		{"v t", nil, mode(ViewTree)},
		{"v e", nil, mode(ViewEpics)},
		{"v i", nil, mode(ViewInsights)},
		{"v h", nil, mode(ViewHistory)},
		{"v a", nil, mode(ViewActionable)},
		{"v f", nil, mode(ViewFlowMatrix)},
		{"v L", nil, mode(ViewLabelDashboard)},
		{"v A", nil, mode(ViewAttention)},
		{"v m", nil, mode(ViewMemories)},
		{"v b", func(t *testing.T, m Model) Model { return inView(t, m, "view.board") }, mode(ViewList)},
		{"f o", func(t *testing.T, m Model) Model { m.SetFilter("all"); return m }, filter("open")},
		{"f c", nil, filter("closed")},
		{"f r", nil, filter("ready")},
		{"f s", nil, func(t *testing.T, before layoutState, m Model) {
			if m.filter.currentFilter == before.filter {
				t.Errorf("status filter did not cycle from %q", before.filter)
			}
		}},
		{"f c", func(t *testing.T, m Model) Model { return inView(t, m, "view.board") }, filter("closed")},
		{"f l", nil, modal(ModalLabelPicker)},
		{"f R", nil, modal(ModalRecipePicker)},
		{"f x", func(t *testing.T, m Model) Model { m.SetFilter("closed"); return m }, func(t *testing.T, _ layoutState, m Model) {
			if m.hasActiveFilters() {
				t.Errorf("filters still active: %q", m.filter.currentFilter)
			}
		}},
		{"s s", nil, func(t *testing.T, before layoutState, m Model) {
			if m.filter.sortMode == before.sort {
				t.Error("sort did not cycle")
			}
		}},
		{"s S", nil, func(t *testing.T, before layoutState, m Model) {
			if m.filter.sortMode == before.sort {
				t.Error("sort did not cycle back")
			}
		}},
		{"s t", nil, func(t *testing.T, _ layoutState, m Model) {
			if m.filter.activeRecipe == nil || m.filter.activeRecipe.Name != "triage" {
				t.Errorf("activeRecipe = %v, want triage", m.filter.activeRecipe)
			}
		}},
		{"b m", nil, modal(ModalClaimConfirm)},
		{"b Y", func(t *testing.T, m Model) Model {
			stubClipboard(t)
			m.selectIssueByID("bt-1")
			return m
		}, func(t *testing.T, _ layoutState, m Model) {
			if got := *clipboardText; !strings.Contains(got, "**ID:** bt-1") {
				t.Errorf("clipboard = %q, want the bt-1 markdown", got)
			}
		}},
		{"b o", func(t *testing.T, m Model) Model {
			t.Chdir(t.TempDir()) // no .beads here: the editor is never launched
			return m
		}, status("No .beads")},
		{"b x", func(t *testing.T, m Model) Model {
			t.Chdir(t.TempDir())
			return m
		}, func(t *testing.T, _ layoutState, m Model) {
			if !strings.Contains(m.statusMsg, "Exported 4 issues") {
				t.Errorf("status = %q, want an export", m.statusMsg)
			}
			if files, _ := filepath.Glob("beads_report_*.md"); len(files) != 1 {
				t.Errorf("export files in the temp dir = %v, want one", files)
			}
		}},
		{"b c", func(t *testing.T, m Model) Model {
			t.Setenv("PATH", t.TempDir()) // no cass binary: correlation is never run
			return m
		}, status("cass not available")},
		{"b g", func(t *testing.T, m Model) Model {
			m.historyView = NewHistoryModel(createTestHistoryReport(), m.theme)
			m.historyView.SetSize(m.width, m.height-1)
			m.mode = ViewHistory
			m.focused = focusHistory
			return m
		}, func(t *testing.T, _ layoutState, m Model) {
			if m.mode != ViewGraph || !strings.Contains(m.statusMsg, "Graph view:") {
				t.Errorf("mode=%v status=%q, want the graph on the history bead", m.mode, m.statusMsg)
			}
		}},
		{"x", func(t *testing.T, m Model) Model {
			m.pluginActions = &fakePluginActions{actions: []plugin.Action{dispatchAction}}
			return m
		}, func(t *testing.T, _ layoutState, m Model) {
			if m.activeModal != ModalPluginPrompt || m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptMenu {
				t.Errorf("modal = %v, want the plugin action menu", m.activeModal)
			}
		}},
		{"b E", func(t *testing.T, m Model) Model { m.selectIssueByID("bt-e"); return m }, modal(ModalEpicCard)},
		{"p p", nil, func(t *testing.T, before layoutState, m Model) {
			if m.showWisps == before.wisps {
				t.Error("wisps did not toggle")
			}
		}},
		{"p h", nil, status("multi-project")},
		{"p w", nil, func(t *testing.T, before layoutState, m Model) {
			if m.showWisps == before.wisps {
				t.Error("wisps did not toggle")
			}
		}},
		{"w l", split, func(t *testing.T, _ layoutState, m Model) {
			if m.focused != focusDetail {
				t.Errorf("focus = %v, want detail", m.focused)
			}
		}},
		{"w h", func(t *testing.T, m Model) Model { m = split(t, m); m.focused = focusDetail; return m }, func(t *testing.T, _ layoutState, m Model) {
			if m.focused != focusList {
				t.Errorf("focus = %v, want list", m.focused)
			}
		}},
		{"w <", split, func(t *testing.T, before layoutState, m Model) {
			if m.splitPaneRatio >= before.ratio {
				t.Errorf("ratio %v -> %v, want smaller", before.ratio, m.splitPaneRatio)
			}
		}},
		{"w >", split, func(t *testing.T, before layoutState, m Model) {
			if m.splitPaneRatio <= before.ratio {
				t.Errorf("ratio %v -> %v, want larger", before.ratio, m.splitPaneRatio)
			}
		}},
		{"w 1", single, func(t *testing.T, _ layoutState, m Model) {
			if m.fullscreen != fullscreenIssues {
				t.Errorf("fullscreen = %v, want issues", m.fullscreen)
			}
		}},
		{"w 2", single, func(t *testing.T, _ layoutState, m Model) {
			if m.fullscreen != fullscreenDetails {
				t.Errorf("fullscreen = %v, want details", m.fullscreen)
			}
		}},
		{"w d", nil, func(t *testing.T, _ layoutState, m Model) {
			if !m.showDebugDims {
				t.Error("dimension chip not shown")
			}
		}},
		{"n n", nil, func(t *testing.T, _ layoutState, m Model) {
			if m.activeModal != ModalAlerts || m.activeTab != TabNotifications {
				t.Errorf("modal=%v tab=%v, want notifications", m.activeModal, m.activeTab)
			}
		}},
		{"n a", nil, func(t *testing.T, _ layoutState, m Model) {
			if m.activeModal != ModalAlerts && !strings.Contains(m.statusMsg, "No active alerts") {
				t.Errorf("modal=%v status=%q, want alerts", m.activeModal, m.statusMsg)
			}
		}},
		{"n h", nil, func(t *testing.T, before layoutState, m Model) {
			if m.ac.showPriorityHints == before.hints {
				t.Error("priority hints did not toggle")
			}
		}},
		{"/ h", nil, status("ybrid")},
		{"r", nil, status("Refresh")},
		{"t", nil, func(t *testing.T, _ layoutState, m Model) {
			if m.focused != focusTimeTravelInput {
				t.Errorf("focus = %v, want time travel input", m.focused)
			}
		}},
		{",", nil, modal(ModalSettingsMenu)},
		{"T", nil, modal(ModalTutorial)},
		{"U", nil, status("update")},
	}
	for _, r := range rows {
		t.Run("SPC "+r.keys, func(t *testing.T) {
			t.Chdir(t.TempDir()) // exports and indexes stay out of the package dir
			m := layoutModel(t)
			if r.setup != nil {
				m = r.setup(t, m)
			}
			before := snap(m)
			m = press(t, m, append([]string{"space"}, strings.Fields(r.keys)...)...)
			if m.seq.pending() {
				t.Fatalf("sequence still pending: %+v", m.seq)
			}
			if strings.Contains(m.statusMsg, "is not bound") {
				t.Fatalf("status = %q", m.statusMsg)
			}
			r.check(t, before, m)
		})
	}
}

// TestLayout_LeaderTree checks every leader row of the spec resolves to its
// action ID in its scope, including the rows whose actions reach outside the
// process (editor, export, cass, plugins, self-update, and the search ranker
// cycle, which builds an index on disk).
func TestLayout_LeaderTree(t *testing.T) {
	rows := []struct {
		seq, id string
		mode    ViewMode
	}{
		{"v l", "view.list", ViewList}, {"v b", "view.board", ViewList},
		{"v g", "view.graph", ViewList}, {"v t", "view.tree", ViewList},
		{"v e", "view.epics", ViewList}, {"v i", "view.insights", ViewList},
		{"v h", "view.history", ViewList}, {"v a", "view.actionable", ViewList},
		{"v f", "view.flow", ViewList}, {"v L", "view.labels", ViewList},
		{"v A", "view.attention", ViewList}, {"v m", "view.memories", ViewList},
		{"f o", "filter.open", ViewList}, {"f c", "filter.closed", ViewList},
		{"f r", "filter.ready", ViewList}, {"f s", "filter.cycle_status", ViewList},
		{"f l", "filter.labels", ViewList}, {"f R", "filter.recipes", ViewList},
		{"f x", "filter.clear", ViewList},
		{"s s", "sort.cycle", ViewList}, {"s S", "sort.cycle_reverse", ViewList},
		{"s t", "sort.triage", ViewList},
		{"b m", "bead.claim", ViewList}, {"b Y", "bead.copy_issue", ViewList},
		{"b o", "bead.open_editor", ViewList}, {"b x", "bead.export", ViewList},
		{"b c", "bead.cass", ViewList}, {"b E", "bead.epic_card", ViewList},
		{"b g", "history.jump_to_graph", ViewHistory},
		{"p p", "project.picker", ViewList}, {"p h", "project.home_all", ViewList},
		{"p w", "project.wisps", ViewList},
		{"w h", "window.focus_left", ViewList}, {"w l", "window.focus_right", ViewList},
		{"w <", "window.shrink", ViewList}, {"w >", "window.grow", ViewList},
		{"w 1", "window.fullscreen_list", ViewList}, {"w 2", "window.fullscreen_details", ViewList},
		{"w d", "window.debug_dims", ViewList},
		{"n n", "notify.notifications", ViewList}, {"n a", "notify.alerts", ViewList},
		{"n h", "notify.priority_hints", ViewList},
		{"/ m", "search.cycle_mode", ViewList}, {"/ h", "search.hybrid_preset", ViewList},
		{"x", "plugin.menu", ViewList}, {"r", "app.refresh", ViewList},
		{"t", "app.time_travel", ViewList}, {",", "app.settings", ViewList},
		{"T", "app.tutorial", ViewList}, {"U", "app.self_update", ViewList},
		{"q", "app.quit", ViewList},
		// Global rows stay reachable from other views.
		{"v b", "view.board", ViewBoard}, {"f o", "filter.open", ViewTree},
	}
	for _, r := range rows {
		m := layoutModel(t)
		m.mode = r.mode
		node := m.seqTree(keys.LeaderPrefix)
		for _, k := range strings.Fields(r.seq) {
			node = node.Child(k)
		}
		if node == nil || node.ID != r.id {
			t.Errorf("SPC %s in %v = %+v, want %s", r.seq, r.mode, node, r.id)
		}
	}
	if _, errs := keys.BuildSeqTree(keys.LeaderPrefix, keys.SeqEntries(keys.LeaderPrefix, layoutModel(t).seqMaps()...), keys.LeaderGroups); len(errs) > 0 {
		t.Errorf("leader tree errors: %v", errs)
	}
}

func TestLayout_DirectKeys(t *testing.T) {
	t.Run("j k move", func(t *testing.T) {
		m := press(t, layoutModel(t), "j")
		if m.list.Index() != 1 {
			t.Fatalf("j: index = %d, want 1", m.list.Index())
		}
		if m = press(t, m, "k"); m.list.Index() != 0 {
			t.Errorf("k: index = %d, want 0", m.list.Index())
		}
	})
	t.Run("gg and G", func(t *testing.T) {
		m := press(t, layoutModel(t), "G")
		last := len(m.list.Items()) - 1
		if m.list.Index() != last {
			t.Fatalf("G: index = %d, want %d", m.list.Index(), last)
		}
		if m = press(t, m, "g", "g"); m.list.Index() != 0 {
			t.Errorf("gg: index = %d, want 0", m.list.Index())
		}
		m = press(t, m, "end", "home")
		if m.list.Index() != 0 {
			t.Errorf("end home: index = %d, want 0", m.list.Index())
		}
	})
	t.Run("lone g does nothing", func(t *testing.T) {
		m := press(t, layoutModel(t), "j", "g")
		if m.mode != ViewList || m.list.Index() != 1 {
			t.Errorf("g: mode=%v index=%d, want list at 1", m.mode, m.list.Index())
		}
	})
	t.Run("h l move between list and details", func(t *testing.T) {
		m := layoutModel(t)
		m.isSplitView = true
		if m = press(t, m, "l"); m.focused != focusDetail {
			t.Fatalf("l: focus = %v, want detail", m.focused)
		}
		if m = press(t, m, "h"); m.focused != focusList {
			t.Errorf("h: focus = %v, want list", m.focused)
		}
	})
	t.Run("enter opens details", func(t *testing.T) {
		m := layoutModel(t)
		m.isSplitView = true
		if m = press(t, m, "enter"); m.focused != focusDetail {
			t.Errorf("focus = %v, want detail", m.focused)
		}
	})
	t.Run("tab cycles focus", func(t *testing.T) {
		m := layoutModel(t)
		m.isSplitView = true
		if m = press(t, m, "tab"); m.focused != focusDetail {
			t.Errorf("focus = %v, want detail", m.focused)
		}
	})
	t.Run("slash searches", func(t *testing.T) {
		if m := press(t, layoutModel(t), "/"); m.list.FilterState() != list.Filtering {
			t.Errorf("filter state = %v, want Filtering", m.list.FilterState())
		}
	})
	t.Run("colon opens BQL", func(t *testing.T) {
		if m := press(t, layoutModel(t), ":"); m.activeModal != ModalBQLQuery {
			t.Errorf("modal = %v, want BQL", m.activeModal)
		}
	})
	t.Run("question mark and f1 open help", func(t *testing.T) {
		for _, k := range []string{"?", "f1"} {
			m := press(t, layoutModel(t), k)
			if m.activeModal != ModalHelp {
				t.Errorf("%s: modal = %v, want help", k, m.activeModal)
			}
			if m = press(t, m, k); m.activeModal != ModalNone {
				t.Errorf("%s twice: modal = %v, want closed", k, m.activeModal)
			}
		}
	})
	t.Run("ctrl+c quits", func(t *testing.T) {
		if _, quit := pressAll(t, layoutModel(t), "ctrl+c"); !quit {
			t.Error("ctrl+c did not quit")
		}
	})
	t.Run("ctrl+d pages", func(t *testing.T) {
		m := press(t, layoutModel(t), "ctrl+d")
		if m.list.Index() == 0 {
			t.Error("ctrl+d did not move")
		}
	})
}

func TestLayout_EditFromViews(t *testing.T) {
	for _, view := range []string{"view.board", "view.tree", "view.epics"} {
		t.Run(view, func(t *testing.T) {
			m := inView(t, layoutModel(t), view)
			for i := 0; i < 3 && m.selectedIssue() == nil; i++ {
				m = press(t, m, "j") // past an epics project header
			}
			if m.selectedIssue() == nil {
				t.Fatalf("setup: %s has no selection", view)
			}
			want := m.selectedIssue().ID
			focused := m.focused
			m = press(t, m, "e")
			if m.activeModal != ModalFieldSelect {
				t.Fatalf("modal = %v, want field select", m.activeModal)
			}
			if m.fieldEditTargetID != want {
				t.Errorf("target = %q, want %q", m.fieldEditTargetID, want)
			}
			if m = press(t, m, "esc"); m.focused != focused {
				t.Errorf("after esc focus = %v, want %v", m.focused, focused)
			}
		})
	}
}

func TestLayout_QQuits(t *testing.T) {
	setups := map[string]func(t *testing.T, m Model) Model{
		"list":  func(t *testing.T, m Model) Model { return m },
		"board": func(t *testing.T, m Model) Model { return inView(t, m, "view.board") },
		"details": func(t *testing.T, m Model) Model {
			m.isSplitView = true
			m.focused = focusDetail
			return m
		},
		"alerts": func(t *testing.T, m Model) Model {
			return runKey(t, m, tokenMsg("notify.notifications"))
		},
		"label picker": func(t *testing.T, m Model) Model { return runKey(t, m, tokenMsg("filter.labels")) },
		"help":         func(t *testing.T, m Model) Model { return press(t, m, "?") },
		"leader":       func(t *testing.T, m Model) Model { return press(t, m, "space") },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			m := setup(t, layoutModel(t))
			if _, quit := pressAll(t, m, "q"); !quit {
				t.Errorf("q did not quit (modal=%v focus=%v)", m.activeModal, m.focused)
			}
		})
	}
}

func TestLayout_QTypedInTextInput(t *testing.T) {
	t.Run("list filter", func(t *testing.T) {
		m, quit := pressAll(t, layoutModel(t), "/", "q")
		if quit {
			t.Fatal("q quit while filtering")
		}
		if got := m.list.FilterInput.Value(); got != "q" {
			t.Errorf("filter input = %q, want q", got)
		}
	})
	t.Run("BQL", func(t *testing.T) {
		m, quit := pressAll(t, layoutModel(t), ":", "q")
		if quit {
			t.Fatal("q quit in BQL")
		}
		if m.activeModal != ModalBQLQuery {
			t.Errorf("modal = %v, want BQL still open", m.activeModal)
		}
	})
	t.Run("label picker search", func(t *testing.T) {
		m := runKey(t, layoutModel(t), tokenMsg("filter.labels"))
		if _, quit := pressAll(t, m, "/", "q"); quit {
			t.Fatal("q quit in label picker search")
		}
	})
	t.Run("time travel", func(t *testing.T) {
		m := runKey(t, layoutModel(t), tokenMsg("app.time_travel"))
		if _, quit := pressAll(t, m, "q"); quit {
			t.Fatal("q quit in time travel input")
		}
	})
}

func TestLayout_Esc(t *testing.T) {
	t.Run("base list is a no-op", func(t *testing.T) {
		m := press(t, layoutModel(t), "esc")
		if m.activeModal != ModalNone || m.mode != ViewList {
			t.Errorf("esc at base: modal=%v mode=%v, want nothing", m.activeModal, m.mode)
		}
	})
	t.Run("clears filters", func(t *testing.T) {
		m := layoutModel(t)
		m.SetFilter("closed")
		m = press(t, m, "esc")
		if m.hasActiveFilters() {
			t.Errorf("filters still active: %q", m.filter.currentFilter)
		}
		if m.activeModal != ModalNone {
			t.Errorf("modal = %v, want none", m.activeModal)
		}
	})
	t.Run("leaves tree", func(t *testing.T) {
		m := press(t, inView(t, layoutModel(t), "view.tree"), "esc")
		if m.mode != ViewList || m.focused != focusList {
			t.Errorf("mode=%v focus=%v, want list", m.mode, m.focused)
		}
	})
	t.Run("q does not reach quit confirm", func(t *testing.T) {
		m, _ := pressAll(t, layoutModel(t), "q")
		if m.activeModal == ModalQuitConfirm {
			t.Error("q opened the quit confirm")
		}
	})
}

func TestLayout_OldDirectKeysInert(t *testing.T) {
	for _, k := range []string{"b", "i", "a", "f", "E", "u", "w", "W", "o", "c", "r", "s", "m", "x", "p", "!", "1", "T", "'", "[", "]", "`", "H", "C", "O", "V", "U", "R", "S", "t", "#", "2", "3", "<", ">", "F", "d"} {
		t.Run(k, func(t *testing.T) {
			t.Chdir(t.TempDir()) // any stray export or index lands here
			m := layoutModel(t)
			before := snap(m)
			m = press(t, m, k)
			if after := snap(m); after != before {
				t.Errorf("state changed:\n before %+v\n after  %+v", before, after)
			}
			if m.statusMsg != "" {
				t.Errorf("status = %q", m.statusMsg)
			}
		})
	}
}

func TestLayout_AlertsTabKeysDoNotClose(t *testing.T) {
	m := runKey(t, layoutModel(t), tokenMsg("notify.notifications"))
	if m.activeModal != ModalAlerts || m.activeTab != TabNotifications {
		t.Fatalf("setup: modal=%v tab=%v", m.activeModal, m.activeTab)
	}
	m = press(t, m, "1")
	if m.activeModal != ModalAlerts || m.activeTab != TabNotifications {
		t.Errorf("1 on its own tab: modal=%v tab=%v, want unchanged", m.activeModal, m.activeTab)
	}
	m = press(t, m, "!")
	if m.activeModal != ModalAlerts || m.activeTab != TabAlerts {
		t.Errorf("!: modal=%v tab=%v, want alerts tab", m.activeModal, m.activeTab)
	}
	m = press(t, m, "!")
	if m.activeModal != ModalAlerts {
		t.Errorf("! on its own tab closed the modal")
	}
}

func TestLayout_LabelDashboardHealthOnI(t *testing.T) {
	m := inView(t, layoutModel(t), "view.labels")
	if len(m.labelDashboard.labels) == 0 {
		t.Fatal("setup: no labels")
	}
	if m = press(t, m, "i"); m.activeModal != ModalLabelHealthDetail {
		t.Errorf("i: modal = %v, want label health", m.activeModal)
	}
}

// TestLayout_SearchRankerCycle checks SPC / m by the state it sets and the
// index build it emits; the build command is not run.
func TestLayout_SearchRankerCycle(t *testing.T) {
	t.Chdir(t.TempDir())
	m := layoutModel(t)
	if m.currentSearchMode() != searchModeFuzzy {
		t.Fatalf("setup: search mode = %v, want fuzzy", m.currentSearchMode())
	}
	m, cmd := pressCmd(t, m, "space", "/", "m")
	if m.currentSearchMode() != searchModeHybrid {
		t.Errorf("search mode = %v, want hybrid", m.currentSearchMode())
	}
	if cmd == nil || !m.semanticIndexBuilding {
		t.Errorf("cmd=%v building=%v, want an index build emitted", cmd, m.semanticIndexBuilding)
	}
}

// TestLayout_HistoryFileTreePassesGlobalActions: file-tree focus keeps j/k
// and esc local, but completed leader sequences reach the Global actions.
func TestLayout_HistoryFileTreePassesGlobalActions(t *testing.T) {
	setup := func(t *testing.T) Model {
		m := layoutModel(t)
		m.historyView = NewHistoryModel(createTestHistoryReport(), m.theme)
		m.historyView.SetSize(m.width, m.height-1)
		m.mode = ViewHistory
		m.focused = focusHistory
		m.historyView.SetFileTreeFocus(true)
		return m
	}
	t.Run("SPC f c", func(t *testing.T) {
		m := press(t, setup(t), "space", "f", "c")
		if m.filter.currentFilter != "closed" || m.mode != ViewHistory {
			t.Errorf("filter=%q mode=%v, want closed in history", m.filter.currentFilter, m.mode)
		}
	})
	t.Run("SPC f x", func(t *testing.T) {
		m := setup(t)
		m.SetFilter("closed")
		m = press(t, m, "space", "f", "x")
		if m.hasActiveFilters() {
			t.Errorf("filters still active: %q", m.filter.currentFilter)
		}
	})
	t.Run("SPC v l", func(t *testing.T) {
		if m := press(t, setup(t), "space", "v", "l"); m.mode != ViewList || m.focused != focusList {
			t.Errorf("mode=%v focus=%v, want the list", m.mode, m.focused)
		}
	})
	t.Run("esc stays local", func(t *testing.T) {
		m := press(t, setup(t), "esc")
		if m.mode != ViewHistory || m.historyView.FileTreeHasFocus() {
			t.Errorf("mode=%v treeFocus=%v, want history with the tree blurred", m.mode, m.historyView.FileTreeHasFocus())
		}
	})
}

func TestLayout_PendingLeaderOwnsQ(t *testing.T) {
	m, quit := pressAll(t, layoutModel(t), "space", "v", "q")
	if quit {
		t.Fatal("q inside SPC v quit")
	}
	if !strings.Contains(m.statusMsg, "SPC v q is not bound") {
		t.Errorf("status = %q, want SPC v q is not bound", m.statusMsg)
	}
	if _, quit := pressAll(t, layoutModel(t), "space", "q"); !quit {
		t.Error("SPC q did not quit")
	}
}

// TestLayout_ForceQuitEverywhere: ctrl+c quits from every modal and text
// state; the plugin prompt answers its plugin first.
func TestLayout_ForceQuitEverywhere(t *testing.T) {
	setups := map[string]func(t *testing.T, m Model) Model{
		"agent prompt": func(t *testing.T, m Model) Model {
			m.agentPromptModal = NewAgentPromptModal("/test/AGENTS.md", "AGENTS.md", m.theme)
			m.agentPromptModal.keys = m.keys.AgentPrompt
			m.openModal(ModalAgentPrompt)
			return m
		},
		"cass": func(t *testing.T, m Model) Model {
			m.cassModal = NewCassSessionModal("bt-1", cass.CorrelationResult{BeadID: "bt-1"}, m.theme)
			m.cassModal.keys = m.keys.Cass
			m.openModal(ModalCassSession)
			return m
		},
		"epic card": func(t *testing.T, m Model) Model { m.openEpicCard("bt-e"); return m },
		"update": func(t *testing.T, m Model) Model {
			m.updateModal = NewUpdateModal("v9.9.9", "https://example", m.theme)
			m.updateModal.keys = m.keys.Update
			m.openModal(ModalUpdate)
			return m
		},
		"alerts":   func(t *testing.T, m Model) Model { return runKey(t, m, tokenMsg("notify.notifications")) },
		"help":     func(t *testing.T, m Model) Model { return press(t, m, "?") },
		"tutorial": func(t *testing.T, m Model) Model { return runKey(t, m, tokenMsg("app.tutorial")) },
		"settings": func(t *testing.T, m Model) Model { return runKey(t, m, tokenMsg("app.settings")) },
		"bql":      func(t *testing.T, m Model) Model { return press(t, m, ":") },
		"field input": func(t *testing.T, m Model) Model {
			if m = press(t, m, "e", "t"); m.activeModal != ModalFieldInput {
				t.Fatalf("setup: modal = %v, want field input", m.activeModal)
			}
			return m
		},
		"longform": func(t *testing.T, m Model) Model {
			if m = press(t, m, "e", "d"); m.activeModal != ModalLongformEdit {
				t.Fatalf("setup: modal = %v, want longform", m.activeModal)
			}
			return m
		},
		"time travel": func(t *testing.T, m Model) Model { return runKey(t, m, tokenMsg("app.time_travel")) },
		"list filter": func(t *testing.T, m Model) Model { return press(t, m, "/", "x") },
		"label search": func(t *testing.T, m Model) Model {
			return press(t, runKey(t, m, tokenMsg("filter.labels")), "/")
		},
		"board search": func(t *testing.T, m Model) Model { return press(t, inView(t, m, "view.board"), "/") },
		"history search": func(t *testing.T, m Model) Model {
			m.historyView = NewHistoryModel(createTestHistoryReport(), m.theme)
			m.historyView.SetSize(m.width, m.height-1)
			m.mode = ViewHistory
			m.focused = focusHistory
			return press(t, m, "/")
		},
		"memories search": func(t *testing.T, m Model) Model {
			m.memories = NewMemoriesModel(m.theme)
			m.mode = ViewMemories
			m.focused = focusMemories
			return press(t, m, "/")
		},
		"pending leader": func(t *testing.T, m Model) Model { return press(t, m, "space", "v") },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			m := setup(t, layoutModel(t))
			if m.activeModal == ModalNone && !m.inTextInput() && !m.seq.pending() {
				t.Fatalf("setup reached no modal, text state or sequence (focus %v)", m.focused)
			}
			if _, quit := pressAll(t, m, "ctrl+c"); !quit {
				t.Errorf("ctrl+c did not quit (modal=%v focus=%v)", m.activeModal, m.focused)
			}
		})
	}
	t.Run("plugin prompt answers first", func(t *testing.T) {
		m := layoutModel(t)
		var r recorder
		updated, _ := m.Update(selectPrompt(r.reply))
		m = updated.(Model)
		if m.activeModal != ModalPluginPrompt {
			t.Fatalf("setup: modal = %v", m.activeModal)
		}
		if _, quit := pressAll(t, m, "ctrl+c"); !quit {
			t.Error("ctrl+c did not quit the plugin prompt")
		}
		if len(r.answers) != 1 || r.answers[0] != nil {
			t.Errorf("answers = %v, want [nil] before quitting", r.answers)
		}
	})
}

// TestLayout_TopBottomInCursorViews checks gg/home and G/end in every cursor
// view of the spec: list, board, tree, epics, flow matrix, label dashboard.
func TestLayout_TopBottomInCursorViews(t *testing.T) {
	views := []struct {
		name   string
		enter  func(t *testing.T, m Model) Model
		cursor func(m Model) int
	}{
		{"list", func(t *testing.T, m Model) Model { return m }, func(m Model) int { return m.list.Index() }},
		{"board", func(t *testing.T, m Model) Model { return inView(t, m, "view.board") }, func(m Model) int {
			if iss := m.board.SelectedIssue(); iss != nil {
				return int(iss.ID[len(iss.ID)-1])
			}
			return -1
		}},
		{"tree", func(t *testing.T, m Model) Model { return inView(t, m, "view.tree") }, func(m Model) int {
			if iss := m.tree.SelectedIssue(); iss != nil {
				return int(iss.ID[len(iss.ID)-1])
			}
			return -1
		}},
		{"epics", func(t *testing.T, m Model) Model { return inView(t, m, "view.epics") }, func(m Model) int { return m.epicsTree.cursor }},
		{"flow", func(t *testing.T, m Model) Model { return inView(t, m, "view.flow") }, func(m Model) int { return m.flowMatrix.cursor }},
		{"labels", func(t *testing.T, m Model) Model { return inView(t, m, "view.labels") }, func(m Model) int { return m.labelDashboard.cursor }},
	}
	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			m := v.enter(t, layoutModel(t))
			top := v.cursor(m)
			m = press(t, m, "G")
			bottom := v.cursor(m)
			if bottom == top {
				t.Fatalf("G did not move (cursor %d)", top)
			}
			if m = press(t, m, "g", "g"); v.cursor(m) != top {
				t.Errorf("gg: cursor %d, want %d", v.cursor(m), top)
			}
			if m = press(t, m, "end"); v.cursor(m) != bottom {
				t.Errorf("end: cursor %d, want %d", v.cursor(m), bottom)
			}
			if m = press(t, m, "home"); v.cursor(m) != top {
				t.Errorf("home: cursor %d, want %d", v.cursor(m), top)
			}
		})
	}
}

func TestLayout_Paging(t *testing.T) {
	var many []model.Issue
	for i := 0; i < 80; i++ {
		many = append(many, model.Issue{ID: "p-" + string(rune('A'+i/26)) + string(rune('a'+i%26)), Title: "Paged", Status: model.StatusOpen, CreatedAt: time.Now()})
	}
	for _, keysDown := range [][2]string{{"ctrl+d", "ctrl+u"}, {"pgdown", "pgup"}} {
		t.Run(keysDown[0], func(t *testing.T) {
			m := newSizedModel(t, many, 140, 40)
			m = press(t, m, keysDown[0])
			down := m.list.Index()
			if down == 0 {
				t.Fatalf("%s did not move", keysDown[0])
			}
			if m = press(t, m, keysDown[1]); m.list.Index() >= down {
				t.Errorf("%s: index %d, want below %d", keysDown[1], m.list.Index(), down)
			}
		})
	}
	t.Run("details , and .", func(t *testing.T) {
		m := layoutModel(t)
		m.isSplitView = true
		m.selectIssueByID("bt-e")
		m.updateViewportContent()
		m = press(t, m, ".")
		if m.viewport.YOffset() == 0 {
			t.Fatal(". did not page the details")
		}
		if m = press(t, m, ","); m.viewport.YOffset() != 0 {
			t.Errorf(", left offset %d, want 0", m.viewport.YOffset())
		}
	})
}

func TestLayout_CopyID(t *testing.T) {
	got := stubClipboard(t)
	m := layoutModel(t)
	m.selectIssueByID("bt-2")
	press(t, m, "y")
	if *got != "bt-2" {
		t.Errorf("list y copied %q, want bt-2", *got)
	}
	b := inView(t, layoutModel(t), "view.board")
	want := b.board.SelectedIssue().ID
	press(t, b, "y")
	if *got != want {
		t.Errorf("board y copied %q, want %s", *got, want)
	}
}

func TestLayout_MatchNav(t *testing.T) {
	m := inView(t, layoutModel(t), "view.board")
	m = press(t, m, "/", "b", "t", "-", "enter")
	if m.board.IsSearchMode() || m.board.SearchMatchCount() < 2 {
		t.Fatalf("setup: search=%v matches=%d", m.board.IsSearchMode(), m.board.SearchMatchCount())
	}
	first := m.board.SelectedIssue().ID
	m = press(t, m, "n")
	second := m.board.SelectedIssue().ID
	if second == first {
		t.Fatal("n did not move to the next match")
	}
	if m = press(t, m, "N"); m.board.SelectedIssue().ID != first {
		t.Errorf("N: selected %s, want %s", m.board.SelectedIssue().ID, first)
	}
}
