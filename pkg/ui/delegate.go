package ui

import (
	"fmt"
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
	// pending row shows ClaimSpinner in its right-side indicator column. Both are
	// zero-value safe: a nil map and empty frame render exactly as before.
	PendingClaims map[string]bool
	ClaimSpinner  string

	// Slots supplies row badges (overdue/stale and any registered provider).
	// Nil renders no slot badges.
	Slots *slots.Registry

	// prepared is local to one list render, never retained across updates.
	prepared *issueListRender
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

	now := time.Now()
	var layout issueListLayout
	if d.prepared != nil {
		layout, now = d.prepared.layout, d.prepared.now
	} else {
		layout = d.listLayout(m, width, now)
	}
	repo, chip, id := issueListLeftCells(i, d.ShowRepoBadges)
	left := ""
	if layout.repoWidth > 0 {
		left = padIssueListCell(repo, layout.repoWidth) + " "
	}
	left += padIssueListCell(chip, layout.chipWidth) + " " + t.SecondaryText.Render(padIssueListCell(id, layout.idWidth)) + " "
	cells := d.issueListRightCells(i, width, layout.slotBudget, now)
	var rightParts []string
	for column, cellWidth := range layout.rightWidths {
		if cellWidth == 0 {
			continue
		}
		cell := cells[column]
		if column == issueColAge {
			cell = strings.Repeat(" ", max(0, cellWidth-lipgloss.Width(cell))) + cell
		} else {
			cell = padIssueListCell(cell, cellWidth)
		}
		rightParts = append(rightParts, cell)
	}
	right := strings.Join(rightParts, " ")
	titleWidth := max(0, width-lipgloss.Width(left)-layout.rightWidth())
	title := padIssueListCell(truncateRunesHelper(i.Issue.Title, titleWidth, "…"), titleWidth)
	titleStyle := lipgloss.NewStyle().Foreground(ColorTextSecondary)
	if i.Issue.Ephemeral != nil && *i.Issue.Ephemeral {
		titleStyle = titleStyle.Foreground(ColorMuted).Italic(true)
	}
	row := left + titleStyle.Render(title)
	if right != "" {
		row += " " + right
	}
	markerWidth := layout.markerWidth
	quickWinMarker := strings.Repeat(" ", markerWidth)
	if i.IsQuickWin && lipgloss.Width(activeGlyphs.Bolt) <= markerWidth {
		quickWinMarker = strings.Repeat(" ", markerWidth-lipgloss.Width(activeGlyphs.Bolt)) + t.TriageStar.Render(activeGlyphs.Bolt)
	}

	// Clip and pad the body before appending the reserved marker so even an
	// ultra-narrow row cannot wrap or clip the quick-win bolt off the edge.
	bodyWidth := width - markerWidth
	row = ansi.Truncate(row, bodyWidth, "")
	row += strings.Repeat(" ", bodyWidth-lipgloss.Width(row)) + quickWinMarker

	// Classic full-row selection replaces inline styles so nested ANSI resets
	// and badge backgrounds cannot punch holes in the highlight.
	rowStyle := lipgloss.NewStyle().Width(width).MaxWidth(width)
	if isSelected {
		row = rowStyle.Background(t.Highlight).Foreground(ColorText).Render(ansi.Strip(row))
	} else {
		row = rowStyle.Render(row)
	}

	fmt.Fprint(w, row)
}

// Column widths are shared by the rows on the current page. Empty columns take
// no space; empty cells in populated columns retain their neighbors' anchors.
const (
	issueColHint = iota
	issueColTriage
	issueColGate
	issueColSlots
	issueColEpic
	issueColDiff
	issueColPending
	issueColAge
	issueColComments
	issueColSpark
	issueColAssignee
	issueColAuthor
	issueColLabels
	issueColCount
)

type issueListLayout struct {
	repoWidth, chipWidth, idWidth int
	markerWidth, slotBudget       int
	rightWidths                   [issueColCount]int
}

type issueListRender struct {
	layout issueListLayout
	now    time.Time
}

// Unlike plain-text padding, metadata padding must ignore its ANSI styling.
func padIssueListCell(s string, width int) string {
	s = ansi.Truncate(s, width, "")
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

func (l issueListLayout) leftWidth() int {
	w := l.chipWidth + 1 + l.idWidth + 1
	if l.repoWidth > 0 {
		w += l.repoWidth + 1
	}
	return w
}

func (l issueListLayout) rightWidth() int {
	w := l.markerWidth
	for _, cellWidth := range l.rightWidths {
		if cellWidth > 0 {
			w += cellWidth + 1
		}
	}
	return w
}

func displayedIssueItems(m list.Model) []list.Item {
	items := m.VisibleItems()
	start := min(len(items), max(0, m.Paginator.Page)*max(1, m.Paginator.PerPage))
	end := min(len(items), start+max(1, m.Paginator.PerPage))
	return items[start:end]
}

func issueListLeftCells(i IssueItem, showRepo bool) (repo, chip, id string) {
	if showRepo && i.RepoPrefix != "" {
		repo = RenderRepoBadge(model.DisplayRepoName(i.RepoPrefix))
	}
	chip = RenderIssueChip(string(i.Issue.IssueType), string(i.Issue.Status), i.Issue.Priority)
	id = truncateRunesHelper(CompactIssueID(i.Issue.ID, i.RepoPrefix), 35, "…")
	return
}

func issueListLeftLayout(m list.Model, showRepo bool) issueListLayout {
	return measureIssueListLeft(displayedIssueItems(m), showRepo)
}

func measureIssueListLeft(items []list.Item, showRepo bool) issueListLayout {
	l := issueListLayout{chipWidth: 5, idWidth: 2}
	if showRepo {
		l.repoWidth = 4 // REPO header when the page is empty
	}
	for _, item := range items {
		if i, ok := item.(IssueItem); ok {
			repo, chip, id := issueListLeftCells(i, showRepo)
			l.repoWidth = max(l.repoWidth, lipgloss.Width(repo))
			l.chipWidth = max(l.chipWidth, lipgloss.Width(chip))
			l.idWidth = max(l.idWidth, lipgloss.Width(id))
		}
	}
	return l
}

func (d IssueDelegate) listLayout(m list.Model, width int, now time.Time) issueListLayout {
	// VisibleItems copies filtered results, so fetch once and reuse the page.
	items := displayedIssueItems(m)
	l := measureIssueListLeft(items, d.ShowRepoBadges)
	l.markerWidth = min(width, lipgloss.Width(activeGlyphs.Bolt)+1)
	l.slotBudget = maxBadgeStripWidth
	for _, item := range items {
		if i, ok := item.(IssueItem); ok {
			cells := d.issueListRightCells(i, width, l.slotBudget, now)
			for column, cell := range cells {
				l.rightWidths[column] = max(l.rightWidths[column], lipgloss.Width(cell))
			}
		}
	}
	// Slot providers only get space above the protected title minimum. Measure
	// their reduced strips again so a dropped badge cannot leave a phantom gap.
	l.rightWidths[issueColSlots] = 0
	l.slotBudget = max(0, width-l.leftWidth()-l.rightWidth()-minTitleWidthWithBadges-1)
	for _, item := range items {
		if i, ok := item.(IssueItem); ok && width > 80 {
			_, w := renderBadgeStrip(d.Slots.Badges(&i.Issue), l.slotBudget, now)
			l.rightWidths[issueColSlots] = max(l.rightWidths[issueColSlots], w)
		}
	}
	// When unusually long IDs/metadata exhaust the title, drop lower-value
	// columns consistently for the whole page, not independently per row.
	for _, column := range []int{issueColLabels, issueColAuthor, issueColSpark, issueColAssignee, issueColSlots, issueColEpic, issueColComments, issueColAge, issueColHint, issueColDiff, issueColTriage, issueColGate, issueColPending} {
		if width-l.leftWidth()-l.rightWidth() >= minTitleWidthWithBadges {
			break
		}
		l.rightWidths[column] = 0
	}
	return l
}

func (d IssueDelegate) issueListRightCells(i IssueItem, width, slotBudget int, now time.Time) [issueColCount]string {
	t := d.Theme
	var cells [issueColCount]string
	if d.ShowPriorityHints {
		if hint := d.PriorityHints[i.Issue.ID]; hint != nil {
			switch hint.Direction {
			case "increase":
				cells[issueColHint] = t.PriorityUpArrow.Render("↑")
			case "decrease":
				cells[issueColHint] = t.PriorityDownArrow.Render("↓")
			}
		}
	}
	if !i.IsQuickWin && i.UnblocksCount > 0 {
		if i.IsBlocker {
			cells[issueColTriage] = t.TriageUnblocks.Render(fmt.Sprintf("%s%d", activeGlyphs.Unlock, i.UnblocksCount))
		} else {
			cells[issueColTriage] = t.TriageUnblocksAlt.Render(fmt.Sprintf("↪%d", i.UnblocksCount))
		}
	}
	if width > 80 {
		switch {
		case i.GateAwaitType != "":
			cells[issueColGate] = RenderGateBadge(i.GateAwaitType)
		case i.Issue.AwaitType != nil:
			cells[issueColGate] = RenderGateBadge(*i.Issue.AwaitType)
		case hasHumanLabel(i.Issue.Labels):
			cells[issueColGate] = RenderHumanAdvisoryBadge()
		}
		cells[issueColSlots], _ = renderBadgeStrip(d.Slots.Badges(&i.Issue), slotBudget, now)
		if i.Issue.IssueType == model.TypeEpic && i.EpicTotal > 0 {
			fg := ColorMuted
			if i.EpicDone == i.EpicTotal {
				fg = ColorSuccess
			} else if i.EpicDone > 0 {
				fg = ColorInfo
			}
			cells[issueColEpic] = lipgloss.NewStyle().Foreground(fg).Render(fmt.Sprintf("%d/%d", i.EpicDone, i.EpicTotal))
		}
	}
	cells[issueColDiff] = i.DiffStatus.Badge()
	if d.PendingClaims[i.Issue.ID] {
		spinner := d.ClaimSpinner
		if spinner == "" {
			spinner = claimSpinnerFrame(0)
		}
		cells[issueColPending] = lipgloss.NewStyle().Foreground(t.Warning).Render(spinner)
	}
	if width > 60 {
		ageStyle := t.MutedText
		if !i.Issue.UpdatedAt.Equal(i.Issue.CreatedAt) {
			ageStyle = t.MutedTextItalic
		}
		cells[issueColAge] = ageStyle.Render(FormatTimeRel(i.Issue.UpdatedAt))
		if len(i.Issue.Comments) > 0 {
			cells[issueColComments] = t.InfoText.Render(fmt.Sprintf("%s%d", activeGlyphs.Comment, len(i.Issue.Comments)))
		}
	}
	if width > 100 && i.Issue.Assignee != "" {
		cells[issueColAssignee] = t.SecondaryText.Render("@" + truncateRunesHelper(i.Issue.Assignee, 12, "…"))
	}
	if width > 120 {
		cells[issueColSpark] = lipgloss.NewStyle().Foreground(GetHeatmapColor(i.GraphScore, t)).Render(RenderSparkline(i.GraphScore, 5))
		if i.Issue.Author != "" && i.Issue.Author != i.Issue.Assignee {
			cells[issueColAuthor] = t.MutedText.Render(activeGlyphs.Pencil + truncateRunesHelper(i.Issue.Author, 10, "…"))
		}
	}
	if width > 140 && len(i.Issue.Labels) > 0 {
		cells[issueColLabels] = lipgloss.NewStyle().Foreground(ColorPrimary).Background(ColorBgSubtle).Padding(0, 1).
			Render(truncateRunesHelper(strings.Join(i.Issue.Labels, ","), 20, "…"))
	}
	return cells
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
