package ui

import (
	"fmt"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

// registerBuiltinSlots registers bt's own slot providers. They go first so
// built-in badges and sections precede any extension's.
func registerBuiltinSlots(reg *slots.Registry) {
	reg.AddBadges(slots.BadgeFunc(timeBadges))
	reg.AddSections(slots.SectionFunc(capabilitySections))
}

// capabilitySections lists a bead's cross-project capability labels. It
// only applies in workspace mode, where capabilities link projects.
func capabilitySections(issue *model.Issue, ctx slots.Context) []slots.Section {
	if !ctx.WorkspaceMode {
		return nil
	}
	caps := parseCapabilities(*issue)
	if len(caps) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, cap := range caps {
		switch cap.Type {
		case "export":
			sb.WriteString(fmt.Sprintf("- **exports** `%s`\n", cap.Capability))
		case "provides":
			sb.WriteString(fmt.Sprintf("- **provides** `%s`\n", cap.Capability))
		case "external":
			sb.WriteString(fmt.Sprintf("- **needs** `%s` from `%s`\n", cap.Capability, cap.TargetProject))
		}
	}
	return []slots.Section{{Title: activeGlyphs.Link + " Capabilities", Markdown: sb.String()}}
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
