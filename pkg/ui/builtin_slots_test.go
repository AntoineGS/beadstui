package ui

import (
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestTimeBadges(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	overdue := &model.Issue{ID: "x-1", Status: model.StatusOpen, DueDate: &past, UpdatedAt: time.Now()}
	if got := timeBadges(overdue); len(got) != 1 || got[0].Text != activeGlyphs.Overdue+"DUE" || got[0].Style == nil {
		t.Fatalf("overdue badges = %+v, want one styled DUE badge", got)
	}

	stale := &model.Issue{ID: "x-2", Status: model.StatusOpen, DueDate: &future, UpdatedAt: time.Now().Add(-60 * 24 * time.Hour)}
	if got := timeBadges(stale); len(got) != 1 || got[0].Text != activeGlyphs.Stale {
		t.Fatalf("stale badges = %+v, want one stale badge", got)
	}

	fresh := &model.Issue{ID: "x-3", Status: model.StatusOpen, UpdatedAt: time.Now()}
	if got := timeBadges(fresh); len(got) != 0 {
		t.Fatalf("fresh badges = %+v, want none", got)
	}
}
