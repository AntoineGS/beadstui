package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

// maxBadgeStripWidth caps the cells all slot badges on one row may take, so
// a chatty provider can never crowd out the title.
const maxBadgeStripWidth = 20

// minTitleWidthWithBadges is the title width renderers protect before giving
// any cells to badges: on narrow terminals badges go first.
const minTitleWidthWithBadges = 20

// pendingRows marks beads with a pending write or plugin action. Board, tree
// and epics rows lead their badge strip with the spinner frame, so it takes
// badge cells rather than widening the row.
type pendingRows struct {
	ids   map[string]bool
	frame string
}

// badges returns issue's slot badges, led by the spinner when issue is pending.
func (p pendingRows) badges(reg *slots.Registry, issue *model.Issue) []slots.Badge {
	b := reg.Badges(issue)
	if issue == nil || !p.ids[issue.ID] {
		return b
	}
	frame := p.frame
	if frame == "" {
		frame = claimSpinnerFrame(0)
	}
	return append([]slots.Badge{{Text: frame, Tone: slots.ToneWarn}}, b...)
}

// badgeStyle maps a badge's tone to theme colours, unless the badge brings
// its own style.
func badgeStyle(b slots.Badge) lipgloss.Style {
	if b.Style != nil {
		return *b.Style
	}
	var fg color.Color
	switch b.Tone {
	case slots.ToneAccent:
		fg = ColorPrimary
	case slots.ToneOK:
		fg = ColorSuccess
	case slots.ToneWarn:
		fg = ColorWarning
	case slots.ToneError:
		fg = ColorDanger
	default:
		fg = ColorMuted
	}
	// Provider tones carry domain meaning; inherit text attributes without
	// turning an unfilled tone indicator into a general filled badge.
	return ActiveTextStyles.Badge.UnsetBackground().Foreground(fg)
}

// compactAge formats a duration for a badge: "<1m", "4m", "3h", "2d".
// Negative durations (clock skew) read as "<1m".
func compactAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// renderBadgeStrip renders badges left to right, one space apart, stopping
// at the first badge that would exceed budget (itself capped at
// maxBadgeStripWidth). Widths are display cells. Returns "" and 0 when no
// badge fits.
func renderBadgeStrip(badges []slots.Badge, budget int, now time.Time) (string, int) {
	if budget > maxBadgeStripWidth {
		budget = maxBadgeStripWidth
	}
	var parts []string
	used := 0
	for _, b := range badges {
		text := b.Text
		if !b.Since.IsZero() {
			text += " " + compactAge(now.Sub(b.Since))
		}
		rendered := badgeStyle(b).Render(text)
		need := lipgloss.Width(rendered)
		if len(parts) > 0 {
			need++
		}
		if used+need > budget {
			break
		}
		parts = append(parts, rendered)
		used += need
	}
	return strings.Join(parts, " "), used
}

// renderSlotSection renders a provider section as detail-pane markdown.
func renderSlotSection(s slots.Section) string {
	return "### " + s.Title + "\n\n" + s.Markdown + "\n\n"
}
