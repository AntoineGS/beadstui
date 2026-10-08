package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"
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

// mixedIssues has three open and two in-progress beads, all non-closed so
// the default open filter lists every one.
func mixedIssues() []model.Issue {
	return []model.Issue{
		{ID: "m-1", Title: "Alpha", Status: model.StatusOpen, Priority: 1},
		{ID: "m-2", Title: "Bravo", Status: model.StatusInProgress, Priority: 1},
		{ID: "m-3", Title: "Charlie", Status: model.StatusOpen, Priority: 1},
		{ID: "m-4", Title: "Delta", Status: model.StatusInProgress, Priority: 1},
		{ID: "m-5", Title: "Echo", Status: model.StatusOpen, Priority: 1},
	}
}

func TestStatusFilterChangeMovesCursorToFirstRow(t *testing.T) {
	m := newSizedModel(t, mixedIssues(), 140, 40)
	m.list.Select(1)
	m, _ = sendMsg(m, keyRune('#')) // open -> in_progress: two rows, index 1 still in range
	if m.CurrentFilter() != "in_progress" {
		t.Fatalf("setup: filter = %q, want in_progress", m.CurrentFilter())
	}
	if got := m.list.Index(); got != 0 {
		t.Fatalf("cursor after filter change = %d, want 0", got)
	}
}

func TestLabelFilterChangeMovesCursorToFirstRow(t *testing.T) {
	issues := mixedIssues()
	for i := range issues {
		issues[i].Labels = []string{"ui"}
	}
	m := newSizedModel(t, issues, 140, 40)
	m.list.Select(2)
	m.filter.labelFilter = "ui"
	m.applyFilter()
	if got := m.list.Index(); got != 0 {
		t.Fatalf("cursor after label filter = %d, want 0", got)
	}
}

func TestRefreshKeepsCursor(t *testing.T) {
	m := newSizedModel(t, mixedIssues(), 140, 40)
	m.list.Select(2)
	m.reapplyActiveFilter() // what a background reload runs
	if got := m.list.Index(); got != 2 {
		t.Fatalf("cursor after refresh = %d, want 2", got)
	}
}

func TestSearchTypingMovesCursorToFirstRow(t *testing.T) {
	m := newSizedModel(t, mixedIssues(), 140, 40)
	m.list.Select(3)
	m, _ = sendMsg(m, keyRune('/'))
	if got := m.list.Index(); got != 3 {
		t.Fatalf("cursor after opening / = %d, want 3 (bt-qka1 keeps it)", got)
	}
	m, _ = sendMsg(m, keyRune('a'))
	if got := m.list.Index(); got != 0 {
		t.Fatalf("cursor after typing = %d, want 0", got)
	}
}

func TestFilterKeySameForRecipeStartupAndApplied(t *testing.T) {
	// cmd/bt starts a recipe with currentFilter "all"; the background
	// snapshot later renames it "recipe:<name>". That is not a filter change,
	// so a later refresh must not jump the cursor.
	r := &recipe.Recipe{Name: "triage"}
	m := NewModel(mixedIssues(), r, "", nil, nil)
	startup := m.filterKey()
	m.filter.currentFilter = "recipe:" + r.Name
	if got := m.filterKey(); got != startup {
		t.Fatalf("filterKey changed from %q to %q for the same recipe", startup, got)
	}
}
