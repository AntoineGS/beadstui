package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/drift"
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

func TestAlertsFollowVisibleSet(t *testing.T) {
	m := newSizedModel(t, filterMatrixFixture(), 140, 40)
	m.alerts = []drift.Alert{
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, IssueID: "proja-1"},
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, IssueID: "proja-2"},
		{Type: drift.AlertIssueCountChange, Severity: drift.SeverityInfo}, // corpus-level
	}
	m.SetFilter("open") // hides closed proja-2
	got := map[string]bool{}
	for _, a := range m.visibleAlerts() {
		got[a.IssueID] = true
	}
	if len(got) != 2 || !got["proja-1"] || !got[""] {
		t.Fatalf("visible alert issue IDs = %v, want proja-1 and the corpus-level alert", got)
	}
}

func TestAlertsBeforeFirstApplyAreNotHidden(t *testing.T) {
	m := newSizedModel(t, filterMatrixFixture(), 140, 40)
	m.filter.visibleIDs = nil
	a := drift.Alert{Type: drift.AlertStale, Severity: drift.SeverityWarning, IssueID: "proja-2"}
	if !m.passesAlertBaseScope(a) {
		t.Fatal("alert hidden before the visible set exists")
	}
}

// rawCorpusReaders are the functions allowed to read m.data.issues directly.
// Everything else must read the visible set (bt-imh). Each entry is a
// deliberate full-data reader from the design's exception list.
var rawCorpusReaders = map[string]string{
	"epic_card.go:handleEpicCardKeys":            "epic progress",
	"epic_card.go:renderEpicCard":                "epic progress",
	"model_alerts.go:notificationRepoScope":      "repo scope of loaded data",
	"model_alerts_header.go:alertsHeaderLines":   "corpus size line",
	"model_alerts_header.go:sourceIssueCounts":   "per-source corpus counts",
	"model_filter.go:applyFilter":                "epic progress index",
	"model_filter.go:applySpec":                  "the engine's input",
	"model_filter.go:updateViewportContent":      "epic progress in details",
	"model.go:Init":                              "history preload needs any data",
	"model.go:replaceIssues":                     "data load",
	"model_modes.go:enterTimeTravelMode":         "time travel",
	"model_modes.go:Issues":                      "full-corpus accessor",
	"model_update_analysis.go:handlePhase2Ready": "computeAlerts and recipe re-sort",
	"model_update_data.go:handleFileChanged":     "data load",
	"model_update_data.go:handleSnapshotReady":   "data load",
	"plugin_host.go:syncPluginsWithHash":         "plugin sync",
	"semantic_search.go:issuesForAsync":          "semantic index",
}

func TestNoRawCorpusReads(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := name + ":" + fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "issues" {
					return true
				}
				inner, ok := sel.X.(*ast.SelectorExpr)
				if !ok || inner.Sel.Name != "data" {
					return true
				}
				if _, allowed := rawCorpusReaders[key]; !allowed {
					t.Errorf("%s reads m.data.issues at %s; read m.filter.visible / m.visibleIssues() or m.applySpec(...) instead (bt-imh)",
						key, fset.Position(sel.Pos()))
				}
				return true
			})
		}
	}
}

func TestFilterConsistencyMatrix(t *testing.T) {
	p01 := &recipe.Recipe{Name: "p01", Filters: recipe.FilterConfig{Priority: []int{0, 1}}}
	openQ := mustBQL(t, "status = open")
	for _, scope := range []map[string]bool{nil, {"proja": true}} {
		for _, primary := range []string{"all", "open", "closed", "ready", "recipe", "bql"} {
			for _, labels := range []string{"", "tests", "docs,ops"} {
				for _, wisps := range []bool{false, true} {
					name := fmt.Sprintf("scope=%v/%s/labels=%q/wisps=%t", scope, primary, labels, wisps)
					t.Run(name, func(t *testing.T) {
						m := newSizedModel(t, filterMatrixFixture(), 140, 40)
						m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"}})
						m.SetActiveRepos(scope)
						m.filter.labelFilter = labels
						m.showWisps = wisps
						switch primary {
						case "recipe":
							m.applyRecipe(p01)
						case "bql":
							m.setActiveRecipe(nil)
							m.applyBQL(openQ, "status = open")
						default:
							m.setActiveRecipe(nil)
							m.filter.activeBQLExpr = nil
							m.SetFilter(primary)
						}
						vis := idsOf(m.filter.visible)
						if got := idsOf(m.FilteredIssues()); fmt.Sprint(got) != fmt.Sprint(vis) {
							t.Fatalf("list %v != visible %v", got, vis)
						}
						if got := m.board.TotalCount(); got != len(vis) {
							t.Fatalf("board total %d != visible %d", got, len(vis))
						}
						m.mode = ViewTree
						m.rebuildTreeForCurrentFilter()
						if got := m.tree.NodeCount(); got != len(vis) {
							t.Fatalf("tree nodes %d != visible %d", got, len(vis))
						}
						m.mode = ViewList
						// Facet: picker counts = labels over Without(DimLabels).
						facet := m.applySpec(m.filterSpec().Without(DimLabels))
						want := map[string]int{}
						for _, iss := range facet {
							for _, l := range iss.Labels {
								want[l]++
							}
						}
						updated, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
						m = updated.(Model)
						for l, n := range want {
							if got := m.labelPicker.labelCounts[l]; got != n {
								t.Fatalf("picker %s = %d, want %d", l, got, n)
							}
						}
						for _, id := range idsOf(m.filterIssuesByLabel("tests")) {
							if _, ok := m.filter.visibleIDs[id]; !ok {
								t.Fatalf("label drilldown includes hidden %s", id)
							}
						}
					})
				}
			}
		}
	}
}

// EXPAND adds related issues after the WHERE clause; they must still pass
// scope, labels and wisps (bt-imh final review).
func TestFilterSpecBQLExpandRespectsBaseDimensions(t *testing.T) {
	issues := filterMatrixFixture()
	env := fixtureEnv(issues)
	q := "id = projb-2 expand up"
	got := idsOf(FilterSpec{BQL: mustBQL(t, q), BQLText: q}.Apply(issues, env))
	if len(got) != 2 {
		t.Fatalf("precondition: expand up from projb-2 = %v, want projb-1 and projb-2", got)
	}
	got = idsOf(FilterSpec{BQL: mustBQL(t, q), BQLText: q, Labels: []string{"ops"}}.Apply(issues, env))
	if len(got) != 1 || got[0] != "projb-2" {
		t.Fatalf("expand under label ops = %v, want only projb-2", got)
	}
	got = idsOf(FilterSpec{BQL: mustBQL(t, q), BQLText: q, Workspace: true, Repos: map[string]bool{"proja": true}}.Apply(issues, env))
	if len(got) != 0 {
		t.Fatalf("expand under proja scope = %v, want none", got)
	}
}

// A cycle is shown only when every member is visible, so insights never list
// a hidden issue (bt-imh final review).
func TestVisibleInsightsDropsPartlyHiddenCycles(t *testing.T) {
	m := NewModel(filterMatrixFixture(), nil, "", nil, nil)
	m.SetFilter("all")
	m.EnableWorkspaceMode(WorkspaceInfo{Enabled: true, RepoCount: 2, RepoPrefixes: []string{"proja", "projb"}})
	m.SetActiveRepos(map[string]bool{"proja": true})
	ins := m.visibleInsights(analysis.Insights{Cycles: [][]string{
		{"proja-1", "proja-3"},
		{"proja-1", "projb-1"},
	}})
	if len(ins.Cycles) != 1 || len(ins.Cycles[0]) != 2 || ins.Cycles[0][1] != "proja-3" {
		t.Fatalf("cycles = %v, want only [proja-1 proja-3]", ins.Cycles)
	}
}
