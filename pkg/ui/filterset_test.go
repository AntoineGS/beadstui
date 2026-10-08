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
