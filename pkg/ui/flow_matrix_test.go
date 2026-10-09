package ui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui"
)

func TestFlowDetailColumnsTextRoleInvariant(t *testing.T) {
	for _, wide := range []bool{false, true} {
		t.Run(fmt.Sprintf("wide_%t", wide), func(t *testing.T) {
			labels := []string{"center"}
			for i := 0; i < 7; i++ {
				name := fmt.Sprintf("out-%d", i)
				if wide {
					name += strings.Repeat("界", 45)
				}
				labels = append(labels, name)
			}
			for i := 0; i < 7; i++ {
				labels = append(labels, fmt.Sprintf("in-%d", i))
			}
			matrix := make([][]int, len(labels))
			for i := range matrix {
				matrix[i] = make([]int, len(labels))
			}
			for i := 1; i <= 7; i++ {
				matrix[0][i] = 2
				matrix[i+7][0] = 1
			}
			flow := &analysis.CrossLabelFlow{Labels: labels, FlowMatrix: matrix, TotalCrossLabelDeps: 21}
			var baseline []int
			for _, styled := range []bool{false, true} {
				theme := ui.DefaultTheme()
				role := lipgloss.NewStyle().Bold(false).Italic(false).Underline(false)
				if styled {
					role = role.Foreground(theme.Warning).Background(theme.Primary).Underline(true)
				}
				theme.Text.Body, theme.Text.Heading, theme.Text.Metadata = role, role, role
				m := ui.NewFlowMatrixModel(theme)
				m.SetSize(200, 30)
				m.SetData(flow, nil)
				if m.SelectedLabel() != "center" {
					t.Fatal("bidirectional fixture did not select center")
				}
				var positions []int
				for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
					sep := strings.Index(line, "│")
					if sep < 0 {
						continue
					}
					detailStart := ansi.StringWidth(line[:sep+len("│")]) + 1
					for _, marker := range []string{"← BLOCKED BY", "in-0", "+1 more"} {
						idx := strings.LastIndex(line, marker)
						if idx > sep {
							positions = append(positions, ansi.StringWidth(line[:idx]))
							offset := 63
							if marker == "in-0" {
								offset += 8
							}
							if marker == "+1 more" {
								offset += 2
							}
							if got := ansi.StringWidth(line[:idx]); got != detailStart+offset {
								t.Errorf("styled=%t marker=%s right-column position=%d, want %d", styled, marker, got, detailStart+offset)
							}
						}
					}
				}
				if len(positions) != 3 {
					t.Fatalf("expected header/entry/more right-column positions, got %v", positions)
				}
				if !styled {
					baseline = positions
				} else {
					for i := range baseline {
						if baseline[i] != positions[i] {
							t.Errorf("Body cosmetics moved right column: plain=%v styled=%v", baseline, positions)
						}
					}
				}
			}
		})
	}
}

func TestFlowTextRole(t *testing.T) {
	theme := ui.DefaultTheme()
	theme.Text.Title = lipgloss.NewStyle().Underline(true).Bold(false)
	theme.Text.Metadata = lipgloss.NewStyle().Underline(true).Italic(false)
	theme.Text.Body = lipgloss.NewStyle().Foreground(theme.Warning).Italic(true).Bold(false)
	m := ui.NewFlowMatrixModel(theme)
	m.SetData(&analysis.CrossLabelFlow{Labels: []string{"api", "web"}, FlowMatrix: [][]int{{0, 2}, {0, 0}}, TotalCrossLabelDeps: 2}, nil)
	m.SetSize(120, 30)
	out := m.View()
	if !strings.Contains(out, theme.Text.Title.PaddingRight(2).Render("DEPENDENCY FLOW")) {
		t.Error("flow title ignored role")
	}
	if plain := ansi.Strip(out); strings.Contains(plain, "Enter") || strings.Contains(plain, "j/k") {
		t.Error("flow view spells out assumed keys")
	}
	if !strings.Contains(out, theme.Text.Body.Render("web (2)")) {
		t.Error("populated outgoing label ignored body role")
	}
}

func TestFlowPlainSelectedTextRole(t *testing.T) {
	theme := ui.DefaultTheme()
	theme.Text.Selected = lipgloss.NewStyle().Foreground(theme.Warning).Background(theme.Primary).Bold(false).Italic(false).Underline(false)
	theme.Text.Heading = lipgloss.NewStyle().Underline(true).Bold(false)
	theme.Text.Metadata = lipgloss.NewStyle().Italic(false).Bold(false)
	m := ui.NewFlowMatrixModel(theme)
	m.SetData(&analysis.CrossLabelFlow{Labels: []string{"api", "web"}, FlowMatrix: [][]int{{0, 2}, {0, 0}}, TotalCrossLabelDeps: 2, BottleneckLabels: []string{"api"}}, nil)
	m.SetSize(120, 30)
	out := m.View()
	if !inSelectedSpan(out, theme, "api", "2") {
		t.Error("selected label row is not one list highlight")
	}
	if !strings.Contains(out, theme.Text.Heading.Render("IMPACT SUMMARY")) {
		t.Error("flow heading ignored role")
	}
	m.SetData(&analysis.CrossLabelFlow{Labels: []string{"api", "web"}, FlowMatrix: [][]int{{0, 2}, {0, 0}}, TotalCrossLabelDeps: 2}, []model.Issue{{ID: "ordinary-id", Title: "ordinary title", Labels: []string{"api"}, Status: model.StatusOpen}})
	m.OpenDrilldown()
	out = m.View()
	if !inSelectedSpan(out, theme, "ordinary-id", "ordinary title") {
		t.Error("selected drilldown row is not one list highlight")
	}
	if plain := ansi.Strip(out); strings.Contains(plain, "j/k") || strings.Contains(plain, "Esc") {
		t.Error("drilldown spells out assumed keys")
	}
}

// inSelectedSpan reports whether all texts sit inside one Text.Selected span,
// the shared list highlight.
func inSelectedSpan(out string, theme ui.Theme, texts ...string) bool {
	open := strings.TrimSuffix(theme.Text.Selected.Render("x"), "x\x1b[m")
	for _, part := range strings.Split(out, open)[1:] {
		span := part
		if i := strings.Index(span, "\x1b["); i >= 0 {
			span = span[:i]
		}
		all := true
		for _, text := range texts {
			all = all && strings.Contains(span, text)
		}
		if all {
			return true
		}
	}
	return false
}

// =============================================================================
// FlowMatrixModel Tests (Interactive Dashboard) - bv-w4l0
// =============================================================================

func testFlowTheme() ui.Theme {
	return ui.DefaultTheme()
}

func TestNewFlowMatrixModel(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	// Should be able to call View() without panic
	view := m.View()
	if view == "" {
		t.Error("NewFlowMatrixModel().View() should not return empty string")
	}

	// SelectedLabel should return empty string for empty model
	if label := m.SelectedLabel(); label != "" {
		t.Errorf("SelectedLabel() = %q, want empty string for new model", label)
	}
}

func TestFlowMatrixModelSetData(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels: []string{"api", "web", "db"},
		FlowMatrix: [][]int{
			{0, 2, 1},
			{1, 0, 3},
			{0, 0, 0},
		},
		TotalCrossLabelDeps: 7,
		BottleneckLabels:    []string{"api"},
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	view := m.View()
	if strings.Contains(view, "No cross-label dependencies found") {
		t.Error("View() should not show 'no dependencies' after SetData with valid flow")
	}

	if label := m.SelectedLabel(); label == "" {
		t.Error("SelectedLabel() should return a label after SetData")
	}
}

func TestFlowMatrixModelSetDataEmpty(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	m.SetData(nil, nil)
	m.SetSize(80, 24)

	view := m.View()
	if !strings.Contains(view, "No cross-label dependencies found") {
		t.Error("View() should show 'no dependencies' for nil flow data")
	}
}

func TestFlowMatrixModelNavigation(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels: []string{"api", "web", "db", "auth", "core"},
		FlowMatrix: [][]int{
			{0, 2, 1, 0, 1},
			{1, 0, 3, 1, 0},
			{0, 0, 0, 0, 0},
			{2, 1, 0, 0, 1},
			{0, 0, 1, 0, 0},
		},
		TotalCrossLabelDeps: 13,
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	initialLabel := m.SelectedLabel()
	if initialLabel == "" {
		t.Fatal("SelectedLabel() should return a label after SetData")
	}

	m.MoveDown()
	newLabel := m.SelectedLabel()
	if newLabel == initialLabel {
		t.Error("MoveDown() should change selected label")
	}

	m.MoveUp()
	backLabel := m.SelectedLabel()
	if backLabel != initialLabel {
		t.Errorf("MoveUp() should restore selection, got %q want %q", backLabel, initialLabel)
	}

	m.GoToEnd()
	if m.SelectedLabel() == "" {
		t.Error("GoToEnd() should select a label")
	}

	m.GoToStart()
	if m.SelectedLabel() != initialLabel {
		t.Errorf("GoToStart() should select first label")
	}
}

func TestFlowMatrixModelBoundary(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels:     []string{"only-one"},
		FlowMatrix: [][]int{{0}},
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	// Should not panic on boundary
	m.MoveDown()
	m.MoveDown()
	m.MoveUp()
	m.MoveUp()
	m.MoveUp()

	if m.SelectedLabel() != "only-one" {
		t.Errorf("SelectedLabel() = %q, want %q", m.SelectedLabel(), "only-one")
	}
}

func TestFlowMatrixModelTogglePanel(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels:     []string{"a", "b"},
		FlowMatrix: [][]int{{0, 1}, {1, 0}},
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	m.TogglePanel()
	view1 := m.View()
	m.TogglePanel()
	view2 := m.View()

	if view1 == "" || view2 == "" {
		t.Error("View() should not be empty after TogglePanel")
	}
}

func TestFlowMatrixModelDrilldown(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels:     []string{"api"},
		FlowMatrix: [][]int{{0}},
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	if m.SelectedDrilldownIssue() != nil {
		t.Error("SelectedDrilldownIssue() should return nil before OpenDrilldown")
	}

	m.OpenDrilldown()
	view := m.View()
	if view == "" {
		t.Error("View() should not be empty after OpenDrilldown")
	}
}

func TestFlowMatrixModelViewRendersContent(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels: []string{"backend", "frontend"},
		FlowMatrix: [][]int{
			{0, 5},
			{2, 0},
		},
		TotalCrossLabelDeps: 7,
		BottleneckLabels:    []string{"backend"},
	}

	m.SetData(flow, nil)
	m.SetSize(100, 30)

	view := m.View()
	if len(view) < 100 {
		t.Errorf("View() seems too short: %d chars", len(view))
	}
}

func TestFlowMatrixModelInvalidMatrix(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	flow := &analysis.CrossLabelFlow{
		Labels:     []string{"a", "b", "c"},
		FlowMatrix: [][]int{{0}}, // Invalid: 1 row for 3 labels
	}

	m.SetData(flow, nil)
	m.SetSize(80, 24)

	// Should handle gracefully without panic
	_ = m.View()
}

func TestFlowMatrixModelEmptyOperations(t *testing.T) {
	theme := testFlowTheme()
	m := ui.NewFlowMatrixModel(theme)

	// Should not panic on empty model
	m.GoToEnd()
	m.GoToStart()
	m.MoveUp()
	m.MoveDown()
	m.TogglePanel()
	m.OpenDrilldown()

	if m.SelectedLabel() != "" {
		t.Errorf("SelectedLabel() = %q, want empty for empty model", m.SelectedLabel())
	}
}
