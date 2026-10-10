package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// laneTestMode builds a mode with n lanes; an issue goes to the lane
// named by its Assignee ("0".."n-1"), and any other assignee is left off.
func laneTestMode(n int) BoardMode {
	lanes := make([]BoardLane, n)
	for i := range lanes {
		lanes[i] = BoardLane{
			Title: fmt.Sprintf("L%d", i),
			Icon:  func() string { return "*" },
			Color: func(Theme) color.Color { return lipgloss.Color("#ff8800") },
		}
	}
	return BoardMode{
		Key:   SwimLaneMode(fmt.Sprintf("test-%d", n)),
		Name:  fmt.Sprintf("Test %d", n),
		Lanes: lanes,
		Lane: func(issue model.Issue) (int, bool) {
			var lane int
			if _, err := fmt.Sscanf(issue.Assignee, "%d", &lane); err != nil {
				return 0, false
			}
			return lane, true
		},
		ShowEmpty: true,
	}
}

func laneTestBoard(t *testing.T, n int) BoardModel {
	t.Helper()
	var issues []model.Issue
	for i := 0; i < n; i++ {
		issues = append(issues, model.Issue{
			ID:       fmt.Sprintf("lane-%d", i),
			Title:    fmt.Sprintf("Issue in lane %d", i),
			Status:   model.StatusOpen,
			Assignee: fmt.Sprint(i),
		})
	}
	issues = append(issues, model.Issue{ID: "off-board", Title: "No lane", Status: model.StatusOpen, Assignee: "none"})

	b := NewBoardModel(issues, DefaultTheme())
	mode := laneTestMode(n)
	b.modes = append(append([]BoardMode{}, builtinBoardModes...), mode)
	b.SetSwimLaneMode(mode.Key)
	if b.GetSwimLaneMode() != mode.Key {
		t.Fatalf("n=%d: mode not switched, got %q", n, b.GetSwimLaneMode())
	}
	return b
}

func TestBoardVariableLaneCountRendersAndNavigates(t *testing.T) {
	for n := 1; n <= 8; n++ {
		t.Run(fmt.Sprintf("lanes=%d", n), func(t *testing.T) {
			b := laneTestBoard(t, n)

			if got := b.VisibleColumnCount(); got != n {
				t.Fatalf("VisibleColumnCount = %d, want %d", got, n)
			}
			if got := b.TotalCount(); got != n {
				t.Errorf("TotalCount = %d, want %d (issue without a lane must be left off)", got, n)
			}

			out := ansi.Strip(b.View(240, 40))
			for i := 0; i < n; i++ {
				if want := fmt.Sprintf("L%d (1)", i); !strings.Contains(out, want) {
					t.Errorf("missing lane header %q", want)
				}
			}
			if strings.Contains(out, "No lane") {
				t.Error("issue without a lane was rendered")
			}

			for i := 0; i < n; i++ {
				sel := b.SelectedIssue()
				if want := fmt.Sprintf("lane-%d", i); sel == nil || sel.ID != want {
					t.Fatalf("at lane %d selected %v, want %s", i, sel, want)
				}
				b.MoveRight()
			}
			if sel := b.SelectedIssue(); sel == nil || sel.ID != fmt.Sprintf("lane-%d", n-1) {
				t.Errorf("MoveRight past the last lane moved off it, selected %v", sel)
			}

			b.JumpToFirstColumn()
			b.JumpToColumn(n - 1)
			if sel := b.SelectedIssue(); sel == nil || sel.ID != fmt.Sprintf("lane-%d", n-1) {
				t.Errorf("JumpToColumn(%d) selected %v", n-1, sel)
			}
			b.JumpToColumn(n) // out of range: ignored
			if sel := b.SelectedIssue(); sel == nil || sel.ID != fmt.Sprintf("lane-%d", n-1) {
				t.Errorf("out-of-range JumpToColumn moved focus, selected %v", sel)
			}
		})
	}
}

// Lanes that do not fit shrink by the existing width rules instead of
// overflowing the terminal.
func TestBoardManyLanesFitWidth(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		b := laneTestBoard(t, 8)
		for i, line := range strings.Split(b.View(width, 30), "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Fatalf("width %d: line %d is %d wide", width, i, w)
			}
		}
	}
}

func TestBoardModeKeepsItsOwnCursor(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < 5; i++ {
		issues = append(issues, model.Issue{
			ID:        fmt.Sprintf("open-%d", i),
			Status:    model.StatusOpen,
			Priority:  i % 2,
			IssueType: model.TypeTask,
			CreatedAt: createTimeAgo(i),
		})
	}
	issues = append(issues, model.Issue{ID: "closed-0", Status: model.StatusClosed, Priority: 1, IssueType: model.TypeBug})

	b := NewBoardModel(issues, DefaultTheme())
	b.MoveDown()
	b.MoveDown()
	statusSel := b.SelectedIssue().ID

	b.CycleSwimLaneMode() // Priority: fresh cursor
	if sel := b.SelectedIssue(); sel == nil || sel.ID != b.columns[b.actualFocusedCol()][0].ID {
		t.Fatalf("first visit to Priority should start at the top, got %v", sel)
	}
	b.MoveRight()
	b.MoveDown()
	prioritySel := b.SelectedIssue().ID

	b.CycleSwimLaneMode() // Type
	b.CycleSwimLaneMode() // Status again
	if got := b.SelectedIssue().ID; got != statusSel {
		t.Errorf("Status cursor = %s, want %s", got, statusSel)
	}

	b.CycleSwimLaneMode() // Priority again
	if got := b.SelectedIssue().ID; got != prioritySel {
		t.Errorf("Priority cursor = %s, want %s", got, prioritySel)
	}
}

func TestBoardModeCursorClampedAfterFilter(t *testing.T) {
	issues := []model.Issue{
		{ID: "a", Status: model.StatusOpen, CreatedAt: createTimeAgo(0)},
		{ID: "b", Status: model.StatusOpen, CreatedAt: createTimeAgo(1)},
		{ID: "c", Status: model.StatusOpen, CreatedAt: createTimeAgo(2)},
	}
	b := NewBoardModel(issues, DefaultTheme())
	b.MoveToBottom()
	b.CycleSwimLaneMode()
	b.SetIssues(issues[:1])
	b.CycleSwimLaneMode()
	b.CycleSwimLaneMode()
	if sel := b.SelectedIssue(); sel == nil || sel.ID != "a" {
		t.Errorf("restored cursor not clamped to the filtered lane, got %v", sel)
	}
}

func TestBoardSnapshotCacheKeyedByMode(t *testing.T) {
	issues := []model.Issue{
		{ID: "x", Status: model.StatusOpen, Assignee: "0"},
		{ID: "y", Status: model.StatusClosed, Assignee: "1"},
	}
	snap := NewSnapshotBuilder(issues).Build()
	b := NewBoardModel(nil, DefaultTheme())
	b.SetSnapshot(snap)

	cached, ok := snap.BoardState.ColumnsForMode(SwimByStatus)
	if !ok || len(b.columns) != len(cached) || &b.columns[0] != &cached[0] {
		t.Fatal("status lanes should come from the snapshot cache")
	}

	// A mode the snapshot did not precompute is grouped on demand.
	mode := laneTestMode(2)
	b.modes = append(append([]BoardMode{}, builtinBoardModes...), mode)
	b.SetSwimLaneMode(mode.Key)
	if got := b.TotalCount(); got != 2 {
		t.Errorf("uncached mode TotalCount = %d, want 2", got)
	}
}

func createTimeAgo(hours int) time.Time {
	return time.Now().Add(time.Duration(-hours) * time.Hour)
}
