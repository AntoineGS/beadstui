package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"
)

func defaultFilterIssues() []model.Issue {
	return []model.Issue{
		{ID: "a-open", Status: model.StatusOpen},
		{ID: "b-closed", Status: model.StatusClosed},
		{ID: "c-progress", Status: model.StatusInProgress},
	}
}

func filteredIDs(m Model) map[string]bool {
	ids := map[string]bool{}
	for _, iss := range m.FilteredIssues() {
		ids[iss.ID] = true
	}
	return ids
}

func TestNewModelStartsWithOpenFilter(t *testing.T) {
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	if got := m.CurrentFilter(); got != "open" {
		t.Fatalf("CurrentFilter() = %q, want open", got)
	}
	ids := filteredIDs(m)
	if len(ids) != 2 || !ids["a-open"] || !ids["c-progress"] {
		t.Fatalf("visible = %v, want the two non-closed beads", ids)
	}
}

func TestNewModelWithRecipeKeepsAllFilter(t *testing.T) {
	// cmd/bt pre-filters the issues for a recipe; the status default must
	// not narrow them further.
	m := NewModel(defaultFilterIssues(), &recipe.Recipe{Name: "everything"}, "", nil, nil)
	if got := m.CurrentFilter(); got != "all" {
		t.Fatalf("CurrentFilter() = %q, want all", got)
	}
	if n := len(m.FilteredIssues()); n != 3 {
		t.Fatalf("visible = %d, want 3", n)
	}
}

func TestClearAllFiltersResetsToOpen(t *testing.T) {
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	m.SetFilter("closed")
	m.clearAllFilters()
	if got := m.CurrentFilter(); got != "open" {
		t.Fatalf("CurrentFilter() = %q, want open", got)
	}
	if ids := filteredIDs(m); ids["b-closed"] {
		t.Fatalf("closed bead visible after clearAllFilters: %v", ids)
	}
}

func TestOpenKeyTogglesToAllFromDefault(t *testing.T) {
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	m, _ = sendMsg(m, keyRune('o'))
	if got := m.CurrentFilter(); got != "all" {
		t.Fatalf("CurrentFilter() after o = %q, want all", got)
	}
	if n := len(m.FilteredIssues()); n != 3 {
		t.Fatalf("visible = %d, want 3", n)
	}
}

func TestEscFromAllReturnsToOpen(t *testing.T) {
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	m.SetFilter("all")
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if got := m.CurrentFilter(); got != "open" {
		t.Fatalf("CurrentFilter() after esc = %q, want open", got)
	}
	if m.activeModal != ModalNone {
		t.Fatalf("esc with a non-default filter opened modal %v; it should only clear filters", m.activeModal)
	}
}

func TestSelectIssueByIDWidensDefaultFilterForClosedBead(t *testing.T) {
	// Epics, alerts and notifications jump to beads the default filter hides.
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	if !m.selectIssueByID("b-closed") {
		t.Fatal("selectIssueByID(b-closed) = false, want true")
	}
	if got := m.CurrentFilter(); got != "all" {
		t.Fatalf("CurrentFilter() = %q, want all", got)
	}
	if sel, ok := m.list.SelectedItem().(IssueItem); !ok || sel.Issue.ID != "b-closed" {
		t.Fatalf("selected = %v, want b-closed", m.list.SelectedItem())
	}
}

func TestSelectIssueByIDKeepsExplicitFilter(t *testing.T) {
	m := NewModel(defaultFilterIssues(), nil, "", nil, nil)
	m.SetFilter("in_progress")
	if m.selectIssueByID("b-closed") {
		t.Fatal("selectIssueByID(b-closed) = true under an explicit in_progress filter")
	}
	if got := m.CurrentFilter(); got != "in_progress" {
		t.Fatalf("CurrentFilter() = %q, want in_progress", got)
	}
}

func TestSelectIssueByIDKeepsDefaultWhenLabelHidesBead(t *testing.T) {
	// Widening only helps closed beads; a bead the label filter hides stays
	// hidden and the status filter must not change behind the user's back.
	issues := defaultFilterIssues()
	issues[0].Labels = []string{"ui"}
	m := NewModel(issues, nil, "", nil, nil)
	m.filter.labelFilter = "ui"
	m.applyFilter()
	if m.selectIssueByID("c-progress") {
		t.Fatal("selectIssueByID(c-progress) = true, but the label filter hides it")
	}
	if got := m.CurrentFilter(); got != "open" {
		t.Fatalf("CurrentFilter() = %q, want open", got)
	}
}
