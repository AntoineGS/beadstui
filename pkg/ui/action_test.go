package ui

import (
	"errors"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

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
	m.SetKeyConfig(keys.Config{Bind: map[string]keys.BindValue{"filter.open": {"o"}}}, nil)
	m = runKey(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	if m.filter.currentFilter != "open" {
		t.Errorf("currentFilter = %q, want open", m.filter.currentFilter)
	}
}

func TestSetKeyConfigCountsJoinedLoadErrorsIndividually(t *testing.T) {
	m := actionTestModel(t)
	m.SetKeyConfig(keys.Config{}, errors.Join(errors.New("bad first bind"), errors.New("bad second bind")))
	if len(m.keyConfigErrs) != 2 {
		t.Errorf("retained %d errors, want 2: %v", len(m.keyConfigErrs), m.keyConfigErrs)
	}
	if m.statusMsg != "keys: 2 config error(s) - see ?" {
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
