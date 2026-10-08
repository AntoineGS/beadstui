package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
)

func assertActionableBounds(t *testing.T, m *ActionableModel) string {
	t.Helper()
	out := ansi.Strip(m.Render())
	if m.width <= 0 || m.height <= 0 {
		if out != "" {
			t.Fatalf("invalid size rendered %q", out)
		}
		return out
	}
	if lipgloss.Height(out) > m.height {
		t.Fatalf("height exceeded: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("width exceeded: %q", line)
		}
	}
	return out
}

func TestActionableSelectedItemVisibleAfterNavigation(t *testing.T) {
	plan := analysis.ExecutionPlan{Summary: analysis.PlanSummary{HighestImpact: "issue-00", ImpactReason: "Unblocks work", UnblocksCount: 2}}
	for i := 0; i < 24; i++ {
		plan.Tracks = append(plan.Tracks, analysis.ExecutionTrack{TrackID: fmt.Sprintf("track-%02d", i), Reason: "Single actionable item", Items: []analysis.PlanItem{{ID: fmt.Sprintf("issue-%02d", i), Title: "Readable title", UnblocksIDs: []string{"next"}}}})
	}
	m := NewActionableModel(plan, DefaultTheme())
	m.SetSize(64, 9)
	for i := 0; i < 24; i++ {
		out := assertActionableBounds(t, &m)
		if !strings.Contains(out, m.SelectedIssueID()) {
			t.Fatalf("selection %s missing:\n%s", m.SelectedIssueID(), out)
		}
		if !strings.Contains(out, "ACTIONABLE ITEMS") || !strings.Contains(out, "RECOMMENDED") {
			t.Fatalf("chrome scrolled away:\n%s", out)
		}
		if !strings.Contains(out, "Unblocks: next") {
			t.Fatalf("selected detail missing:\n%s", out)
		}
		m.MoveDown()
	}
}

func TestActionableBounds(t *testing.T) {
	for _, size := range [][2]int{{-1, 8}, {0, 8}, {8, 0}, {1, 1}, {12, 1}, {20, 2}, {40, 6}, {80, 20}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := NewActionableModel(analysis.ExecutionPlan{Tracks: []analysis.ExecutionTrack{{TrackID: "track-界面", Reason: strings.Repeat("reason\n", 50), Items: []analysis.PlanItem{{ID: "x", Title: strings.Repeat("界面 e\u0301\n\t", 50), UnblocksIDs: []string{"next"}}}}}}, DefaultTheme())
			m.SetSize(size[0], size[1])
			out := assertActionableBounds(t, &m)
			if size == [2]int{12, 1} && !strings.Contains(out, "x") {
				t.Fatalf("tiny view lost selection: %q", out)
			}
		})
	}
}

func newTestTheme() Theme {
	return DefaultTheme()
}

func TestActionableRenderEmpty(t *testing.T) {
	m := NewActionableModel(analysis.ExecutionPlan{}, newTestTheme())
	m.SetSize(80, 20)

	out := m.Render()
	if !strings.Contains(out, "No actionable items") {
		t.Fatalf("expected empty state message, got:\n%s", out)
	}
}

func TestActionableNavigationAcrossTracks(t *testing.T) {
	plan := analysis.ExecutionPlan{
		Tracks: []analysis.ExecutionTrack{
			{TrackID: "track-A", Items: []analysis.PlanItem{{ID: "A1", Title: "First"}}},
			{TrackID: "track-B", Items: []analysis.PlanItem{{ID: "B1", Title: "Second"}}},
		},
	}

	m := NewActionableModel(plan, newTestTheme())
	m.SetSize(80, 20)

	if got := m.SelectedIssueID(); got != "A1" {
		t.Fatalf("expected initial selection A1, got %s", got)
	}

	m.MoveDown() // should move to next track/item
	if got := m.SelectedIssueID(); got != "B1" {
		t.Fatalf("expected selection B1 after MoveDown, got %s", got)
	}

	m.MoveUp()
	if got := m.SelectedIssueID(); got != "A1" {
		t.Fatalf("expected selection back to A1 after MoveUp, got %s", got)
	}
}

func TestActionableRenderShowsSummary(t *testing.T) {
	plan := analysis.ExecutionPlan{
		Tracks: []analysis.ExecutionTrack{
			{
				TrackID: "track-A",
				Items:   []analysis.PlanItem{{ID: "ROOT", Title: "Root", Priority: 1, UnblocksIDs: []string{"X", "Y"}}},
			},
		},
		Summary: analysis.PlanSummary{
			HighestImpact: "ROOT",
			ImpactReason:  "Unblocks multiple tasks",
			UnblocksCount: 2,
		},
	}

	m := NewActionableModel(plan, newTestTheme())
	m.SetSize(100, 30)

	out := m.Render()
	if !strings.Contains(out, "Start with ROOT") {
		t.Fatalf("expected summary callout for ROOT, got:\n%s", out)
	}
	if !strings.Contains(out, "→2") {
		t.Fatalf("expected unblocks count badge, got:\n%s", out)
	}
}

func TestActionablePagingMeasuredBodyStep(t *testing.T) {
	plan := analysis.ExecutionPlan{Summary: analysis.PlanSummary{HighestImpact: "issue-00", ImpactReason: "Unblocks work"}}
	for i := 0; i < 60; i++ {
		plan.Tracks = append(plan.Tracks, analysis.ExecutionTrack{TrackID: fmt.Sprintf("track-%02d", i), Items: []analysis.PlanItem{{ID: fmt.Sprintf("issue-%02d", i), Title: "界面 e\u0301", UnblocksIDs: []string{"next-a", "next-b"}}}})
	}
	for _, height := range []int{12, 40} {
		m := NewActionableModel(plan, DefaultTheme())
		m.SetSize(120, height)
		for i := 0; i < 20; i++ {
			m.MoveDown()
		}
		step := max(1, m.layout().bodyHeight/2)
		if step >= 20 || step < 1 {
			t.Fatalf("fixture saturates endpoint: step=%d", step)
		}
		m.PageDown()
		if got, want := m.SelectedIssueID(), fmt.Sprintf("issue-%02d", 20+step); got != want {
			t.Fatalf("height %d page down = %q, want %q (measured step %d)", height, got, want, step)
		}
		assertActionableBounds(t, &m)
		m.PageUp()
		if got := m.SelectedIssueID(); got != "issue-20" {
			t.Fatalf("height %d page up = %q, want issue-20", height, got)
		}
		assertActionableBounds(t, &m)
	}
}

func TestActionableEmptyTracksAndResize(t *testing.T) {
	plan := analysis.ExecutionPlan{Tracks: []analysis.ExecutionTrack{
		{TrackID: "leading"},
		{TrackID: "track-A", Items: []analysis.PlanItem{{ID: "a1", Title: "First", UnblocksIDs: []string{"next"}}}},
		{TrackID: "middle"},
		{TrackID: "track-B", Items: []analysis.PlanItem{{ID: "b1", Title: "Second"}, {ID: "c1", Title: "Third"}}},
		{TrackID: "trailing"},
	}}
	m := NewActionableModel(plan, DefaultTheme())
	visible := func(want string) {
		t.Helper()
		if got := m.SelectedIssueID(); got != want {
			t.Fatalf("selection = %q, want %q", got, want)
		}
		if out := assertActionableBounds(t, &m); !strings.Contains(out, want) {
			t.Fatalf("selection %q missing:\n%s", want, out)
		}
	}
	m.SetSize(64, 20)
	visible("a1")
	if !strings.Contains(m.Render(), "Unblocks: next") {
		t.Fatal("selected detail missing")
	}
	m.MoveDown()
	visible("b1")
	m.SetSize(12, 1)
	visible("b1")
	m.PageDown()
	visible("c1")
	m.MoveDown()
	visible("c1")
	m.PageUp()
	visible("b1")
	m.MoveUp()
	visible("a1")
	m.MoveUp()
	visible("a1")
	m.SetSize(64, 20)
	visible("a1")
	m.PageDown()
	visible("c1")
	m.PageUp()
	visible("a1")
	for _, indices := range [][2]int{{-100, -100}, {100, 100}, {2, -1}, {3, 100}} {
		m.selectedTrack, m.selectedItem = indices[0], indices[1]
		assertActionableBounds(t, &m)
		if m.SelectedIssueID() == "" {
			t.Fatal("invalid indices not normalized")
		}
		m.MoveUp()
		m.MoveDown()
		m.PageUp()
		m.PageDown()
	}
}

func TestActionableNoSelection(t *testing.T) {
	for _, plan := range []analysis.ExecutionPlan{{}, {Tracks: []analysis.ExecutionTrack{{TrackID: "A"}, {TrackID: "B"}}}} {
		m := NewActionableModel(plan, DefaultTheme())
		for _, size := range [][2]int{{80, 20}, {1, 1}, {-1, -1}} {
			m.SetSize(size[0], size[1])
			m.MoveUp()
			m.MoveDown()
			m.PageUp()
			m.PageDown()
			assertActionableBounds(t, &m)
			if m.SelectedIssueID() != "" {
				t.Fatal("empty plan selected an issue")
			}
			if size[0] == 80 && !strings.Contains(m.Render(), "No actionable items") {
				t.Fatal("missing empty state")
			}
		}
	}
}

func TestActionableCellAllocationAndRoleStyles(t *testing.T) {
	theme := DefaultTheme()
	role := func(fg, bg string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg)).Bold(false).Italic(false).Underline(false)
	}
	theme.Text.Title = role("#123456", "#654321")
	theme.Text.Badge = role("#234567", "#765432")
	theme.Text.Selected = role("#345678", "#876543")
	theme.Text.Body = role("#456789", "#987654")
	theme.Text.Metadata = role("#56789a", "#a98765")
	theme.Text.Callout = role("#6789ab", "#ba9876")
	plan := analysis.ExecutionPlan{Tracks: []analysis.ExecutionTrack{{TrackID: "track-界面", Reason: "reason", Items: []analysis.PlanItem{
		{ID: "x", Title: strings.Repeat("界面 e\u0301 ", 30)},
		{ID: "y", Title: "ordinary"},
	}}}, Summary: analysis.PlanSummary{HighestImpact: "x", ImpactReason: strings.Repeat("reason\n\t", 40), UnblocksCount: 2}}
	m := NewActionableModel(plan, theme)
	m.SetSize(60, 20)
	l := m.layout()
	if l.chrome[0] != theme.Text.Title.Padding(0, 2).Width(60).Render(ansi.Truncate(fmt.Sprintf("%s ACTIONABLE ITEMS  │  2 items in 1 tracks", activeGlyphs.Bolt), 56, "…")) {
		t.Fatal("title role not respected")
	}
	summary := fmt.Sprintf("%s RECOMMENDED: Start with x → %s (unblocks 2)", activeGlyphs.Bulb, plan.Summary.ImpactReason)
	if l.chrome[2] != theme.Text.Callout.Padding(0, 2).Width(60).Render(ansi.Truncate(popupChromeLine(summary), 56, "…")) {
		t.Fatal("callout role not respected")
	}
	badge := " TRACK 界面 "
	if !strings.HasPrefix(l.body[0], theme.Text.Badge.Render(badge)) {
		t.Fatal("badge role not respected")
	}
	prefix := "▸ ├─ " + ansi.Strip(GetPriorityIcon(0)) + " x "
	text := ansi.Truncate(plan.Tracks[0].Items[0].Title, 60-ansi.StringWidth(prefix), "…")
	want := theme.Text.Selected.Width(60).Render(prefix + text)
	if l.body[l.selectedStart] != want {
		t.Fatalf("selected row has conflicting styles:\ngot %q\nwant %q", l.body[l.selectedStart], want)
	}
	if ansi.StringWidth(ansi.Strip(want)) != 60 || !strings.Contains(ansi.Strip(want), "界面 e\u0301") {
		t.Fatal("available title cells not used")
	}
	if !strings.Contains(l.body[l.selectedEnd], theme.Text.Body.Render("ordinary")) {
		t.Fatal("ordinary body role not respected")
	}
	assertActionableBounds(t, &m)
	m.plan.Tracks[0].Items[0].ID = strings.Repeat("界面", 100)
	m.plan.Tracks[0].Items[0].Title = "one\n\ttwo\x1b[31mthree"
	m.plan.Tracks[0].Items[0].UnblocksIDs = []string{strings.Repeat("next\n", 100)}
	for _, width := range []int{1, 12, 60} {
		m.SetSize(width, 6)
		assertActionableBounds(t, &m)
	}
}
