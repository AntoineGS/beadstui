package ui

import (
	"fmt"
	"image/color"
	"io"
	"strings"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// IssueDelegate renders issue items in the list
type IssueDelegate struct {
	Theme             Theme
	ShowPriorityHints bool
	PriorityHints     map[string]*analysis.PriorityRecommendation
	ShowRepoBadges    bool // Row display policy, independent of workspace mode

	// PendingClaims marks bead IDs awaiting a write settle (bt-oiaj.10); a
	// pending row shows ClaimSpinner beside its title. Both are
	// zero-value safe: a nil map and empty frame render exactly as before.
	PendingClaims map[string]bool
	ClaimSpinner  string

	// Slots supplies row badges (overdue/stale and any registered provider).
	// Nil renders no slot badges.
	Slots *slots.Registry
}

func (d IssueDelegate) Height() int {
	return 1
}

func (d IssueDelegate) Spacing() int {
	return 0
}

func (d IssueDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d IssueDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(IssueItem)
	if !ok {
		return
	}

	t := d.Theme
	width := m.Width()
	if width <= 0 {
		width = 80
	}
	isSelected := index == m.Index()

	// ══════════════════════════════════════════════════════════════════════════
	// POLISHED ROW LAYOUT - Stripe-level visual hierarchy
	// Layout: [repo] [type status priority] [ID] [title...] [meta]
	// ══════════════════════════════════════════════════════════════════════════

	// Get all the data. Type, status and priority render as one chip
	// (bt-evuf.2) rather than three separately-padded badges; see
	// RenderIssueChip for why.
	chip := RenderIssueChip(string(i.Issue.IssueType), string(i.Issue.Status), i.Issue.Priority)
	idStr := CompactIssueID(i.Issue.ID, i.RepoPrefix)
	title := i.Issue.Title
	ageStr := FormatTimeRel(i.Issue.UpdatedAt)
	commentCount := len(i.Issue.Comments)

	// Measure actual display width; glyphs vary between 1 and 2 cells.
	chipWidth := lipgloss.Width(chip)

	// Calculate widths for right-side columns (fixed)
	rightWidth := 0
	var rightParts []string

	// Show Age and Comments only if we have reasonable width
	if width > 60 {
		// When the bead has been edited since creation (UpdatedAt !=
		// CreatedAt), prefix the age cell with '~' AND render italic so the
		// signal carries on terminals that don't render italic (the prefix
		// alone suffices) and reads more strongly on those that do. Cell
		// widened to 9 to fit the longest possible value "~11mo ago"
		// (bt-v7um).
		ageStyle := t.MutedText
		if !i.Issue.UpdatedAt.Equal(i.Issue.CreatedAt) {
			ageStr = "~" + ageStr
			ageStyle = t.MutedTextItalic
		}
		rightParts = append(rightParts, ageStyle.Render(fmt.Sprintf("%9s", ageStr)))
		rightWidth += 10

		// Comments with icon - use lipgloss.Width for accurate emoji measurement
		if commentCount > 0 {
			commentStr := fmt.Sprintf("%s%d", activeGlyphs.Comment, commentCount)
			rightParts = append(rightParts, t.InfoText.Render(commentStr))
			rightWidth += lipgloss.Width(commentStr) + 1 // +1 for spacing
		} else {
			rightParts = append(rightParts, "   ")
			rightWidth += 3
		}
	}

	// Sparkline (Graph Score) - visualization of importance
	if width > 120 {
		spark := RenderSparkline(i.GraphScore, 5)
		sparkColor := GetHeatmapColor(i.GraphScore, t)
		sparkStyle := lipgloss.NewStyle().Foreground(sparkColor)
		rightParts = append(rightParts, sparkStyle.Render(spark))
		rightWidth += 6 // 5 + 1 spacing
	}

	// Assignee column - reserved when above width threshold so columns stay
	// aligned across rows (bt-foit). Rows with no assignee render blank
	// padding of the same cell width as a populated cell.
	if width > 100 {
		if i.Issue.Assignee != "" {
			assignee := truncateRunesHelper(i.Issue.Assignee, 12, "…")
			rightParts = append(rightParts, t.SecondaryText.Render(fmt.Sprintf("@%-12s", assignee)))
		} else {
			rightParts = append(rightParts, strings.Repeat(" ", 13))
		}
		rightWidth += 14
	}

	// Author column - creation-time actor, distinct from Assignee. Gated at
	// width > 120. Rendered only when Author differs from Assignee to avoid
	// visual duplication (bt-aw4h). Reserve column space even when hidden so
	// later columns stay aligned across rows (bt-foit).
	if width > 120 {
		if i.Issue.Author != "" && i.Issue.Author != i.Issue.Assignee {
			author := truncateRunesHelper(i.Issue.Author, 10, "…")
			rightParts = append(rightParts, t.MutedText.Render(fmt.Sprintf("%s%-10s", activeGlyphs.Pencil, author)))
		} else {
			rightParts = append(rightParts, strings.Repeat(" ", 11))
		}
		rightWidth += 12
	}

	// Labels column - render as mini tags. Reserve full label-tag width
	// (20 chars + 2 padding = 22 cells) even when row has no labels so the
	// column anchor stays fixed across rows (bt-foit).
	if width > 140 {
		if len(i.Issue.Labels) > 0 {
			labelStr := truncateRunesHelper(strings.Join(i.Issue.Labels, ","), 20, "…")
			labelStyle := lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Background(ColorBgSubtle).
				Padding(0, 1)
			rendered := labelStyle.Render(labelStr)
			// Pad to a stable 22-cell width so column right-edge is aligned.
			if w := lipgloss.Width(rendered); w < 22 {
				rendered = rendered + strings.Repeat(" ", 22-w)
			}
			rightParts = append(rightParts, rendered)
		} else {
			rightParts = append(rightParts, strings.Repeat(" ", 22))
		}
		rightWidth += 23
	}

	// Left side fixed columns.
	// [repo-badge 0-6] [chip measured] [hint 1-2] [id dynamic] [space]
	// Use the measured chip width rather than a hardcoded value so 2-cell
	// glyphs stay aligned.
	leftFixedWidth := chipWidth + 1 // chip(measured) + space(1)

	// Pending writes use an inline indicator, not a permanent gutter.
	var pendingIndicator string
	if d.PendingClaims[i.Issue.ID] {
		pendingIndicator = d.ClaimSpinner
		if pendingIndicator == "" {
			pendingIndicator = claimSpinnerFrame(0)
		}
		leftFixedWidth += lipgloss.Width(pendingIndicator) + 1
	}

	// Account for repo width only when the current project scope needs it.
	var repoBadge string
	if d.ShowRepoBadges && i.RepoPrefix != "" {
		// Create a compact repo badge like [API] or [WEB]. DisplayRepoName
		// aliases the beads_global namespace's bare ID-prefix "global" to
		// "atlas" for display (bt-z1pzj) - RepoPrefix is always ID-derived
		// (ExtractRepoPrefix), so this is the only place it can surface.
		repoBadge = RenderRepoBadge(model.DisplayRepoName(i.RepoPrefix))
		leftFixedWidth += lipgloss.Width(repoBadge) + 1
	}

	// Priority hint indicator
	if d.ShowPriorityHints {
		leftFixedWidth += 2
	}

	// Triage indicator width (bv-151) - use lipgloss.Width for accurate glyph measurement
	if i.IsQuickWin {
		leftFixedWidth += lipgloss.Width(activeGlyphs.Star) + 1 // glyph + space
	} else if i.IsBlocker && i.UnblocksCount > 0 {
		leftFixedWidth += lipgloss.Width(fmt.Sprintf("%s%d", activeGlyphs.Unlock, i.UnblocksCount)) + 1 // glyph+count + space
	} else if i.UnblocksCount > 0 {
		leftFixedWidth += lipgloss.Width(fmt.Sprintf("↪%d", i.UnblocksCount)) + 1 // arrow+count + space
	}

	// Gate/human indicator width (bt-c69c) - only at width > 80
	var gateBadge string
	if width > 80 {
		if i.GateAwaitType != "" {
			gateBadge = RenderGateBadge(i.GateAwaitType)
			leftFixedWidth += lipgloss.Width(gateBadge) + 1
		} else if i.Issue.AwaitType != nil {
			gateBadge = RenderGateBadge(*i.Issue.AwaitType)
			leftFixedWidth += lipgloss.Width(gateBadge) + 1
		} else if hasHumanLabel(i.Issue.Labels) {
			gateBadge = RenderHumanAdvisoryBadge()
			leftFixedWidth += lipgloss.Width(gateBadge) + 1
		}
	}

	// Epic progress indicator (bt-waeh) - only at width > 80
	var epicBadge string
	if width > 80 && i.Issue.IssueType == model.TypeEpic && i.EpicTotal > 0 {
		epicLabel := fmt.Sprintf("%d/%d", i.EpicDone, i.EpicTotal)
		var epicFg color.Color
		if i.EpicDone == i.EpicTotal {
			epicFg = ColorSuccess
		} else if i.EpicDone > 0 {
			epicFg = ColorInfo
		} else {
			epicFg = ColorMuted
		}
		epicBadge = lipgloss.NewStyle().Foreground(epicFg).Render(epicLabel)
		leftFixedWidth += lipgloss.Width(epicBadge) + 1
	}

	// Slot badges (overdue/stale and registered providers) - only at width > 80.
	// Their budget is settled after the ID and diff badge are measured, below.
	var slotBadges []slots.Badge
	if width > 80 {
		slotBadges = d.Slots.Badges(&i.Issue)
	}

	// ID width - use actual visual width, but cap reasonably
	idWidth := lipgloss.Width(idStr)
	if idWidth > 35 {
		idWidth = 35
		idStr = truncateRunesHelper(idStr, 35, "…")
	}
	leftFixedWidth += idWidth + 1

	// Diff badge width adjustment
	if badge := i.DiffStatus.Badge(); badge != "" {
		leftFixedWidth += lipgloss.Width(badge) + 1
	}

	// Badges only get cells the title can spare above its protected minimum.
	badgeStrip, badgeWidth := renderBadgeStrip(slotBadges,
		width-leftFixedWidth-rightWidth-2-minTitleWidthWithBadges, time.Now())
	if badgeWidth > 0 {
		leftFixedWidth += badgeWidth + 1
	}

	// Title gets everything in between
	titleWidth := issueListTitleWidth(width, leftFixedWidth, rightWidth)

	// Truncate title if needed
	title = truncateRunesHelper(title, titleWidth, "…")

	// Pad title to fill space
	currentWidth := lipgloss.Width(title)
	if currentWidth < titleWidth {
		title = title + strings.Repeat(" ", titleWidth-currentWidth)
	}

	// ══════════════════════════════════════════════════════════════════════════
	// BUILD THE ROW
	// ══════════════════════════════════════════════════════════════════════════
	var leftSide strings.Builder

	// Repo badge (workspace mode)
	if repoBadge != "" {
		leftSide.WriteString(repoBadge)
		leftSide.WriteString(" ")
	}

	// Type + status + priority as one chip (bt-evuf.2)
	leftSide.WriteString(chip)
	leftSide.WriteString(" ")

	// Priority hint indicator (↑/↓) - using pre-computed styles
	if d.ShowPriorityHints && d.PriorityHints != nil {
		if hint, ok := d.PriorityHints[i.Issue.ID]; ok {
			if hint.Direction == "increase" {
				leftSide.WriteString(t.PriorityUpArrow.Render("↑"))
			} else if hint.Direction == "decrease" {
				leftSide.WriteString(t.PriorityDownArrow.Render("↓"))
			}
		} else {
			leftSide.WriteString(" ")
		}
		leftSide.WriteString(" ")
	}

	// Triage indicators (bv-151): Quick win star and Unblocks count - using pre-computed styles
	triageIndicator := ""
	if i.IsQuickWin {
		triageIndicator = t.TriageStar.Render(activeGlyphs.Star)
	} else if i.IsBlocker && i.UnblocksCount > 0 {
		triageIndicator = t.TriageUnblocks.Render(fmt.Sprintf("%s%d", activeGlyphs.Unlock, i.UnblocksCount))
	} else if i.UnblocksCount > 0 {
		triageIndicator = t.TriageUnblocksAlt.Render(fmt.Sprintf("↪%d", i.UnblocksCount))
	}
	if triageIndicator != "" {
		leftSide.WriteString(triageIndicator)
		leftSide.WriteString(" ")
	}

	// Gate/human indicator (bt-c69c)
	if gateBadge != "" {
		leftSide.WriteString(gateBadge)
		leftSide.WriteString(" ")
	}

	// Slot badges (overdue/stale and registered providers)
	if badgeStrip != "" {
		leftSide.WriteString(badgeStrip)
		leftSide.WriteString(" ")
	}

	// Epic progress (bt-waeh)
	if epicBadge != "" {
		leftSide.WriteString(epicBadge)
		leftSide.WriteString(" ")
	}

	// ID with secondary styling (using pre-computed style base)
	leftSide.WriteString(t.SecondaryText.Render(idStr))
	leftSide.WriteString(" ")

	// Diff badge (time-travel mode)
	if badge := i.DiffStatus.Badge(); badge != "" {
		leftSide.WriteString(badge)
		leftSide.WriteString(" ")
	}

	if pendingIndicator != "" {
		leftSide.WriteString(lipgloss.NewStyle().Foreground(t.Warning).Render(pendingIndicator))
		leftSide.WriteString(" ")
	}

	titleStyle := lipgloss.NewStyle().Foreground(ColorTextSecondary)
	// bt-9kdo: dim wisps
	if i.Issue.Ephemeral != nil && *i.Issue.Ephemeral {
		titleStyle = titleStyle.Foreground(ColorMuted).Italic(true)
	}
	leftSide.WriteString(titleStyle.Render(title))

	// Right side
	rightSide := strings.Join(rightParts, " ")

	// Combine: left + padding + right
	leftLen := lipgloss.Width(leftSide.String())
	rightLen := lipgloss.Width(rightSide)
	padding := width - leftLen - rightLen
	if padding < 0 {
		padding = 0
	}

	// Construct the row string
	row := leftSide.String() + strings.Repeat(" ", padding) + rightSide

	// Clip before styling: Width wraps overflowing content before MaxWidth can
	// clamp it, which would turn an ultra-narrow list item into multiple rows.
	row = ansi.Truncate(row, width, "")

	// Classic full-row selection replaces inline styles so nested ANSI resets
	// and badge backgrounds cannot punch holes in the highlight.
	rowStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isSelected {
		row = rowStyle.Background(t.Primary).Foreground(ColorBgContrast).Render(ansi.Strip(row))
	} else {
		row = rowStyle.Render(row)
	}

	fmt.Fprint(w, row)
}

func issueListTitleWidth(width, leftFixedWidth, rightWidth int) int {
	titleWidth := width - leftFixedWidth - rightWidth - 2
	if titleWidth < 5 {
		return 5
	}
	return titleWidth
}

// hasHumanLabel returns true if labels contains "human".
func hasHumanLabel(labels []string) bool {
	for _, l := range labels {
		if l == "human" {
			return true
		}
	}
	return false
}
