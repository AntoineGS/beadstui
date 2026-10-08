package ui

import (
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"
)

// filterMatrixFixture is two projects with mixed statuses, labels, one wisp
// and one blocked dependency. Used by the engine and consistency tests.
func filterMatrixFixture() []model.Issue {
	wisp := true
	now := time.Now()
	return []model.Issue{
		{ID: "proja-1", Title: "a1", Status: model.StatusOpen, Priority: 1, Labels: []string{"tests"}, CreatedAt: now, UpdatedAt: now},
		{ID: "proja-2", Title: "a2", Status: model.StatusClosed, Priority: 2, Labels: []string{"tests", "docs"}, CreatedAt: now, UpdatedAt: now},
		{ID: "proja-3", Title: "a3", Status: model.StatusInProgress, Priority: 0, Labels: []string{"docs"}, CreatedAt: now, UpdatedAt: now},
		{ID: "proja-4", Title: "a4 wisp", Status: model.StatusOpen, Priority: 2, Labels: []string{"tests"}, Ephemeral: &wisp, CreatedAt: now, UpdatedAt: now},
		{ID: "projb-1", Title: "b1", Status: model.StatusOpen, Priority: 1, Labels: []string{"tests"}, CreatedAt: now, UpdatedAt: now},
		{ID: "projb-2", Title: "b2", Status: model.StatusBlocked, Priority: 3, Labels: []string{"ops"}, CreatedAt: now, UpdatedAt: now,
			Dependencies: []*model.Dependency{{DependsOnID: "projb-1", Type: model.DepBlocks}}},
	}
}

func fixtureEnv(issues []model.Issue) FilterEnv {
	issueMap := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}
	return FilterEnv{IssueMap: issueMap, BQL: bql.NewMemoryExecutor(), BQLOpts: bql.ExecuteOpts{IssueMap: issueMap}}
}

func idsOf(issues []model.Issue) []string {
	ids := make([]string, len(issues))
	for i := range issues {
		ids[i] = issues[i].ID
	}
	sort.Strings(ids)
	return ids
}

func mustBQL(t *testing.T, q string) *bql.Query {
	t.Helper()
	parsed, err := bql.Parse(q)
	if err != nil {
		t.Fatalf("bql.Parse(%q): %v", q, err)
	}
	if err := bql.Validate(parsed); err != nil {
		t.Fatalf("bql.Validate(%q): %v", q, err)
	}
	return parsed
}

func TestFilterSpecApply(t *testing.T) {
	issues := filterMatrixFixture()
	env := fixtureEnv(issues)
	p01 := &recipe.Recipe{Name: "p01", Filters: recipe.FilterConfig{Priority: []int{0, 1}}}
	for _, tc := range []struct {
		name string
		spec FilterSpec
		want []string
	}{
		{"unfiltered hides wisps", FilterSpec{Status: "all"}, []string{"proja-1", "proja-2", "proja-3", "projb-1", "projb-2"}},
		{"empty status means all", FilterSpec{}, []string{"proja-1", "proja-2", "proja-3", "projb-1", "projb-2"}},
		{"wisps shown", FilterSpec{Status: "all", ShowWisps: true}, []string{"proja-1", "proja-2", "proja-3", "proja-4", "projb-1", "projb-2"}},
		{"scope", FilterSpec{Status: "all", Workspace: true, Repos: map[string]bool{"proja": true}}, []string{"proja-1", "proja-2", "proja-3"}},
		{"scope ignored outside workspace", FilterSpec{Status: "all", Repos: map[string]bool{"proja": true}}, []string{"proja-1", "proja-2", "proja-3", "projb-1", "projb-2"}},
		{"open", FilterSpec{Status: "open"}, []string{"proja-1", "proja-3", "projb-1", "projb-2"}},
		{"in_progress", FilterSpec{Status: "in_progress"}, []string{"proja-3"}},
		{"blocked", FilterSpec{Status: "blocked"}, []string{"projb-2"}},
		{"closed", FilterSpec{Status: "closed"}, []string{"proja-2"}},
		{"ready", FilterSpec{Status: "ready"}, []string{"proja-1", "proja-3", "projb-1"}},
		{"unknown status matches nothing", FilterSpec{Status: "bogus"}, nil},
		{"labels OR", FilterSpec{Status: "all", Labels: []string{"docs", "ops"}}, []string{"proja-2", "proja-3", "projb-2"}},
		{"open + tests + proja (screenshot shape)", FilterSpec{Status: "open", Labels: []string{"tests"}, Workspace: true, Repos: map[string]bool{"proja": true}}, []string{"proja-1"}},
		{"recipe composes with labels", FilterSpec{Recipe: p01, Labels: []string{"tests"}}, []string{"proja-1", "projb-1"}},
		{"recipe ignores status", FilterSpec{Status: "closed", Recipe: p01}, []string{"proja-1", "proja-3", "projb-1"}},
		{"bql composes with labels and wisps", FilterSpec{BQL: mustBQL(t, "status = open"), BQLText: "status = open", Labels: []string{"tests"}}, []string{"proja-1", "projb-1"}},
		{"bql wins over recipe", FilterSpec{Recipe: p01, BQL: mustBQL(t, "status = closed"), BQLText: "status = closed"}, []string{"proja-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := idsOf(tc.spec.Apply(issues, env))
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("got %v, want %v", got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("got %v, want %v", got, want)
				}
			}
		})
	}
}

func TestFilterSpecRecipeOrder(t *testing.T) {
	issues := filterMatrixFixture()
	r := &recipe.Recipe{Name: "by-prio", Sort: recipe.SortConfig{Field: "priority", Direction: "desc"}}
	got := FilterSpec{Recipe: r}.Apply(issues, fixtureEnv(issues))
	for i := 1; i < len(got); i++ {
		if got[i-1].Priority < got[i].Priority {
			t.Fatalf("recipe sort not applied: %v", idsOf(got))
		}
	}
}

func TestFilterSpecWithout(t *testing.T) {
	full := FilterSpec{
		Workspace: true, Repos: map[string]bool{"proja": true}, Status: "open",
		Recipe: &recipe.Recipe{Name: "r"}, BQL: &bql.Query{}, BQLText: "x",
		Labels: []string{"tests"}, ShowWisps: false,
	}
	if s := full.Without(DimScope); s.Repos != nil || s.Status != "open" {
		t.Fatalf("DimScope: %+v", s)
	}
	if s := full.Without(DimPrimary); s.Status != "all" || s.Recipe != nil || s.BQL != nil || s.BQLText != "" || len(s.Labels) != 1 {
		t.Fatalf("DimPrimary: %+v", s)
	}
	if s := full.Without(DimLabels); s.Labels != nil || s.Status != "open" {
		t.Fatalf("DimLabels: %+v", s)
	}
	if s := full.Without(DimWisps); !s.ShowWisps {
		t.Fatalf("DimWisps: %+v", s)
	}
	if s := full.Without(DimPrimary | DimLabels); s.Status != "all" || s.Labels != nil || s.Repos == nil {
		t.Fatalf("combined: %+v", s)
	}
	if full.Labels == nil || full.Recipe == nil {
		t.Fatal("Without mutated the receiver")
	}
}

func TestFilterSpecIsUnfiltered(t *testing.T) {
	for _, tc := range []struct {
		spec FilterSpec
		want bool
	}{
		{FilterSpec{}, true},
		{FilterSpec{Status: "all"}, true},
		{FilterSpec{Status: "all", ShowWisps: false}, true},
		{FilterSpec{Status: "all", Repos: map[string]bool{"a": true}}, true}, // not workspace
		{FilterSpec{Status: "all", Workspace: true, Repos: map[string]bool{"a": true}}, false},
		{FilterSpec{Status: "open"}, false},
		{FilterSpec{Recipe: &recipe.Recipe{Name: "r"}}, false},
		{FilterSpec{BQL: &bql.Query{}}, false},
		{FilterSpec{Status: "all", Labels: []string{"x"}}, false},
	} {
		if got := tc.spec.IsUnfiltered(); got != tc.want {
			t.Errorf("IsUnfiltered(%+v) = %t, want %t", tc.spec, got, tc.want)
		}
	}
}

func TestFilterSpecKey(t *testing.T) {
	r := &recipe.Recipe{Name: "triage"}
	// cmd/bt starts a recipe with Status "all"; the snapshot later renames
	// currentFilter to "recipe:<name>". Same recipe, same key.
	if a, b := (FilterSpec{Status: "all", Recipe: r}).Key(), (FilterSpec{Status: "recipe:triage", Recipe: r}).Key(); a != b {
		t.Fatalf("recipe key depends on status: %q vs %q", a, b)
	}
	if a, b := (FilterSpec{Labels: []string{"b", "a"}}).Key(), (FilterSpec{Labels: []string{"a", "b"}}).Key(); a != b {
		t.Fatalf("label order changed key: %q vs %q", a, b)
	}
	scoped := FilterSpec{Workspace: true, Repos: map[string]bool{"b": true, "a": true, "c": false}}
	if scoped.Key() != (FilterSpec{Workspace: true, Repos: map[string]bool{"a": true, "b": true}}).Key() {
		t.Fatal("disabled repo entries or map order changed key")
	}
	distinct := []FilterSpec{
		{}, {Status: "open"}, {Labels: []string{"a"}}, {ShowWisps: true},
		{Workspace: true, Repos: map[string]bool{"a": true}},
		{Recipe: r}, {BQL: &bql.Query{}, BQLText: "status = open"},
	}
	seen := map[string]int{}
	for i, s := range distinct {
		if j, dup := seen[s.Key()]; dup {
			t.Fatalf("specs %d and %d share key %q", j, i, s.Key())
		}
		seen[s.Key()] = i
	}
	if (FilterSpec{ShowWisps: true}).Without(DimWisps).Key() != (FilterSpec{}).Without(DimWisps).Key() {
		t.Fatal("Without(DimWisps) must erase the wisp setting from the key")
	}
}

func TestRecipeComposesWithLabelFilterInList(t *testing.T) {
	m := NewModel(filterMatrixFixture(), nil, "", nil, nil)
	r := &recipe.Recipe{Name: "p01", Filters: recipe.FilterConfig{Priority: []int{0, 1}}}
	m.filter.labelFilter = "tests"
	m.setActiveRecipe(r)
	m.applyRecipe(r)
	got := idsOf(m.FilteredIssues())
	if len(got) != 2 || got[0] != "proja-1" || got[1] != "projb-1" {
		t.Fatalf("recipe+label list = %v, want [proja-1 projb-1]", got)
	}
	if b := m.board.TotalCount(); b != 2 {
		t.Fatalf("board total = %d, want 2 (list and board must agree)", b)
	}
}

func TestBQLHidesWispsInList(t *testing.T) {
	m := NewModel(filterMatrixFixture(), nil, "", nil, nil)
	q := mustBQL(t, "status = open")
	m.applyBQL(q, "status = open")
	for _, iss := range m.FilteredIssues() {
		if iss.ID == "proja-4" {
			t.Fatal("BQL result included a hidden wisp")
		}
	}
}

func TestVisibleSetMatchesList(t *testing.T) {
	m := NewModel(filterMatrixFixture(), nil, "", nil, nil)
	m.SetFilter("open")
	list := idsOf(m.FilteredIssues())
	vis := idsOf(m.filter.visible)
	if len(list) != len(vis) {
		t.Fatalf("list %v != visible %v", list, vis)
	}
	for i := range list {
		if list[i] != vis[i] {
			t.Fatalf("list %v != visible %v", list, vis)
		}
	}
	if len(m.filter.visibleIDs) != len(vis) {
		t.Fatalf("visibleIDs has %d entries, want %d", len(m.filter.visibleIDs), len(vis))
	}
}

func TestCanUseSnapshotRequiresMatchingCount(t *testing.T) {
	issues := filterMatrixFixture() // 6 issues, one wisp
	m := NewModel(issues, nil, "", nil, nil)
	m.data.snapshot = &DataSnapshot{Issues: issues}
	spec := FilterSpec{Status: "all"} // wisps hidden
	if m.canUseSnapshot(spec, 5) {
		t.Fatal("snapshot reused while hidden wisps make the visible set smaller")
	}
	if !m.canUseSnapshot(spec, 6) {
		t.Fatal("snapshot not reused for an unfiltered, full-size visible set")
	}
	if m.canUseSnapshot(FilterSpec{Status: "open"}, 6) {
		t.Fatal("snapshot reused under a status filter")
	}
}

func TestWispToggleKeepsCursor(t *testing.T) {
	m := newSizedModel(t, filterMatrixFixture(), 140, 40)
	m.list.Select(2)
	m.toggleWisps()
	if got := m.list.Index(); got != 2 {
		t.Fatalf("cursor after wisp toggle = %d, want 2", got)
	}
}

func TestEpicProgressIndexMatchesEpicProgress(t *testing.T) {
	issues := epicProgressFixture()
	idx := epicProgressIndex(issues)
	for _, iss := range issues {
		if iss.IssueType != model.TypeEpic {
			continue
		}
		done, total := epicProgress(iss.ID, issues)
		if got := idx[iss.ID]; got.done != done || got.total != total {
			t.Errorf("%s: index %+v, epicProgress (%d,%d)", iss.ID, got, done, total)
		}
	}
}

// A tree rebuilt from the visible set (not the snapshot) keeps its selection
// on refresh, like BuildFromSnapshot does (bt-imh).
func TestTreeRefreshUnderFilterKeepsSelection(t *testing.T) {
	issues := []model.Issue{
		{ID: "parent", Title: "Parent", Status: model.StatusOpen, IssueType: model.TypeTask},
		{ID: "child", Title: "Child", Status: model.StatusOpen, IssueType: model.TypeTask,
			Dependencies: []*model.Dependency{{DependsOnID: "parent", Type: model.DepParentChild}}},
		{ID: "done", Title: "Done", Status: model.StatusClosed, IssueType: model.TypeTask},
	}
	m := newSizedModel(t, issues, 140, 40)
	m.tree.SetBeadsDir(t.TempDir())
	m.mode = ViewTree
	m.SetFilter("open")
	if !m.tree.SelectByID("child") {
		t.Fatal("precondition: child not in tree")
	}
	m.applyFilter() // a refresh under the same filter
	if sel := m.tree.SelectedIssue(); sel == nil || sel.ID != "child" {
		t.Fatalf("tree selection after refresh = %v, want child", sel)
	}
}

// cmd/bt sets the startup project scope through SetActiveRepos and never
// calls applyFilter, so the setter itself must refresh the visible set.
func TestSetActiveReposAppliesScope(t *testing.T) {
	m := NewModel(filterMatrixFixture(), nil, "", nil, nil)
	m.SetFilter("all")
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	got := idsOf(m.FilteredIssues())
	if len(got) != 3 || got[0] != "proja-1" || got[2] != "proja-3" {
		t.Fatalf("list after SetActiveRepos = %v, want proja-1..3", got)
	}
}

func TestSnapshotReloadKeepsStatusFilter(t *testing.T) {
	issues := filterMatrixFixture()
	for _, status := range []string{"blocked", "in_progress", "deferred", "open", "ready"} {
		t.Run(status, func(t *testing.T) {
			m := newSizedModel(t, issues, 140, 40)
			m.SetFilter(status)
			before := idsOf(m.FilteredIssues())
			snap := NewSnapshotBuilder(filterMatrixFixture()).Build()
			updated, _ := m.Update(SnapshotReadyMsg{Snapshot: snap})
			m = updated.(Model)
			after := idsOf(m.FilteredIssues())
			if len(before) != len(after) {
				t.Fatalf("status %s: before reload %v, after %v", status, before, after)
			}
			for i := range before {
				if before[i] != after[i] {
					t.Fatalf("status %s: before reload %v, after %v", status, before, after)
				}
			}
		})
	}
}

// TestLabelPickerCountsFollowStatusFilter is the reported case (bt-imh):
// project scoped, status open, label tests -> list of N and picker tests (N).
func TestLabelPickerCountsFollowStatusFilter(t *testing.T) {
	m := newSizedModel(t, filterMatrixFixture(), 140, 40)
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	m.filter.currentFilter = "open"
	m.filter.labelFilter = "tests"
	m.applyFilter()
	if n := len(m.FilteredIssues()); n != 1 {
		t.Fatalf("list = %v, want only proja-1", idsOf(m.FilteredIssues()))
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = updated.(Model)
	if got := m.labelPicker.labelCounts["tests"]; got != 1 {
		t.Fatalf("picker tests count = %d, want 1 (matches the list)", got)
	}
	// Facet rule: other labels count with every filter except labels.
	// proja open, non-wisp: proja-1 (tests), proja-3 (docs).
	if got := m.labelPicker.labelCounts["docs"]; got != 1 {
		t.Fatalf("picker docs count = %d, want 1", got)
	}
	if _, ok := m.labelPicker.labelCounts["ops"]; ok {
		t.Fatal("projb-only label listed while scoped to proja")
	}
}

func TestBoardHintTotalIsProjectScope(t *testing.T) {
	m := newSizedModel(t, filterMatrixFixture(), 140, 40)
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	m.mode = ViewBoard
	m.SetFilter("open")
	// proja non-wisp issues: proja-1, proja-2, proja-3.
	if hint := m.extractHintText(); !strings.Contains(hint, "[open:2/3]") {
		t.Fatalf("board hint = %q, want [open:2/3]", hint)
	}
}

func TestEpicsIgnoreStatusButFollowLabels(t *testing.T) {
	m := newSizedModel(t, epicProgressFixture(), 140, 40)
	m.SetFilter("open")
	got := m.applySpec(m.filterSpec().Without(DimPrimary))
	closed := 0
	for _, iss := range got {
		if isClosedLikeStatus(iss.Status) {
			closed++
		}
	}
	if closed == 0 {
		t.Fatal("epics source dropped closed children under the open filter")
	}
}
