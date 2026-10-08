package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
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

var (
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
)

func TestArrowsMoveBetweenPanesInSplitView(t *testing.T) {
	m := navModel(t, true)
	m, _ = sendMsg(m, keyRight)
	if m.focused != focusDetail {
		t.Fatalf("focus after right = %v, want detail", m.focused)
	}
	m, _ = sendMsg(m, keyRight) // already rightmost
	if m.focused != focusDetail {
		t.Fatalf("focus after second right = %v, want detail", m.focused)
	}
	m, _ = sendMsg(m, keyLeft)
	if m.focused != focusList || !m.isSplitView {
		t.Fatalf("focus after left = %v (split %v), want list in split view", m.focused, m.isSplitView)
	}
}

func TestArrowsOpenAndCloseDetailInSinglePane(t *testing.T) {
	m := navModel(t, false)
	m, _ = sendMsg(m, keyRight)
	if m.focused != focusDetail || !m.showDetails {
		t.Fatalf("after right: focus = %v showDetails = %v, want detail shown", m.focused, m.showDetails)
	}
	m, _ = sendMsg(m, keyLeft)
	if m.focused != focusList || m.showDetails {
		t.Fatalf("after left: focus = %v showDetails = %v, want list", m.focused, m.showDetails)
	}
}

func TestArrowsNoLongerPageTheList(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < 120; i++ {
		issues = append(issues, model.Issue{ID: fmt.Sprintf("p-%03d", i), Title: "Bead", Status: model.StatusOpen, Priority: 2})
	}
	m := newSizedModel(t, issues, 140, 40)
	m.list.Select(60)
	page := m.list.Paginator.Page
	m, _ = sendMsg(m, keyLeft)
	if m.focused != focusList || m.list.Index() != 60 || m.list.Paginator.Page != page {
		t.Fatalf("left on the list: focus = %v index = %d page = %d, want list/60/%d", m.focused, m.list.Index(), m.list.Paginator.Page, page)
	}
}

func TestArrowsEditSearchText(t *testing.T) {
	m := navModel(t, true)
	m, _ = sendMsg(m, keyRune('/'))
	m, _ = sendMsg(m, keyRune('n'))
	m, _ = sendMsg(m, keyRight)
	m, _ = sendMsg(m, keyLeft)
	if m.focused != focusList || m.list.FilterState() != list.Filtering || m.list.FilterValue() != "n" {
		t.Fatalf("focus = %v state = %v value = %q, want typing to continue", m.focused, m.list.FilterState(), m.list.FilterValue())
	}
}

func TestRightLeavesFullscreenIssuesForDetails(t *testing.T) {
	m := navModel(t, true)
	m.toggleFullscreenPane(fullscreenIssues) // list already focused: maximizes
	if m.fullscreen != fullscreenIssues {
		t.Fatalf("setup: fullscreen = %v, want issues", m.fullscreen)
	}
	m, _ = sendMsg(m, keyRight)
	if m.focused != focusDetail || m.fullscreen != fullscreenNone {
		t.Fatalf("focus = %v fullscreen = %v, want detail in the restored split", m.focused, m.fullscreen)
	}
}

func TestCommaPeriodPageDetailsFromEitherPane(t *testing.T) {
	issues := navIssues()
	issues[0].Description = strings.Repeat("A long paragraph line.\n\n", 200)
	m := newSizedModel(t, issues, 140, 40)
	m.list.Select(0)
	m.updateViewportContent()
	m, _ = sendMsg(m, keyRune('.'))
	afterDown := m.viewport.YOffset()
	if m.focused != focusList || afterDown == 0 {
		t.Fatalf("after . on list: focus = %v offset = %d, want list focus and details paged down", m.focused, afterDown)
	}
	m.focused = focusDetail
	m, _ = sendMsg(m, keyRune('.'))
	if got := m.viewport.YOffset(); got <= afterDown {
		t.Fatalf("after . on details: offset = %d, want > %d", got, afterDown)
	}
	m, _ = sendMsg(m, keyRune(','))
	m, _ = sendMsg(m, keyRune(','))
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("after two , presses: offset = %d, want 0", got)
	}
}

func TestLeftFromTreeDetailReturnsToTree(t *testing.T) {
	m := navModel(t, true)
	m.mode = ViewTree
	m.focused = focusDetail
	m, _ = sendMsg(m, keyLeft)
	if m.focused != focusTree {
		t.Fatalf("focus = %v, want tree", m.focused)
	}
}
