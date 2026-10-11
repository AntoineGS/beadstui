package ui

import (
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
