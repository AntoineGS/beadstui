package bql

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

var testIssues = []model.Issue{
	{
		ID:        "bt-001",
		Title:     "Fix auth bug",
		Status:    model.StatusOpen,
		Priority:  0,
		IssueType: model.TypeBug,
		Assignee:  "sms",
		Labels:    []string{"bug", "urgent"},
		CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
	},
	{
		ID:        "bt-002",
		Title:     "Add BQL search",
		Status:    model.StatusInProgress,
		Priority:  1,
		IssueType: model.TypeFeature,
		Assignee:  "sms",
		Labels:    []string{"feature", "search"},
		CreatedAt: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	},
	{
		ID:        "bt-003",
		Title:     "Cleanup old tests",
		Status:    model.StatusClosed,
		Priority:  3,
		IssueType: model.TypeChore,
		Assignee:  "other",
		Labels:    []string{"chore"},
		CreatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	},
	{
		ID:        "bt-004",
		Title:     "Blocked task",
		Status:    model.StatusOpen,
		Priority:  2,
		IssueType: model.TypeTask,
		Labels:    []string{"task"},
		Dependencies: []*model.Dependency{
			{DependsOnID: "bt-001", Type: model.DepBlocks},
		},
		CreatedAt: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC),
	},
}

func testOpts() ExecuteOpts {
	m := make(map[string]*model.Issue, len(testIssues))
	for i := range testIssues {
		m[testIssues[i].ID] = &testIssues[i]
	}
	return ExecuteOpts{IssueMap: m}
}

func exec(t *testing.T, input string) []model.Issue {
	t.Helper()
	query, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	e := NewMemoryExecutor()
	return e.Execute(query, testIssues, testOpts())
}

func TestExecute_StatusFilter(t *testing.T) {
	result := exec(t, "status = open")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2 (bt-001, bt-004)", len(result))
	}
}

func TestExecute_PriorityLessThan(t *testing.T) {
	result := exec(t, "priority < P2")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2 (P0 and P1)", len(result))
	}
	for _, r := range result {
		if r.Priority >= 2 {
			t.Errorf("issue %s has priority %d, want < 2", r.ID, r.Priority)
		}
	}
}

func TestExecute_TypeEquals(t *testing.T) {
	result := exec(t, "type = bug")
	if len(result) != 1 || result[0].ID != "bt-001" {
		t.Fatalf("got %v, want [bt-001]", ids(result))
	}
}

func TestExecute_TitleContains(t *testing.T) {
	result := exec(t, "title ~ auth")
	if len(result) != 1 || result[0].ID != "bt-001" {
		t.Fatalf("got %v, want [bt-001]", ids(result))
	}
}

func TestExecute_TitleNotContains(t *testing.T) {
	result := exec(t, "title !~ auth")
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3", len(result))
	}
}

func TestExecute_LabelEquals(t *testing.T) {
	result := exec(t, "label = urgent")
	if len(result) != 1 || result[0].ID != "bt-001" {
		t.Fatalf("got %v, want [bt-001]", ids(result))
	}
}

func TestExecute_LabelNotEquals(t *testing.T) {
	result := exec(t, "label != urgent")
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3", len(result))
	}
}

func TestExecute_LabelContains(t *testing.T) {
	result := exec(t, "label ~ urg")
	if len(result) != 1 || result[0].ID != "bt-001" {
		t.Fatalf("got %v, want [bt-001]", ids(result))
	}
}

func TestExecute_AssigneeEquals(t *testing.T) {
	result := exec(t, "assignee = sms")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
}

func TestExecute_BlockedTrue(t *testing.T) {
	result := exec(t, "blocked = true")
	if len(result) != 1 || result[0].ID != "bt-004" {
		t.Fatalf("got %v, want [bt-004]", ids(result))
	}
}

func TestExecute_BlockedFalse(t *testing.T) {
	result := exec(t, "blocked = false")
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3", len(result))
	}
}

func TestExecute_And(t *testing.T) {
	result := exec(t, "status = open and priority < P2")
	if len(result) != 1 || result[0].ID != "bt-001" {
		t.Fatalf("got %v, want [bt-001]", ids(result))
	}
}

func TestExecute_Or(t *testing.T) {
	result := exec(t, "type = bug or type = feature")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
}

func TestExecute_Not(t *testing.T) {
	result := exec(t, "not status = closed")
	if len(result) != 3 {
		t.Fatalf("got %d results, want 3", len(result))
	}
}

func TestExecute_Parentheses(t *testing.T) {
	result := exec(t, "(type = bug or type = feature) and priority < P2")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
}

func TestExecute_InExpression(t *testing.T) {
	result := exec(t, "type in (bug, feature)")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
}

func TestExecute_NotInExpression(t *testing.T) {
	result := exec(t, "type not in (bug, feature)")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2 (chore, task)", len(result))
	}
}

// TestExecute_InIsCaseInsensitive covers bt-h5jz.1: IN matches like =, so a
// value the validator accepts case-insensitively also filters.
func TestExecute_InIsCaseInsensitive(t *testing.T) {
	result := exec(t, "type in (Bug, FEATURE)")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2 (bt-001, bt-002)", len(result))
	}
}

// TestExecute_DecisionType covers bt-h5jz.1: a bd v1.3.0 built-in type
// beyond the fork's original five validates and filters end to end.
func TestExecute_DecisionType(t *testing.T) {
	issues := append([]model.Issue{{ID: "bt-dec", Title: "Decision", Status: model.StatusOpen, IssueType: "decision"}}, testIssues...)
	query, err := Parse("type = decision")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := Validate(query); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	result := NewMemoryExecutor().Execute(query, issues, testOpts())
	if len(result) != 1 || result[0].ID != "bt-dec" {
		t.Fatalf("got %v, want only bt-dec", result)
	}
}

func TestExecute_LabelIn(t *testing.T) {
	result := exec(t, "label in (urgent, search)")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2 (bt-001, bt-002)", len(result))
	}
}

func TestExecute_OrderByPriority(t *testing.T) {
	result := exec(t, "order by priority asc")
	if len(result) != 4 {
		t.Fatalf("got %d results, want 4", len(result))
	}
	for i := 1; i < len(result); i++ {
		if result[i].Priority < result[i-1].Priority {
			t.Errorf("not sorted: %d after %d", result[i].Priority, result[i-1].Priority)
		}
	}
}

func TestExecute_OrderByPriorityDesc(t *testing.T) {
	result := exec(t, "order by priority desc")
	if len(result) < 2 {
		t.Fatalf("got %d results", len(result))
	}
	if result[0].Priority < result[len(result)-1].Priority {
		t.Error("expected descending priority order")
	}
}

func TestExecute_FilterWithOrderBy(t *testing.T) {
	result := exec(t, "status = open order by priority asc")
	if len(result) != 2 {
		t.Fatalf("got %d results, want 2", len(result))
	}
	if result[0].Priority > result[1].Priority {
		t.Error("not sorted by priority ascending")
	}
}

func TestExecute_EmptyFilter(t *testing.T) {
	result := exec(t, "order by priority")
	if len(result) != 4 {
		t.Fatalf("got %d results, want 4 (no filter = all issues)", len(result))
	}
}

func TestExecute_EmptyResult(t *testing.T) {
	result := exec(t, "type = nonexistent")
	if len(result) != 0 {
		t.Fatalf("got %d results, want 0", len(result))
	}
}

func TestExecute_ExpandDown(t *testing.T) {
	// bt-001 is depended on by bt-004 (blocks relationship)
	// Filtering for bt-001 and expanding down should also include bt-004
	result := exec(t, "id = bt-001 expand down")
	hasTarget := false
	for _, r := range result {
		if r.ID == "bt-004" {
			hasTarget = true
		}
	}
	if !hasTarget {
		t.Errorf("expand down from bt-001 should include bt-004, got %v", ids(result))
	}
}

func TestExecute_ExpandUp(t *testing.T) {
	// bt-004 depends on bt-001
	// Filtering for bt-004 and expanding up should also include bt-001
	result := exec(t, "id = bt-004 expand up")
	hasTarget := false
	for _, r := range result {
		if r.ID == "bt-001" {
			hasTarget = true
		}
	}
	if !hasTarget {
		t.Errorf("expand up from bt-004 should include bt-001, got %v", ids(result))
	}
}

func TestMatches_SingleIssue(t *testing.T) {
	query, err := Parse("status = open and priority < P2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := NewMemoryExecutor()
	opts := testOpts()

	if !e.Matches(query, testIssues[0], opts) {
		t.Error("bt-001 should match (open, P0)")
	}
	if e.Matches(query, testIssues[2], opts) {
		t.Error("bt-003 should not match (closed)")
	}
}

func TestExecute_NilIssueMap(t *testing.T) {
	query, err := Parse("blocked = true")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := NewMemoryExecutor()
	// With nil IssueMap, blocked check can't find blockers
	result := e.Execute(query, testIssues, ExecuteOpts{})
	// Should return 0 since we can't verify blocking without issueMap
	if len(result) != 0 {
		t.Errorf("got %d results with nil IssueMap, want 0", len(result))
	}
}

func TestExecute_DateEquality(t *testing.T) {
	// bt-001 was created on 2026-03-01. With date-only comparison,
	// "created_at = 2026-03-01" should match even though the issue
	// was created at midnight and the query resolves to midnight.
	// More importantly, an issue created at 14:30 should also match.
	issues := []model.Issue{
		{
			ID:        "dt-001",
			Title:     "Morning issue",
			Status:    model.StatusOpen,
			CreatedAt: time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC),
		},
		{
			ID:        "dt-002",
			Title:     "Evening issue",
			Status:    model.StatusOpen,
			CreatedAt: time.Date(2026, 3, 1, 22, 45, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 3, 1, 22, 45, 0, 0, time.UTC),
		},
		{
			ID:        "dt-003",
			Title:     "Next day issue",
			Status:    model.StatusOpen,
			CreatedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	query, err := Parse("created_at = 2026-03-01")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := NewMemoryExecutor()
	result := e.Execute(query, issues, ExecuteOpts{})
	if len(result) != 2 {
		t.Errorf("got %d results, want 2 (both March 1 issues). IDs: %v", len(result), ids(result))
	}
}

func TestExecute_ISODateComparison(t *testing.T) {
	// Test that ISO dates work with comparison operators
	query, err := Parse("created_at > 2026-02-15")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	e := NewMemoryExecutor()
	result := e.Execute(query, testIssues, ExecuteOpts{})
	// bt-001 (Mar 1), bt-002 (Mar 10), bt-004 (Mar 5) should match; bt-003 (Feb 1) should not
	if len(result) != 3 {
		t.Errorf("got %d results, want 3. IDs: %v", len(result), ids(result))
	}
}

func TestParse_ISODate(t *testing.T) {
	query, err := Parse("created_at > 2026-01-15")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	comp, ok := query.Filter.(*CompareExpr)
	if !ok {
		t.Fatal("expected CompareExpr")
	}
	if comp.Value.Type != ValueDate {
		t.Errorf("got value type %d, want ValueDate (%d)", comp.Value.Type, ValueDate)
	}
	if comp.Value.String != "2026-01-15" {
		t.Errorf("got value string %q, want %q", comp.Value.String, "2026-01-15")
	}
}

func ids(issues []model.Issue) []string {
	out := make([]string, len(issues))
	for i, issue := range issues {
		out[i] = issue.ID
	}
	return out
}

// blockedFixture covers every branch of bt's blocked predicate (bt-5muh4):
//
//	blocker (open)      <-blocks-  epic (open)
//	epic                <-parent-  story (open)      propagated
//	story               <-parent-  subtask (open)    propagated, depth 2
//	epic                <-parent-  done-child (closed): closed never blocked
//	blocker             <-blocks-  closed-dependent (closed): closed never blocked
//	gone (absent)       <-blocks-  orphan (open): missing target never blocks
//	finished (closed)   <-blocks-  unblocked (open): closed blocker never blocks
//	cycle-a <-parent-> cycle-b, cycle-a <-blocks- blocker: cycle terminates
//	upstream-x (other project, open) <-blocks- xproj (open): cross-prefix counts
func blockedFixture() []model.Issue {
	dep := func(target string, typ model.DependencyType) *model.Dependency {
		return &model.Dependency{DependsOnID: target, Type: typ}
	}
	return []model.Issue{
		{ID: "bt-blocker", Status: model.StatusOpen},
		{ID: "bt-epic", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-blocker", model.DepBlocks)}},
		{ID: "bt-story", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-epic", model.DepParentChild)}},
		{ID: "bt-subtask", Status: model.StatusInProgress, Dependencies: []*model.Dependency{dep("bt-story", model.DepParentChild)}},
		{ID: "bt-done-child", Status: model.StatusClosed, Dependencies: []*model.Dependency{dep("bt-epic", model.DepParentChild)}},
		{ID: "bt-closed-dependent", Status: model.StatusClosed, Dependencies: []*model.Dependency{dep("bt-blocker", model.DepBlocks)}},
		{ID: "bt-orphan", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-gone", model.DepBlocks)}},
		{ID: "bt-finished", Status: model.StatusClosed},
		{ID: "bt-unblocked", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-finished", model.DepBlocks)}},
		{ID: "bt-cycle-a", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-cycle-b", model.DepParentChild), dep("bt-blocker", model.DepBlocks)}},
		{ID: "bt-cycle-b", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-cycle-a", model.DepParentChild)}},
		{ID: "bd-upstream-x", Status: model.StatusOpen},
		{ID: "bt-xproj", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bd-upstream-x", model.DepBlocks)}},
		{ID: "bt-related", Status: model.StatusOpen, Dependencies: []*model.Dependency{dep("bt-blocker", model.DepRelated)}},
	}
}

var wantBlockedFixture = []string{"bt-cycle-a", "bt-cycle-b", "bt-epic", "bt-story", "bt-subtask", "bt-xproj"}

func issueMapOf(issues []model.Issue) map[string]*model.Issue {
	m := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		m[issues[i].ID] = &issues[i]
	}
	return m
}

func runBlocked(t *testing.T, issues, corpus []model.Issue) []string {
	t.Helper()
	query, err := Parse("blocked = true")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := ids(NewMemoryExecutor().Execute(query, issues, ExecuteOpts{IssueMap: issueMapOf(corpus)}))
	sort.Strings(got)
	return got
}

func TestBlocked_Semantics(t *testing.T) {
	issues := blockedFixture()
	if got := runBlocked(t, issues, issues); !equalStrings(got, wantBlockedFixture) {
		t.Fatalf("blocked = true: got %v, want %v", got, wantBlockedFixture)
	}
}

// TestBlocked_ScopedInputFullCorpus pins the robot/TUI contract: executing over
// a scoped subset with the full corpus as IssueMap gives each issue the same
// answer it gets unscoped, including blockers outside the subset.
func TestBlocked_ScopedInputFullCorpus(t *testing.T) {
	corpus := blockedFixture()
	var btOnly []model.Issue
	for _, issue := range corpus {
		if issue.ID != "bd-upstream-x" {
			btOnly = append(btOnly, issue)
		}
	}
	if got := runBlocked(t, btOnly, corpus); !equalStrings(got, wantBlockedFixture) {
		t.Fatalf("scoped blocked = true: got %v, want %v", got, wantBlockedFixture)
	}
}

// TestBlocked_IssueOutsideMap covers an evaluated issue the IssueMap lacks: it
// is judged from its own dependencies, including a blocked parent.
func TestBlocked_IssueOutsideMap(t *testing.T) {
	outsiders := []model.Issue{
		{ID: "bt-new-child", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "bt-epic", Type: model.DepParentChild}}},
		{ID: "bt-new-free", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "bt-finished", Type: model.DepBlocks}}},
	}
	if got := runBlocked(t, outsiders, blockedFixture()); !equalStrings(got, []string{"bt-new-child"}) {
		t.Fatalf("got %v, want [bt-new-child]", got)
	}
}

// TestBlocked_ParityWithAnalyzer keeps BQL's blocked predicate and
// analysis.GetActionableIssues (triage, footer triad) from drifting: over the
// same corpus, blocked = true must be exactly the non-closed issues the
// analyzer does not call actionable (bt-5muh4).
func TestBlocked_ParityWithAnalyzer(t *testing.T) {
	issues := blockedFixture()

	actionable := make(map[string]bool)
	for _, issue := range analysis.NewAnalyzer(issues).GetActionableIssues() {
		actionable[issue.ID] = true
	}
	var want []string
	for _, issue := range issues {
		if !isClosedLike(issue.Status) && !actionable[issue.ID] {
			want = append(want, issue.ID)
		}
	}
	sort.Strings(want)

	if got := runBlocked(t, issues, issues); !equalStrings(got, want) {
		t.Fatalf("BQL blocked = true %v != analyzer non-actionable %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func extFixture() ([]model.Issue, ExecuteOpts) {
	issues := []model.Issue{
		{ID: "a", Title: "a", Status: model.StatusOpen},
		{ID: "b", Title: "b", Status: model.StatusOpen},
		{ID: "c", Title: "c", Status: model.StatusOpen}, // no agent.state
	}
	states := map[string]string{"a": "waiting", "b": "running"}
	opts := ExecuteOpts{Fields: func(issue *model.Issue, field string) (string, bool) {
		if field != "agent.state" {
			return "", false
		}
		v, ok := states[issue.ID]
		return v, ok
	}}
	return issues, opts
}

func extIDs(t *testing.T, query string, issues []model.Issue, opts ExecuteOpts) []string {
	t.Helper()
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	var ids []string
	for _, is := range NewMemoryExecutor().Execute(q, issues, opts) {
		ids = append(ids, is.ID)
	}
	return ids
}

func TestExtensionFieldComparisons(t *testing.T) {
	issues, opts := extFixture()
	cases := []struct {
		query string
		want  []string
	}{
		{"agent.state = waiting", []string{"a"}},
		{"agent.state = WAITING", []string{"a"}},
		{"agent.state != waiting", []string{"b"}}, // c has no value: no match
		{"agent.state ~ run", []string{"b"}},
		{"agent.state in (waiting, running)", []string{"a", "b"}},
		{"agent.state not in (waiting)", []string{"b"}}, // c excluded
		{"agent.state = waiting or status = open", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		if got := extIDs(t, c.query, issues, opts); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestExtensionFieldNumberAndBoolLiterals(t *testing.T) {
	issues := []model.Issue{
		{ID: "a", Title: "a", Status: model.StatusOpen},
		{ID: "b", Title: "b", Status: model.StatusOpen},
	}
	vals := map[string]string{"a": "42", "b": "7"}
	opts := ExecuteOpts{Fields: func(issue *model.Issue, field string) (string, bool) {
		if field != "agent.pr" {
			return "", false
		}
		v, ok := vals[issue.ID]
		return v, ok
	}}
	cases := []struct {
		query string
		want  []string
	}{
		{"agent.pr = 42", []string{"a"}},
		{"agent.pr != 42", []string{"b"}},
		{"agent.pr in (42, 7)", []string{"a", "b"}},
		{`agent.pr = "42"`, []string{"a"}},
	}
	for _, c := range cases {
		if got := extIDs(t, c.query, issues, opts); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestExtensionFieldOrderBy(t *testing.T) {
	issues, opts := extFixture()
	if got, want := extIDs(t, "status = open order by agent.state desc", issues, opts), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order by agent.state desc = %v, want %v", got, want)
	}
}

func TestBuiltinFieldShadowsExtension(t *testing.T) {
	issues, opts := extFixture()
	opts.Fields = func(*model.Issue, string) (string, bool) { return "closed", true }
	if got, want := extIDs(t, "status = open", issues, opts), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("status = open with a resolver claiming status = %v, want %v", got, want)
	}
}

func TestExtensionFieldWithoutResolverMatchesNothing(t *testing.T) {
	issues, _ := extFixture()
	if got := extIDs(t, "agent.state = waiting", issues, ExecuteOpts{}); len(got) != 0 {
		t.Fatalf("no resolver: %v, want none", got)
	}
}
