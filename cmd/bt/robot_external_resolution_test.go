package main

import (
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/internal/datasource"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// buildCrossProjectIssues returns a bt→external:cass:x→cass-y chain exercising
// the resolver's happy path: one external dep that resolves.
func buildCrossProjectIssues() []model.Issue {
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	return []model.Issue{
		{
			ID: "bt-a", Title: "bt consumer", Status: model.StatusOpen,
			IssueType: model.TypeTask, CreatedAt: now, UpdatedAt: now,
			Dependencies: []*model.Dependency{
				{IssueID: "bt-a", DependsOnID: "external:cass:x", Type: model.DepBlocks, CreatedAt: now},
			},
		},
		{
			ID: "cass-x", Title: "cass middle", Status: model.StatusOpen,
			IssueType: model.TypeTask, CreatedAt: now, UpdatedAt: now,
			Dependencies: []*model.Dependency{
				{IssueID: "cass-x", DependsOnID: "cass-y", Type: model.DepBlocks, CreatedAt: now},
			},
		},
		{
			ID: "cass-y", Title: "cass root", Status: model.StatusOpen,
			IssueType: model.TypeTask, CreatedAt: now, UpdatedAt: now,
		},
	}
}

// TestAnalysisIssues_SingleProjectIsIdentity confirms the byte-identical
// single-project acceptance criterion: without --global, rc.analysisIssues()
// returns the exact same slice header the caller put in. Equality here is
// reference equality so any silent cloning regression trips the test.
func TestAnalysisIssues_SingleProjectIsIdentity(t *testing.T) {
	prev := flagGlobal
	flagGlobal = false
	t.Cleanup(func() { flagGlobal = prev })

	issues := buildCrossProjectIssues()
	rc := &robotCtx{issues: issues}

	got := rc.analysisIssues()
	if &got[0] != &issues[0] {
		t.Errorf("single-project mode did not return caller's slice (got header mismatch)")
	}
	// External dep must still be present as a raw string — the analyzer will
	// silently skip it, which is the current pre-resolution behavior.
	if got[0].Dependencies[0].DependsOnID != "external:cass:x" {
		t.Errorf("single-project mode mutated deps: got %q", got[0].Dependencies[0].DependsOnID)
	}
}

// TestAnalysisIssues_GlobalResolvesExternals confirms that --global routes
// through the resolver and that the resulting slice is traversable by the
// graph engine across project boundaries.
func TestAnalysisIssues_GlobalResolvesExternals(t *testing.T) {
	prev := flagGlobal
	flagGlobal = true
	t.Cleanup(func() { flagGlobal = prev })

	issues := buildCrossProjectIssues()
	rc := &robotCtx{issues: issues}

	got := rc.analysisIssues()
	if got[0].Dependencies[0].DependsOnID != "cass-x" {
		t.Errorf("global mode did not rewrite external dep: got %q", got[0].Dependencies[0].DependsOnID)
	}

	// Feeding the resolved slice into the real analyzer must produce a
	// cross-project blocker chain, satisfying the acceptance criterion that
	// blocker-chain --global follows chains across project boundaries.
	analyzer := analysis.NewAnalyzer(got)
	_ = analyzer.Analyze()
	chain := analyzer.GetBlockerChain("bt-a")
	if chain == nil || !chain.IsBlocked {
		t.Fatalf("bt-a not blocked after resolution: %+v", chain)
	}
	var crossed bool
	for _, hop := range chain.Chain {
		if hop.ID == "cass-x" || hop.ID == "cass-y" {
			crossed = true
			break
		}
	}
	if !crossed {
		t.Errorf("blocker chain did not cross into cass project: %+v", chain.Chain)
	}

	// Guard against input mutation leaking out of the helper.
	if issues[0].Dependencies[0].DependsOnID != "external:cass:x" {
		t.Errorf("resolver mutated rc.issues caller data: got %q", issues[0].Dependencies[0].DependsOnID)
	}
}

// TestAnalysisIssues_AutoGlobalResolvesExternals guards bt-vdn2m: the no-flag
// default resolves to the shared Dolt server (flagGlobal stays false) and must
// still resolve external deps like --global does.
func TestAnalysisIssues_AutoGlobalResolvesExternals(t *testing.T) {
	prevFlag, prevSrc := flagGlobal, appCtx.selectedSource
	flagGlobal = false
	appCtx.selectedSource = &datasource.DataSource{Type: datasource.SourceTypeDoltGlobal}
	t.Cleanup(func() { flagGlobal, appCtx.selectedSource = prevFlag, prevSrc })

	rc := &robotCtx{issues: buildCrossProjectIssues()}
	got := rc.analysisIssues()
	if got[0].Dependencies[0].DependsOnID != "cass-x" {
		t.Errorf("auto-global mode did not rewrite external dep: got %q", got[0].Dependencies[0].DependsOnID)
	}
}
