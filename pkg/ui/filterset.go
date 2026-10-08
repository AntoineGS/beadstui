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
