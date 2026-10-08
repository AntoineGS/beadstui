package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func navIssues() []model.Issue {
	var out []model.Issue
	for _, id := range []string{"n-1", "n-2", "n-3", "n-4", "n-5"} {
		out = append(out, model.Issue{ID: id, Title: "Bead " + id, Status: model.StatusOpen, Priority: 2})
	}
	return out
}

// navModel returns a list-focused model in split view (wide) or single-pane
// view (narrow).
func navModel(t *testing.T, split bool) Model {
	t.Helper()
	w := 140
	if !split {
		w = 70
	}
	m := newSizedModel(t, navIssues(), w, 40)
	if m.isSplitView != split {
		t.Fatalf("setup: isSplitView = %v at width %d, want %v", m.isSplitView, w, split)
	}
	if m.focused != focusList {
		t.Fatalf("setup: focus = %v, want list", m.focused)
	}
	return m
}

func TestEnterFocusesDetailInSplitView(t *testing.T) {
	m := navModel(t, true)
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.focused != focusDetail {
		t.Fatalf("focus after enter = %v, want detail", m.focused)
	}
	if !m.isSplitView {
		t.Fatal("enter left split view")
	}
}

func TestEnterOpensDetailInSinglePane(t *testing.T) {
	m := navModel(t, false)
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.focused != focusDetail || !m.showDetails {
		t.Fatalf("focus = %v showDetails = %v, want detail shown", m.focused, m.showDetails)
	}
}
