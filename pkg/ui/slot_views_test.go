package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
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

func TestBoardCardHeightIsStableAcrossWidths(t *testing.T) {
	overdue := slotTestIssue()
	overdue.ID = "bt-x33ev"
	past := time.Now().Add(-48 * time.Hour)
	overdue.DueDate = &past
	builtin := slots.NewRegistry()
	registerBuiltinSlots(builtin)

	cases := []struct {
		name  string
		issue model.Issue
		reg   *slots.Registry
	}{
		{"wait badge", slotTestIssue(), waitRegistry()},
		{"overdue builtin", overdue, builtin},
	}
	for _, c := range cases {
		plain := NewBoardModel([]model.Issue{c.issue}, DefaultTheme())
		withSlots := NewBoardModel([]model.Issue{c.issue}, DefaultTheme())
		withSlots.SetSlots(c.reg)
		for width := 16; width <= 60; width++ {
			want := strings.Count(plain.renderCard(c.issue, width, false, 0, 0), "\n")
			got := strings.Count(withSlots.renderCard(c.issue, width, false, 0, 0), "\n")
			if got != want {
				t.Errorf("%s: card width %d has %d lines with slots, %d without", c.name, width, got+1, want+1)
			}
		}
	}
}
