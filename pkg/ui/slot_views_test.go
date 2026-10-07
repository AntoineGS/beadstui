package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func slotTestIssue() model.Issue {
	return model.Issue{
		ID:        "x-1",
		Title:     "A bead with a reasonably long title for layout",
		Status:    model.StatusOpen,
		IssueType: model.TypeTask,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestBoardCardShowsSlotBadges(t *testing.T) {
	issue := slotTestIssue()
	b := NewBoardModel([]model.Issue{issue}, DefaultTheme())
	b.SetSlots(waitRegistry())
	if out := b.renderCard(issue, 40, false, 0, 0); !strings.Contains(out, "WAIT") {
		t.Fatalf("card missing slot badge:\n%s", out)
	}
}

func TestBoardCardDropsBadgesWhenNarrow(t *testing.T) {
	issue := slotTestIssue()
	b := NewBoardModel([]model.Issue{issue}, DefaultTheme())
	b.SetSlots(waitRegistry())
	if out := b.renderCard(issue, 16, false, 0, 0); strings.Contains(out, "WAIT") {
		t.Fatalf("16-cell card should drop the badge:\n%s", out)
	}
}

func TestTreeNodeShowsSlotBadges(t *testing.T) {
	issue := slotTestIssue()
	tr := NewTreeModel(DefaultTheme())
	tr.SetSize(120, 20)
	tr.SetSlots(waitRegistry())
	out := tr.renderNode(&IssueTreeNode{Issue: &issue}, false)
	if !strings.Contains(out, "WAIT") {
		t.Fatalf("tree node missing slot badge: %q", out)
	}
	if strings.Index(out, "WAIT") > strings.Index(out, "x-1") {
		t.Fatalf("badge should precede the ID: %q", out)
	}
}

func TestEpicsChildRowShowsSlotBadges(t *testing.T) {
	issue := slotTestIssue()
	var e EpicsTreeModel
	e.SetTheme(DefaultTheme())
	e.SetSize(100, 20)
	e.SetSlots(waitRegistry())
	out := e.renderChildRow(epicTreeRow{kind: rowChild, issue: &issue, lastKid: []bool{false, true}}, false)
	if !strings.Contains(out, "WAIT") {
		t.Fatalf("epics child row missing slot badge: %q", out)
	}
	if w := lipgloss.Width(out); w > 100 {
		t.Fatalf("epics child row width %d exceeds 100", w)
	}
}
