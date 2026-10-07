package ui

import (
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

// registerBuiltinSlots registers bt's own slot providers. They go first so
// built-in badges and sections precede any extension's.
func registerBuiltinSlots(reg *slots.Registry) {
	reg.AddBadges(slots.BadgeFunc(timeBadges))
}

// timeBadges flags overdue beads, or else stale ones.
func timeBadges(issue *model.Issue) []slots.Badge {
	switch {
	case isOverdue(issue):
		s := overdueBadgeStyle()
		return []slots.Badge{{Text: activeGlyphs.Overdue + "DUE", Style: &s}}
	case isStale(issue):
		s := staleBadgeStyle()
		return []slots.Badge{{Text: activeGlyphs.Stale, Style: &s}}
	}
	return nil
}
