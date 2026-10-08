# Central Filter Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

<!-- Related: bt-imh -->

**Goal:** Every TUI display and count in `pkg/ui` derives from one filtered issue set, produced by one filter engine.

**Architecture:** A pure `FilterSpec` value (`pkg/ui/filterset.go`) describes the active filter in four dimensions (scope, primary, labels, wisps) and evaluates it with `Apply`. The model derives the spec from its existing filter state, stores the result once per filter change or data load in `m.filter.visible`, and every surface reads that slice. Facet-style views (label picker, epics, board hint) evaluate `spec.Without(dim)` instead.

**Tech Stack:** Go 1.25, Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles list, `pkg/bql`, `pkg/recipe`, `pkg/analysis`.

**Spec:** `docs/design/2026-10-08-bt-imh-central-filter-engine.md`

## Global Constraints

- Work in a worktree: `git worktree add .claude/worktrees/bt-imh -b feat/bt-imh-filter-engine` from `main`. All paths below are relative to it.
- After every code change: `go build ./...` and `go vet ./...` must pass.
- `go test ./pkg/ui/` has pre-existing failures. Before Task 1, record them: `go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort > _tmp/bt-imh-baseline.txt`. A task is green when its own tests pass and `go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -` shows no new lines.
- Only two new files: `pkg/ui/filterset.go` and `pkg/ui/filterset_test.go`. Everything else edits existing files.
- No backwards-compatibility shims: replaced functions are deleted, not wrapped.
- Commit format: `type(tui): description (bt-imh)`. Commit with `git commit --only <paths>` (run `git add` first for new files).
- Subagents never run `go install`.
- Robot mode (`cmd/bt`, `bt robot ...`) is out of scope. The `/` fuzzy search stays a list-only refinement.
- Deliberate full-data readers (spec section "Deliberate full-data readers") keep reading `m.data.issues`: graph metrics, epic progress, the corpus line in the alerts header, `computeAlerts`, ID lookups (details, dependency navigation, plugin sync, semantic index, time travel, events diff).

## Review Focus

1. **Hidden wisps with an otherwise unfiltered spec.** Snapshots include wisps, so reusing a precomputed board/graph/tree when wisps are hidden would show them. Expected: snapshot reuse only when the visible count equals the snapshot's issue count. Test: `TestCanUseSnapshotRequiresMatchingCount` (Task 2).
2. **Background snapshot reload while status is `blocked`, `in_progress` or `deferred`.** The old snapshot path had no cases for these and emptied the list. Expected: the list keeps exactly the filtered issues after a reload. Test: `TestSnapshotReloadKeepsStatusFilter` (Task 3).
3. **An applied label that has no issues under the other filters.** Expected: it stays listed in the picker with count 0 and stays selected, so Enter doesn't silently drop it. Test: existing `TestLabelPickerRespectsProjectScope` keeps passing, plus `TestLabelPickerCountsFollowStatusFilter` (Task 4).
4. **Alerts before the first apply** (`visibleIDs` nil, e.g. models built without `NewModel`). Expected: nothing is hidden. Test: `TestAlertsBeforeFirstApplyAreNotHidden` (Task 6).
5. **Toggling wisps does not count as a new filter for the cursor** (bt-qc3 keeps the cursor on refresh, resets it on a new filter). Expected: wisp toggle keeps the cursor row. Test: `TestWispToggleKeepsCursor` (Task 2).

---

### Task 1: The filter engine

**Files:**
- Create: `pkg/ui/filterset.go`
- Create: `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: existing `IssueRepoKey(model.Issue) string` (`item.go`), `isClosedLikeStatus(model.Status) bool`, `issueMatchesRecipe(model.Issue, map[string]*model.Issue, *recipe.Recipe) bool` and `sortIssuesByRecipe([]model.Issue, *analysis.GraphStats, *recipe.Recipe)` (`snapshot.go`), `(*bql.MemoryExecutor).Execute(*bql.Query, []model.Issue, bql.ExecuteOpts) []model.Issue`.
- Produces:
  - `type FilterDim uint8` with `DimScope`, `DimPrimary`, `DimLabels`, `DimWisps`
  - `type FilterSpec struct { Workspace bool; Repos map[string]bool; Status string; Recipe *recipe.Recipe; BQL *bql.Query; BQLText string; Labels []string; ShowWisps bool }`
  - `type FilterEnv struct { IssueMap map[string]*model.Issue; BQL *bql.MemoryExecutor; BQLOpts bql.ExecuteOpts; Stats *analysis.GraphStats }`
  - `func (s FilterSpec) Apply(issues []model.Issue, env FilterEnv) []model.Issue`
  - `func (s FilterSpec) Without(d FilterDim) FilterSpec`
  - `func (s FilterSpec) IsUnfiltered() bool`
  - `func (s FilterSpec) Key() string`
  - `func matchesStatusFilter(issue model.Issue, status string, issueMap map[string]*model.Issue) bool`

- [ ] **Step 1: Write the failing engine tests**

Create `pkg/ui/filterset_test.go`:

```go
package ui

import (
	"sort"
	"testing"
	"time"

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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/ui/ -run 'TestFilterSpec' 2>&1 | head -20`
Expected: build failure, `undefined: FilterSpec` (and `FilterEnv`, `DimScope`, ...).

- [ ] **Step 3: Implement the engine**

Create `pkg/ui/filterset.go`:

```go
package ui

import (
	"sort"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"
)

// FilterDim is one independent dimension of the active filter. See
// docs/design/2026-10-08-bt-imh-central-filter-engine.md.
type FilterDim uint8

const (
	DimScope   FilterDim = 1 << iota // workspace project selection (w)
	DimPrimary                       // exactly one of: status | ready | recipe | BQL
	DimLabels                        // l picker, OR across selected labels
	DimWisps                         // ephemeral visibility
)

// FilterSpec is the whole active filter. Apply is the only place that decides
// which issues are in view (bt-imh).
type FilterSpec struct {
	Workspace bool            // Repos only applies in workspace mode
	Repos     map[string]bool // nil = all projects
	Status    string          // "", "all", "open", "in_progress", "blocked", "deferred", "closed", "ready"
	Recipe    *recipe.Recipe  // set => Status ignored
	BQL       *bql.Query      // set => Status and Recipe ignored
	BQLText   string          // the query as typed; identifies BQL in Key
	Labels    []string
	ShowWisps bool
}

// FilterEnv is what Apply needs beyond the issues themselves.
type FilterEnv struct {
	IssueMap map[string]*model.Issue // ready / actionable blocker lookups
	BQL      *bql.MemoryExecutor
	BQLOpts  bql.ExecuteOpts
	Stats    *analysis.GraphStats // recipe sort order
}

// Apply returns the issues that pass every dimension, in the primary
// filter's order: recipe sort, BQL ORDER BY, else input order.
func (s FilterSpec) Apply(issues []model.Issue, env FilterEnv) []model.Issue {
	out := make([]model.Issue, 0, len(issues))
	for _, issue := range issues {
		if s.passesBase(issue) && s.passesPrimary(issue, env.IssueMap) {
			out = append(out, issue)
		}
	}
	switch {
	case s.BQL != nil:
		if env.BQL != nil {
			out = env.BQL.Execute(s.BQL, out, env.BQLOpts)
		}
	case s.Recipe != nil:
		sortIssuesByRecipe(out, env.Stats, s.Recipe)
	}
	return out
}

func (s FilterSpec) passesBase(issue model.Issue) bool {
	if s.Workspace && s.Repos != nil {
		if key := IssueRepoKey(issue); key != "" && !s.Repos[key] {
			return false
		}
	}
	if !s.ShowWisps && issue.Ephemeral != nil && *issue.Ephemeral {
		return false
	}
	if len(s.Labels) > 0 && !hasAnyLabel(issue, s.Labels) {
		return false
	}
	return true
}

func (s FilterSpec) passesPrimary(issue model.Issue, issueMap map[string]*model.Issue) bool {
	switch {
	case s.BQL != nil:
		return true // BQL runs over the whole surviving set in Apply
	case s.Recipe != nil:
		return issueMatchesRecipe(issue, issueMap, s.Recipe)
	default:
		return matchesStatusFilter(issue, s.Status, issueMap)
	}
}

// Without returns a copy with the given dimensions cleared, for facet counts
// and scope totals.
func (s FilterSpec) Without(d FilterDim) FilterSpec {
	if d&DimScope != 0 {
		s.Repos = nil
	}
	if d&DimPrimary != 0 {
		s.Status, s.Recipe, s.BQL, s.BQLText = "all", nil, nil, ""
	}
	if d&DimLabels != 0 {
		s.Labels = nil
	}
	if d&DimWisps != 0 {
		s.ShowWisps = true
	}
	return s
}

// IsUnfiltered reports whether no dimension narrows the corpus. Wisps are not
// part of it: snapshots include wisps, so callers that reuse a snapshot also
// compare counts.
func (s FilterSpec) IsUnfiltered() bool {
	scoped := s.Workspace && s.Repos != nil
	return !scoped && s.status() == "all" && s.Recipe == nil && s.BQL == nil && len(s.Labels) == 0
}

// Key identifies the filter. Equal keys select the same issues from the same
// data.
func (s FilterSpec) Key() string {
	primary := "status:" + s.status()
	switch {
	case s.BQL != nil:
		primary = "bql:" + s.BQLText
	case s.Recipe != nil:
		primary = "recipe:" + s.Recipe.Name
	}
	scope := "*"
	if s.Workspace && s.Repos != nil {
		var repos []string
		for repo, on := range s.Repos {
			if on {
				repos = append(repos, repo)
			}
		}
		sort.Strings(repos)
		scope = strings.Join(repos, ",")
	}
	labels := append([]string(nil), s.Labels...)
	sort.Strings(labels)
	wisps := "wisps:hidden"
	if s.ShowWisps {
		wisps = "wisps:shown"
	}
	return strings.Join([]string{primary, strings.Join(labels, ","), scope, wisps}, "\x00")
}

func (s FilterSpec) status() string {
	if s.Status == "" {
		return "all"
	}
	return s.Status
}

func hasAnyLabel(issue model.Issue, labels []string) bool {
	for _, want := range labels {
		for _, l := range issue.Labels {
			if l == want {
				return true
			}
		}
	}
	return false
}

// matchesStatusFilter is the status / ready primary filter.
func matchesStatusFilter(issue model.Issue, status string, issueMap map[string]*model.Issue) bool {
	switch status {
	case "", "all":
		return true
	case "open":
		return !isClosedLikeStatus(issue.Status)
	case "in_progress":
		return issue.Status == model.StatusInProgress
	case "blocked":
		return issue.Status == model.StatusBlocked
	case "deferred":
		return issue.Status == model.StatusDeferred
	case "closed":
		return isClosedLikeStatus(issue.Status)
	case "ready":
		// Ready = not closed, not blocked, and no open blocking dependency.
		if isClosedLikeStatus(issue.Status) || issue.Status == model.StatusBlocked {
			return false
		}
		for _, dep := range issue.Dependencies {
			if dep == nil || !dep.Type.IsBlocking() {
				continue
			}
			if blocker, ok := issueMap[dep.DependsOnID]; ok && !isClosedLikeStatus(blocker.Status) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pkg/ui/ -run 'TestFilterSpec' -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|ok|FAIL)' | tail -30`
Expected: every `TestFilterSpec*` subtest PASS. (`status = open` is valid BQL; `slot_bql_test.go` uses it.)

- [ ] **Step 5: Build, vet, commit**

```bash
go build ./... && go vet ./pkg/ui/
git add pkg/ui/filterset.go pkg/ui/filterset_test.go
git commit --only pkg/ui/filterset.go pkg/ui/filterset_test.go -m "feat(tui): add FilterSpec filter engine (bt-imh)"
```

---

### Task 2: One visible set and one apply path

**Files:**
- Modify: `pkg/ui/model.go` (`FilterState` struct ~line 706; end of `NewModel` ~line 1628-1631)
- Modify: `pkg/ui/model_filter.go` (`setListItems` cursor check ~81, `filterKey` ~89-110, `matchesCurrentFilter` ~267-334, `filteredIssuesForActiveView` ~349-398, `refreshBoardAndGraphForCurrentFilter` ~431-470, `rebuildTreeForCurrentFilter` ~479-492, `applyFilter` ~507-574, `applyRecipe` ~790-1029, `applyBQL` ~1897-1938)
- Modify: `pkg/ui/helpers.go` (add `epicProgressIndex` after `epicProgress` ~line 110)
- Modify: `pkg/ui/model_footer.go` (~line 460, legacy `label:` case)
- Modify: `pkg/ui/model_update_input.go` (~line 1168, actionable view)
- Modify: `pkg/ui/model_update_analysis.go` (~line 433, `recomputePriorityHints`)
- Modify: `pkg/ui/list_navigation_test.go` (`TestFilterKeySameForRecipeStartupAndApplied`)
- Test: `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: Task 1's `FilterSpec`, `FilterEnv`, `Dim*`.
- Produces (later tasks rely on these exact names):
  - `FilterState` fields: `visible []model.Issue` (list order), `visibleIDs map[string]struct{}`, `visibleKey string`, `scopeCount int`
  - `func (m *Model) filterSpec() FilterSpec`
  - `func (m *Model) filterEnv() FilterEnv`
  - `func (m *Model) applySpec(spec FilterSpec) []model.Issue`
  - `func (m *Model) refreshVisible()`
  - `func (m *Model) visibleIssues() []model.Issue`
  - `func (m *Model) cursorKey() string`
  - `func (m *Model) canUseSnapshot(spec FilterSpec, visibleCount int) bool`
  - `func (m *Model) buildIssueItem(issue model.Issue, epics map[string]epicProgressCount) IssueItem`
  - `type epicProgressCount struct{ done, total int }` and `func epicProgressIndex(allIssues []model.Issue) map[string]epicProgressCount`
  - `applyRecipe(r)` / `applyBQL(q, s)` remain with the same signatures but only set state and call `applyFilter()`.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/ui/filterset_test.go` (no new imports needed):

```go
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
```

Update `pkg/ui/list_navigation_test.go` `TestFilterKeySameForRecipeStartupAndApplied`: replace both `m.filterKey()` calls with `m.cursorKey()`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./pkg/ui/ -run 'TestRecipeComposes|TestBQLHidesWisps|TestVisibleSetMatchesList|TestCanUseSnapshot|TestWispToggleKeepsCursor|TestEpicProgressIndex|TestFilterKeySame' 2>&1 | head -20`
Expected: build failure (`m.filter.visible undefined`, `canUseSnapshot undefined`, `epicProgressIndex undefined`, `cursorKey undefined`).

- [ ] **Step 3: Add the visible-set fields**

In `pkg/ui/model.go`, `FilterState`, replace the `appliedKey` line and add the new fields:

```go
	appliedKey    string     // cursorKey of the list's current contents (bt-qc3)

	// The visible set (bt-imh): every display and count reads these. Set by
	// refreshVisible and reordered to list order by applyFilter.
	visible    []model.Issue
	visibleIDs map[string]struct{}
	visibleKey string // FilterSpec.Key of visible
	scopeCount int    // issues in project scope, ignoring primary and labels
}
```

- [ ] **Step 4: Add epicProgressIndex**

In `pkg/ui/helpers.go`, after `epicProgress`:

```go
type epicProgressCount struct{ done, total int }

// epicProgressIndex is epicProgress for every parent in one pass.
func epicProgressIndex(allIssues []model.Issue) map[string]epicProgressCount {
	out := make(map[string]epicProgressCount)
	for i := range allIssues {
		deps := allIssues[i].Dependencies
		for j, dep := range deps {
			if dep == nil || dep.Type != model.DepParentChild || parentSeen(deps[:j], dep.DependsOnID) {
				continue
			}
			c := out[dep.DependsOnID]
			c.total++
			if allIssues[i].Status.IsClosed() {
				c.done++
			}
			out[dep.DependsOnID] = c
		}
	}
	return out
}

func parentSeen(deps []*model.Dependency, parentID string) bool {
	for _, d := range deps {
		if d != nil && d.Type == model.DepParentChild && d.DependsOnID == parentID {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Replace the filter plumbing in model_filter.go**

1. Delete `filterKey()` (lines ~87-110) and add in its place:

```go
// filterSpec derives the engine's spec from the model's filter state.
func (m *Model) filterSpec() FilterSpec {
	spec := FilterSpec{
		Workspace: m.workspaceMode,
		Repos:     m.activeRepos,
		Status:    m.filter.currentFilter,
		ShowWisps: m.showWisps,
	}
	if m.filter.labelFilter != "" {
		spec.Labels = strings.Split(m.filter.labelFilter, ",")
	}
	switch {
	case m.filter.activeBQLExpr != nil && strings.HasPrefix(m.filter.currentFilter, "bql:"):
		spec.BQL = m.filter.activeBQLExpr
		spec.BQLText = strings.TrimPrefix(m.filter.currentFilter, "bql:")
	case m.filter.activeRecipe != nil:
		spec.Recipe = m.filter.activeRecipe
	}
	return spec
}

func (m *Model) filterEnv() FilterEnv {
	return FilterEnv{
		IssueMap: m.data.issueMap,
		BQL:      m.filter.bqlEngine,
		BQLOpts:  m.bqlExecuteOpts(),
		Stats:    m.data.analysis,
	}
}

// applySpec evaluates a variant of the active filter (facet counts, scope
// totals) against the full corpus.
func (m *Model) applySpec(spec FilterSpec) []model.Issue {
	return spec.Apply(m.data.issues, m.filterEnv())
}

// refreshVisible recomputes the visible set from the active filter.
func (m *Model) refreshVisible() {
	spec := m.filterSpec()
	m.filter.visible = m.applySpec(spec)
	m.filter.visibleIDs = make(map[string]struct{}, len(m.filter.visible))
	for i := range m.filter.visible {
		m.filter.visibleIDs[m.filter.visible[i].ID] = struct{}{}
	}
	m.filter.visibleKey = spec.Key()
	m.filter.scopeCount = len(m.applySpec(spec.Without(DimPrimary | DimLabels)))
}

// visibleIssues is the visible set in list order.
func (m *Model) visibleIssues() []model.Issue {
	return m.filter.visible
}

// cursorKey identifies a filter for bt-qc3 (a new filter starts at the first
// row). Toggling wisps is a refresh, not a new filter.
func (m *Model) cursorKey() string {
	return m.filterSpec().Without(DimWisps).Key()
}

// canUseSnapshot reports whether the precomputed board/graph/tree in the
// snapshot shows exactly the visible set. The count check catches hidden
// wisps, which snapshots include.
func (m *Model) canUseSnapshot(spec FilterSpec, visibleCount int) bool {
	s := m.data.snapshot
	if s == nil || visibleCount != len(s.Issues) {
		return false
	}
	if spec.Recipe != nil && spec.BQL == nil {
		return spec.Without(DimPrimary).IsUnfiltered() &&
			s.RecipeName == spec.Recipe.Name && s.RecipeHash == recipeFingerprint(spec.Recipe)
	}
	return spec.IsUnfiltered()
}

// buildIssueItem is the one place list items are built from issues.
func (m *Model) buildIssueItem(issue model.Issue, epics map[string]epicProgressCount) IssueItem {
	item := IssueItem{
		Issue:      issue,
		DiffStatus: m.getDiffStatus(issue.ID),
		RepoPrefix: ExtractRepoPrefix(issue.ID),
	}
	if m.data.analysis != nil {
		item.GraphScore = m.data.analysis.GetPageRankScore(issue.ID)
		item.Impact = m.data.analysis.GetCriticalPathScore(issue.ID)
	}
	item.TriageScore = m.ac.triageScores[issue.ID]
	if reasons, ok := m.ac.triageReasons[issue.ID]; ok {
		item.TriageReason = reasons.Primary
		item.TriageReasons = reasons.All
	}
	item.IsQuickWin = m.ac.quickWinSet[issue.ID]
	item.IsBlocker = m.ac.blockerSet[issue.ID]
	item.UnblocksCount = len(m.ac.unblocksMap[issue.ID])
	if issue.IssueType == model.TypeEpic {
		c := epics[issue.ID]
		item.EpicDone, item.EpicTotal = c.done, c.total
	}
	item.GateAwaitType = gateAwaitFromBlockers(issue, m.data.issueMap)
	return item
}
```

2. In `setListItems`, change the cursor check to:

```go
	if key := m.cursorKey(); key != m.filter.appliedKey {
```

3. Delete `matchesCurrentFilter` (lines ~267-334) entirely.

4. Delete `filteredIssuesForActiveView` (lines ~349-398) entirely.

5. Replace `refreshBoardAndGraphForCurrentFilter` (lines ~431-470) with:

```go
func (m *Model) refreshBoardAndGraphForCurrentFilter() {
	if m.mode != ViewBoard && m.mode != ViewGraph {
		return
	}
	m.refreshBoardAndGraph(m.filter.visible)
}

// refreshBoardAndGraph feeds the board and graph from the visible set,
// reusing the snapshot's precomputed layout only when it shows the same set.
func (m *Model) refreshBoardAndGraph(issues []model.Issue) {
	snap := m.canUseSnapshot(m.filterSpec(), len(issues))
	if snap && m.data.snapshot.BoardState != nil {
		m.board.SetSnapshot(m.data.snapshot)
	} else {
		m.board.SetIssues(issues)
	}
	if snap && m.data.snapshot.GraphLayout != nil {
		m.graphView.SetSnapshot(m.data.snapshot)
	} else {
		var ins analysis.Insights
		if m.data.analysis != nil {
			ins = m.data.analysis.GenerateInsights(len(issues))
		}
		m.graphView.SetIssues(issues, &ins)
	}
}
```

6. Replace `rebuildTreeForCurrentFilter` (lines ~472-492, including its doc comment) with:

```go
// rebuildTreeForCurrentFilter rebuilds the tree view from the visible set
// (bt-imh). A child whose parent is filtered out becomes a root. No-op outside
// the tree view.
func (m *Model) rebuildTreeForCurrentFilter() {
	if m.mode != ViewTree {
		return
	}
	if m.canUseSnapshot(m.filterSpec(), len(m.filter.visible)) {
		m.tree.BuildFromSnapshot(m.data.snapshot)
		return
	}
	m.tree.Build(m.filter.visible)
}
```

7. Replace `applyFilter` (lines ~507-574) with:

```go
// applyFilter is the single apply path (bt-imh): it refreshes the visible set
// and feeds every surface from it. Recipes and BQL go through it too.
func (m *Model) applyFilter() {
	m.refreshVisible()

	issues := append([]model.Issue(nil), m.filter.visible...)
	epics := epicProgressIndex(m.data.issues) // epic progress counts all children
	items := make([]list.Item, len(issues))
	for i := range issues {
		items[i] = m.buildIssueItem(issues[i], epics)
	}
	// Recipes and BQL keep their own order; the s sort mode applies otherwise.
	if spec := m.filterSpec(); spec.Recipe == nil && spec.BQL == nil {
		m.sortFilteredItems(items, issues)
	}
	m.filter.visible = issues

	m.setListItems(items)
	m.updateSemanticIDs(items)
	m.refreshBoardAndGraph(issues)
	m.rebuildTreeForCurrentFilter()
	m.refreshEpicsForCurrentFilter()
	if m.ac.showPriorityHints {
		m.recomputePriorityHints()
	}

	if len(items) > 0 && m.list.Index() >= len(items) {
		m.list.Select(0)
	}
	m.updateViewportContent()
}
```

8. Replace `applyRecipe` (lines ~789-1029, the whole function including the inline matcher and sort) with:

```go
// applyRecipe makes r the primary filter and applies it.
func (m *Model) applyRecipe(r *recipe.Recipe) {
	if r == nil {
		return
	}
	if m.filter.activeRecipe != r {
		m.setActiveRecipe(r)
	}
	m.filter.activeBQLExpr = nil
	m.filter.currentFilter = "recipe:" + r.Name
	m.applyFilter()
}
```

9. Replace `applyBQL` (lines ~1897-1938, including its doc comment) with:

```go
// applyBQL makes query the primary filter and applies it. BQL runs after the
// per-issue dimensions inside FilterSpec.Apply because of ORDER BY / EXPAND.
func (m *Model) applyBQL(query *bql.Query, queryStr string) {
	m.filter.activeBQLExpr = query
	m.filter.currentFilter = "bql:" + queryStr
	m.applyFilter()
}
```

10. Remove imports that are now unused in `model_filter.go` (the compiler will name them; `go build` tells you).

- [ ] **Step 6: Point the remaining readers at the visible set**

- `pkg/ui/model_update_input.go` ~line 1168: `analyzer := analysis.NewAnalyzer(m.filteredIssuesForActiveView())` becomes `analyzer := analysis.NewAnalyzer(m.visibleIssues())`. Update the comment above it to say "from the visible set (bt-imh)".
- `pkg/ui/model_update_analysis.go` ~line 433 in `recomputePriorityHints`: `issues := m.filteredIssuesForActiveView()` becomes `issues := m.visibleIssues()`. In its doc comment replace `filteredIssuesForActiveView()` with `the visible set`.
- `pkg/ui/model_filter.go` `reapplyActiveFilter` (still present until Task 3): leave its body unchanged; `applyRecipe`/`applyBQL` now route through `applyFilter`, so it stays correct.
- `pkg/ui/model_footer.go` ~line 460: delete the `case strings.HasPrefix(cf, "label:"):` branch and its two comment lines. Nothing sets `label:` any more.

- [ ] **Step 7: Always apply at startup**

In `pkg/ui/model.go` at the end of `NewModel` replace:

```go
	if startFilter != "all" {
		m.applyFilter()
	}
	m.filter.appliedKey = m.filterKey()
```

with:

```go
	m.applyFilter()
	m.filter.appliedKey = m.cursorKey()
```

- [ ] **Step 8: Run the new tests and the filter regression tests**

Run: `go test ./pkg/ui/ -run 'TestFilterSpec|TestRecipeComposes|TestBQLHidesWisps|TestVisibleSetMatchesList|TestCanUseSnapshot|TestWispToggleKeepsCursor|TestEpicProgressIndex|TestFilterKeySame|Workspace|Recipe|BQL|Filter|Tree|Priority' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)'`
Expected: `ok`. If an existing test fails, read it before changing it: tests asserting that a recipe or BQL ignores the label filter or wisps encode the old behavior and are updated to the new one (spec "Behavior changes"); anything else is a bug in this task.

- [ ] **Step 9: Full suite against the baseline, build, vet, commit**

```bash
go build ./... && go vet ./...
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
git commit --only pkg/ui/model.go pkg/ui/model_filter.go pkg/ui/helpers.go pkg/ui/model_footer.go pkg/ui/model_update_input.go pkg/ui/model_update_analysis.go pkg/ui/list_navigation_test.go pkg/ui/filterset_test.go -m "refactor(tui): route list, recipe and BQL through one visible set (bt-imh)"
```

Expected: the diff prints nothing.

---

### Task 3: Reload paths use the apply path

**Files:**
- Modify: `pkg/ui/model_update_data.go` (`handleSnapshotReady` ~195-358 and ~376-378; `handleDataSourceReload` ~457-463; `handleFileChanged` ~766-893)
- Modify: `pkg/ui/model.go` (`replaceIssues` ~1770-1797)
- Modify: `pkg/ui/model_update_analysis.go` (`handlePhase2Ready` ~282-290 and ~404-414)
- Modify: `pkg/ui/model_modes.go` (`rebuildListWithDiffInfo` ~429-435)
- Modify: `pkg/ui/model_filter.go` (`setListItems` ~20-59; delete `reapplyActiveFilter` ~400-429)
- Modify: `pkg/ui/list_navigation_test.go` (`TestRefreshKeepsCursor`)
- Test: `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: Task 2's `applyFilter`, `canUseSnapshot`, `filterSpec`, `m.filter.visible`.
- Produces: `reapplyActiveFilter` no longer exists; every reload ends in `applyFilter()`. `setListItems` no longer filters.

- [ ] **Step 1: Write the failing test**

Append to `pkg/ui/filterset_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/ui/ -run TestSnapshotReloadKeepsStatusFilter -v 2>&1 | grep -E '^(    |---)' | head`
Expected: FAIL for `blocked` and `in_progress` (the old snapshot path's status switch has no cases for them, so the list empties). `deferred` passes trivially (the fixture has no deferred issue) and stays as a guard.

- [ ] **Step 3: handleSnapshotReady**

In `pkg/ui/model_update_data.go`, replace the whole block from `// Update list/board/graph views while preserving the current recipe/filter state.` (~line 195) through the end of `// Restore selection in recipe mode` block (~line 344) with:

```go
	// One apply path for every reload (bt-imh).
	if m.filter.activeRecipe != nil && m.filter.activeBQLExpr == nil {
		m.filter.currentFilter = "recipe:" + m.filter.activeRecipe.Name
	}
	m.applyFilter()

	// Restore selection by ID against the visible (possibly / searched) view.
	// Indexing into the unfiltered set would drive Paginator.Page out of
	// bounds when the filter narrows results (bt-nzsy follow-up).
	if selectedID != "" {
		for i, it := range m.list.VisibleItems() {
			if item, ok := it.(IssueItem); ok && item.Issue.ID == selectedID {
				m.list.Select(i)
				break
			}
		}
	}
	if visible := m.list.VisibleItems(); len(visible) > 0 && m.list.Index() >= len(visible) {
		m.list.Select(0)
	}
```

Then replace the tree block (~351-358):

```go
	if m.focused == focusTree {
		m.rebuildTreeForCurrentFilter()
		m.tree.SetSize(m.width, m.height-2)
	}
```

with (applyFilter already rebuilt the tree):

```go
	if m.focused == focusTree {
		m.tree.SetSize(m.width, m.height-2)
	}
```

And delete the epics refresh (~376-378, the comment plus `m.refreshEpicsForCurrentFilter()`); `applyFilter` does it.

- [ ] **Step 4: replaceIssues and handleDataSourceReload**

In `pkg/ui/model.go` `replaceIssues`, replace from `// Rebuild list items` (~1770) through `m.setListItems(items)` (~1797) with:

```go
	m.clearSemanticScores()
	if m.semanticSearch != nil {
		m.semanticSearch.ResetCache()
		m.semanticSearch.SetMetricsCache(nil)
	}
	m.semanticHybridReady = false
	m.semanticHybridBuilding = false
	m.applyFilter()
```

(`applyFilter` calls `updateSemanticIDs` itself.)

In `pkg/ui/model_update_data.go` `handleDataSourceReload`, delete the comment and call after `m.replaceIssues(msg.Issues)` (~460-463):

```go
	// replaceIssues rebuilds items from the full corpus; re-apply any active
	// BQL/recipe filter so a Dolt poll doesn't silently un-filter the view
	// (bt-hhg1r.1).
	m.reapplyActiveFilter()
```

- [ ] **Step 5: handleFileChanged**

In `pkg/ui/model_update_data.go` `handleFileChanged`:

1. Replace from `// Rebuild list items (preserve triage data to avoid flicker)` (~766) through `m.setListItems(items)` (~808) with:

```go
	// Rebuild list items through the single apply path (bt-imh); triage maps
	// are kept, so badges don't flicker.
	var listStart time.Time
	if profileRefresh {
		listStart = time.Now()
	}
	m.clearSemanticScores()
	if m.semanticSearch != nil {
		m.semanticSearch.ResetCache()
		m.semanticSearch.SetMetricsCache(nil)
	}
	m.semanticHybridReady = false
	m.semanticHybridBuilding = false
	if m.semanticHybridEnabled {
		m.semanticHybridBuilding = true
		cmds = append(cmds, BuildHybridMetricsCmd(m.issuesForAsync()))
	}
	m.applyFilter()
	if profileRefresh {
		recordTiming("list_items", time.Since(listStart))
	}
```

2. Delete the board/graph block (`if needsGraph || m.mode == ViewBoard { ... m.refreshBoardAndGraphForCurrentFilter() ... }`, ~873-882); `applyFilter` already refreshed both.
3. Delete the re-apply block (~884-889, comment plus `m.reapplyActiveFilter()`) and the epics refresh (~891-893, comment plus `m.refreshEpicsForCurrentFilter()`).

- [ ] **Step 6: handlePhase2Ready**

In `pkg/ui/model_update_analysis.go`:

1. Replace the graph block (~283-290):

```go
	if m.data.snapshot != nil {
		if m.data.snapshot.GraphLayout != nil {
			m.data.snapshot.GraphLayout.UpdatePhase2Ranks(msg.Stats)
		}
		m.graphView.SetSnapshot(m.data.snapshot)
	} else {
		m.graphView.SetIssues(m.data.issues, &ins)
	}
```

with:

```go
	if m.data.snapshot != nil && m.data.snapshot.GraphLayout != nil {
		m.data.snapshot.GraphLayout.UpdatePhase2Ranks(msg.Stats)
	}
	if m.canUseSnapshot(m.filterSpec(), len(m.filter.visible)) && m.data.snapshot.GraphLayout != nil {
		m.graphView.SetSnapshot(m.data.snapshot)
	} else {
		m.graphView.SetIssues(m.filter.visible, &ins)
	}
```

2. Replace the re-apply block (~404-414) with:

```go
	// Re-apply filters. When nothing filters, refreshListItemsPhase2 is an
	// in-place score refresh that keeps the selection; otherwise the single
	// apply path rebuilds (a recipe may have re-sorted by a Phase 2 metric).
	filterStart := time.Now()
	if m.filterSpec().IsUnfiltered() {
		m.refreshListItemsPhase2()
	} else {
		m.applyFilter()
	}
	debug.LogTiming("phase2.filter.reapply", time.Since(filterStart))
```

- [ ] **Step 7: Remove reapplyActiveFilter and the setListItems safety net**

- `pkg/ui/model_modes.go`: `rebuildListWithDiffInfo` body becomes `m.applyFilter()`; its comment becomes `// rebuildListWithDiffInfo recreates list items with current diff state.`
- `pkg/ui/model_filter.go`: delete `reapplyActiveFilter` and its doc comment.
- `pkg/ui/model_filter.go` `setListItems`: delete the `if m.workspaceMode && m.activeRepos != nil { ... items = filtered }` block, and replace the doc comment's second bullet (the `activeRepos` paragraph) with: `Every caller passes items built from the visible set (bt-imh), so this function no longer filters.` Update the first sentence to `setListItems sets list items while preserving any active Bubbles filter (bt-nzsy).`
- `pkg/ui/list_navigation_test.go` `TestRefreshKeepsCursor`: `m.reapplyActiveFilter()` becomes `m.applyFilter()`.
- Run `grep -n "reapplyActiveFilter\|m.setListItems(" pkg/ui/*.go`. Expected: no `reapplyActiveFilter` outside comments in tests, and `m.setListItems(` only in `applyFilter` and `refreshListItemsPhase2`. Fix stale comments that mention `reapplyActiveFilter` in non-test files.

- [ ] **Step 8: Run the tests**

Run: `go test ./pkg/ui/ -run 'TestSnapshotReload|Filter|Snapshot|Reload|Refresh|Recipe|BQL|Workspace|Phase2' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)'`
Expected: `ok`.

- [ ] **Step 9: Full suite against the baseline, build, vet, commit**

```bash
go build ./... && go vet ./...
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
git commit --only pkg/ui/model_update_data.go pkg/ui/model.go pkg/ui/model_update_analysis.go pkg/ui/model_modes.go pkg/ui/model_filter.go pkg/ui/list_navigation_test.go pkg/ui/filterset_test.go -m "refactor(tui): every reload goes through the single apply path (bt-imh)"
```

---

### Task 4: Epics, label picker, board hint, export

**Files:**
- Modify: `pkg/ui/epics_view.go` (`refreshEpicsForCurrentFilter` ~12-48)
- Modify: `pkg/ui/model_update_input.go` (label picker case ~1500-1530)
- Modify: `pkg/ui/model_footer.go` (`extractHintText` ~597-602)
- Modify: `pkg/ui/model_export.go` (`exportToMarkdown` ~86-99)
- Modify: `pkg/ui/model_filter.go` (delete `matchesLabelFilter`)
- Test: `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: `m.applySpec`, `m.filterSpec`, `DimPrimary`, `DimLabels`, `m.filter.scopeCount`, `m.filter.visible`.
- Produces: nothing new.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/ui/filterset_test.go`:

```go
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
```

Add `"strings"` and `tea "charm.land/bubbletea/v2"` to the test file's imports.

(`epicProgressFixture()` in `pkg/ui/epic_progress_test.go` has one closed child, `ep.1`.)

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/ui/ -run 'TestLabelPickerCountsFollowStatusFilter|TestBoardHintTotalIsProjectScope|TestEpicsIgnoreStatus' -v 2>&1 | grep -E '^(    |---)' | head`
Expected: `TestLabelPickerCountsFollowStatusFilter` fails on the tests count (3: the old picker counts the closed proja-2 and the hidden wisp proja-4), `TestBoardHintTotalIsProjectScope` fails (denominator is the corpus, 6). `TestEpicsIgnoreStatusButFollowLabels` may already pass; it pins behavior for Step 3.

- [ ] **Step 3: Epics**

In `pkg/ui/epics_view.go`, replace the body section from `issues := m.workspacePrefilter(m.data.issues)` through the end of the `for` loop that builds `scoped` with:

```go
	scoped := m.applySpec(m.filterSpec().Without(DimPrimary))
```

Update the doc comment's sourcing note: replace "the scope + label + wisp-filtered set WITHOUT the status filter" with "the active filter without its primary dimension (`spec.Without(DimPrimary)`, bt-imh)". Remove now-unused imports (`model` if unused).

- [ ] **Step 4: Label picker**

In `pkg/ui/model_update_input.go`, the `LabelPicker` case:

- `if len(m.data.issues) == 0 {` becomes `if len(m.data.issueMap) == 0 {`
- Replace the comment and extraction:

```go
			// Labels and counts follow the project scope (bt-obw) but not the
			// status/BQL/recipe/label filters: counts that shifted with the
			// label filter would hide the labels the user is choosing between.
			labelExtraction := analysis.ExtractLabels(m.workspacePrefilter(m.data.issues))
```

with:

```go
			// Facet counts (bt-imh): every filter except the label selection,
			// so counts match the list while other labels stay choosable.
			labelExtraction := analysis.ExtractLabels(m.applySpec(m.filterSpec().Without(DimLabels)))
```

The count-0 loop for applied labels that follows stays unchanged.

- [ ] **Step 5: Board hint and export**

`pkg/ui/model_footer.go` `extractHintText`: `total := len(m.data.issues)` becomes `total := m.filter.scopeCount`.

`pkg/ui/model_export.go`:

```go
// exportToMarkdown exports the visible issues to a Markdown file with an
// auto-generated filename.
func (m *Model) exportToMarkdown() {
	// Generate smart filename: beads_report_<project>_YYYY-MM-DD.md
	filename := m.generateExportFilename()

	issues := m.visibleIssues()
	err := export.SaveMarkdownToFile(issues, filename)
	if err != nil {
		m.setFailure(fmt.Sprintf("%s Export failed: %v", activeGlyphs.Cross, err))
		return
	}

	m.setStatus(fmt.Sprintf("%s Exported %d issues to %s", activeGlyphs.Success, len(issues), filename))
}
```

- [ ] **Step 6: Delete matchesLabelFilter**

Run `grep -n "matchesLabelFilter" pkg/ui/*.go`. Its last callers were removed in Tasks 2-4; delete the function from `pkg/ui/model_filter.go`. If a test still calls it, switch that test to `hasAnyLabel(issue, strings.Split(x, ","))`.

- [ ] **Step 7: Run the tests**

Run: `go test ./pkg/ui/ -run 'TestLabelPicker|TestBoardHint|TestEpics|Export|Epic' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)'`
Expected: `ok`, including the existing `TestLabelPickerRespectsProjectScope`.

- [ ] **Step 8: Full suite against the baseline, build, vet, commit**

```bash
go build ./... && go vet ./...
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
git commit --only pkg/ui/epics_view.go pkg/ui/model_update_input.go pkg/ui/model_footer.go pkg/ui/model_export.go pkg/ui/model_filter.go pkg/ui/filterset_test.go -m "feat(tui): picker, epics, board hint and export follow the filter (bt-imh)"
```

---

### Task 5: Triage and analysis views follow the visible set

**Files:**
- Modify: `pkg/ui/model.go` (`AnalysisCache` ~720-735; `NewModel` `ac:` literal ~1552; `getCrossFlowsForLabel` ~1090; `filterIssuesByLabel` ~1136)
- Modify: `pkg/ui/model_update_analysis.go` (`handlePhase2Ready` ~293-360)
- Modify: `pkg/ui/model_update_input.go` (label subgraph ~190; label dashboard ~1283-1296; attention ~1306-1326; flow ~1337-1351; `openInsightsView` ~2924-2972)
- Modify: `pkg/ui/model_update_data.go` (`handleSnapshotReady` ~147-151 and ~187; `handleFileChanged` ~709-710, ~822-872)
- Modify: `pkg/ui/model.go` `replaceIssues` (~1746-1747)
- Modify: `pkg/ui/model_filter.go` (`applyFilter`; add helpers; delete `workspacePrefilter`)
- Test: `pkg/ui/workspace_filter_test.go`, `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: `m.filter.visible`, `m.filter.visibleIDs`, `m.filter.visibleKey`.
- Produces:
  - `AnalysisCache` fields `triage *analysis.TriageResult`, `triageKey string`, `triageWaitPhase2 bool`
  - `func (m *Model) ensureTriageForVisible(force bool)`
  - `func (m *Model) setTriage(t *analysis.TriageResult)`
  - `func triageDataHash(t *analysis.TriageResult) string`
  - `func (m *Model) visibleInsights(ins analysis.Insights) analysis.Insights`
  - `func (m *Model) refreshLabelDashboard()`, `refreshAttentionView()`, `refreshFlowMatrix()`, `refreshAnalysisViews()`

- [ ] **Step 1: Write the failing tests**

Append to `pkg/ui/workspace_filter_test.go`:

```go
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
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/ui/ -run 'TestTriageFollowsLabelFilter|TestLabelDashboardFollowsLabelFilter|TestInsightsListsOnlyVisibleIssues' 2>&1 | head`
Expected: build failure (`m.visibleInsights undefined`). To see the behavioral failures first, temporarily comment out `TestInsightsListsOnlyVisibleIssues`: `TestTriageFollowsLabelFilter` and `TestLabelDashboardFollowsLabelFilter` then fail on the leaked IDs. Restore it before Step 3.

- [ ] **Step 3: Triage state and helpers**

In `pkg/ui/model.go` `AnalysisCache`, add after `showPriorityHints bool`:

```go
	// Triage over the visible set (bt-imh). triageKey is the visibleKey it was
	// computed for; triageWaitPhase2 holds recomputation between new data and
	// Phase2Ready so a reload pays for triage once.
	triage           *analysis.TriageResult
	triageKey        string
	triageWaitPhase2 bool
```

In `NewModel`'s `ac: &AnalysisCache{` literal add `triageWaitPhase2: true,` (startup keeps its phase-1 triage until Phase 2 arrives).

In `pkg/ui/model_filter.go` add:

```go
// ensureTriageForVisible ranks triage over the visible set: list-row badges,
// insights top picks and recommendations. It recomputes once per filter key
// and data load; force skips the Phase 2 wait (opening insights).
func (m *Model) ensureTriageForVisible(force bool) {
	if m.ac.triage != nil && m.ac.triageKey == m.filter.visibleKey {
		return
	}
	if m.ac.triageWaitPhase2 && !force {
		return
	}
	triage := analysis.ComputeTriageWithOptions(m.filter.visible, analysis.TriageOptions{WaitForPhase2: true})
	m.setTriage(&triage)
	m.ac.triageKey = m.filter.visibleKey
}

// setTriage installs a triage result as the list-row badge maps.
func (m *Model) setTriage(t *analysis.TriageResult) {
	scores := make(map[string]float64, len(t.Recommendations))
	reasons := make(map[string]analysis.TriageReasons, len(t.Recommendations))
	unblocks := make(map[string][]string, len(t.Recommendations))
	quickWins := make(map[string]bool, len(t.QuickWins))
	blockers := make(map[string]bool, len(t.BlockersToClear))
	for _, rec := range t.Recommendations {
		scores[rec.ID] = rec.Score
		if len(rec.Reasons) > 0 {
			reasons[rec.ID] = analysis.TriageReasons{Primary: rec.Reasons[0], All: rec.Reasons, ActionHint: rec.Action}
		}
		unblocks[rec.ID] = rec.UnblocksIDs
	}
	for _, qw := range t.QuickWins {
		quickWins[qw.ID] = true
	}
	for _, bl := range t.BlockersToClear {
		blockers[bl.ID] = true
	}
	m.ac.triage = t
	m.ac.triageScores, m.ac.triageReasons, m.ac.unblocksMap = scores, reasons, unblocks
	m.ac.quickWinSet, m.ac.blockerSet = quickWins, blockers
}

func triageDataHash(t *analysis.TriageResult) string {
	return fmt.Sprintf("v%s@%s#%d", t.Meta.Version, t.Meta.GeneratedAt.Format("15:04:05"), t.Meta.IssueCount)
}

// visibleInsights drops hidden issues from the graph-metric rankings. The
// metrics themselves stay computed over the full graph (spec exceptions);
// ClusterDensity and Velocity are whole-graph values and pass through.
func (m *Model) visibleInsights(ins analysis.Insights) analysis.Insights {
	ids := m.filter.visibleIDs
	if ids == nil || len(m.filter.visible) == len(m.data.issueMap) {
		return ins
	}
	keepItems := func(items []analysis.InsightItem) []analysis.InsightItem {
		out := items[:0:0]
		for _, it := range items {
			if _, ok := ids[it.ID]; ok {
				out = append(out, it)
			}
		}
		return out
	}
	keepIDs := func(in []string) []string {
		out := in[:0:0]
		for _, id := range in {
			if _, ok := ids[id]; ok {
				out = append(out, id)
			}
		}
		return out
	}
	ins.Bottlenecks = keepItems(ins.Bottlenecks)
	ins.Keystones = keepItems(ins.Keystones)
	ins.Influencers = keepItems(ins.Influencers)
	ins.Hubs = keepItems(ins.Hubs)
	ins.Authorities = keepItems(ins.Authorities)
	ins.Cores = keepItems(ins.Cores)
	ins.Slack = keepItems(ins.Slack)
	ins.Articulation = keepIDs(ins.Articulation)
	ins.Orphans = keepIDs(ins.Orphans)
	var cycles [][]string
	for _, c := range ins.Cycles {
		if len(keepIDs(c)) > 0 {
			cycles = append(cycles, c)
		}
	}
	ins.Cycles = cycles
	return ins
}

// refreshLabelDashboard computes label health over the visible set.
func (m *Model) refreshLabelDashboard() {
	if !m.labelHealthCached {
		cfg := analysis.DefaultLabelHealthConfig()
		m.labelHealthCache = analysis.ComputeAllLabelHealth(m.filter.visible, cfg, time.Now().UTC(), m.data.analysis)
		m.labelHealthCached = true
	}
	m.labelDashboard.SetData(m.labelHealthCache.Labels)
	m.labelDashboard.SetSize(m.width, m.height-1)
	m.setStatus(fmt.Sprintf("Labels: %d total • critical %d • warning %d", m.labelHealthCache.TotalLabels, m.labelHealthCache.CriticalCount, m.labelHealthCache.WarningCount))
}

// refreshAttentionView computes attention scores and text over the visible set.
func (m *Model) refreshAttentionView() {
	if !m.attentionCached {
		cfg := analysis.DefaultLabelHealthConfig()
		m.attentionCache = analysis.ComputeLabelAttentionScores(m.filter.visible, cfg, time.Now().UTC())
		m.attentionCached = true
	}
	attText, _ := ComputeAttentionView(m.filter.visible, max(40, m.width-4))
	m.insightsPanel = NewInsightsModel(analysis.Insights{}, m.data.issueMap, m.theme)
	m.insightsPanel.labelAttention = m.attentionCache.Labels
	m.insightsPanel.extraText = attText
	m.insightsPanel.SetSize(m.width, max(3, m.height-2))
}

// refreshFlowMatrix computes cross-label flow over the visible set.
func (m *Model) refreshFlowMatrix() {
	cfg := analysis.DefaultLabelHealthConfig()
	flow := analysis.ComputeCrossLabelFlow(m.filter.visible, cfg)
	m.flowMatrix = NewFlowMatrixModel(m.theme)
	m.flowMatrix.SetData(&flow, m.filter.visible)
	m.flowMatrix.SetSize(m.width, max(3, m.height-2))
}

// refreshAnalysisViews recomputes the open analysis view after a filter change.
func (m *Model) refreshAnalysisViews() {
	switch m.mode {
	case ViewLabelDashboard:
		m.refreshLabelDashboard()
	case ViewAttention:
		m.refreshAttentionView()
	case ViewFlowMatrix:
		m.refreshFlowMatrix()
	case ViewInsights:
		m.openInsightsView()
	}
}
```

Add `"fmt"` and `"time"` imports to `model_filter.go` if missing (`fmt` and `time` are already imported there).

- [ ] **Step 4: Wire triage and analysis refresh into applyFilter**

In `applyFilter` (Task 2), change the first line and add triage before items are built:

```go
func (m *Model) applyFilter() {
	prevKey := m.filter.visibleKey
	m.refreshVisible()
	keyChanged := m.filter.visibleKey != prevKey
	if keyChanged {
		m.labelHealthCached = false
		m.attentionCached = false
		m.labelDrilldownCache = make(map[string][]model.Issue)
	}
	m.ensureTriageForVisible(false)
```

and at the end, before `if len(items) > 0 && m.list.Index() >= len(items)`:

```go
	if keyChanged {
		m.refreshAnalysisViews()
	}
```

- [ ] **Step 5: New data invalidates triage**

At each data-assignment site add, just before the site's `applyFilter()` call (or, in `handleSnapshotReady`, next to the existing cache clears at ~147-151):

```go
	m.ac.triage = nil
	m.ac.triageWaitPhase2 = true
```

Sites: `handleSnapshotReady` (next to `m.labelHealthCached = false`), `replaceIssues` (next to `m.labelHealthCached = false`), `handleFileChanged` (next to `m.labelHealthCached = false` at ~709).

- [ ] **Step 6: handlePhase2Ready**

In `pkg/ui/model_update_analysis.go`, replace from the comment `// Generate triage for priority panel, scoped to the active workspace` (~293) through `m.insightsPanel.SetRecommendations(triage.Recommendations, dataHash)` (~337) with:

```go
	// Triage over the visible set (bt-imh); Phase 2 is the point new data
	// gets ranked.
	triageStart := time.Now()
	m.ac.triageWaitPhase2 = false
	m.ac.triage = nil
	m.ensureTriageForVisible(false)
	debug.LogTiming("phase2.ComputeTriageFromAnalyzer", time.Since(triageStart))
	m.insightsPanel.SetTopPicks(m.ac.triage.QuickRef.TopPicks)
	m.insightsPanel.SetRecommendations(m.ac.triage.Recommendations, triageDataHash(m.ac.triage))
```

Change `m.insightsPanel.SetInsights(ins)` (~273) to `m.insightsPanel.SetInsights(m.visibleInsights(ins))`.

Replace the label health block (~350-360) with:

```go
	// Label health is recomputed over the visible set (bt-imh).
	m.labelHealthCached = false
	if m.focused == focusLabelDashboard {
		m.refreshLabelDashboard()
	}
```

- [ ] **Step 7: Key handlers and openInsightsView**

In `pkg/ui/model_update_input.go`:

- Label subgraph (~190): `analysis.ComputeLabelSubgraph(m.data.issues, m.labelDrilldownLabel)` becomes `analysis.ComputeLabelSubgraph(m.visibleIssues(), m.labelDrilldownLabel)`.
- Label dashboard (~1283-1296): replace the comment and everything from `if !m.labelHealthCached {` through the `m.setStatus(...)` line with `m.refreshLabelDashboard()`.
- Attention (~1306-1325): replace from the comment `// Attention view: compute attention scores` through `m.insightsPanel.SetSize(m.width, panelHeight)` with:

```go
			m.mode = ViewAttention
			m.focused = focusInsights
			m.refreshAttentionView()
```

- Flow (~1337-1350): replace from the comment `// Scope to the active workspace repo filter` through `m.flowMatrix.SetSize(m.width, panelHeight)` with:

```go
			m.mode = ViewFlowMatrix
			m.focused = focusFlowMatrix
			m.refreshFlowMatrix()
```

- `openInsightsView` (~2924): `ins = m.data.analysis.GenerateInsights(len(m.data.issues))` becomes `ins = m.data.analysis.GenerateInsights(len(m.filter.visible))`; `m.insightsPanel = NewInsightsModel(ins, m.data.issueMap, m.theme)` becomes `m.insightsPanel = NewInsightsModel(m.visibleInsights(ins), m.data.issueMap, m.theme)`; replace the comment block starting `// Include priority triage (bv-91)` and the three lines `triage := ...`, `m.insightsPanel.SetTopPicks(...)`, `dataHash := ...`, `m.insightsPanel.SetRecommendations(...)` with:

```go
		// Triage over the visible set (bt-imh), shared with the list-row
		// badges through ensureTriageForVisible.
		m.ensureTriageForVisible(true)
		m.insightsPanel.SetTopPicks(m.ac.triage.QuickRef.TopPicks)
		m.insightsPanel.SetRecommendations(m.ac.triage.Recommendations, triageDataHash(m.ac.triage))
```

- [ ] **Step 8: Reload paths and drilldown helpers**

- `pkg/ui/model_update_data.go` `handleSnapshotReady` ~187: `m.insightsPanel.SetInsights(m.data.snapshot.Insights)` stays (the panel is rebuilt by `refreshAnalysisViews`/`openInsightsView` when shown).
- `handleFileChanged` ~830: `ins = m.data.analysis.GenerateInsights(len(m.data.issues))` becomes `ins = m.data.analysis.GenerateInsights(len(m.filter.visible))`; ~841 `NewInsightsModel(ins, ...)` becomes `NewInsightsModel(m.visibleInsights(ins), ...)`.
- `handleFileChanged` attention block (~852-872): replace the body inside `if m.mode == ViewAttention {` with:

```go
		var attentionStart time.Time
		if profileRefresh {
			attentionStart = time.Now()
		}
		m.attentionCached = false
		m.refreshAttentionView()
		if profileRefresh {
			recordTiming("attention_view", time.Since(attentionStart))
		}
```

- `pkg/ui/model.go` `getCrossFlowsForLabel`: replace the two comment lines and the `flow :=` line with:

```go
	// Scoped to the visible set (bt-imh).
	flow := analysis.ComputeCrossLabelFlow(m.filter.visible, cfg)
```

- `pkg/ui/model.go` `filterIssuesByLabel`: `for _, iss := range m.data.issues {` becomes `for _, iss := range m.filter.visible {`.

- [ ] **Step 9: Delete workspacePrefilter and update stale comments**

`grep -n "workspacePrefilter" pkg/ui/*.go` must show only its definition; delete it from `pkg/ui/model_filter.go`. Then `grep -n "bt-dcby.3\|status-filter-independent\|survive status" pkg/ui/*.go | grep -v _test` and rewrite comments that now describe project-only scoping to say the code reads the visible set (bt-imh).

- [ ] **Step 10: Run the tests**

Run: `go test ./pkg/ui/ -run 'TestTriage|TestLabelDashboard|TestInsights|TestAttention|TestFlowMatrix|TestOpenInsights|TestPhase2|TestGetCrossFlows|TestRecompute|TestApplyFilter|TestPriorityHints|TestActionable|Insights|Label' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)'`
Expected: `ok`. The existing bt-dcby.3 tests in `workspace_filter_test.go` stay valid (a project filter is still part of the visible set); update only comments that call the scope "workspace-only".

- [ ] **Step 11: Full suite against the baseline, build, vet, commit**

```bash
go build ./... && go vet ./...
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
git commit --only pkg/ui/model.go pkg/ui/model_update_analysis.go pkg/ui/model_update_input.go pkg/ui/model_update_data.go pkg/ui/model_filter.go pkg/ui/workspace_filter_test.go -m "feat(tui): triage and analysis views follow the visible set (bt-imh)"
```

---

### Task 6: Alerts follow the visible set

**Files:**
- Modify: `pkg/ui/model_alerts_header.go` (`passesAlertBaseScope` ~43-61)
- Test: `pkg/ui/filterset_test.go`

**Interfaces:**
- Consumes: `m.filter.visibleIDs`.
- Produces: nothing new.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/ui/filterset_test.go` (add `"github.com/seanmartinsmith/beadstui/pkg/drift"` to imports):

```go
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
```

(`visibleAlerts` sorts its result, so the test compares sets, not order.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./pkg/ui/ -run 'TestAlertsFollowVisibleSet|TestAlertsBeforeFirstApply' -v 2>&1 | grep -E '^(    |---)' | head`
Expected: `TestAlertsFollowVisibleSet` fails (proja-2's alert still visible).

- [ ] **Step 3: Implement**

In `passesAlertBaseScope`, replace the `if m.workspaceMode && m.activeRepos != nil && a.IssueID != "" { ... }` block with:

```go
	// Issue-linked alerts follow the visible set (bt-imh). Before the first
	// apply (visibleIDs nil) nothing is hidden; alerts without an issue and
	// alerts for unknown issues always pass.
	if a.IssueID != "" && m.filter != nil && m.filter.visibleIDs != nil {
		if _, known := m.data.issueMap[a.IssueID]; known {
			if _, shown := m.filter.visibleIDs[a.IssueID]; !shown {
				return false
			}
		}
	}
```

Update the function's doc comment: "dismissed state and (workspace) active-repo scope" becomes "dismissed state and membership in the visible set".

- [ ] **Step 4: Run tests, full suite, build, vet, commit**

```bash
go test ./pkg/ui/ -run 'Alert|Notification' 2>&1 | grep -E '^(--- FAIL|ok|FAIL)'
go build ./... && go vet ./...
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
git commit --only pkg/ui/model_alerts_header.go pkg/ui/filterset_test.go -m "feat(tui): alerts follow the visible set (bt-imh)"
```

Expected: `ok`, empty diff. Existing alert tests that set `activeRepos` without calling `applyFilter` may now see no filtering; fix them by calling `m.applyFilter()` after setting scope (that is the real flow), not by changing the implementation.

---

### Task 7: Guard tests and close-out

**Files:**
- Test: `pkg/ui/filterset_test.go`
- Modify: `docs/design/2026-10-08-bt-imh-central-filter-engine.md` (only if the implemented allowlist differs from the spec's exception list)

**Interfaces:**
- Consumes: everything above.
- Produces: `TestNoRawCorpusReads`, `TestFilterConsistencyMatrix`.

- [ ] **Step 1: Static guard**

Append to `pkg/ui/filterset_test.go` (imports: `"go/ast"`, `"go/parser"`, `"go/token"`, `"os"`, `"path/filepath"`):

```go
// rawCorpusReaders are the functions allowed to read m.data.issues directly.
// Everything else must read the visible set (bt-imh). Each entry is a
// deliberate full-data reader from the design's exception list.
var rawCorpusReaders = map[string]string{
	"epic_card.go:handleEpicCardKeys":         "epic progress",
	"epic_card.go:renderEpicCard":             "epic progress",
	"model_alerts.go:notificationRepoScope":   "repo scope of loaded data",
	"model_alerts_header.go:alertsHeaderLines": "corpus size line",
	"model_alerts_header.go:sourceIssueCounts": "per-source corpus counts",
	"model_filter.go:applyFilter":             "epic progress index",
	"model_filter.go:applySpec":               "the engine's input",
	"model_filter.go:updateViewportContent":   "epic progress in details",
	"model.go:Init":                           "history preload needs any data",
	"model.go:replaceIssues":                  "data load",
	"model_modes.go:enterTimeTravelMode":      "time travel",
	"model_modes.go:Issues":                   "full-corpus accessor",
	"model_update_analysis.go:handlePhase2Ready": "computeAlerts and recipe re-sort",
	"model_update_data.go:handleFileChanged":  "data load",
	"model_update_data.go:handleSnapshotReady": "data load",
	"plugin_host.go:syncPluginsWithHash":      "plugin sync",
	"semantic_search.go:issuesForAsync":       "semantic index",
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
```

Run: `go test ./pkg/ui/ -run TestNoRawCorpusReads -v 2>&1 | tail -20`
Expected: PASS. Any failure names a function that still reads the raw corpus: convert it to the visible set unless it is one of the design's deliberate full-data readers; in that case add it to `rawCorpusReaders` with the reason and add the same line to the design doc's exception list. Also delete allowlist entries that no longer match any read (a stale entry hides future regressions): temporarily add `t.Logf("hit %s", key)` inside the selector branch, check each entry appears, then remove the log.

- [ ] **Step 2: Consistency matrix**

Append to `pkg/ui/filterset_test.go`:

```go
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
```

Add `"fmt"` to imports. Run: `go test ./pkg/ui/ -run TestFilterConsistencyMatrix 2>&1 | grep -E '^(--- FAIL|ok|FAIL)|    '| head -20`
Expected: `ok`. The fixture only uses statuses that the board has columns for (open, in_progress, blocked, closed). If `NodeCount` counts collapsed nodes differently, the fixture has no parent-child links, so every node is a root and the counts must match; a mismatch is a real bug.

- [ ] **Step 3: Full verification**

```bash
gofmt -l pkg/ui
go build ./... && go vet ./...
go test ./... 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | grep -v '^ok' 
go test ./pkg/ui/ 2>&1 | grep -E '^--- FAIL' | sort | diff _tmp/bt-imh-baseline.txt -
go test ./pkg/ui/ -race -run 'TestFilter|TestVisible|TestSnapshotReload|TestLabelPicker|TestAlerts|TestTriage' 2>&1 | tail -3
```

Expected: no failures beyond the baseline, empty diff, race run `ok`.

- [ ] **Step 4: Commit the guards**

```bash
git commit --only pkg/ui/filterset_test.go -m "test(tui): guard raw corpus reads and filter consistency (bt-imh)"
```

If the design doc's exception list changed in Step 1, commit it too:
`git commit --only docs/design/2026-10-08-bt-imh-central-filter-engine.md -m "docs(tui): align filter engine exceptions with implementation (bt-imh)"`.

- [ ] **Step 5: Close the bead**

Write `.beads/tmp/bt-imh-close.md` (ASCII only) with the close template from `.beads/conventions/reference.md`: Summary, Change, Files, Verify (commands from Step 3 and their result), Risk (triage now recomputes on filter change; measured cost is ~3 ms per 500 issues per `BenchmarkRobotTriage_Sparse500`), Notes (behavior changes from the design's "Behavior changes users will notice"; GitHub #61 reply still pending). Then:

```bash
bd close bt-imh --reason-file .beads/tmp/bt-imh-close.md
```

Do not push and do not merge to `main`; report back for the owner's review.
