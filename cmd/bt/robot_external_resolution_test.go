package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/internal/bdroute"
	"github.com/seanmartinsmith/beadstui/internal/datasource"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/workspace"
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

// TestAnalysisIssues_ResolvesAgainstLoadedCorpus confirms aliases are resolved
// against the loaded corpus before candidate filtering regardless of scope.
func TestAnalysisIssues_ResolvesAgainstLoadedCorpus(t *testing.T) {
	prev := flagGlobal
	flagGlobal = false
	t.Cleanup(func() { flagGlobal = prev })

	issues := buildCrossProjectIssues()
	rc := &robotCtx{issues: issues}

	got := rc.analysisIssues()
	if got[0].Dependencies[0].DependsOnID != "cass-x" {
		t.Errorf("loaded target was not resolved: got %q", got[0].Dependencies[0].DependsOnID)
	}
	if issues[0].Dependencies[0].DependsOnID != "external:cass:x" {
		t.Fatal("resolver mutated caller input")
	}
}

func TestExportTriageRetainsClosedHiddenExternalGate(t *testing.T) {
	allIssues := buildCrossProjectIssues()
	allIssues[1].SourceRepo = "hidden-repo"
	allIssues[1].Status = model.StatusClosed
	selected := []model.Issue{allIssues[0]}
	_, triage := buildExportAnalysis(selected, allIssues, time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC))
	if triage.QuickRef.ActionableCount != 1 {
		t.Fatalf("export actionable count = %d, want 1 with the closed hidden external prerequisite", triage.QuickRef.ActionableCount)
	}
}

func TestExportWatchReplacementRebuildsFullReadiness(t *testing.T) {
	allIssues := buildCrossProjectIssues()
	allIssues[1].SourceRepo = "hidden-repo"
	selected := []model.Issue{allIssues[0]}
	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

	_, before := buildExportAnalysis(selected, allIssues, now)
	if before.QuickRef.ActionableCount != 0 {
		t.Fatalf("open hidden external gate produced %d actionable candidates, want 0", before.QuickRef.ActionableCount)
	}

	// The watch callback supplies each freshly loaded corpus to the same export
	// analysis builder; closing a hidden prerequisite replaces the old authority.
	allIssues[1].Status = model.StatusClosed
	_, after := buildExportAnalysis(selected, allIssues, now)
	if after.QuickRef.ActionableCount != 1 {
		t.Fatalf("replacement closed gate produced %d actionable candidates, want 1", after.QuickRef.ActionableCount)
	}
}

func TestExportHistoryMembershipUsesRepoScopeBeforeClosedFilter(t *testing.T) {
	fullSource := []model.Issue{
		{ID: "api-open", Status: model.StatusOpen},
		{ID: "api-closed", Status: model.StatusClosed},
		{ID: "web-hidden", Status: model.StatusOpen},
	}
	candidates := filterByRepo(fullSource, "api")
	historyIssues, exportIssues, readinessIssues := exportIssueScopes(candidates, fullSource, false)

	if len(historyIssues) != 2 {
		t.Fatalf("history membership = %+v, want only the two repo-scoped rows", historyIssues)
	}
	if got := []string{historyIssues[0].ID, historyIssues[1].ID}; !reflect.DeepEqual(got, []string{"api-open", "api-closed"}) {
		t.Fatalf("history membership = %v, want repo-scoped rows before closed filtering", got)
	}
	if len(exportIssues) != 1 || exportIssues[0].ID != "api-open" {
		t.Fatalf("export membership = %+v, want only open repo candidate", exportIssues)
	}
	if len(readinessIssues) != len(fullSource) || readinessIssues[2].ID != "web-hidden" {
		t.Fatalf("readiness membership = %+v, want the complete source including hidden repo", readinessIssues)
	}
}

func TestClaimableRecommendationsExcludeNonClaimableIssues(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	deferUntil := now.Add(time.Hour)
	issues := []model.Issue{
		{ID: "ready", Status: model.StatusOpen},
		{ID: "flight", Status: model.StatusInProgress},
		{ID: "assigned", Status: model.StatusOpen, Assignee: "worker"},
		{ID: "deferred", Status: model.StatusOpen, DeferUntil: &deferUntil},
		{ID: "epic", Status: model.StatusOpen, IssueType: model.TypeEpic},
		{ID: "blocked", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "missing", Type: model.DepBlocks}}},
	}
	recommendations := make([]analysis.Recommendation, 0, len(issues))
	for _, issue := range issues {
		recommendations = append(recommendations, analysis.Recommendation{ID: issue.ID})
	}
	got := claimableRecommendations(recommendations, model.NewReadinessIndex(issues), now)
	if len(got) != 1 || got[0].ID != "ready" {
		t.Fatalf("claimable script actions = %+v, want only ready", got)
	}
}

func TestRobotClaimCommandsUseIssueSpecificWorkspaceRoute(t *testing.T) {
	rc := &robotCtx{
		issuesForSearch: []model.Issue{{ID: "cass-a", Status: model.StatusOpen}},
		routeTable:      bdroute.FromWorkspace([]workspace.LoadResult{{Prefix: "cass", AbsPath: "/tmp/cass checkout"}}),
	}
	got := rc.routedBDCommand("cass-a", "update", "cass-a", "--status=in_progress")
	want := "(cd '/tmp/cass checkout' && 'bd' 'update' 'cass-a' '--status=in_progress')"
	if got != want {
		t.Fatalf("routed claim command = %q, want %q", got, want)
	}
	if got := (&robotCtx{issuesForSearch: rc.issuesForSearch}).routedBDCommand("cass-a", "update", "cass-a"); got != "" {
		t.Fatalf("missing route emitted unsafe command %q", got)
	}
	global := &robotCtx{
		issuesForSearch: []model.Issue{{ID: "bg-a", SourceRepo: bdroute.BeadsGlobalDB, Status: model.StatusOpen}},
		routeTable:      bdroute.FromSettings(nil, []string{bdroute.BeadsGlobalDB}),
	}
	if got := global.routedBDCommand("bg-a", "update", "bg-a", "--claim"); got != "" {
		t.Fatalf("unsupported global write emitted unscoped command %q", got)
	}
}

func TestRoutedBDCommandExecutesPosixWithQuotedProjectPath(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal("POSIX sh is required for the routed-command execution test")
	}
	root := t.TempDir()
	project := filepath.Join(root, "project with ' quote")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	stubDir := filepath.Join(root, "stub-bin")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "bd.log")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" > \"$BT_BD_LOG\"\n"
	if err := os.WriteFile(filepath.Join(stubDir, "bd"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	rc := &robotCtx{
		issuesForSearch: []model.Issue{{ID: "bt-a", Status: model.StatusOpen}},
		routeTable:      bdroute.SingleProject(project),
	}
	command := rc.routedBDCommand("bt-a", "show", "bt-a")
	cmd := exec.Command(sh, "-c", command)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"), "BT_BD_LOG="+logPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("execute routed command %q: %v\n%s", command, err, out)
	}
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read stub log: %v", err)
	}
	want := []string{project, "show", "bt-a"}
	if lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n"); !reflect.DeepEqual(lines, want) {
		t.Fatalf("stub cwd/args = %q, want %q", lines, want)
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
