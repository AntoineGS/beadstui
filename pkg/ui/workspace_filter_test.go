package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func repoColumnFixture() (Model, []model.Issue) {
	issues := []model.Issue{
		{ID: "se-123", SourceRepo: "statsengine", Title: strings.Repeat("x", 220), Status: model.StatusOpen, IssueType: model.TypeTask, UpdatedAt: time.Now()},
		{ID: "web-456", SourceRepo: "web", Title: "Other project", Status: model.StatusClosed, IssueType: model.TypeTask, UpdatedAt: time.Now()},
	}
	m := NewModel(issues, nil, "", nil, nil)
	m.SetFilter("all") // web-456 is closed and must stay listed
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled: true, RepoCount: 3, RepoPrefixes: []string{"statsengine", "web", "api"},
	})
	m.list.SetSize(80, 10)
	return m, issues
}

func assertRepoColumn(t *testing.T, m Model, want bool) {
	t.Helper()
	if header := ansi.Strip(m.splitViewHeader()); strings.Contains(header, "REPO") != want {
		t.Errorf("repo header visibility = %t, want %t: %q", strings.Contains(header, "REPO"), want, header)
	}
	// A scope with no enabled project lists nothing, so only the header can
	// show the column then.
	if len(m.list.Items()) == 0 {
		return
	}
	if rows := ansi.Strip(m.list.View()); strings.Contains(rows, "[SE]") != want {
		t.Errorf("repo badge visibility = %t, want %t: %q", strings.Contains(rows, "[SE]"), want, rows)
	}
}

// Catch using workspace mode alone, result counts, or map length for visibility.
func TestRepoColumnFollowsCommittedProjectScope(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workspace bool
		scope     map[string]bool
		want      bool
	}{
		{"local", false, nil, false},
		{"all", true, nil, true},
		{"empty", true, map[string]bool{}, true},
		{"single", true, map[string]bool{"statsengine": true}, false},
		{"multiple", true, map[string]bool{"statsengine": true, "web": true}, true},
		{"single with disabled entry", true, map[string]bool{"statsengine": true, "web": false}, false},
		{"no enabled entries", true, map[string]bool{"statsengine": false}, true},
		{"unavailable single project", true, map[string]bool{"missing": true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := repoColumnFixture()
			m.EnableWorkspaceMode(WorkspaceInfo{
				Enabled: tc.workspace, RepoCount: 1, RepoPrefixes: []string{"statsengine"},
			})
			// Startup sets scope before applying the initial issue filter.
			m.SetActiveRepos(tc.scope)
			assertRepoColumn(t, m, tc.want)
			m.updateListDelegate()
			assertRepoColumn(t, m, tc.want)
		})
	}
}

func TestRepoColumnSingleProjectReclaimsTitleWidth(t *testing.T) {
	setGlyphs(t, asciiGlyphs)
	m, _ := repoColumnFixture()
	visibleTitleWidth := func() int {
		widest := 0
		for _, line := range strings.Split(ansi.Strip(m.list.View()), "\n") {
			if n := strings.Count(line, "x"); n > widest {
				widest = n
			}
		}
		return widest
	}
	for _, width := range []int{50, 80, 120, 160} {
		m.list.SetWidth(width)
		m.SetActiveRepos(nil)
		before := visibleTitleWidth()
		m.SetActiveRepos(map[string]bool{"statsengine": true})
		// The displayed workspace also contains [WEB], so [SE] shares its
		// five-cell repo column. Hiding that column reclaims six cells.
		if gain := visibleTitleWidth() - before; gain != 6 {
			t.Errorf("width=%d: title gained %d cells, want 6 (shared [WEB] width plus separator)", width, gain)
		}
		row := ansi.Strip(m.list.View())
		if !strings.HasPrefix(row, "t o 0 ") || !strings.Contains(row, "123 ") {
			t.Errorf("width=%d: hidden repo left a gutter or changed the compact ID: %q", width, row)
		}
		for _, line := range strings.Split(row, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("row width=%d exceeds available width=%d", got, width)
			}
		}
	}
	item := m.list.Items()[0].(IssueItem)
	if got := issueIDForClipboard(item); got != "se-123" {
		t.Errorf("canonical clipboard ID changed: %q", got)
	}
}

func TestRepoColumnProjectPickerCommitsVisibility(t *testing.T) {
	m, _ := repoColumnFixture()
	for _, tc := range []struct {
		name   string
		scope  map[string]bool
		before bool
		want   bool
	}{
		{"single", map[string]bool{}, true, false}, // No checkmarks commits the cursor project.
		{"multiple", map[string]bool{"statsengine": true, "web": true}, false, true},
		{"all", map[string]bool{"statsengine": true, "web": true, "api": true}, true, true},
		{"single again", map[string]bool{"statsengine": true}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.repoPicker = NewRepoPickerModel(m.availableRepos, m.theme)
			m.repoPicker.selected = tc.scope
			for i, repo := range m.repoPicker.filtered {
				if repo == "statsengine" {
					m.repoPicker.selectedIndex = i
				}
			}
			assertRepoColumn(t, m, tc.before) // Uncommitted selection must not affect layout.
			m = m.applyRepoPickerSelection()
			assertRepoColumn(t, m, tc.want)
			if !m.workspaceMode {
				t.Fatal("hiding repo badges disabled workspace mode")
			}
		})
	}
}

func TestRepoColumnHomeAllToggleWithRecipe(t *testing.T) {
	for _, useRecipe := range []bool{false, true} {
		m, _ := repoColumnFixture()
		m.SetCurrentProjectDB("statsengine")
		if useRecipe {
			m.setActiveRecipe(&recipe.Recipe{Name: "all"})
		}
		for _, want := range []bool{false, true, false} {
			updated, _ := m.Update(tea.KeyPressMsg{Code: 'W', Text: "W"})
			m = updated.(Model)
			assertRepoColumn(t, m, want)
			if !want && (len(m.list.Items()) != 1 || m.list.Items()[0].(IssueItem).Issue.ID != "se-123") {
				t.Fatalf("home filter used the displayed prefix instead of the database key: %v", m.list.Items())
			}
		}
	}
}

func TestRepoColumnScopeSurvivesRefreshAndSecondaryFilters(t *testing.T) {
	m, issues := repoColumnFixture()
	m.SetActiveRepos(map[string]bool{"statsengine": true})
	m.applyFilter()
	m, _ = m.handleDataSourceReload(DataSourceReloadMsg{Issues: issues})
	assertRepoColumn(t, m, false)
	for _, width := range []int{60, 160} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m = updated.(Model)
		assertRepoColumn(t, m, false)
	}
	m.SetActiveRepos(nil)
	m.filter.currentFilter = "open" // Only statsengine has an open issue.
	m.applyFilter()
	if len(m.list.Items()) != 1 {
		t.Fatalf("expected secondary filter to leave one issue, got %d", len(m.list.Items()))
	}
	assertRepoColumn(t, m, true)
	m.filter.currentFilter = "all"
	m.applyFilter()
	if got := len(m.list.VisibleItems()); got != 2 {
		t.Fatalf("expected two projects before searching, got %d visible issues", got)
	}
	m.list.SetFilterText("xxxxxxxx")
	if got := len(m.list.VisibleItems()); got != 1 {
		t.Fatalf("expected search to leave one project's issue, got %d visible issues", got)
	}
	assertRepoColumn(t, m, true)
	m.SetActiveRepos(map[string]bool{"missing": true})
	m.applyFilter()
	if len(m.list.Items()) != 0 {
		t.Fatal("unavailable project must leave the issue list empty")
	}
	if header := ansi.Strip(m.splitViewHeader()); strings.Contains(header, "REPO") {
		t.Errorf("empty single-project scope restored the repo header: %q", header)
	}
}

// TestLabelPickerRespectsProjectScope is the repro for bt-obw (gh-61): after
// narrowing to a project with w, the l picker listed every workspace label
// with workspace-wide counts.
func TestLabelPickerRespectsProjectScope(t *testing.T) {
	issues := []model.Issue{
		{ID: "proja-1", Title: "A one", Status: model.StatusOpen, Labels: []string{"shared", "a-only"}},
		{ID: "projb-1", Title: "B one", Status: model.StatusOpen, Labels: []string{"shared", "b-only"}},
		{ID: "projb-2", Title: "B two", Status: model.StatusOpen, Labels: []string{"shared"}},
	}
	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"},
	})

	openPicker := func(m Model) Model {
		t.Helper()
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
		m = updated.(Model)
		if m.activeModal != ModalLabelPicker {
			t.Fatalf("expected label picker open, got modal %v", m.activeModal)
		}
		m = m.handleLabelPickerKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
		return m
	}
	assertLabels := func(m Model, want map[string]int) {
		t.Helper()
		got := make(map[string]int, len(m.labelPicker.allLabels))
		for _, l := range m.labelPicker.allLabels {
			got[l] = m.labelPicker.labelCounts[l]
		}
		if len(got) != len(want) {
			t.Fatalf("labels = %v, want %v", got, want)
		}
		for l, n := range want {
			if c, ok := got[l]; !ok || c != n {
				t.Fatalf("labels = %v, want %v", got, want)
			}
		}
	}

	// No project filter: whole workspace, as before.
	m = openPicker(m)
	assertLabels(m, map[string]int{"shared": 3, "a-only": 1, "b-only": 1})

	// Narrow to proja through the w picker's commit path, then press l.
	m.repoPicker = NewRepoPickerModel(m.availableRepos, m.theme)
	m.repoPicker.selected = map[string]bool{} // No checkmarks commits the cursor project.
	for i, repo := range m.repoPicker.filtered {
		if repo == "proja" {
			m.repoPicker.selectedIndex = i
		}
	}
	m = m.applyRepoPickerSelection()
	if !m.activeRepos["proja"] || m.activeRepos["projb"] {
		t.Fatalf("precondition: expected scope {proja}, got %v", m.activeRepos)
	}
	m = openPicker(m)
	assertLabels(m, map[string]int{"shared": 1, "a-only": 1})

	// An applied label with no issues in scope stays listed (count 0) and
	// selected, so Enter does not silently drop it from the filter.
	m.filter.labelFilter = "b-only"
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	assertLabels(m, map[string]int{"shared": 1, "a-only": 1, "b-only": 0})
	if sel := m.labelPicker.SelectedLabels(); len(sel) != 1 || sel[0] != "b-only" {
		t.Fatalf("applied out-of-scope label should stay selected, got %v", sel)
	}
}

// buildRecommendationFixture returns a "blocker" issue plus three dependents
// under the given repo prefix. This mirrors
// analysis.TestGenerateRecommendationsHighImpactLowPriority's fixture shape
// (a stale, medium-priority issue blocking several others), which reliably
// clears GenerateRecommendations' default confidence threshold so the
// blocker issue always produces a PriorityRecommendation when analyzed.
func buildRecommendationFixture(prefix string) []model.Issue {
	now := time.Now()
	blockerID := prefix + "-blocker"
	return []model.Issue{
		{ID: blockerID, Title: prefix + " blocker", Status: model.StatusOpen, Priority: 3, UpdatedAt: now.AddDate(0, 0, -20)},
		{ID: prefix + "-dep1", Title: prefix + " dep1", Status: model.StatusOpen, Priority: 1, Dependencies: []*model.Dependency{
			{DependsOnID: blockerID, Type: model.DepBlocks},
		}},
		{ID: prefix + "-dep2", Title: prefix + " dep2", Status: model.StatusOpen, Priority: 1, Dependencies: []*model.Dependency{
			{DependsOnID: blockerID, Type: model.DepBlocks},
		}},
		{ID: prefix + "-dep3", Title: prefix + " dep3", Status: model.StatusOpen, Priority: 1, Dependencies: []*model.Dependency{
			{DependsOnID: blockerID, Type: model.DepBlocks},
		}},
	}
}

// TestRecomputePriorityHintsRespectsActiveRepos is the direct repro for
// bt-gcuv: pressing 'p' in global/workspace mode with a project filter
// active computed recommendations from all cross-project issues instead of
// the filtered set. recomputePriorityHints must build its Analyzer from
// the visible set (bt-imh), so a projb-only recommendation can never
// appear once activeRepos narrows the view to proja.
func TestRecomputePriorityHintsRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})

	// Sanity check: with no project filter, both projects' blockers clear
	// the recommendation threshold. This proves the fixture is valid and
	// that a later absence of projb-blocker is due to the repo filter, not
	// the fixture failing to generate a recommendation at all.
	m.recomputePriorityHints()
	if _, ok := m.ac.priorityHints["proja-blocker"]; !ok {
		t.Fatalf("expected proja-blocker recommendation with no filter, hints=%v", m.ac.priorityHints)
	}
	if _, ok := m.ac.priorityHints["projb-blocker"]; !ok {
		t.Fatalf("expected projb-blocker recommendation with no filter, hints=%v", m.ac.priorityHints)
	}

	// bt-gcuv: filtering to proja must scope recommendations to proja only.
	m.SetActiveRepos(map[string]bool{"proja": true})
	m.recomputePriorityHints()

	if _, ok := m.ac.priorityHints["projb-blocker"]; ok {
		t.Fatalf("projb recommendation leaked into proja-filtered priority hints: %v", m.ac.priorityHints)
	}
	if _, ok := m.ac.priorityHints["proja-blocker"]; !ok {
		t.Fatalf("expected proja-blocker recommendation to survive the proja filter")
	}
	for id := range m.ac.priorityHints {
		if got := ExtractRepoPrefix(id); got != "proja" {
			t.Fatalf("non-proja issue %q leaked into filtered priority hints", id)
		}
	}
}

// TestApplyFilterRecomputesPriorityHintsWhenShown covers the bt-gcuv
// acceptance criterion "changing project filter recalculates priority
// hints": applyFilter is the common path the repo picker (Enter) and the
// W (home/all) toggle both route through, so it must refresh
// m.ac.priorityHints whenever hints are currently displayed.
func TestApplyFilterRecomputesPriorityHintsWhenShown(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})

	m.ac.showPriorityHints = true
	m.recomputePriorityHints() // seed with the global (unfiltered) computation
	if _, ok := m.ac.priorityHints["projb-blocker"]; !ok {
		t.Fatalf("expected seeded global hints to include projb-blocker")
	}

	// Changing the project filter through applyFilter (as the repo picker
	// and W toggle do) must recalculate the hints rather than leaving the
	// stale global-set recommendation on screen.
	m.activeRepos = map[string]bool{"proja": true}
	m.applyFilter()

	if _, ok := m.ac.priorityHints["projb-blocker"]; ok {
		t.Fatalf("priority hints were not recomputed after the project filter changed: %v", m.ac.priorityHints)
	}
	if _, ok := m.ac.priorityHints["proja-blocker"]; !ok {
		t.Fatalf("expected proja-blocker to remain after the filter change")
	}
}

// TestApplyFilterDoesNotRecomputePriorityHintsWhenHidden locks in the
// perf-conscious gating: applyFilter runs on many high-frequency actions
// (sort changes, label picker toggles, drilldowns), so it must not pay the
// synchronous Analyzer/GenerateRecommendations cost while the hints
// overlay isn't even visible.
func TestApplyFilterDoesNotRecomputePriorityHintsWhenHidden(t *testing.T) {
	issues := buildRecommendationFixture("proja")

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)

	m.ac.showPriorityHints = false
	sentinel := &analysis.PriorityRecommendation{IssueID: "sentinel"}
	m.ac.priorityHints = map[string]*analysis.PriorityRecommendation{"sentinel": sentinel}

	m.applyFilter()

	if got, ok := m.ac.priorityHints["sentinel"]; !ok || got != sentinel {
		t.Fatalf("applyFilter should not touch priority hints while hidden, got %v", m.ac.priorityHints)
	}
}

// TestPriorityHintsToggleUsesActiveRepoFilter is the end-to-end repro via
// the actual 'p' keypress (model_update_input.go), covering the exact user
// flow described in bt-gcuv: filter to one project in workspace mode, press
// p, expect arrows scoped to that project only.
func TestPriorityHintsToggleUsesActiveRepoFilter(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = updated2.(Model)

	if !m.ac.showPriorityHints {
		t.Fatalf("expected priority hints toggled on")
	}
	if _, ok := m.ac.priorityHints["projb-blocker"]; ok {
		t.Fatalf("pressing p with a project filter active surfaced a cross-project hint: %v", m.ac.priorityHints)
	}
	if _, ok := m.ac.priorityHints["proja-blocker"]; !ok {
		t.Fatalf("expected proja-blocker hint when filtered to proja")
	}
}

func TestApplyFilterRespectsWorkspaceRepoFilter(t *testing.T) {
	issues := []model.Issue{
		{ID: "api-AUTH-1", Title: "API", Status: model.StatusOpen},
		{ID: "web-UI-1", Title: "Web", Status: model.StatusOpen},
	}

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)

	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"api-", "web-"},
	})

	// Filter to api only
	m.activeRepos = map[string]bool{"api": true}
	m.applyFilter()

	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("expected 1 visible item after repo filter, got %d", got)
	}
	item, ok := m.list.Items()[0].(IssueItem)
	if !ok {
		t.Fatalf("expected IssueItem")
	}
	if item.Issue.ID != "api-AUTH-1" {
		t.Fatalf("expected api issue, got %s", item.Issue.ID)
	}

	// Clear repo filter (nil = all repos)
	m.activeRepos = nil
	m.applyFilter()
	if got := len(m.list.Items()); got != 2 {
		t.Fatalf("expected 2 visible items with no repo filter, got %d", got)
	}
}

// TestTreeViewRespectsActiveRepos asserts the tree view rebuilds from the
// activeRepos-filtered slice, matching the dogfood repro on bt-dcby.2:
// before the fix the tree showed all beads regardless of project filter.
func TestTreeViewRespectsActiveRepos(t *testing.T) {
	issues := []model.Issue{
		{ID: "api-AUTH-1", Title: "API auth", Status: model.StatusOpen},
		{ID: "api-AUTH-2", Title: "API token", Status: model.StatusOpen},
		{ID: "web-UI-1", Title: "Web UI", Status: model.StatusOpen},
	}

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)

	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"api-", "web-"},
	})

	// Enter tree mode so the helper actually rebuilds.
	m.mode = ViewTree
	m.SetActiveRepos(map[string]bool{"api": true})
	m.rebuildTreeForCurrentFilter()

	if got := m.tree.NodeCount(); got != 2 {
		t.Fatalf("expected 2 visible tree nodes for api filter, got %d", got)
	}
	for i := 0; i < m.tree.NodeCount(); i++ {
		node := m.tree.flatList[i]
		if node == nil || node.Issue == nil {
			t.Fatalf("nil node/issue at index %d", i)
		}
		if got := IssueRepoKey(*node.Issue); got != "api" {
			t.Fatalf("non-api bead leaked into tree: id=%s repo=%s", node.Issue.ID, got)
		}
	}

	// Toggling activeRepos to nil should restore the full set on the next
	// applyFilter. This covers the user flow: open tree, toggle project filter
	// off via shortcut, expect tree to rebuild without re-pressing E.
	m.activeRepos = nil
	m.applyFilter()
	if got := m.tree.NodeCount(); got != 3 {
		t.Fatalf("expected 3 visible tree nodes after clearing activeRepos, got %d", got)
	}

	// Tightening to the web project should narrow the tree, again live.
	m.activeRepos = map[string]bool{"web": true}
	m.applyFilter()
	if got := m.tree.NodeCount(); got != 1 {
		t.Fatalf("expected 1 visible tree node after web filter, got %d", got)
	}
	if id := m.tree.flatList[0].Issue.ID; id != "web-UI-1" {
		t.Fatalf("expected web-UI-1, got %s", id)
	}
}

// TestRebuildTreeForCurrentFilterIsNoOpOutsideTreeMode asserts that the
// helper does not pay the build cost (or stomp tree state) when the active
// view is not the tree. Non-tree filter changes should not touch the tree.
func TestRebuildTreeForCurrentFilterIsNoOpOutsideTreeMode(t *testing.T) {
	issues := []model.Issue{
		{ID: "api-AUTH-1", Title: "API auth", Status: model.StatusOpen},
	}

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	// mode defaults to ViewList; tree was never built.
	if m.tree.IsBuilt() {
		t.Fatalf("tree should not be built before any rebuild call")
	}

	m.rebuildTreeForCurrentFilter()
	if m.tree.IsBuilt() {
		t.Errorf("tree should remain unbuilt outside ViewTree mode")
	}
}

// bt-dcby.3: the five surfaces below (actionable view, insights/triage,
// label health, label attention, cross-label flow) fed analysis from the
// unfiltered m.data.issues instead of the workspace-filtered set, the same
// root-cause shape bt-gcuv fixed for priority hints. Tests mirror the
// fixtures/patterns above.

// buildLabelFlowFixture returns two issues per project prefix with distinct,
// project-scoped labels and one cross-label blocking dependency, giving
// ComputeAllLabelHealth / ComputeLabelAttentionScores / ComputeCrossLabelFlow
// real per-project signal (label taxonomy, health counts, one cross-label
// dependency) to compute over.
func buildLabelFlowFixture(prefix string) []model.Issue {
	now := time.Now()
	blockerID := prefix + "-blocker"
	blockedID := prefix + "-blocked"
	return []model.Issue{
		{
			ID: blockerID, Title: prefix + " blocker", Status: model.StatusOpen,
			Priority: 2, Labels: []string{prefix + "-labelA"}, UpdatedAt: now,
		},
		{
			ID: blockedID, Title: prefix + " blocked", Status: model.StatusOpen,
			Priority: 2, Labels: []string{prefix + "-labelB"}, UpdatedAt: now,
			Dependencies: []*model.Dependency{
				{DependsOnID: blockerID, Type: model.DepBlocks},
			},
		},
	}
}

// TestActionableViewRespectsActiveRepos is the repro for surface 1
// (model_update_input.go's Actionable-view toggle): the execution plan must
// be built from the visible set (bt-imh), not the full cross-project
// m.data.issues, so a projb-only item can never appear once activeRepos
// narrows the view to proja.
func TestActionableViewRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated2.(Model)

	if m.mode != ViewActionable {
		t.Fatalf("expected actionable view mode, got %v", m.mode)
	}
	found := false
	for _, track := range m.actionableView.plan.Tracks {
		for _, item := range track.Items {
			if ExtractRepoPrefix(item.ID) != "proja" {
				t.Fatalf("non-proja issue leaked into actionable plan: %s", item.ID)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected at least one proja item in the execution plan")
	}
}

// TestOpenInsightsViewRespectsActiveRepos is the repro for surface 2's
// openInsightsView call site ('i' toggle, model_update_input.go):
// TopPicks/Recommendations must be ranked from a fresh analyzer built over
// the workspace-filtered set, not the global analyzer/stats.
func TestOpenInsightsViewRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated2.(Model)

	if m.mode != ViewInsights {
		t.Fatalf("expected insights view mode, got %v", m.mode)
	}
	for _, pick := range m.insightsPanel.topPicks {
		if ExtractRepoPrefix(pick.ID) != "proja" {
			t.Fatalf("non-proja top pick leaked into insights panel: %s", pick.ID)
		}
	}
	for _, rec := range m.insightsPanel.recommendations {
		if ExtractRepoPrefix(rec.ID) != "proja" {
			t.Fatalf("non-proja recommendation leaked into insights panel: %s", rec.ID)
		}
	}
}

// TestPhase2ReadyTriageBadgesRespectActiveRepos is the repro for surface 2's
// other call site (handlePhase2Ready, model_update_analysis.go): the triage
// scores/quick-win/blocker badges applied to list rows must be scoped to the
// workspace-filtered set, mirroring TestOpenInsightsViewRespectsActiveRepos
// so both entry points into the same triage data stay consistent.
func TestPhase2ReadyTriageBadgesRespectActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	ins := m.data.analysis.GenerateInsights(len(issues))
	updated2, _ := m.Update(Phase2ReadyMsg{Stats: m.data.analysis, Insights: ins})
	m = updated2.(Model)

	if len(m.ac.triageScores) == 0 {
		t.Fatalf("expected triage scores to be populated after Phase2Ready")
	}
	for id := range m.ac.triageScores {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("non-proja issue leaked into triageScores: %s", id)
		}
	}
	for id := range m.ac.quickWinSet {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("non-proja issue leaked into quickWinSet: %s", id)
		}
	}
	for id := range m.ac.blockerSet {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("non-proja issue leaked into blockerSet: %s", id)
		}
	}
}

// TestLabelDashboardRespectsActiveRepos is the repro for surface 3
// (Label Health, '[' toggle, model_update_input.go): the label taxonomy and
// counts must come from the workspace-filtered set so a projb-only label
// can never appear once activeRepos narrows the view to proja.
func TestLabelDashboardRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildLabelFlowFixture("proja")...)
	issues = append(issues, buildLabelFlowFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated2.(Model)

	if m.mode != ViewLabelDashboard {
		t.Fatalf("expected label dashboard mode, got %v", m.mode)
	}
	if len(m.labelHealthCache.Labels) == 0 {
		t.Fatalf("expected label health data to be populated")
	}
	for _, lh := range m.labelHealthCache.Labels {
		if ExtractRepoPrefix(lh.Label) != "proja" {
			t.Fatalf("non-proja label leaked into label health dashboard: %s", lh.Label)
		}
		for _, issueID := range lh.Issues {
			if ExtractRepoPrefix(issueID) != "proja" {
				t.Fatalf("non-proja issue leaked into label health issues: %s", issueID)
			}
		}
	}
}

// TestAttentionViewRespectsActiveRepos is the repro for surface 4
// (Label Attention, ']' toggle, model_update_input.go).
func TestAttentionViewRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildLabelFlowFixture("proja")...)
	issues = append(issues, buildLabelFlowFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	m = updated2.(Model)

	if m.mode != ViewAttention {
		t.Fatalf("expected attention view mode, got %v", m.mode)
	}
	if len(m.attentionCache.Labels) == 0 {
		t.Fatalf("expected attention scores to be populated")
	}
	for _, score := range m.attentionCache.Labels {
		if ExtractRepoPrefix(score.Label) != "proja" {
			t.Fatalf("non-proja label leaked into attention view: %s", score.Label)
		}
	}
}

// TestFlowMatrixRespectsActiveRepos is the repro for surface 5
// (Cross-Label Flow, 'f' toggle, model_update_input.go).
func TestFlowMatrixRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildLabelFlowFixture("proja")...)
	issues = append(issues, buildLabelFlowFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})
	m.SetActiveRepos(map[string]bool{"proja": true})

	updated2, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated2.(Model)

	if m.mode != ViewFlowMatrix {
		t.Fatalf("expected flow matrix mode, got %v", m.mode)
	}
	if m.flowMatrix.flow == nil {
		t.Fatalf("expected flow data to be populated")
	}
	for _, lbl := range m.flowMatrix.flow.Labels {
		if ExtractRepoPrefix(lbl) != "proja" {
			t.Fatalf("non-proja label leaked into flow matrix: %s", lbl)
		}
	}
	for _, dep := range m.flowMatrix.flow.Dependencies {
		if ExtractRepoPrefix(dep.FromLabel) != "proja" || ExtractRepoPrefix(dep.ToLabel) != "proja" {
			t.Fatalf("non-proja dependency leaked into flow matrix: %+v", dep)
		}
	}
}

// buildSharedLabelFlowFixture returns a bug-blocks-feature dependency pair
// per project, using the SAME label names ("bug"/"feature") across projects.
// getCrossFlowsForLabel's return type (labelFlowSummary) aggregates by label
// name with a plain count, not issue IDs, so a project-prefixed label (as in
// buildLabelFlowFixture) can never collide across projects and the leak
// would go undetected. Sharing the label name is what makes the aggregate
// count observably wrong when projb's contribution isn't excluded.
func buildSharedLabelFlowFixture(prefix string) []model.Issue {
	now := time.Now()
	bugID := prefix + "-bug-issue"
	featureID := prefix + "-feature-issue"
	return []model.Issue{
		{ID: bugID, Title: prefix + " bug", Status: model.StatusOpen, Priority: 2, Labels: []string{"bug"}, UpdatedAt: now},
		{
			ID: featureID, Title: prefix + " feature", Status: model.StatusOpen, Priority: 2, Labels: []string{"feature"}, UpdatedAt: now,
			Dependencies: []*model.Dependency{
				{DependsOnID: bugID, Type: model.DepBlocks},
			},
		},
	}
}

// incomingCountFor looks up the aggregate count for a source label within a
// labelFlowSummary's Incoming list (0 if absent).
func incomingCountFor(summary labelFlowSummary, label string) int {
	for _, c := range summary.Incoming {
		if c.Label == label {
			return c.Count
		}
	}
	return 0
}

// TestGetCrossFlowsForLabelRespectsActiveRepos is the repro for the second
// cross-label-flow call site, the label-drilldown helper getCrossFlowsForLabel
// (model.go), which is invoked independently of the 'f' toggle above.
func TestGetCrossFlowsForLabelRespectsActiveRepos(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildSharedLabelFlowFixture("proja")...)
	issues = append(issues, buildSharedLabelFlowFixture("projb")...)

	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{
		Enabled:      true,
		RepoCount:    2,
		RepoPrefixes: []string{"proja-", "projb-"},
	})

	// Sanity check: with no project filter, both projects' "bug" issues
	// block "feature", so the incoming count aggregates both contributions.
	summary := m.getCrossFlowsForLabel("feature")
	if got := incomingCountFor(summary, "bug"); got != 2 {
		t.Fatalf("expected aggregated incoming count 2 with no filter, got %d", got)
	}

	// bt-dcby.3: filtering to proja must scope the flow summary to proja's
	// own contribution only, not the cross-project aggregate.
	m.SetActiveRepos(map[string]bool{"proja": true})
	summary = m.getCrossFlowsForLabel("feature")
	if got := incomingCountFor(summary, "bug"); got != 1 {
		t.Fatalf("expected incoming count scoped to 1 after proja filter, got %d (projb contribution leaked)", got)
	}
}

// bt-imh: analysis surfaces follow the full filter, not only project scope.
func TestTriageFollowsLabelFilter(t *testing.T) {
	var issues []model.Issue
	labeled := buildRecommendationFixture("proja")
	for i := range labeled {
		labeled[i].Labels = []string{"focus"}
	}
	issues = append(issues, labeled...)
	issues = append(issues, buildRecommendationFixture("projb")...)
	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	ins := m.data.analysis.GenerateInsights(len(issues))
	updated, _ = m.Update(Phase2ReadyMsg{Stats: m.data.analysis, Insights: ins})
	m = updated.(Model)
	if _, ok := m.ac.triageScores["projb-blocker"]; !ok {
		t.Fatal("precondition: projb-blocker scored with no filter")
	}

	m.filter.labelFilter = "focus"
	m.applyFilter()
	for id := range m.ac.triageScores {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("issue %s outside the label filter has a triage score", id)
		}
	}
	if len(m.ac.triageScores) == 0 {
		t.Fatal("expected triage scores for the labeled issues")
	}
}

func TestLabelDashboardFollowsLabelFilter(t *testing.T) {
	issues := buildLabelFlowFixture("proja")
	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.filter.labelFilter = "proja-labelA"
	m.applyFilter()
	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)
	for _, lh := range m.labelHealthCache.Labels {
		for _, id := range lh.Issues {
			if _, ok := m.filter.visibleIDs[id]; !ok {
				t.Fatalf("label health includes %s, which the filter hides", id)
			}
		}
	}
}

func TestInsightsListsOnlyVisibleIssues(t *testing.T) {
	var issues []model.Issue
	issues = append(issues, buildRecommendationFixture("proja")...)
	issues = append(issues, buildRecommendationFixture("projb")...)
	m := NewModel(issues, nil, "", nil, nil)
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja-", "projb-"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	m.applyFilter()
	ins := m.visibleInsights(m.data.analysis.GenerateInsights(len(issues)))
	for _, list := range [][]analysis.InsightItem{ins.Bottlenecks, ins.Keystones, ins.Influencers, ins.Hubs, ins.Authorities, ins.Cores, ins.Slack} {
		for _, it := range list {
			if ExtractRepoPrefix(it.ID) != "proja" {
				t.Fatalf("insights list includes hidden issue %s", it.ID)
			}
		}
	}
}

func focusTriageFixture() []model.Issue {
	labeled := buildRecommendationFixture("proja")
	for i := range labeled {
		labeled[i].Labels = []string{"focus"}
	}
	return append(labeled, buildRecommendationFixture("projb")...)
}

func assertTriageOnlyProja(t *testing.T, m Model) {
	t.Helper()
	if len(m.ac.triageScores) == 0 {
		t.Fatal("expected triage scores for the visible issues")
	}
	for id := range m.ac.triageScores {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("hidden issue %s has a triage score", id)
		}
	}
}

// A snapshot that already finished Phase 2 gets no further Phase2ReadyMsg
// processing (bt-kfkrb), so its arrival must rank triage over the visible
// set itself instead of keeping the snapshot's corpus-wide triage (bt-imh).
func TestPhase2SnapshotRanksTriageOverVisibleSet(t *testing.T) {
	issues := focusTriageFixture()
	m := newSizedModel(t, issues, 140, 40)
	m.filter.labelFilter = "focus"
	m.applyFilter()

	snap := NewSnapshotBuilder(focusTriageFixture()).Build()
	snap.Analysis.WaitForPhase2()
	snap.Phase2Ready = true
	if _, ok := snap.TriageScores["projb-blocker"]; !ok {
		t.Fatal("precondition: snapshot triage is corpus-wide")
	}
	updated, _ := m.Update(SnapshotReadyMsg{Snapshot: snap})
	assertTriageOnlyProja(t, updated.(Model))
}

// A reload that keeps the old Phase-2-ready snapshot (replaceIssues) still
// ranks triage over the visible set once its own Phase 2 lands (bt-imh).
func TestPhase2AfterReloadRanksTriageOverVisibleSet(t *testing.T) {
	issues := focusTriageFixture()
	m := newSizedModel(t, issues, 140, 40)
	snap := NewSnapshotBuilder(focusTriageFixture()).Build()
	snap.Analysis.WaitForPhase2()
	snap.Phase2Ready = true
	updated, _ := m.Update(SnapshotReadyMsg{Snapshot: snap})
	m = updated.(Model)
	if _, ok := m.ac.triageScores["projb-blocker"]; !ok {
		t.Fatal("precondition: unfiltered triage scores projb-blocker")
	}

	m.replaceIssues(focusTriageFixture())
	m.filter.labelFilter = "focus"
	m.applyFilter()
	m.data.analysis.WaitForPhase2()
	updated, _ = m.Update(Phase2ReadyMsg{Stats: m.data.analysis})
	assertTriageOnlyProja(t, updated.(Model))
}

// An open actionable view follows later filter changes (bt-imh final review).
func TestActionableViewRefreshesOnFilterChange(t *testing.T) {
	m := NewModel(buildRecommendationFixture("proja"), nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = updated.(Model)
	if m.mode != ViewActionable || len(m.actionableView.plan.Tracks) == 0 {
		t.Fatalf("precondition: actionable view open with tracks (mode %v)", m.mode)
	}
	m.SetFilter("closed") // fixture has no closed issue
	if n := len(m.actionableView.plan.Tracks); n != 0 {
		t.Fatalf("actionable plan has %d tracks after the filter emptied the view", n)
	}
}

func insightIDs(ins analysis.Insights) []string {
	var ids []string
	for _, list := range [][]analysis.InsightItem{ins.Bottlenecks, ins.Keystones, ins.Influencers, ins.Hubs, ins.Authorities, ins.Cores, ins.Slack} {
		for _, it := range list {
			ids = append(ids, it.ID)
		}
	}
	ids = append(ids, ins.Articulation...)
	ids = append(ids, ins.Orphans...)
	for _, c := range ins.Cycles {
		ids = append(ids, c...)
	}
	return ids
}

// A reload under the same filter refreshes an open insights view from the
// visible set instead of installing the snapshot's corpus-wide insights
// (bt-imh final review).
func TestSnapshotReloadKeepsOpenInsightsFiltered(t *testing.T) {
	fixture := func() []model.Issue {
		return append(buildRecommendationFixture("proja"), buildRecommendationFixture("projb")...)
	}
	m := NewModel(fixture(), nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja-", "projb-"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated.(Model)

	snap := NewSnapshotBuilder(fixture()).Build()
	snap.Analysis.WaitForPhase2()
	snap.Phase2Ready = true
	snap.Insights = snap.Analysis.GenerateInsights(len(snap.Issues))
	leaks := false
	for _, id := range insightIDs(snap.Insights) {
		leaks = leaks || ExtractRepoPrefix(id) == "projb"
	}
	if !leaks {
		t.Fatal("precondition: snapshot insights rank projb issues")
	}
	updated, _ = m.Update(SnapshotReadyMsg{Snapshot: snap})
	m = updated.(Model)
	for _, id := range insightIDs(m.insightsPanel.insights) {
		if ExtractRepoPrefix(id) != "proja" {
			t.Fatalf("insights panel lists hidden %s after reload", id)
		}
	}
}

// A reload refreshes an open label dashboard even though the filter is
// unchanged (bt-imh final review).
func TestReloadRefreshesOpenLabelDashboard(t *testing.T) {
	m := NewModel(buildLabelFlowFixture("proja"), nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	m = updated.(Model)

	relabeled := buildLabelFlowFixture("proja")
	for i := range relabeled {
		relabeled[i].Labels = []string{"new-label"}
	}
	m.replaceIssues(relabeled)
	var labels []string
	for _, lh := range m.labelHealthCache.Labels {
		labels = append(labels, lh.Label)
	}
	if len(labels) != 1 || labels[0] != "new-label" {
		t.Fatalf("label dashboard after reload = %v, want [new-label]", labels)
	}
}

// Refreshing an open insights view on reload keeps the user's pane and row.
func TestInsightsReloadKeepsCursor(t *testing.T) {
	issues := append(buildRecommendationFixture("proja"), buildRecommendationFixture("projb")...)
	m := NewModel(issues, nil, "", nil, nil)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = updated.(Model)
	panel := PanelKeystones
	m.insightsPanel.RestoreCursor(panel, 2) // the user moved to row 2 of keystones
	idx := m.insightsPanel.SelectedIndexFor(panel)
	if m.insightsPanel.FocusedPanel() != panel || idx != 2 {
		t.Fatalf("precondition: cursor on keystones row 2, got %v row %d", m.insightsPanel.FocusedPanel(), idx)
	}
	m.replaceIssues(append(buildRecommendationFixture("proja"), buildRecommendationFixture("projb")...))
	if got := m.insightsPanel.FocusedPanel(); got != panel {
		t.Fatalf("focused panel after reload = %v, want %v", got, panel)
	}
	if got := m.insightsPanel.SelectedIndexFor(panel); got != idx {
		t.Fatalf("insights row after reload = %d, want %d", got, idx)
	}
}
