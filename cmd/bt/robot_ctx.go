package main

import (
	"os"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// robotCtx holds loaded/computed state shared across robot command handlers.
// Flag values (robotTriage, diffSince, labelFilter, etc.) remain package-level
// pflag vars and are accessed directly by handlers.
type robotCtx struct {
	issues            []model.Issue
	issuesForSearch   []model.Issue // pre-label-scope issues for search
	readiness         *model.ReadinessIndex
	evaluatedAt       time.Time
	enc               robotEncoder          // output encoder (json or toon)
	cwd               string                // working directory
	beadsPath         string                // path to beads file (for file-based sources)
	repoName          string                // project/repo name
	dataHash          string                // stable hash of issue data
	labelScopeContext *analysis.LabelHealth // label health context when --label is used
	projectDir        string                // project root (for baselines)
}

// newRobotCtx constructs a robotCtx from loaded/computed state.
func newRobotCtx(issues, issuesForSearch []model.Issue, dataHash, cwd, beadsPath, projectDir string, labelScopeContext *analysis.LabelHealth) *robotCtx {
	if issuesForSearch == nil {
		issuesForSearch = issues
	}
	evaluatedAt := time.Now()
	return &robotCtx{
		issues:            issues,
		issuesForSearch:   issuesForSearch,
		readiness:         model.NewReadinessIndex(analysis.ResolveExternalDeps(issuesForSearch)),
		evaluatedAt:       evaluatedAt,
		enc:               newRobotEncoder(os.Stdout),
		cwd:               cwd,
		beadsPath:         beadsPath,
		repoName:          "",
		dataHash:          dataHash,
		labelScopeContext: labelScopeContext,
		projectDir:        projectDir,
	}
}

// newEncoder creates a fresh robot encoder writing to stdout.
func (rc *robotCtx) newEncoder() robotEncoder {
	return newRobotEncoder(os.Stdout)
}

// analysisIssues returns the issue slice to feed the analysis engine. In
// global mode, external:<project>:<id> deps are resolved against the global
// set before returning so cross-project blockers become real graph edges. In
// single-project mode returns rc.issues unchanged so wire output stays
// byte-identical to pre-resolution history.
//
// Composition rule: this is the single point that returns the graph-ready
// slice. Future preprocessing (label normalization, ID aliasing, etc.)
// composes INSIDE this function — it wraps the existing chain, it does not
// add a sibling rc.Xissues() helper. One pipeline, not N helpers.
func (rc *robotCtx) analysisIssues() []model.Issue {
	corpus := rc.issuesForSearch
	if corpus == nil {
		corpus = rc.issues
	}
	resolved := analysis.ResolveExternalDeps(corpus)
	selected := make(map[string]bool, len(rc.issues))
	for _, issue := range rc.issues {
		selected[issue.ID] = true
	}
	out := make([]model.Issue, 0, len(rc.issues))
	for _, issue := range resolved {
		if selected[issue.ID] {
			out = append(out, issue)
		}
	}
	return out
}

func (rc *robotCtx) selectedIDs() []string {
	ids := make([]string, 0, len(rc.issues))
	for _, issue := range rc.issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

func (rc *robotCtx) newAnalyzer() *analysis.Analyzer {
	return analysis.NewAnalyzerWithReadiness(rc.analysisIssues(), rc.readiness, rc.evaluatedAt)
}

func (rc *robotCtx) triageOptions(opts analysis.TriageOptions) analysis.TriageOptions {
	opts.Readiness = rc.readiness
	opts.SelectedIDs = rc.selectedIDs()
	opts.EvaluatedAt = rc.evaluatedAt
	return opts
}
