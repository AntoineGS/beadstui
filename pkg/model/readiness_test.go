package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReadinessMissingBlockerIsUnknown(t *testing.T) {
	r := NewReadinessIndex([]Issue{{ID: "bt-a", Status: StatusOpen,
		Dependencies: []*Dependency{{DependsOnID: "bt-missing", Type: DepBlocks}}}})
	if got := r.DependencyState("bt-a"); got != DependenciesUnknown {
		t.Fatalf("state = %s", got)
	}
	if r.Ready("bt-a", time.Unix(0, 0)) {
		t.Fatal("missing blocker became ready")
	}
}

func TestReadinessEligibility(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	tests := []struct {
		name      string
		issue     Issue
		others    []Issue
		ready     bool
		claimable bool
		state     DependencyState
	}{
		{name: "open leaf", issue: Issue{ID: "a", Status: StatusOpen}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "in progress ready not claimable", issue: Issue{ID: "a", Status: StatusInProgress}, ready: true, state: DependenciesSatisfied},
		{name: "assigned", issue: Issue{ID: "a", Status: StatusOpen, Assignee: "worker"}, ready: true, state: DependenciesSatisfied},
		{name: "epic", issue: Issue{ID: "a", Status: StatusOpen, IssueType: TypeEpic}, ready: true, state: DependenciesSatisfied},
		{name: "deferred", issue: Issue{ID: "a", Status: StatusOpen, DeferUntil: &future}, state: DependenciesSatisfied},
		{name: "expires exactly now", issue: Issue{ID: "a", Status: StatusOpen, DeferUntil: &now}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "expired", issue: Issue{ID: "a", Status: StatusOpen, DeferUntil: &past}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "unsupported status", issue: Issue{ID: "a", Status: StatusBlocked}, state: DependenciesSatisfied},
		{name: "closed target satisfies", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepBlocks}}}, others: []Issue{{ID: "b", Status: StatusClosed}}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "tombstone target satisfies", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepBlocks}}}, others: []Issue{{ID: "b", Status: StatusTombstone}}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "open blocker", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepBlocks}}}, others: []Issue{{ID: "b", Status: StatusOpen}}, state: DependenciesUnsatisfied},
		{name: "unknown dominates unsatisfied", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepBlocks}, {DependsOnID: "missing", Type: DepBlocks}}}, others: []Issue{{ID: "b", Status: StatusOpen}}, state: DependenciesUnknown},
		{name: "informational unknown target ignored", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "missing", Type: DepRelated}}}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "blocking variant", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: "waits-for"}}}, others: []Issue{{ID: "b", Status: StatusOpen}}, state: DependenciesUnsatisfied},
		{name: "parent inherits blocker", issue: Issue{ID: "child", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "parent", Type: DepParentChild}}}, others: []Issue{{ID: "parent", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "gate", Type: DepBlocks}}}, {ID: "gate", Status: StatusOpen}}, state: DependenciesUnsatisfied},
		{name: "parent open itself does not block", issue: Issue{ID: "child", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "parent", Type: DepParentChild}}}, others: []Issue{{ID: "parent", Status: StatusOpen}}, ready: true, claimable: true, state: DependenciesSatisfied},
		{name: "parent cycle unknown", issue: Issue{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepParentChild}}}, others: []Issue{{ID: "b", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "a", Type: DepParentChild}}}}, state: DependenciesUnknown},
		{name: "child has open immediate child", issue: Issue{ID: "parent", Status: StatusOpen}, others: []Issue{{ID: "child", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "parent", Type: DepParentChild}}}}, ready: true, state: DependenciesSatisfied},
		{name: "parent child closed", issue: Issue{ID: "parent", Status: StatusOpen}, others: []Issue{{ID: "child", Status: StatusClosed, Dependencies: []*Dependency{{DependsOnID: "parent", Type: DepParentChild}}}}, ready: true, claimable: true, state: DependenciesSatisfied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := append([]Issue{tt.issue}, tt.others...)
			r := NewReadinessIndex(issues)
			if got := r.DependencyState(tt.issue.ID); got != tt.state {
				t.Errorf("DependencyState = %q, want %q", got, tt.state)
			}
			if got := r.Ready(tt.issue.ID, now); got != tt.ready {
				t.Errorf("Ready = %v, want %v", got, tt.ready)
			}
			if got := r.Claimable(tt.issue.ID, now); got != tt.claimable {
				t.Errorf("Claimable = %v, want %v", got, tt.claimable)
			}
		})
	}
}

func TestReadinessCopiesInputsAndReadyAfter(t *testing.T) {
	dep := &Dependency{DependsOnID: "b", Type: DepBlocks}
	deferUntil := time.Now().Add(time.Hour)
	issues := []Issue{{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{dep}, DeferUntil: &deferUntil}, {ID: "b", Status: StatusOpen}}
	r := NewReadinessIndex(issues)
	dep.DependsOnID = "missing"
	deferUntil = time.Time{}
	issues[1].Status = StatusClosed
	if r.DependencyState("a") != DependenciesUnsatisfied || r.Ready("a", time.Now()) {
		t.Fatal("index changed after caller mutation")
	}
	if !r.ReadyAfter("a", time.Now().Add(2*time.Hour), map[string]bool{"b": true}) {
		t.Fatal("completed prerequisite should satisfy what-if")
	}
	if r.DependencyState("a") != DependenciesUnsatisfied {
		t.Fatal("what-if mutated readiness index")
	}
}

func TestReadinessReadyAfterCannotCompleteMissingReference(t *testing.T) {
	r := NewReadinessIndex([]Issue{{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "missing", Type: DepBlocks}}}})
	if r.ReadyAfter("a", time.Now(), map[string]bool{"missing": true}) {
		t.Fatal("hypothetical completion of absent issue resolved an unknown blocker")
	}
}

func TestReadinessDepthBeyondFifty(t *testing.T) {
	issues := make([]Issue, 0, 60)
	for i := 0; i < 60; i++ {
		id := string(rune('a' + i))
		issue := Issue{ID: id, Status: StatusOpen}
		if i > 0 {
			issue.Dependencies = []*Dependency{{DependsOnID: string(rune('a' + i - 1)), Type: DepParentChild}}
		}
		issues = append(issues, issue)
	}
	if got := NewReadinessIndex(issues).DependencyState(issues[len(issues)-1].ID); got != DependenciesSatisfied {
		t.Fatalf("deep parent state = %s", got)
	}
}

func TestReadinessAllStatusesAreExplicitlyEligible(t *testing.T) {
	now := time.Now()
	for _, status := range []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusDeferred, StatusPinned, StatusHooked, StatusReview, StatusClosed, StatusTombstone, Status("custom-state")} {
		t.Run(string(status), func(t *testing.T) {
			got := NewReadinessIndex([]Issue{{ID: "a", Status: status}}).Ready("a", now)
			want := status == StatusOpen || status == StatusInProgress
			if got != want {
				t.Fatalf("Ready(%s) = %v, want %v", status, got, want)
			}
		})
	}
}

func TestReadinessDirectCycleIsUnsatisfiedAndMissingParentUnknown(t *testing.T) {
	cycle := NewReadinessIndex([]Issue{
		{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "b", Type: DepBlocks}}},
		{ID: "b", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "a", Type: DepBlocks}}},
	})
	if got := cycle.DependencyState("a"); got != DependenciesUnsatisfied {
		t.Fatalf("direct cycle = %s", got)
	}
	missingParent := NewReadinessIndex([]Issue{{ID: "child", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "missing-parent", Type: DepParentChild}}}})
	if got := missingParent.DependencyState("child"); got != DependenciesUnknown {
		t.Fatalf("missing parent = %s", got)
	}
}

func TestReadinessBlockersSortedAndDeduplicated(t *testing.T) {
	r := NewReadinessIndex([]Issue{{ID: "a", Status: StatusOpen, Dependencies: []*Dependency{{DependsOnID: "z", Type: DepBlocks}, {DependsOnID: "x", Type: DepBlocks}, {DependsOnID: "z", Type: "waits-for"}}}})
	got := r.Blockers("a")
	if len(got) != 2 || got[0] != "x" || got[1] != "z" {
		t.Fatalf("Blockers = %v", got)
	}
}

func TestIssueDeferUntilJSONAndClone(t *testing.T) {
	const value = "2026-10-11T12:00:00Z"
	var issue Issue
	if err := json.Unmarshal([]byte(`{"id":"a","defer_until":"`+value+`"}`), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.DeferUntil == nil || issue.DeferUntil.Format(time.RFC3339) != value {
		t.Fatalf("DeferUntil = %v", issue.DeferUntil)
	}
	clone := issue.Clone()
	if clone.DeferUntil == issue.DeferUntil {
		t.Fatal("Clone shared DeferUntil pointer")
	}
	*clone.DeferUntil = time.Time{}
	if issue.DeferUntil.Format(time.RFC3339) != value {
		t.Fatal("clone mutation changed source")
	}
}
