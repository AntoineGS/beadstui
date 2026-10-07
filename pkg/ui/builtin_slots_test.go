package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

func TestCapabilitySections(t *testing.T) {
	issue := &model.Issue{ID: "x-1", Labels: []string{"export:auth", "provides:login", "external:api:tokens", "area:ui"}}

	if got := capabilitySections(issue, slots.Context{}); len(got) != 0 {
		t.Fatalf("outside workspace mode = %+v, want none", got)
	}

	got := capabilitySections(issue, slots.Context{WorkspaceMode: true})
	if len(got) != 1 || !strings.HasSuffix(got[0].Title, "Capabilities") {
		t.Fatalf("workspace mode = %+v, want one Capabilities section", got)
	}
	for _, want := range []string{"- **exports** `auth`", "- **provides** `login`", "- **needs** `tokens` from `api`"} {
		if !strings.Contains(got[0].Markdown, want) {
			t.Errorf("section missing %q:\n%s", want, got[0].Markdown)
		}
	}

	plain := &model.Issue{ID: "x-2", Labels: []string{"area:ui"}}
	if got := capabilitySections(plain, slots.Context{WorkspaceMode: true}); len(got) != 0 {
		t.Fatalf("no capability labels = %+v, want none", got)
	}
}

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
