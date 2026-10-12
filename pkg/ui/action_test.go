package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

func TestBubblesSyncPreservesDefaultsAndAddsSearchToken(t *testing.T) {
	m := actionTestModel(t)
	prevPageDefaults := append([]string(nil), m.list.KeyMap.PrevPage.Keys()...)
	if !slices.Contains(prevPageDefaults, "pgup") {
		t.Fatalf("setup: Bubbles PrevPage keys = %v, want bt's existing pgup binding", prevPageDefaults)
	}
	m.SetKeyConfig(keys.Config{}, nil)

	if got := m.list.KeyMap.PrevPage.Keys(); !slices.Equal(got, prevPageDefaults) {
		t.Errorf("Bubbles PrevPage changed without a related override: got %v, want %v", got, prevPageDefaults)
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
