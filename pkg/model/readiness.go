package model

import (
	"sort"
	"strings"
	"time"
)

// DependencyState describes the confidence and satisfaction of blocking gates.
type DependencyState string

const (
	DependenciesSatisfied   DependencyState = "satisfied"
	DependenciesUnsatisfied DependencyState = "unsatisfied"
	DependenciesUnknown     DependencyState = "unknown"
)

// ReadinessIndex is an immutable snapshot of issue fields used for eligibility.
// Build one from the complete loaded corpus before applying candidate filters.
type ReadinessIndex struct {
	issues          map[string]readinessIssue
	states          map[string]DependencyState
	openChild       map[string]bool
	reverseBlocking map[string][]string
	children        map[string][]string
}

type readinessIssue struct {
	status     Status
	typeName   IssueType
	assignee   string
	deferUntil *time.Time
	blocking   []string
	parents    []string
}

// NewReadinessIndex copies relevant issue data and precomputes dependency state.
func NewReadinessIndex(issues []Issue) *ReadinessIndex {
	r := &ReadinessIndex{
		issues:          make(map[string]readinessIssue, len(issues)),
		states:          make(map[string]DependencyState, len(issues)),
		openChild:       make(map[string]bool, len(issues)),
		reverseBlocking: make(map[string][]string),
		children:        make(map[string][]string),
	}
	for _, issue := range issues {
		item := readinessIssue{status: issue.Status, typeName: issue.IssueType, assignee: issue.Assignee}
		if issue.DeferUntil != nil {
			v := *issue.DeferUntil
			item.deferUntil = &v
		}
		for _, dep := range issue.Dependencies {
			if dep == nil {
				continue
			}
			if dep.Type == DepParentChild {
				item.parents = append(item.parents, dep.DependsOnID)
				r.children[dep.DependsOnID] = append(r.children[dep.DependsOnID], issue.ID)
			} else if blockingReadinessType(dep.Type) {
				item.blocking = append(item.blocking, dep.DependsOnID)
				r.reverseBlocking[dep.DependsOnID] = append(r.reverseBlocking[dep.DependsOnID], issue.ID)
			}
		}
		r.issues[issue.ID] = item
	}
	for _, issue := range r.issues {
		for _, parent := range issue.parents {
			if p, ok := r.issues[parent]; ok && !resolvedReadinessStatus(p.status) && !resolvedReadinessStatus(issue.status) {
				r.openChild[parent] = true
			}
		}
	}
	visiting := make(map[string]bool, len(issues))
	var state func(string) DependencyState
	state = func(id string) DependencyState {
		if v, ok := r.states[id]; ok {
			return v
		}
		issue, ok := r.issues[id]
		if !ok {
			return DependenciesUnknown
		}
		if resolvedReadinessStatus(issue.status) {
			r.states[id] = DependenciesSatisfied
			return DependenciesSatisfied
		}
		if visiting[id] {
			return DependenciesUnknown
		}
		visiting[id] = true
		result := DependenciesSatisfied
		for _, target := range issue.blocking {
			other, exists := r.issues[target]
			if !exists {
				result = DependenciesUnknown
			} else if !resolvedReadinessStatus(other.status) && result != DependenciesUnknown {
				result = DependenciesUnsatisfied
			}
		}
		for _, parent := range issue.parents {
			other, exists := r.issues[parent]
			if !exists {
				result = DependenciesUnknown
				continue
			}
			if resolvedReadinessStatus(other.status) {
				continue
			}
			parentState := state(parent)
			if parentState == DependenciesUnknown {
				result = DependenciesUnknown
			} else if parentState == DependenciesUnsatisfied && result != DependenciesUnknown {
				result = DependenciesUnsatisfied
			}
		}
		delete(visiting, id)
		r.states[id] = result
		return result
	}
	for id := range r.issues {
		state(id)
	}
	return r
}

// AffectedByCompletion returns the sorted issue IDs whose readiness could
// change after completing id. It follows reverse blocking references from id
// and propagates only through descendants whose readiness inherits a parent.
// Callers still evaluate each result with ReadyAfter against the full index.
func (r *ReadinessIndex) AffectedByCompletion(id string) []string {
	if r == nil {
		return nil
	}
	if _, ok := r.issues[id]; !ok {
		return nil
	}
	affected := make(map[string]struct{})
	parents := []string{id}
	for _, dependent := range r.reverseBlocking[id] {
		if dependent != id {
			affected[dependent] = struct{}{}
			parents = append(parents, dependent)
		}
	}
	visited := make(map[string]bool, len(parents))
	for len(parents) > 0 {
		parent := parents[0]
		parents = parents[1:]
		if visited[parent] {
			continue
		}
		visited[parent] = true
		for _, child := range r.children[parent] {
			if child == id {
				continue
			}
			affected[child] = struct{}{}
			parents = append(parents, child)
		}
	}
	out := make([]string, 0, len(affected))
	for affectedID := range affected {
		out = append(out, affectedID)
	}
	sort.Strings(out)
	return out
}

func blockingReadinessType(kind DependencyType) bool {
	switch strings.TrimSpace(string(kind)) {
	case "", "blocks", "conditional-blocks", "waits-for":
		return true
	default:
		return false
	}
}

func resolvedReadinessStatus(status Status) bool {
	return status == StatusClosed || status == StatusTombstone
}

// DependencyState returns the precomputed state, or unknown for an absent ID.
func (r *ReadinessIndex) DependencyState(id string) DependencyState {
	if r == nil {
		return DependenciesUnknown
	}
	if state, ok := r.states[id]; ok {
		return state
	}
	return DependenciesUnknown
}

// Ready reports whether the issue is eligible for planning at now.
func (r *ReadinessIndex) Ready(id string, now time.Time) bool {
	if r == nil {
		return false
	}
	issue, ok := r.issues[id]
	return ok && (issue.status == StatusOpen || issue.status == StatusInProgress) &&
		(issue.deferUntil == nil || !issue.deferUntil.After(now)) && r.DependencyState(id) == DependenciesSatisfied
}

// Claimable reports whether a ready issue can be recommended as a new claim.
func (r *ReadinessIndex) Claimable(id string, now time.Time) bool {
	issue, ok := r.issue(id)
	return ok && r.Ready(id, now) && issue.status == StatusOpen && issue.assignee == "" &&
		issue.typeName != TypeEpic && !r.openChild[id]
}

// HasDeferralExpiryBetween reports whether any indexed deferral crossed its
// inclusive expiry boundary after start and no later than end.
func (r *ReadinessIndex) HasDeferralExpiryBetween(start, end time.Time) bool {
	if r == nil || !end.After(start) {
		return false
	}
	for _, issue := range r.issues {
		if issue.deferUntil != nil && issue.deferUntil.After(start) && !issue.deferUntil.After(end) {
			return true
		}
	}
	return false
}

func (r *ReadinessIndex) issue(id string) (readinessIssue, bool) {
	if r == nil {
		return readinessIssue{}, false
	}
	issue, ok := r.issues[id]
	return issue, ok
}

// Blockers returns sorted, deduplicated unresolved references, including inherited gates.
func (r *ReadinessIndex) Blockers(id string) []string {
	if r == nil {
		return nil
	}
	if _, ok := r.issues[id]; !ok {
		return nil
	}
	set := make(map[string]struct{})
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	type visit struct {
		id   string
		exit bool
	}
	stack := []visit{{id: id}}
	for len(stack) > 0 {
		last := len(stack) - 1
		step := stack[last]
		stack = stack[:last]
		current := step.id
		if step.exit {
			delete(visiting, current)
			continue
		}
		if visiting[current] {
			set[current] = struct{}{}
			continue
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		visiting[current] = true
		stack = append(stack, visit{id: current, exit: true})
		issue, ok := r.issues[current]
		if !ok {
			set[current] = struct{}{}
			continue
		}
		for _, target := range issue.blocking {
			other, exists := r.issues[target]
			if !exists || !resolvedReadinessStatus(other.status) {
				set[target] = struct{}{}
			}
		}
		for _, parent := range issue.parents {
			other, exists := r.issues[parent]
			if !exists {
				set[parent] = struct{}{}
			} else if !resolvedReadinessStatus(other.status) {
				stack = append(stack, visit{id: parent})
			}
		}
	}
	out := make([]string, 0, len(set))
	for blocker := range set {
		out = append(out, blocker)
	}
	sort.Strings(out)
	return out
}

// ReadyAfter evaluates readiness under a hypothetical completion set without mutating the index.
func (r *ReadinessIndex) ReadyAfter(id string, now time.Time, completed map[string]bool) bool {
	if r == nil {
		return false
	}
	issue, ok := r.issues[id]
	if !ok || (issue.status != StatusOpen && issue.status != StatusInProgress) ||
		(issue.deferUntil != nil && issue.deferUntil.After(now)) {
		return false
	}
	memo := make(map[string]DependencyState)
	visiting := make(map[string]bool)
	var state func(string) DependencyState
	state = func(key string) DependencyState {
		if v, ok := memo[key]; ok {
			return v
		}
		current, exists := r.issues[key]
		if !exists {
			return DependenciesUnknown
		}
		if completed[key] {
			return DependenciesSatisfied
		}
		if resolvedReadinessStatus(current.status) {
			return DependenciesSatisfied
		}
		if visiting[key] {
			return DependenciesUnknown
		}
		visiting[key] = true
		result := DependenciesSatisfied
		for _, target := range current.blocking {
			other, exists := r.issues[target]
			if !exists {
				result = DependenciesUnknown
			} else if completed[target] {
				continue
			} else if !resolvedReadinessStatus(other.status) && result != DependenciesUnknown {
				result = DependenciesUnsatisfied
			}
		}
		for _, parent := range current.parents {
			other, exists := r.issues[parent]
			if !exists {
				result = DependenciesUnknown
				continue
			}
			if completed[parent] {
				continue
			}
			if resolvedReadinessStatus(other.status) {
				continue
			}
			ps := state(parent)
			if ps == DependenciesUnknown {
				result = DependenciesUnknown
			} else if ps == DependenciesUnsatisfied && result != DependenciesUnknown {
				result = DependenciesUnsatisfied
			}
		}
		delete(visiting, key)
		memo[key] = result
		return result
	}
	return state(id) == DependenciesSatisfied
}
