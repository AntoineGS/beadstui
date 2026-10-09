package ui

import (
	"fmt"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ActionableModel represents the actionable items view grouped by tracks
type ActionableModel struct {
	plan          analysis.ExecutionPlan
	selectedTrack int
	selectedItem  int
	scrollOffset  int
	width         int
	height        int
	theme         Theme
}

// NewActionableModel creates a new actionable view from execution plan
func NewActionableModel(plan analysis.ExecutionPlan, theme Theme) ActionableModel {
	return ActionableModel{
		plan:          plan,
		selectedTrack: 0,
		selectedItem:  0,
		scrollOffset:  0,
		theme:         theme,
	}
}

// SetSize updates the view dimensions
func (m *ActionableModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.ensureVisible()
}

// PageUp moves selection up by a page
func (m *ActionableModel) PageUp() {
	if len(m.plan.Tracks) == 0 {
		return
	}
	pageSize := m.layout().bodyHeight / 2
	if pageSize < 1 {
		pageSize = 1
	}
	for i := 0; i < pageSize; i++ {
		m.MoveUp()
	}
}

// PageDown moves selection down by a page
func (m *ActionableModel) PageDown() {
	if len(m.plan.Tracks) == 0 {
		return
	}
	pageSize := m.layout().bodyHeight / 2
	if pageSize < 1 {
		pageSize = 1
	}
	for i := 0; i < pageSize; i++ {
		m.MoveDown()
	}
}

// MoveUp moves selection up
func (m *ActionableModel) MoveUp() {
	m.normalizeSelection()
	if m.selectedTrack < 0 {
		return
	}

	if m.selectedItem > 0 {
		m.selectedItem--
	} else {
		for i := m.selectedTrack - 1; i >= 0; i-- {
			if len(m.plan.Tracks[i].Items) > 0 {
				m.selectedTrack = i
				m.selectedItem = len(m.plan.Tracks[i].Items) - 1
				break
			}
		}
	}
	m.ensureVisible()
}

// MoveDown moves selection down
func (m *ActionableModel) MoveDown() {
	m.normalizeSelection()
	if m.selectedTrack < 0 {
		return
	}

	track := m.plan.Tracks[m.selectedTrack]
	if m.selectedItem < len(track.Items)-1 {
		m.selectedItem++
	} else {
		for i := m.selectedTrack + 1; i < len(m.plan.Tracks); i++ {
			if len(m.plan.Tracks[i].Items) > 0 {
				m.selectedTrack, m.selectedItem = i, 0
				break
			}
		}
	}
	m.ensureVisible()
}

// SelectedIssueID returns the ID of the currently selected issue
func (m *ActionableModel) SelectedIssueID() string {
	m.normalizeSelection()
	if m.selectedTrack < 0 {
		return ""
	}
	track := m.plan.Tracks[m.selectedTrack]
	return track.Items[m.selectedItem].ID
}

// normalizeSelection chooses the nearest nonempty track, preferring the next
// track on ties. An entirely empty plan has no selection (-1, -1).
func (m *ActionableModel) normalizeSelection() {
	if len(m.plan.Tracks) > 0 {
		origin := min(max(0, m.selectedTrack), len(m.plan.Tracks)-1)
		for distance := 0; distance < len(m.plan.Tracks); distance++ {
			for _, i := range []int{origin + distance, origin - distance} {
				if i >= 0 && i < len(m.plan.Tracks) && len(m.plan.Tracks[i].Items) > 0 {
					if i != m.selectedTrack {
						m.selectedItem = 0
					}
					m.selectedTrack = i
					m.selectedItem = min(max(0, m.selectedItem), len(m.plan.Tracks[i].Items)-1)
					return
				}
			}
		}
	}
	m.selectedTrack, m.selectedItem = -1, -1
}

// ensureVisible adjusts scroll to keep selection visible
func (m *ActionableModel) ensureVisible() {
	m.keepVisible(m.layout())
}

type actionableLayout struct {
	chrome                     []string
	body                       []string
	selectedStart, selectedEnd int // Body indices; end exclusive, -1 when empty.
	bodyHeight                 int
}

func (m *ActionableModel) keepVisible(l actionableLayout) {
	if l.selectedStart >= 0 && l.bodyHeight > 0 {
		m.scrollOffset = min(m.scrollOffset, l.selectedStart)
		end := l.selectedEnd
		if end-l.selectedStart > l.bodyHeight {
			end = l.selectedStart + 1
		}
		m.scrollOffset = max(m.scrollOffset, end-l.bodyHeight)
	}
	m.scrollOffset = min(max(0, m.scrollOffset), max(0, len(l.body)-l.bodyHeight))
}

// layout measures exactly the single-line chrome and scrollable body rendered.
func (m *ActionableModel) layout() actionableLayout {
	m.normalizeSelection()
	l := actionableLayout{selectedStart: -1, selectedEnd: -1}
	if m.width <= 0 || m.height <= 0 {
		return l
	}
	t := m.theme
	fit := func(text string, width int) string {
		return ansi.Truncate(popupChromeLine(text), max(0, width), "…")
	}
	bar := func(style lipgloss.Style, text string) string {
		pad := min(2, max(0, (m.width-1)/2))
		return style.Padding(0, pad).Width(m.width).Render(fit(text, m.width-2*pad))
	}
	total := 0
	for _, track := range m.plan.Tracks {
		total += len(track.Items)
	}
	title := bar(t.Text.Title, fmt.Sprintf("%s ACTIONABLE ITEMS  │  %d items in %d tracks", activeGlyphs.Bolt, total, len(m.plan.Tracks)))
	recommendation := ""
	if m.plan.Summary.HighestImpact != "" && m.plan.Summary.UnblocksCount > 0 {
		recommendation = bar(t.Text.Callout, fmt.Sprintf("%s RECOMMENDED: Start with %s → %s (unblocks %d)", activeGlyphs.Bulb, m.plan.Summary.HighestImpact, m.plan.Summary.ImpactReason, m.plan.Summary.UnblocksCount))
	}
	// Keep a useful body row even for empty-state text. Drop spacers first,
	// recommendation second, title last; chrome never participates in scrolling.
	budget := max(0, m.height-1)
	if budget > 0 {
		l.chrome = append(l.chrome, title)
	}
	if recommendation != "" && budget > 1 {
		if budget > 2 {
			l.chrome = append(l.chrome, "")
		}
		l.chrome = append(l.chrome, recommendation)
		if budget > 3 {
			l.chrome = append(l.chrome, "")
		}
	} else if budget > 1 {
		l.chrome = append(l.chrome, "")
	}
	l.bodyHeight = m.height - len(l.chrome)
	if total == 0 {
		l.body = []string{t.Text.Metadata.Render(fit(activeGlyphs.Success+" No actionable items. All tasks are either blocked or completed.", m.width))}
		return l
	}
	for trackIdx, track := range m.plan.Tracks {
		badge := fit(" TRACK "+strings.TrimPrefix(track.TrackID, "track-")+" ", m.width)
		reasonWidth := max(0, m.width-ansi.StringWidth(badge)-1)
		header := t.Text.Badge.Render(badge)
		if reasonWidth > 0 {
			header += " " + t.Text.Metadata.Render(fit(track.Reason, reasonWidth))
		}
		l.body = append(l.body, header, lipgloss.NewStyle().Foreground(t.Border).Render(strings.Repeat("·", max(0, m.width-4))))
		for itemIdx, item := range track.Items {
			selected := trackIdx == m.selectedTrack && itemIdx == m.selectedItem
			connector := "└─ "
			if itemIdx < len(track.Items)-1 {
				connector = "├─ "
			}
			// Priority remains a domain-semantic icon on ordinary rows; the
			// selected row takes the shared list highlight instead.
			prefix := "  " + connector + GetPriorityIcon(item.Priority) + " " + popupChromeLine(item.ID) + " "
			prefix = ansi.Truncate(prefix, m.width, "…")
			suffix := ""
			if len(item.UnblocksIDs) > 0 {
				suffix = fmt.Sprintf(" →%d", len(item.UnblocksIDs))
			}
			suffix = fit(suffix, max(0, m.width-ansi.StringWidth(prefix)))
			text := fit(item.Title, max(0, m.width-ansi.StringWidth(prefix)-ansi.StringWidth(suffix)))
			if selected {
				l.selectedStart = len(l.body)
				l.body = append(l.body, renderSelectedRow(t, prefix+text+suffix, m.width))
				if len(item.UnblocksIDs) > 0 {
					l.body = append(l.body, t.Text.Metadata.Render(fit("        ↳ Unblocks: "+strings.Join(item.UnblocksIDs, ", "), m.width)))
				}
				l.selectedEnd = len(l.body)
			} else {
				l.body = append(l.body, t.Text.Metadata.Render(prefix)+t.Text.Body.Render(text)+t.Text.Metadata.Render(suffix))
			}
		}
		l.body = append(l.body, "")
	}
	return l
}

// Render scrolls only measured body lines, retaining fixed chrome.
func (m *ActionableModel) Render() string {
	l := m.layout()
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	m.keepVisible(l)
	end := min(len(l.body), m.scrollOffset+l.bodyHeight)
	return strings.Join(append(l.chrome, l.body[m.scrollOffset:end]...), "\n")
}
