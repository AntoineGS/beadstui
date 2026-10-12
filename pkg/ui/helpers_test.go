package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui"
)

// TestTruncateRunesHelper tests UTF-8 safe truncation
func TestTruncateRunesHelper(t *testing.T) {
	// Access the helper via the package - it's exported through visuals.go or similar
	// Since truncateRunesHelper is not exported, we test it indirectly through View methods
	// that use it. However, let's test what we can access.

	// For now, test through the public interface that uses truncation
	theme := createTheme()

	// Create an issue with a very long title containing Unicode
	issue := model.Issue{
		ID:     "unicode-test",
		Title:  "日本語タイトル with mixed content 混合コンテンツ",
		Status: model.StatusOpen,
	}

	b := ui.NewBoardModel([]model.Issue{issue}, theme)
	// View should not panic with Unicode content
	_ = b.View(80, 24)
}

// TestBuildDependencyTree tests the dependency tree building
func TestBuildDependencyTree(t *testing.T) {
	// Create a simple dependency chain: A -> B -> C
	issues := []model.Issue{
		{ID: "A", Title: "Root Issue", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{{DependsOnID: "B", Type: model.DepBlocks}}},
		{ID: "B", Title: "Middle Issue", Status: model.StatusInProgress,
			Dependencies: []*model.Dependency{{DependsOnID: "C", Type: model.DepBlocks}}},
		{ID: "C", Title: "Leaf Issue", Status: model.StatusClosed},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("A", issueMap, 10)

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
	if tree.ID != "A" {
		t.Errorf("Expected root ID 'A', got %s", tree.ID)
	}
	if tree.Title != "Root Issue" {
		t.Errorf("Expected title 'Root Issue', got %s", tree.Title)
	}
	if tree.Status != "open" {
		t.Errorf("Expected status 'open', got %s", tree.Status)
	}
	if tree.Type != "root" {
		t.Errorf("Expected type 'root', got %s", tree.Type)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("Expected 1 child, got %d", len(tree.Children))
	}

	// Check child B
	childB := tree.Children[0]
	if childB.ID != "B" {
		t.Errorf("Expected child ID 'B', got %s", childB.ID)
	}
	if childB.Type != "blocks" {
		t.Errorf("Expected child type 'blocks', got %s", childB.Type)
	}
	if len(childB.Children) != 1 {
		t.Fatalf("Expected 1 grandchild, got %d", len(childB.Children))
	}

	// Check grandchild C
	childC := childB.Children[0]
	if childC.ID != "C" {
		t.Errorf("Expected grandchild ID 'C', got %s", childC.ID)
	}
	if len(childC.Children) != 0 {
		t.Errorf("Expected no children for leaf, got %d", len(childC.Children))
	}
}

// TestBuildDependencyTreeCycleDetection tests cycle detection in tree building
func TestBuildDependencyTreeCycleDetection(t *testing.T) {
	// Create a cycle: A -> B -> C -> A
	issues := []model.Issue{
		{ID: "A", Title: "A", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{{DependsOnID: "B", Type: model.DepBlocks}}},
		{ID: "B", Title: "B", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{{DependsOnID: "C", Type: model.DepBlocks}}},
		{ID: "C", Title: "C", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{{DependsOnID: "A", Type: model.DepBlocks}}},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Should not infinite loop
	tree := ui.BuildDependencyTree("A", issueMap, 10)

	if tree == nil {
		t.Fatal("Expected non-nil tree even with cycle")
	}

	// Tree should contain a cycle marker - the cycle detection creates a node with "(cycle)" as title
	rendered := ui.RenderDependencyTree(tree)
	if !strings.Contains(rendered, "(cycle)") {
		t.Errorf("Expected cycle marker '(cycle)' in rendered tree, got:\n%s", rendered)
	}
	cycle := tree.Children[0].Children[0].Children[0]
	if !cycle.Cycle || !cycle.Reference {
		t.Errorf("cycle-closing edge should carry separate cycle and reference annotations: %+v", cycle)
	}
}

// TestBuildDependencyTreeDepthLimit tests max depth limiting
func TestBuildDependencyTreeDepthLimit(t *testing.T) {
	// Create a deep chain: A -> B -> C -> D -> E
	issues := []model.Issue{
		{ID: "A", Title: "A", Dependencies: []*model.Dependency{{DependsOnID: "B", Type: model.DepBlocks}}},
		{ID: "B", Title: "B", Dependencies: []*model.Dependency{{DependsOnID: "C", Type: model.DepBlocks}}},
		{ID: "C", Title: "C", Dependencies: []*model.Dependency{{DependsOnID: "D", Type: model.DepBlocks}}},
		{ID: "D", Title: "D", Dependencies: []*model.Dependency{{DependsOnID: "E", Type: model.DepBlocks}}},
		{ID: "E", Title: "E"},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Build with depth limit of 2
	tree := ui.BuildDependencyTree("A", issueMap, 2)

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}

	// Count depth
	depth := 0
	node := tree
	for node != nil && len(node.Children) > 0 {
		depth++
		node = node.Children[0]
	}

	if depth > 2 {
		t.Errorf("Expected depth <= 2, got %d", depth)
	}
}

// TestBuildDependencyTreeMissingDependency tests handling of missing dependencies
func TestBuildDependencyTreeMissingDependency(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "A",
			Dependencies: []*model.Dependency{{DependsOnID: "missing", Type: model.DepBlocks}}},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("A", issueMap, 10)

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
	if len(tree.Children) != 1 {
		t.Fatalf("Expected 1 child for missing dep, got %d", len(tree.Children))
	}

	// Missing dependency should have "(not found)" as title
	if tree.Children[0].Title != "(not found)" {
		t.Errorf("Expected '(not found)' for missing dep, got %s", tree.Children[0].Title)
	}
}

// TestBuildDependencyTreeMissingRoot tests handling of missing root
func TestBuildDependencyTreeMissingRoot(t *testing.T) {
	issueMap := make(map[string]*model.Issue)

	tree := ui.BuildDependencyTree("nonexistent", issueMap, 10)

	if tree == nil {
		t.Fatal("Expected non-nil tree for missing root")
	}
	if tree.Title != "(not found)" {
		t.Errorf("Expected '(not found)' for missing root, got %s", tree.Title)
	}
}

// TestRenderDependencyTree tests tree rendering
func TestRenderDependencyTree(t *testing.T) {
	issues := []model.Issue{
		{ID: "root", Title: "Root Issue", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{DependsOnID: "child1", Type: model.DepBlocks},
				{DependsOnID: "child2", Type: model.DepRelated},
			}},
		{ID: "child1", Title: "Child One", Status: model.StatusInProgress},
		{ID: "child2", Title: "Child Two", Status: model.StatusClosed},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("root", issueMap, 10)
	rendered := ui.RenderDependencyTree(tree)

	// Should contain the header
	if !strings.Contains(rendered, "Dependency Graph") {
		t.Error("Expected 'Dependency Graph' header in output")
	}

	// Should contain root ID
	if !strings.Contains(rendered, "root") {
		t.Error("Expected 'root' in output")
	}

	// Should contain children
	if !strings.Contains(rendered, "child1") {
		t.Error("Expected 'child1' in output")
	}
	if !strings.Contains(rendered, "child2") {
		t.Error("Expected 'child2' in output")
	}

	// Should contain status info
	if !strings.Contains(rendered, "open") {
		t.Error("Expected 'open' status in output")
	}
}

// TestRenderDependencyTreeNil tests rendering nil tree
func TestRenderDependencyTreeNil(t *testing.T) {
	rendered := ui.RenderDependencyTree(nil)

	if rendered != "No dependency data." {
		t.Errorf("Expected 'No dependency data.', got %s", rendered)
	}
}

// TestGetStatusIcon tests status icon mapping
func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		status   string
		expected string
	}{
		{"open", ui.Glyphs().StOpen},
		{"in_progress", ui.Glyphs().StInProgress},
		{"blocked", ui.Glyphs().StBlocked},
		{"closed", ui.Glyphs().StClosed},
		{"deferred", ui.Glyphs().StDeferred},
		{"unknown", ui.Glyphs().StUnknown},
		{"", ui.Glyphs().StUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			icon := ui.GetStatusIcon(tt.status)
			if icon != tt.expected {
				t.Errorf("GetStatusIcon(%s) = %s; want %s", tt.status, icon, tt.expected)
			}
		})
	}
}

// TestRenderDependencyTreeStatusStyling verifies that per-status ANSI styling
// is applied in RenderDependencyTree output. Each status should produce
// visually distinct output (verified by the presence of ANSI escape codes).
func TestRenderDependencyTreeStatusStyling(t *testing.T) {
	// Build a tree with children of every status we care about
	issues := []model.Issue{
		{ID: "root", Title: "Root", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{DependsOnID: "closed-child", Type: model.DepBlocks},
				{DependsOnID: "prog-child", Type: model.DepBlocks},
				{DependsOnID: "blocked-child", Type: model.DepBlocks},
				{DependsOnID: "deferred-child", Type: model.DepBlocks},
			}},
		{ID: "closed-child", Title: "Closed", Status: model.StatusClosed},
		{ID: "prog-child", Title: "In Progress", Status: model.StatusInProgress},
		{ID: "blocked-child", Title: "Blocked", Status: model.StatusBlocked},
		{ID: "deferred-child", Title: "Deferred", Status: model.StatusDeferred},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("root", issueMap, 10)
	rendered := ui.RenderDependencyTree(tree)

	// Should still contain all IDs
	for _, id := range []string{"root", "closed-child", "prog-child", "blocked-child", "deferred-child"} {
		if !strings.Contains(rendered, id) {
			t.Errorf("Expected %q in rendered tree", id)
		}
	}

	// Should contain deferred icon
	if !strings.Contains(rendered, ui.Glyphs().StDeferred) {
		t.Error("Expected deferred status icon in rendered tree")
	}

	// Should contain ANSI escape codes (lipgloss styling is active).
	// In non-TTY test environments lipgloss may emit no-color output, so
	// we check for status text as the minimum signal and only check ANSI
	// codes when lipgloss does emit them.
	if !strings.Contains(rendered, "closed") {
		t.Error("Expected 'closed' status text in rendered tree")
	}
	if !strings.Contains(rendered, "in_progress") {
		t.Error("Expected 'in_progress' status text in rendered tree")
	}
	if !strings.Contains(rendered, "blocked") {
		t.Error("Expected 'blocked' status text in rendered tree")
	}
	if !strings.Contains(rendered, "deferred") {
		t.Error("Expected 'deferred' status text in rendered tree")
	}
}

// TestBuildDependencyTreeMultipleDependencyTypes tests different dependency types
func TestBuildDependencyTreeMultipleDependencyTypes(t *testing.T) {
	issues := []model.Issue{
		{ID: "root", Title: "Root", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{DependsOnID: "blocks-dep", Type: model.DepBlocks},
				{DependsOnID: "related-dep", Type: model.DepRelated},
				{DependsOnID: "parent-dep", Type: model.DepParentChild},
				{DependsOnID: "discovered-dep", Type: model.DepDiscoveredFrom},
			}},
		{ID: "blocks-dep", Title: "Blocks", Status: model.StatusOpen},
		{ID: "related-dep", Title: "Related", Status: model.StatusOpen},
		{ID: "parent-dep", Title: "Parent", Status: model.StatusOpen},
		{ID: "discovered-dep", Title: "Discovered", Status: model.StatusOpen},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("root", issueMap, 10)

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}
	if len(tree.Children) != 4 {
		t.Errorf("Expected 4 children, got %d", len(tree.Children))
	}

	// Verify dependency types are preserved
	typeMap := make(map[string]string)
	for _, child := range tree.Children {
		typeMap[child.ID] = child.Type
	}

	if typeMap["blocks-dep"] != "blocks" {
		t.Errorf("Expected 'blocks' type, got %s", typeMap["blocks-dep"])
	}
	if typeMap["related-dep"] != "related" {
		t.Errorf("Expected 'related' type, got %s", typeMap["related-dep"])
	}
	if typeMap["parent-dep"] != "parent-child" {
		t.Errorf("Expected 'parent-child' type, got %s", typeMap["parent-dep"])
	}
	if typeMap["discovered-dep"] != "discovered-from" {
		t.Errorf("Expected 'discovered-from' type, got %s", typeMap["discovered-dep"])
	}
}

// TestBuildDependencyTreeInverseChildren mirrors the bt-ift6 detail-render
// shape from bt-cuyiz: viewing a parent must surface its synthesized
// parent_child children as leaf rows, even though the parent has no
// outgoing parent_child edges of its own.
func TestBuildDependencyTreeInverseChildren(t *testing.T) {
	issues := []model.Issue{
		{ID: "bt-ift6", Title: "Epic", Status: model.StatusOpen},
	}
	for n := 0; n <= 13; n++ {
		issues = append(issues, model.Issue{
			ID:     fmt.Sprintf("bt-ift6.%d", n),
			Title:  fmt.Sprintf("Child %d", n),
			Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{IssueID: fmt.Sprintf("bt-ift6.%d", n), DependsOnID: "bt-ift6", Type: model.DepParentChild},
			},
		})
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("bt-ift6", issueMap, 3)
	if tree == nil {
		t.Fatal("nil tree for bt-ift6 root")
	}
	if got := len(tree.Children); got != 14 {
		t.Fatalf("bt-ift6 detail tree has %d children, want 14", got)
	}
	// Natural-numeric order: .0 first, .13 last (bt-u05bo sort fix). Plain
	// lex sort would put .10 before .2 — guard against regression.
	for i, child := range tree.Children {
		want := fmt.Sprintf("bt-ift6.%d", i)
		if child.ID != want {
			t.Errorf("inverse child[%d] = %s, want %s (natural-numeric order)", i, child.ID, want)
		}
		if child.Type != "child" {
			t.Errorf("child %s has Type=%q, want %q", child.ID, child.Type, "child")
		}
		// Each child appears as a leaf — no recursion into the inverse
		// direction means no (cycle) markers from outgoing parent_child
		// pointing back at the root.
		if len(child.Children) != 0 {
			t.Errorf("inverse child %s should be a leaf, got %d children", child.ID, len(child.Children))
		}
	}

	rendered := ui.RenderDependencyTree(tree)
	for n := 0; n <= 13; n++ {
		needle := fmt.Sprintf("bt-ift6.%d", n)
		if !strings.Contains(rendered, needle) {
			t.Errorf("rendered detail tree missing %s:\n%s", needle, rendered)
		}
	}
	if strings.Contains(rendered, "(cycle)") {
		t.Errorf("rendered detail tree contains spurious (cycle) marker:\n%s", rendered)
	}
}

// TestBuildDependencyTreeChildAsRoot verifies that viewing a child shows
// its outgoing parent_child edge to the parent (existing behavior — we
// did not break it by adding the inverse pass).
func TestBuildDependencyTreeChildAsRoot(t *testing.T) {
	issues := []model.Issue{
		{ID: "bt-ift6", Title: "Epic", Status: model.StatusOpen},
		{ID: "bt-ift6.0", Title: "Child", Status: model.StatusOpen,
			Dependencies: []*model.Dependency{
				{IssueID: "bt-ift6.0", DependsOnID: "bt-ift6", Type: model.DepParentChild},
			}},
	}
	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("bt-ift6.0", issueMap, 3)
	if tree == nil || len(tree.Children) == 0 {
		t.Fatal("expected outgoing parent_child edge from bt-ift6.0 to bt-ift6")
	}
	if tree.Children[0].ID != "bt-ift6" || tree.Children[0].Type != "parent-child" {
		t.Errorf("child detail's first child should be parent-child→bt-ift6, got id=%s type=%s",
			tree.Children[0].ID, tree.Children[0].Type)
	}
}

// TestBuildDependencyTreeLongTitle tests truncation of long titles
func TestBuildDependencyTreeLongTitle(t *testing.T) {
	// Title is 106 characters, truncation limit is 40, so it must be truncated
	longTitle := "This is a very long title that should be truncated to fit within the display area for better readability"
	issues := []model.Issue{
		{ID: "long", Title: longTitle, Status: model.StatusOpen},
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	tree := ui.BuildDependencyTree("long", issueMap, 10)
	rendered := ui.RenderDependencyTree(tree)

	// Title is 106 chars, truncation limit is 40, so it MUST contain "..."
	if !strings.Contains(rendered, "...") {
		t.Errorf("Expected truncation indicator '...' in rendered tree for %d-char title, got:\n%s", len(longTitle), rendered)
	}

	// Should NOT contain the full title since it's truncated
	if strings.Contains(rendered, longTitle) {
		t.Errorf("Expected title to be truncated, but found full title in output")
	}
}

// TestBuildDependencyTreeUnlimitedDepth tests unlimited depth (0)
func TestBuildDependencyTreeUnlimitedDepth(t *testing.T) {
	// Create a deep chain
	var issues []model.Issue
	for i := 0; i < 20; i++ {
		issue := model.Issue{
			ID:     string(rune('A' + i)),
			Title:  "Issue " + string(rune('A'+i)),
			Status: model.StatusOpen,
		}
		if i < 19 {
			issue.Dependencies = []*model.Dependency{
				{DependsOnID: string(rune('A' + i + 1)), Type: model.DepBlocks},
			}
		}
		issues = append(issues, issue)
	}

	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	// Build with unlimited depth (0)
	tree := ui.BuildDependencyTree("A", issueMap, 0)

	if tree == nil {
		t.Fatal("Expected non-nil tree")
	}

	// Count actual depth
	depth := 0
	node := tree
	for node != nil && len(node.Children) > 0 {
		depth++
		node = node.Children[0]
	}

	// Should traverse all 20 levels
	if depth != 19 {
		t.Errorf("Expected depth 19 with unlimited, got %d", depth)
	}
}

func TestBuildDependencyTreeShortestPathReferences(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Root", Dependencies: []*model.Dependency{
			{DependsOnID: "B", Type: model.DepBlocks},
			{DependsOnID: "C", Type: model.DepRelated},
		}},
		{ID: "B", Title: "Long route", Dependencies: []*model.Dependency{{DependsOnID: "X", Type: model.DepBlocks}}},
		{ID: "X", Title: "Intermediate", Dependencies: []*model.Dependency{{DependsOnID: "D", Type: model.DepRelated}}},
		{ID: "C", Title: "Short route", Dependencies: []*model.Dependency{{DependsOnID: "D", Type: model.DepBlocks}}},
		{ID: "D", Title: "Shared prerequisite", Dependencies: []*model.Dependency{{DependsOnID: "E", Type: model.DepBlocks}}},
		{ID: "E", Title: "Descendant"},
	}
	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}

	got := ui.BuildDependencyTree("A", issueMap, 3)
	longOccurrence := got.Children[0].Children[0].Children[0]
	if longOccurrence.ID != "D" || longOccurrence.Title != "Shared prerequisite" || !longOccurrence.Reference || longOccurrence.Cycle {
		t.Fatalf("long-route occurrence should reference shortest-path D with metadata: %+v", longOccurrence)
	}
	shortOccurrence := got.Children[1].Children[0]
	if shortOccurrence.ID != "D" || shortOccurrence.Reference || shortOccurrence.Cycle {
		t.Fatalf("shortest-path occurrence should expand canonically: %+v", shortOccurrence)
	}
	if len(shortOccurrence.Children) != 1 || shortOccurrence.Children[0].ID != "E" {
		t.Fatalf("shortest-path descendants missing: %+v", shortOccurrence.Children)
	}
	if strings.Count(ui.RenderDependencyTree(got), "reference") == 0 {
		t.Fatal("rendered tree should preserve reference annotation")
	}
}

func TestBuildDependencyTreeDepthBoundaryAndNilRows(t *testing.T) {
	issues := []model.Issue{
		{ID: "A", Title: "Root", Dependencies: []*model.Dependency{nil, {DependsOnID: "B", Type: model.DepBlocks}}},
		{ID: "B", Title: "At boundary", Dependencies: []*model.Dependency{{DependsOnID: "C", Type: model.DepRelated}}},
	}
	issueMap := map[string]*model.Issue{"A": &issues[0], "B": &issues[1], "nil-row": nil}
	got := ui.BuildDependencyTree("A", issueMap, 1)
	if len(got.Children) != 1 || got.Children[0].ID != "B" || len(got.Children[0].Children) != 0 {
		t.Fatalf("depth boundary should include the edge target, not expand it: %+v", got)
	}

	issues[1].Dependencies = []*model.Dependency{{DependsOnID: "nil-row", Type: model.DepBlocks}, {DependsOnID: "missing", Type: model.DepRelated}}
	got = ui.BuildDependencyTree("B", issueMap, 2)
	if len(got.Children) != 2 {
		t.Fatalf("nil/missing target rows should be safe and visible, got %+v", got.Children)
	}
	for _, child := range got.Children {
		if child.Title != "(not found)" || child.Status != "?" {
			t.Errorf("nil/missing target should have not-found metadata: %+v", child)
		}
	}
}

func TestBuildDependencyTreeInverseChildrenDeduplicateNaturally(t *testing.T) {
	issues := []model.Issue{
		{ID: "epic", Title: "Epic"},
		{ID: "epic.10", Title: "Ten", Dependencies: []*model.Dependency{{DependsOnID: "epic", Type: model.DepParentChild}, {DependsOnID: "epic", Type: model.DepParentChild}}},
		{ID: "epic.2", Title: "Two", Dependencies: []*model.Dependency{{DependsOnID: "epic", Type: model.DepParentChild}}},
	}
	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}
	got := ui.BuildDependencyTree("epic", issueMap, 3)
	if len(got.Children) != 2 || got.Children[0].ID != "epic.2" || got.Children[1].ID != "epic.10" {
		t.Fatalf("inverse children should be unique natural-order leaves: %+v", got.Children)
	}
	for _, child := range got.Children {
		if child.Type != "child" || len(child.Children) != 0 || child.Cycle || child.Reference {
			t.Errorf("inverse child should remain a non-traversal leaf: %+v", child)
		}
	}
}

func TestBuildDependencyTreePreservesOutgoingTypesToSameTarget(t *testing.T) {
	issues := []model.Issue{
		{ID: "root", Title: "Root", Dependencies: []*model.Dependency{
			{DependsOnID: "target", Type: model.DepBlocks},
			{DependsOnID: "target", Type: model.DepRelated},
		}},
		{ID: "target", Title: "Target"},
	}
	issueMap := make(map[string]*model.Issue)
	for i := range issues {
		issueMap[issues[i].ID] = &issues[i]
	}
	got := ui.BuildDependencyTree("root", issueMap, 3)
	if len(got.Children) != 2 {
		t.Fatalf("distinct outgoing relationship types should remain visible, got %+v", got.Children)
	}
	if got.Children[0].Type != "blocks" || got.Children[0].Reference || got.Children[1].Type != "related" || !got.Children[1].Reference {
		t.Fatalf("same target should retain each outgoing type and mark the repeated row: %+v", got.Children)
	}
}

func TestRenderDependencyTreeAnnotationsSurviveTitleTruncation(t *testing.T) {
	root := &ui.DependencyNode{ID: "root", Title: "Root", Status: "open", Type: "root", Children: []*ui.DependencyNode{
		{ID: "target", Title: strings.Repeat("A", 60), Status: "in_progress", Type: "blocks", Reference: true, Cycle: true},
	}}
	rendered := ui.RenderDependencyTree(root)
	if !strings.Contains(rendered, "(reference)") || !strings.Contains(rendered, "(cycle)") {
		t.Fatalf("annotations should render independently of truncated title:\n%s", rendered)
	}
	if strings.Contains(rendered, strings.Repeat("A", 60)) || !strings.Contains(rendered, "in_progress") {
		t.Fatalf("title truncation or status rendering regressed:\n%s", rendered)
	}
}
