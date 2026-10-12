package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// runMsgs executes cmd and returns the messages it produces, flattening
// tea.BatchMsg one level. Commands that block (timers, listeners) are dropped
// after a short wait.
func runMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	exec := func(c tea.Cmd) tea.Msg {
		if c == nil {
			return nil
		}
		ch := make(chan tea.Msg, 1)
		go func() { ch <- c() }()
		select {
		case msg := <-ch:
			return msg
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}
	msg := exec(cmd)
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, exec(c))
		}
		return out
	}
	return []tea.Msg{msg}
}

// runKey feeds msg through Update and then drains any action-token key presses
// the resulting commands emit (one level of tea.Batch is flattened).
func runKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model {
	t.Helper()
	pending := []tea.KeyPressMsg{msg}
	for round := 0; round < 8 && len(pending) > 0; round++ {
		var next []tea.KeyPressMsg
		for _, p := range pending {
			updated, cmd := m.Update(p)
			var ok bool
			if m, ok = updated.(Model); !ok {
				t.Fatalf("Update returned %T, want Model", updated)
			}
			for _, out := range runMsgs(cmd) {
				if kp, ok := out.(tea.KeyPressMsg); ok && keys.IsToken(kp.Text) {
					next = append(next, kp)
				}
			}
		}
		pending = next
	}
	return m
}

func actionTestModel(t *testing.T) Model {
	t.Helper()
	issues := []model.Issue{
		{ID: "bt-1", Title: "One", Status: model.StatusOpen, CreatedAt: time.Now()},
		{ID: "bt-2", Title: "Two", Status: model.StatusClosed, CreatedAt: time.Now()},
		{ID: "bt-3", Title: "Three", Status: model.StatusOpen, CreatedAt: time.Now()},
	}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	m.SetFilter("all")
	return m
}

func tokenMsg(id string) tea.KeyPressMsg { return tea.KeyPressMsg{Text: keys.Token(id)} }

func TestActionCmdEmitsToken(t *testing.T) {
	msg, ok := actionCmd("filter.open")().(tea.KeyPressMsg)
	if !ok || msg.String() != "@filter.open" {
		t.Fatalf("actionCmd emitted %#v, want KeyPressMsg with @filter.open", msg)
	}
}

func TestActionToken_FilterOpen(t *testing.T) {
	m := runKey(t, actionTestModel(t), tokenMsg("filter.open"))
	if m.filter.currentFilter != "open" {
		t.Errorf("currentFilter = %q, want open", m.filter.currentFilter)
	}
}

func TestSetKeyConfigPromotesFilterOpen(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"filter.open": {"y"}}}, nil)
	if slices.Contains(m.keys.ListNormal.CopyID.Keys(), "y") {
		t.Fatal("ListNormal.CopyID retained y after filter.open claimed it")
	}
	m = runKey(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.filter.currentFilter != "open" {
		t.Errorf("currentFilter = %q, want open", m.filter.currentFilter)
	}
}

func TestSetKeyConfigReportsShadowsAndErrors(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"filter.open":    {"y"},
		"unknown.action": {"x"},
	}}, nil)
	if m.statusMsg != "keys: 3 shadowed, 1 config error - see ?" {
		t.Fatalf("status = %q, want shadow and config-error counts", m.statusMsg)
	}
}

func TestSetKeyConfigReportsLeaderReservationAndAliasError(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{
		Leader: "y",
		Bind: map[string]keys.BindValue{
			"view.board": {"<leader>v", "<leader>vb"},
		},
	}, nil)
	if m.statusMsg != "keys: 3 shadowed, 1 config error - see ?" {
		t.Fatalf("status = %q, want leader shadow and alias config-error counts", m.statusMsg)
	}
	if slices.Contains(m.keys.ListNormal.CopyID.Keys(), "y") {
		t.Fatalf("ListNormal.CopyID retained reserved leader y: %v", m.keys.ListNormal.CopyID.Keys())
	}

	m, _ = sendMsg(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	plain := ansi.Strip(m.renderHelpOverlay())
	for _, want := range []string{
		"SHADOWED / ERRORS",
		"bead.copy_id shadowed by keys.leader",
		"keys.bind.view.board",
		"<leader>v",
		"<leader>vb",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("help overlay missing %q:\n%s", want, plain)
		}
	}
}

func TestSetKeyConfigWithoutProblemsDoesNotWarn(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{}, nil)
	if strings.HasPrefix(m.statusMsg, "keys:") {
		t.Fatalf("status = %q, want no key-config warning", m.statusMsg)
	}
}

func TestHelpRendersConfigShadowAndError(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"filter.open":    {"y"},
		"unknown.action": {"x"},
	}}, nil)
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: '?', Text: "?"})
	rendered := m.renderHelpOverlay()
	plain := ansi.Strip(rendered)
	for _, want := range []string{"SHADOWED / ERRORS", "bead.copy_id", "filter.open", "unknown.action"} {
		if !strings.Contains(plain, want) {
			t.Errorf("help overlay missing %q:\n%s", want, plain)
		}
	}
	if !strings.Contains(rendered, ";9m") {
		t.Errorf("help overlay did not render the shadowed binding with strike-through:\n%q", rendered)
	}
}

func TestSetKeyConfigCountsJoinedLoadErrorsIndividually(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{}, errors.Join(errors.New("bad first bind"), errors.New("bad second bind")))
	if len(m.keyConfigErrs) != 2 {
		t.Errorf("retained %d errors, want 2: %v", len(m.keyConfigErrs), m.keyConfigErrs)
	}
	if m.statusMsg != "keys: 0 shadowed, 2 config errors - see ?" {
		t.Errorf("status = %q, want 2 config errors", m.statusMsg)
	}
}

func TestSetKeyConfigUpdatesBubblesNavigationAndFilterBindings(t *testing.T) {
	m := actionTestModel(t)
	m.list.Select(1)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"nav.up":     {"z"},
		"app.search": {";"},
	}}, nil)

	// The old vim navigation key must no longer be owned by Bubbles' copied map.
	m = runKey(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.list.Index() != 1 {
		t.Fatalf("old nav.up key moved selection to %d", m.list.Index())
	}
	m = runKey(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})
	if m.list.Index() != 0 {
		t.Errorf("configured nav.up key left selection at %d, want 0", m.list.Index())
	}

	m = runKey(t, m, tea.KeyPressMsg{Code: ';', Text: ";"})
	if m.list.FilterState() != list.Filtering {
		t.Errorf("configured app.search did not open the list filter: state=%v", m.list.FilterState())
	}
}

func TestBubblesSyncUsesManuallyResolvedShadowBinding(t *testing.T) {
	m := actionTestModel(t)
	m.keys.ListNormal.Up.SetKeys(keys.Token("nav.up"))
	m.syncBubblesKeyBindings(keys.Config{}, nil, []keys.Shadow{{ID: "nav.up", Key: "k", By: "app.search"}})

	got := m.list.KeyMap.CursorUp.Keys()
	if slices.Contains(got, "k") {
		t.Errorf("Bubbles CursorUp retained shadowed key k: %v", got)
	}
	if !slices.Contains(got, keys.Token("nav.up")) {
		t.Errorf("Bubbles CursorUp lost nav.up action token: %v", got)
	}
}

func TestSetKeyConfigSyncsBubblesFromResolvedShadows(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"app.search": {"k"}}}, nil)

	got := m.list.KeyMap.CursorUp.Keys()
	if slices.Contains(got, "k") {
		t.Errorf("Bubbles CursorUp retained key removed by Apply: %v", got)
	}
	if !slices.Contains(got, keys.Token("nav.up")) {
		t.Errorf("Bubbles CursorUp lacks nav.up action token after shadow sync: %v", got)
	}
}

func pagedListModel(t *testing.T) Model {
	t.Helper()
	m := actionTestModel(t)
	items := make([]list.Item, 80)
	for i := range items {
		items[i] = IssueItem{Issue: model.Issue{ID: fmt.Sprintf("bt-%d", i+1), Title: "Issue"}}
	}
	m.list.SetItems(items)
	m.height = 36
	m.list.SetSize(60, 16)
	return m
}

func TestListPagingBindingDispatchesOnce(t *testing.T) {
	cases := []struct {
		name   string
		config keys.Config
		key    tea.KeyPressMsg
	}{
		{name: "default", key: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
		{name: "configured", config: keys.Config{Bind: map[string]keys.BindValue{"nav.page_down": {"ctrl+f"}}}, key: tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := pagedListModel(t)
			m.SetKeyConfig(tc.config, nil)
			m = runKey(t, m, tc.key)
			if got, want := m.list.Index(), m.height/3; got != want {
				t.Fatalf("list index after one page down = %d, want %d", got, want)
			}
		})
	}
}

func filteredPagedListModel(t *testing.T) Model {
	t.Helper()
	m := pagedListModel(t)
	items := make([]list.Item, 80)
	for i := range items {
		title := fmt.Sprintf("Other issue %d", i+1)
		if i < 3 {
			title = fmt.Sprintf("Match issue %d", i+1)
		}
		items[i] = IssueItem{Issue: model.Issue{
			ID:    fmt.Sprintf("bt-%03d", i+1),
			Title: title,
		}}
	}
	m.list.SetItems(items)
	m.list.SetFilterText("match")
	if got := len(m.list.VisibleItems()); got != 3 {
		t.Fatalf("setup: visible matches = %d, want 3", got)
	}
	m.list.SetSize(60, 1)
	return m
}

func TestListNavigationClampsToVisibleFilteredItems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config keys.Config
		msg    tea.KeyPressMsg
		start  int
		want   int
	}{
		{name: "pgdown", msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: 2},
		{name: "ctrl+d", msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, want: 2},
		{name: "G", msg: keyRune('G'), want: 2},
		{name: "end", msg: tea.KeyPressMsg{Code: tea.KeyEnd}, want: 2},
		{name: "pgup", msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, start: 2, want: 0},
		{name: "ctrl+u", msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, start: 2, want: 0},
		{name: "home", msg: tea.KeyPressMsg{Code: tea.KeyHome}, start: 2, want: 0},
		{name: "configured page down", config: keys.Config{Bind: map[string]keys.BindValue{
			"nav.page_down": {"ctrl+f"},
		}}, msg: tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}, want: 2},
		{name: "page-down token", msg: tokenMsg("nav.page_down"), want: 2},
		{name: "configured bottom", config: keys.Config{Bind: map[string]keys.BindValue{
			"nav.bottom": {"ctrl+e"},
		}}, msg: tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}, want: 2},
		{name: "bottom token", msg: tokenMsg("nav.bottom"), want: 2},
		{name: "configured page up", config: keys.Config{Bind: map[string]keys.BindValue{
			"nav.page_up": {"ctrl+b"},
		}}, msg: tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}, start: 2, want: 0},
		{name: "page-up token", msg: tokenMsg("nav.page_up"), start: 2, want: 0},
		{name: "configured top", config: keys.Config{Bind: map[string]keys.BindValue{
			"nav.top": {"ctrl+a"},
		}}, msg: tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, start: 2, want: 0},
		{name: "top token", msg: tokenMsg("nav.top"), start: 2, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := filteredPagedListModel(t)
			m.SetKeyConfig(tc.config, nil)
			m.list.Select(tc.start)
			m = runKey(t, m, tc.msg)
			if got := m.list.Index(); got != tc.want {
				t.Fatalf("list index = %d, want %d", got, tc.want)
			}
			got, ok := m.list.SelectedItem().(IssueItem)
			if !ok {
				t.Fatal("selected item is nil after navigation")
			}
			want, ok := m.list.VisibleItems()[tc.want].(IssueItem)
			if !ok {
				t.Fatalf("visible item %d has type %T, want IssueItem", tc.want, m.list.VisibleItems()[tc.want])
			}
			if got.Issue.ID != want.Issue.ID {
				t.Fatalf("selected issue = %q, want visible issue %q", got.Issue.ID, want.Issue.ID)
			}
			if got, want := m.list.Paginator.Page, tc.want/m.list.Paginator.PerPage; got != want {
				t.Fatalf("paginator page = %d, want %d for selected index %d", got, want, tc.want)
			}
			if m.list.Paginator.Page < 0 || m.list.Paginator.Page >= m.list.Paginator.TotalPages {
				t.Fatalf("paginator page %d outside %d total pages", m.list.Paginator.Page, m.list.Paginator.TotalPages)
			}
		})
	}
}

func TestListSearchTypesVimLettersByDefaultAndUsesConfiguredNavigation(t *testing.T) {
	t.Run("default letters and email", func(t *testing.T) {
		m := actionTestModel(t)
		if strings.Contains(m.keys.ListSearch.Up.Help().Key, "k") || strings.Contains(m.keys.ListSearch.Down.Help().Key, "j") {
			t.Fatalf("list-search navigation help advertises typeable letters: up=%q down=%q",
				m.keys.ListSearch.Up.Help().Key, m.keys.ListSearch.Down.Help().Key)
		}
		m = runKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		for _, r := range "jka@b" {
			m = runKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if got, want := m.list.FilterInput.Value(), "jka@b"; got != want {
			t.Fatalf("list search query = %q, want %q", got, want)
		}
		if got := m.list.FilterState(); got != list.Filtering {
			t.Fatalf("list filter state = %v, want Filtering", got)
		}
	})

	t.Run("default arrows navigate while filtering", func(t *testing.T) {
		m := actionTestModel(t)
		m = runKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		m.list.SetFilterText("open")
		m.list.SetFilterState(list.Filtering)
		m.list.Select(0)

		m = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		if got := m.list.Index(); got != 1 {
			t.Fatalf("down arrow selected %d, want 1", got)
		}
		if got := m.list.FilterInput.Value(); got != "open" {
			t.Fatalf("down arrow changed query to %q", got)
		}
		if got := m.list.FilterState(); got != list.Filtering {
			t.Fatalf("down arrow changed filter state to %v", got)
		}

		m = runKey(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
		if got := m.list.Index(); got != 0 {
			t.Fatalf("up arrow selected %d, want 0", got)
		}
		if got := m.list.FilterInput.Value(); got != "open" {
			t.Fatalf("up arrow changed query to %q", got)
		}
	})

	t.Run("configured physical and action navigation", func(t *testing.T) {
		m := actionTestModel(t)
		m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"nav.down": {"z"}}}, nil)
		m = runKey(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		m.list.SetFilterText("open")
		m.list.SetFilterState(list.Filtering)
		m.list.Select(0)
		m = runKey(t, m, tea.KeyPressMsg{Code: 'z', Text: "z"})
		if got := m.list.Index(); got != 1 {
			t.Fatalf("configured search navigation selected %d, want 1", got)
		}
		if got := m.list.FilterState(); got != list.Filtering {
			t.Fatalf("configured navigation changed filter state to %v", got)
		}
		if got := m.list.FilterValue(); got != "open" {
			t.Fatalf("configured navigation changed query to %q", got)
		}
		if got := m.keys.ListSearch.Down.Help().Key; got != "z" {
			t.Fatalf("list-search down help key = %q, want z", got)
		}
		m.list.Select(0)
		m = runKey(t, m, tokenMsg("nav.down"))
		if got := m.list.Index(); got != 1 {
			t.Fatalf("nav.down token selected %d, want 1", got)
		}
		if got := m.list.FilterValue(); got != "open" {
			t.Fatalf("nav.down token changed query to %q", got)
		}
	})
}

func TestDetailsViewportUsesResolvedNavigation(t *testing.T) {
	newDetailsModel := func(t *testing.T) Model {
		t.Helper()
		m := actionTestModel(t)
		m.focused = focusDetail
		m.showDetails = true
		m.viewport.SetContent(strings.Repeat("detail line\n", 100))
		m.viewport.SetHeight(8)
		m.viewport.GotoTop()
		return m
	}

	t.Run("configured nav down replaces j", func(t *testing.T) {
		m := newDetailsModel(t)
		m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"nav.down": {"z"}}}, nil)
		m = runKey(t, m, keyRune('z'))
		if got := m.viewport.YOffset(); got == 0 {
			t.Fatal("configured nav.down key z did not scroll details")
		}
		m.viewport.GotoTop()
		m = runKey(t, m, keyRune('j'))
		if got := m.viewport.YOffset(); got != 0 {
			t.Fatalf("old nav.down key j scrolled details to %d", got)
		}
	})

	t.Run("default vim page aliases are not viewport controls", func(t *testing.T) {
		for _, r := range "bfud" {
			t.Run(string(r), func(t *testing.T) {
				m := newDetailsModel(t)
				m.viewport.SetYOffset(10)
				m = runKey(t, m, keyRune(r))
				if got := m.viewport.YOffset(); got != 10 {
					t.Fatalf("%q changed detail offset from 10 to %d", r, got)
				}
			})
		}
	})

	t.Run("top and bottom use the action bindings", func(t *testing.T) {
		m := newDetailsModel(t)
		m.viewport.SetYOffset(10)
		m = runKey(t, m, keyRune('g'))
		m = runKey(t, m, keyRune('g'))
		if got := m.viewport.YOffset(); got != 0 {
			t.Fatalf("gg offset = %d, want top", got)
		}
		m = runKey(t, m, keyRune('G'))
		if got := m.viewport.YOffset(); got == 0 {
			t.Fatal("G did not move to the bottom")
		}
	})
}

func TestExitActionTokensRestoreListModeAndFocus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    string
		mode  ViewMode
		focus focus
		setup func(*Model)
	}{
		{name: "tree", id: "tree.back", mode: ViewTree, focus: focusTree},
		{name: "flow matrix", id: "flow.close", mode: ViewFlowMatrix, focus: focusFlowMatrix},
		{name: "insights", id: "insights.exit", mode: ViewInsights, focus: focusInsights},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := actionTestModel(t)
			m.mode = tc.mode
			m.focused = tc.focus
			if tc.setup != nil {
				tc.setup(&m)
			}
			m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{tc.id: {"<leader>a"}}}, nil)
			m = runKey(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
			m = runKey(t, m, keyRune('a'))
			if m.mode != ViewList || m.focused != focusList {
				t.Fatalf("after %s token: mode=%v focus=%v, want list/list", tc.id, m.mode, m.focused)
			}
		})
	}

	t.Run("flow close backs out one level", func(t *testing.T) {
		m := actionTestModel(t)
		m.mode = ViewFlowMatrix
		m.focused = focusFlowMatrix
		m.flowMatrix.showDrilldown = true
		m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"flow.close": {"<leader>a"}}}, nil)
		m = runKey(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
		m = runKey(t, m, keyRune('a'))
		if m.mode != ViewFlowMatrix || m.focused != focusFlowMatrix || m.flowMatrix.showDrilldown {
			t.Fatalf("flow drilldown close: mode=%v focus=%v drilldown=%v", m.mode, m.focused, m.flowMatrix.showDrilldown)
		}
	})
}

func TestUniversalPagingBindingsMoveAllCursorViews(t *testing.T) {
	issues := make([]model.Issue, 80)
	for i := range issues {
		issues[i] = model.Issue{
			ID:        fmt.Sprintf("bt-%03d", i+1),
			Title:     fmt.Sprintf("Issue %d", i+1),
			Status:    model.StatusOpen,
			CreatedAt: time.Now(),
		}
	}

	newModel := func(t *testing.T) Model {
		t.Helper()
		m := NewModel(issues, nil, "", nil, nil)
		m.width = 120
		m.height = 36
		m.ready = true
		m.list.SetSize(60, 16)
		return m
	}
	type cursorView struct {
		name   string
		setup  func(*Model)
		cursor func(Model) int
	}
	views := []cursorView{
		{
			name: "list",
			setup: func(m *Model) {
				m.mode, m.focused = ViewList, focusList
			},
			cursor: func(m Model) int { return m.list.Index() },
		},
		{
			name: "board",
			setup: func(m *Model) {
				m.mode, m.focused = ViewBoard, focusBoard
			},
			cursor: func(m Model) int {
				col, ok := m.board.focusedLane()
				if !ok {
					return 0
				}
				return m.board.selectedRow[col]
			},
		},
		{
			name: "tree",
			setup: func(m *Model) {
				m.mode, m.focused = ViewTree, focusTree
				m.rebuildTreeForCurrentFilter()
				m.tree.SetSize(120, 30)
			},
			cursor: func(m Model) int { return m.tree.cursor },
		},
		{
			name: "epics",
			setup: func(m *Model) {
				epics := make([]model.Issue, len(issues))
				for i := range epics {
					epics[i] = issues[i]
					epics[i].IssueType = model.TypeEpic
				}
				m.mode, m.focused = ViewEpics, focusEpics
				m.epicsTree.Build(epics, EpicsAll, time.Now())
				m.epicsTree.SetSize(120, 30)
			},
			cursor: func(m Model) int { return m.epicsTree.cursor },
		},
		{
			name: "flow matrix",
			setup: func(m *Model) {
				m.mode, m.focused = ViewFlowMatrix, focusFlowMatrix
				m.flowMatrix.labelStats = make([]labelFlowStats, len(issues))
				m.flowMatrix.SetSize(120, 30)
			},
			cursor: func(m Model) int { return m.flowMatrix.cursor },
		},
		{
			name: "label dashboard",
			setup: func(m *Model) {
				labels := make([]analysis.LabelHealth, len(issues))
				for i := range labels {
					labels[i].Label = fmt.Sprintf("label-%03d", i+1)
				}
				m.mode, m.focused = ViewLabelDashboard, focusLabelDashboard
				m.labelDashboard.SetSize(120, 30)
				m.labelDashboard.SetData(labels)
			},
			cursor: func(m Model) int { return m.labelDashboard.cursor },
		},
	}

	for _, view := range views {
		for _, tc := range []struct {
			name string
			msg  tea.KeyPressMsg
		}{
			{name: "ctrl+d/u", msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}},
			{name: "pgdown/up", msg: tea.KeyPressMsg{Code: tea.KeyPgDown}},
		} {
			t.Run(view.name+"/"+tc.name, func(t *testing.T) {
				m := newModel(t)
				view.setup(&m)
				start := view.cursor(m)
				m = runKey(t, m, tc.msg)
				down := view.cursor(m)
				if down <= start {
					t.Fatalf("page down cursor = %d, want > %d", down, start)
				}
				upMsg := tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
				if tc.name == "pgdown/up" {
					upMsg = tea.KeyPressMsg{Code: tea.KeyPgUp}
				}
				m = runKey(t, m, upMsg)
				if up := view.cursor(m); up >= down {
					t.Fatalf("page up cursor = %d, want < %d", up, down)
				}
			})
		}
	}
}

func TestFlowMatrixPagingUsesActiveCursor(t *testing.T) {
	newModel := func(t *testing.T, drilldown bool) Model {
		t.Helper()
		m := actionTestModel(t)
		m.mode, m.focused = ViewFlowMatrix, focusFlowMatrix
		m.flowMatrix.labelStats = make([]labelFlowStats, 80)
		m.flowMatrix.drilldownIssues = make([]flowPairEndpoint, 80)
		m.flowMatrix.SetSize(120, 30)
		m.flowMatrix.cursor = 20
		m.flowMatrix.drilldownCursor = 20
		m.flowMatrix.showDrilldown = drilldown
		return m
	}
	for _, tc := range []struct {
		name      string
		msg       tea.KeyPressMsg
		direction int
	}{
		{name: "ctrl+d", msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, direction: 1},
		{name: "pgdown", msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, direction: 1},
		{name: "page-down token", msg: tokenMsg("nav.page_down"), direction: 1},
		{name: "ctrl+u", msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, direction: -1},
		{name: "pgup", msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, direction: -1},
		{name: "page-up token", msg: tokenMsg("nav.page_up"), direction: -1},
	} {
		for _, drilldown := range []bool{false, true} {
			context := "parent"
			if drilldown {
				context = "drilldown"
			}
			t.Run(context+"/"+tc.name, func(t *testing.T) {
				m := newModel(t, drilldown)
				parentStart := m.flowMatrix.cursor
				drilldownStart := m.flowMatrix.drilldownCursor
				m = runKey(t, m, tc.msg)
				if drilldown {
					got := m.flowMatrix.drilldownCursor
					if tc.direction > 0 && got <= drilldownStart || tc.direction < 0 && got >= drilldownStart {
						t.Fatalf("drilldown cursor = %d, want movement direction %d from %d", got, tc.direction, drilldownStart)
					}
					if got < 0 || got >= len(m.flowMatrix.drilldownIssues) {
						t.Fatalf("drilldown cursor = %d, outside %d issues", got, len(m.flowMatrix.drilldownIssues))
					}
					if got := m.flowMatrix.cursor; got != parentStart {
						t.Fatalf("parent cursor = %d, want unchanged %d", got, parentStart)
					}
					return
				}
				got := m.flowMatrix.cursor
				if tc.direction > 0 && got <= parentStart || tc.direction < 0 && got >= parentStart {
					t.Fatalf("parent cursor = %d, want movement direction %d from %d", got, tc.direction, parentStart)
				}
				if got < 0 || got >= len(m.flowMatrix.labelStats) {
					t.Fatalf("parent cursor = %d, outside %d labels", got, len(m.flowMatrix.labelStats))
				}
				if got := m.flowMatrix.drilldownCursor; got != drilldownStart {
					t.Fatalf("drilldown cursor = %d, want unchanged %d", got, drilldownStart)
				}
			})
		}
	}
}

func TestAttentionOrdinalActionTokenRetainsConfiguredTarget(t *testing.T) {
	m := actionTestModel(t)
	m.mode = ViewAttention
	m.attentionCache.Labels = []analysis.LabelAttentionScore{
		{Label: "one"},
		{Label: "two"},
		{Label: "three"},
	}
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"attention.pick_label_3": {"<leader>z"},
	}}, nil)

	m = runKey(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	m = runKey(t, m, keyRune('z'))
	if got := m.filter.labelFilter; got != "three" {
		t.Fatalf("label filter = %q, want ordinal 3 target three", got)
	}
}

func TestTutorialOrdinalActionTokenRetainsConfiguredTarget(t *testing.T) {
	m := actionTestModel(t)
	m.activeModal = ModalTutorial
	m.focused = focusTutorial
	m.tutorialModel = NewTutorialModel(m.theme)
	m.tutorialModel.pages = []TutorialPage{
		{ID: "one", Title: "One"},
		{ID: "two", Title: "Two"},
		{ID: "three", Title: "Three"},
	}
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"tutorial.jump_page_3": {"x"},
	}}, nil)

	m = runKey(t, m, tokenMsg("tutorial.jump_page_3"))
	if got := m.tutorialModel.CurrentPageID(); got != "three" {
		t.Fatalf("tutorial page after ordinal token = %q, want three", got)
	}
	m.tutorialModel.JumpToPage(0)
	m = runKey(t, m, keyRune('x'))
	if got := m.tutorialModel.CurrentPageID(); got != "three" {
		t.Fatalf("tutorial page = %q, want ordinal 3 target three", got)
	}
}

func TestOrdinalAggregateActionIDsAreUnknown(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{
		"attention.pick_label": {"x"},
		"tutorial.jump_page":   {"y"},
	}}, nil)
	if len(m.keyConfigErrs) != 2 {
		t.Fatalf("config errors = %v, want unknown-action errors for both aggregate IDs", m.keyConfigErrs)
	}
}

func TestBubblesDoesNotOwnApplicationPagingBindings(t *testing.T) {
	m := actionTestModel(t)
	for name, binding := range map[string]key.Binding{
		"previous page": m.list.KeyMap.PrevPage,
		"next page":     m.list.KeyMap.NextPage,
		"top":           m.list.KeyMap.GoToStart,
		"bottom":        m.list.KeyMap.GoToEnd,
	} {
		if got := binding.Keys(); len(got) != 0 {
			t.Errorf("Bubbles %s keys = %v, want application-owned binding", name, got)
		}
	}
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"nav.page_down": {"ctrl+f"}}}, nil)
	if got := m.list.KeyMap.NextPage.Keys(); len(got) != 0 {
		t.Errorf("Bubbles NextPage keys after configured app paging = %v, want none", got)
	}
	filterKeys := m.list.KeyMap.Filter.Keys()
	if !slices.Contains(filterKeys, "/") || !slices.Contains(filterKeys, keys.Token("app.search")) {
		t.Errorf("Bubbles Filter keys = %v, want default slash and app.search token", filterKeys)
	}
}

func TestNewModelBubblesFilterIncludesSearchActionToken(t *testing.T) {
	m := NewModel([]model.Issue{{ID: "bt-1", Title: "One", Status: model.StatusOpen}}, nil, "", nil, nil)
	filterKeys := m.list.KeyMap.Filter.Keys()
	if !slices.Contains(filterKeys, "/") || !slices.Contains(filterKeys, keys.Token("app.search")) {
		t.Fatalf("default Bubbles Filter keys = %v, want slash and app.search token", filterKeys)
	}
}

func TestNewModelBubblesFilterActionsRetainDefaultsAndTokens(t *testing.T) {
	m := NewModel([]model.Issue{{ID: "bt-1", Title: "One", Status: model.StatusOpen}}, nil, "", nil, nil)
	for _, tc := range []struct {
		name    string
		binding key.Binding
		aliases []string
		token   string
	}{
		{
			name:    "apply",
			binding: m.list.KeyMap.AcceptWhileFiltering,
			aliases: []string{"enter", "tab", "shift+tab", "ctrl+k", "up", "ctrl+j", "down"},
			token:   keys.Token("list_search.apply"),
		},
		{
			name:    "cancel",
			binding: m.list.KeyMap.CancelWhileFiltering,
			aliases: []string{"esc"},
			token:   keys.Token("list_search.cancel"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.binding.Keys()
			for _, want := range tc.aliases {
				if !slices.Contains(got, want) {
					t.Errorf("Bubbles keys = %v, lost default alias %q", got, want)
				}
			}
			if !slices.Contains(got, tc.token) {
				t.Errorf("Bubbles keys = %v, missing action token %q", got, tc.token)
			}
		})
	}
}

func filteredListSearchModel(t *testing.T) Model {
	t.Helper()
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{}, nil)
	m.list.SetFilterText("open")
	m.list.SetFilterState(list.Filtering)
	if got := len(m.list.VisibleItems()); got != 2 {
		t.Fatalf("setup: visible matches = %d, want 2", got)
	}
	return m
}

func TestDefaultListSearchApplyTokenPreservesNonemptyQuery(t *testing.T) {
	m := filteredListSearchModel(t)
	wantQuery := m.list.FilterValue()

	m = runKey(t, m, tokenMsg("list_search.apply"))
	if got := m.list.FilterState(); got != list.FilterApplied {
		t.Errorf("filter state = %v, want FilterApplied", got)
	}
	if got := m.list.FilterValue(); got != wantQuery {
		t.Errorf("filter query = %q, want preserved query %q", got, wantQuery)
	}
}

func TestDefaultListSearchCancelTokenClearsNonemptyQuery(t *testing.T) {
	m := filteredListSearchModel(t)

	m = runKey(t, m, tokenMsg("list_search.cancel"))
	if got := m.list.FilterState(); got != list.Unfiltered {
		t.Errorf("filter state = %v, want Unfiltered", got)
	}
	if got := m.list.FilterValue(); got != "" {
		t.Errorf("filter query = %q, want cancel to clear it without inserting the token", got)
	}
}

func TestDefaultListSearchNavigationTokensMoveResultsWithoutEditingQuery(t *testing.T) {
	m := filteredListSearchModel(t)
	wantQuery := m.list.FilterValue()

	m = runKey(t, m, tokenMsg("nav.down"))
	if got := m.list.Index(); got != 1 {
		t.Errorf("nav.down result index = %d, want 1", got)
	}
	if got := m.list.FilterValue(); got != wantQuery {
		t.Errorf("nav.down changed filter query to %q, want %q", got, wantQuery)
	}

	m = runKey(t, m, tokenMsg("nav.up"))
	if got := m.list.Index(); got != 0 {
		t.Errorf("nav.up result index = %d, want 0", got)
	}
	if got := m.list.FilterValue(); got != wantQuery {
		t.Errorf("nav.up changed filter query to %q, want %q", got, wantQuery)
	}
}

func TestActionToken_NavDown(t *testing.T) {
	m := actionTestModel(t)
	if m.list.Index() != 0 {
		t.Fatalf("setup: index = %d, want 0", m.list.Index())
	}
	m = runKey(t, m, tokenMsg("nav.down"))
	if m.list.Index() != 1 {
		t.Errorf("index = %d, want 1", m.list.Index())
	}
}

func TestActionToken_ViewBoard(t *testing.T) {
	m := runKey(t, actionTestModel(t), tokenMsg("view.board"))
	if m.mode != ViewBoard {
		t.Errorf("mode = %v, want ViewBoard", m.mode)
	}
}

func TestActionToken_SortCycleDoesNotFilter(t *testing.T) {
	m := runKey(t, actionTestModel(t), tokenMsg("sort.cycle"))
	if m.list.FilterState() == list.Filtering {
		t.Error("sort.cycle put the list into Filtering")
	}
}

func TestUnmatchedTokenCannotEnterTextInputs(t *testing.T) {
	t.Run("BQL", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalBQLQuery)
		m.bqlQuery.input.SetValue("status:open")
		updated, _ := m.Update(tokenMsg("app.refresh"))
		m = updated.(Model)
		if got := m.bqlQuery.Value(); got != "status:open" {
			t.Errorf("BQL input = %q, want unchanged query", got)
		}
	})

	t.Run("global quit token", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalBQLQuery)
		updated, cmd := m.Update(tokenMsg("app.force_quit"))
		m = updated.(Model)
		for _, produced := range runMsgs(cmd) {
			if _, quits := produced.(tea.QuitMsg); quits {
				t.Fatal("unowned app.force_quit token quit from a text-input context")
			}
		}
		if m.activeModal != ModalBQLQuery {
			t.Errorf("unowned app.force_quit token changed active modal to %v", m.activeModal)
		}
	})

	t.Run("time travel", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalTimeTravelInput)
		m.focused = focusTimeTravelInput
		m.timeTravelInput.SetValue("HEAD~3")
		updated, _ := m.Update(tokenMsg("app.refresh"))
		m = updated.(Model)
		if got := m.timeTravelInput.Value(); got != "HEAD~3" {
			t.Errorf("time-travel input = %q, want unchanged revision", got)
		}
	})

	t.Run("history search", func(t *testing.T) {
		m := actionTestModel(t)
		m.mode = ViewHistory
		m.historyView.StartSearch()
		updated, _ := m.Update(tokenMsg("app.refresh"))
		m = updated.(Model)
		if got := m.historyView.SearchQuery(); got != "" {
			t.Errorf("history search = %q, want unchanged empty query", got)
		}
	})

	t.Run("list filter", func(t *testing.T) {
		m := actionTestModel(t)
		m.list.SetFilterState(list.Filtering)
		m.list.FilterInput.SetValue("keep")
		updated, _ := m.Update(tokenMsg("app.refresh"))
		m = updated.(Model)
		if got := m.list.FilterInput.Value(); got != "keep" {
			t.Errorf("list filter = %q, want unchanged query", got)
		}
	})
}

func TestLiteralAtSignAndEmailInputAreNotActionTokens(t *testing.T) {
	t.Run("list search", func(t *testing.T) {
		m := runKey(t, actionTestModel(t), tea.KeyPressMsg{Code: '/', Text: "/"})
		for _, r := range "jka@b" {
			m = runKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if got, want := m.list.FilterInput.Value(), "jka@b"; got != want {
			t.Fatalf("list search query = %q, want %q", got, want)
		}
	})

	t.Run("BQL", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalBQLQuery)
		m.focused = focusBQLQuery
		m.bqlQuery.Focus()
		for _, r := range "email:a@b.test" {
			m = runKey(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if got, want := m.bqlQuery.Value(), "email:a@b.test"; got != want {
			t.Fatalf("BQL query = %q, want %q", got, want)
		}
	})
}

func TestUnknownActionTokenIsSwallowedFromTextInput(t *testing.T) {
	m := actionTestModel(t)
	m.openModal(ModalBQLQuery)
	updated, _ := m.Update(tokenMsg("not.registered.action"))
	m = updated.(Model)
	if got := m.bqlQuery.Value(); got != "" {
		t.Fatalf("BQL query = %q, want unknown token to be swallowed", got)
	}
}

func TestOwnedInputTokensStillDispatch(t *testing.T) {
	t.Run("BQL apply", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalBQLQuery)
		updated, _ := m.Update(tokenMsg("bql.apply"))
		m = updated.(Model)
		if m.activeModal != ModalNone {
			t.Errorf("BQL apply token left modal %v open", m.activeModal)
		}
	})

	t.Run("BQL history navigation", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalBQLQuery)
		m.bqlQuery.AddToHistory("status:closed")
		m.bqlQuery.input.SetValue("status:open")
		updated, _ := m.Update(tokenMsg("bql.history_prev"))
		m = updated.(Model)
		if got := m.bqlQuery.Value(); got != "status:closed" {
			t.Errorf("BQL history-prev token set query to %q, want status:closed", got)
		}
	})

	t.Run("time-travel cancel", func(t *testing.T) {
		m := actionTestModel(t)
		m.openModal(ModalTimeTravelInput)
		m.focused = focusTimeTravelInput
		updated, _ := m.Update(tokenMsg("time_travel.cancel"))
		m = updated.(Model)
		if m.activeModal != ModalNone {
			t.Errorf("time-travel cancel token left modal %v open", m.activeModal)
		}
	})

	t.Run("history cancel", func(t *testing.T) {
		m := actionTestModel(t)
		m.mode = ViewHistory
		m.historyView.StartSearch()
		updated, _ := m.Update(tokenMsg("history_search.cancel"))
		m = updated.(Model)
		if m.historyView.IsSearchActive() {
			t.Error("history cancel token did not end search")
		}
	})
}
