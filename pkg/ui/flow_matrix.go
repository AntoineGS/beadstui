package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// FlowMatrixModel renders an interactive dependency flow dashboard
// showing how labels block each other with visual impact indicators
type FlowMatrixModel struct {
	keys         keys.FlowMatrixKeys
	flow         *analysis.CrossLabelFlow
	issues       []model.Issue
	labelStats   []labelFlowStats
	cursor       int
	scrollOffset int
	width        int
	height       int
	theme        Theme
	focusPanel   int // 0 = labels list, 1 = detail panel
	ready        bool

	// Drill-down state
	showDrilldown   bool
	drilldownIssues []flowPairEndpoint
	drilldownCursor int
	drilldownScroll int
	drilldownTitle  string
	drilldownLabel  string
	inspecting      bool
	inspectionVP    viewport.Model
	originDetailVP  viewport.Model
	hasOriginDetail bool
	initialized     bool
}

type flowPairKey struct{ blocker, blocked string }

type flowPairEndpoint struct {
	key     flowPairKey
	issue   model.Issue
	blocker bool
}

// labelFlowStats holds computed stats for a single label
type labelFlowStats struct {
	Label           string
	OutgoingCount   int      // How many issues in other labels this blocks
	IncomingCount   int      // How many issues from other labels block this
	OutgoingLabels  []string // Labels this blocks
	IncomingLabels  []string // Labels that block this
	BottleneckScore float64  // Normalized blocking power
	IsBottleneck    bool     // In top bottlenecks
	OnCriticalPath  bool
}

// NewFlowMatrixModel creates a new flow matrix dashboard
func NewFlowMatrixModel(theme Theme) FlowMatrixModel {
	return FlowMatrixModel{
		keys:         keys.NewFlowMatrixKeys(),
		theme:        theme,
		focusPanel:   0,
		inspectionVP: viewport.New(),
		initialized:  true,
	}
}

// SetData initializes the model with flow data
func (m *FlowMatrixModel) SetData(flow *analysis.CrossLabelFlow, issues []model.Issue) {
	selectedLabel := m.SelectedLabel()
	wasDrilling := m.showDrilldown
	wasInspecting := m.inspecting
	oldEndpoint := ""
	oldPair := flowPairKey{}
	oldCursor := m.drilldownCursor
	if selected := m.SelectedDrilldownIssue(); selected != nil {
		oldEndpoint = selected.ID
		oldPair = m.drilldownIssues[m.drilldownCursor].key
	}
	oldLabel := m.drilldownLabel
	m.flow = flow
	m.issues = issues
	m.ready = flow != nil && len(flow.Labels) > 0
	m.computeStats()
	if selectedLabel != "" {
		for i := range m.labelStats {
			if m.labelStats[i].Label == selectedLabel {
				m.cursor = i
				break
			}
		}
	}
	if wasDrilling {
		labelExists := false
		for _, stat := range m.labelStats {
			if stat.Label == oldLabel {
				labelExists = true
				break
			}
		}
		if !labelExists {
			m.showDrilldown, m.inspecting = false, false
			return
		}
		m.drilldownLabel = oldLabel
		m.rebuildDrilldown(oldPair, oldEndpoint, oldCursor)
		m.inspecting = wasInspecting && len(m.drilldownIssues) > 0
		if selected := m.SelectedDrilldownIssue(); selected == nil || selected.ID != oldEndpoint || m.drilldownIssues[m.drilldownCursor].key != oldPair {
			m.EndInspection()
		}
	}
}

func (m *FlowMatrixModel) BeginInspection(vp viewport.Model) {
	vp.SetWidth(max(0, m.width))
	vp.SetHeight(max(0, m.height))
	vp.GotoTop()
	m.inspectionVP = vp
	m.inspecting = true
}

func (m *FlowMatrixModel) UpdateInspectionViewport(vp viewport.Model) {
	offset := m.inspectionVP.YOffset()
	if m.inspecting {
		vp.SetYOffset(offset)
	} else {
		vp.GotoTop()
	}
	vp.SetWidth(max(0, m.width))
	vp.SetHeight(max(0, m.height))
	m.inspectionVP = vp
}

func (m *FlowMatrixModel) EndInspection() { m.inspecting = false }

func (m *FlowMatrixModel) Inspecting() bool { return m.inspecting }

func (m *FlowMatrixModel) SaveOriginDetail(vp viewport.Model) {
	m.originDetailVP = vp
	m.hasOriginDetail = true
}

func (m *FlowMatrixModel) OriginDetail() (viewport.Model, bool) {
	return m.originDetailVP, m.hasOriginDetail
}

func (m *FlowMatrixModel) ClearOriginDetail() { m.hasOriginDetail = false }

func (m *FlowMatrixModel) UpdateInspection(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.inspectionVP, cmd = m.inspectionVP.Update(msg)
	return cmd
}

func (m *FlowMatrixModel) ScrollInspection(delta int) {
	if delta < 0 {
		m.inspectionVP.ScrollUp(-delta)
		return
	}
	m.inspectionVP.ScrollDown(delta)
}

// SetSize sets the available rendering dimensions
func (m *FlowMatrixModel) SetSize(width, height int) {
	m.width = max(0, width)
	m.height = max(0, height)
	if m.inspecting {
		m.inspectionVP.SetWidth(m.width)
		m.inspectionVP.SetHeight(m.height)
	}
}

// computeStats builds per-label statistics from the flow matrix
func (m *FlowMatrixModel) computeStats() {
	if m.flow == nil || len(m.flow.Labels) == 0 {
		m.labelStats = nil
		return
	}

	labels := m.flow.Labels
	n := len(labels)
	matrix := m.flow.FlowMatrix

	// Validate matrix dimensions to prevent panics
	if len(matrix) < n {
		m.labelStats = nil
		return
	}
	for i := 0; i < n; i++ {
		if len(matrix[i]) < n {
			m.labelStats = nil
			return
		}
	}

	// Build index for quick lookup
	labelIndex := make(map[string]int, n)
	for i, l := range labels {
		labelIndex[l] = i
	}

	// Compute stats for each label
	stats := make([]labelFlowStats, n)
	maxOutgoing := 0

	for i, label := range labels {
		stats[i].Label = label

		// Outgoing: row i sums
		for j := 0; j < n; j++ {
			if i != j && matrix[i][j] > 0 {
				stats[i].OutgoingCount += matrix[i][j]
				stats[i].OutgoingLabels = append(stats[i].OutgoingLabels, labels[j])
			}
		}

		// Incoming: column i sums
		for j := 0; j < n; j++ {
			if i != j && matrix[j][i] > 0 {
				stats[i].IncomingCount += matrix[j][i]
				stats[i].IncomingLabels = append(stats[i].IncomingLabels, labels[j])
			}
		}

		if stats[i].OutgoingCount > maxOutgoing {
			maxOutgoing = stats[i].OutgoingCount
		}
	}

	// Compute bottleneck scores (normalized)
	for i := range stats {
		if maxOutgoing > 0 {
			stats[i].BottleneckScore = float64(stats[i].OutgoingCount) / float64(maxOutgoing)
		}
	}

	// Mark bottlenecks (top N or those in BottleneckLabels)
	bottleneckSet := make(map[string]bool)
	for _, bl := range m.flow.BottleneckLabels {
		bottleneckSet[bl] = true
	}
	for i := range stats {
		stats[i].IsBottleneck = bottleneckSet[stats[i].Label]
	}

	// Sort by outgoing count (highest blocking power first)
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].OutgoingCount != stats[j].OutgoingCount {
			return stats[i].OutgoingCount > stats[j].OutgoingCount
		}
		return stats[i].Label < stats[j].Label
	})

	m.labelStats = stats
}

// Update handles keyboard input
func (m *FlowMatrixModel) Update(msg tea.KeyMsg) tea.Cmd {
	if m.inspecting {
		return m.UpdateInspection(msg)
	}
	if m.showDrilldown {
		return m.updateDrilldown(msg)
	}

	k := m.keys
	switch {
	case key.Matches(msg, k.Down):
		m.moveCursor(1)
	case key.Matches(msg, k.Up):
		m.moveCursor(-1)
	case key.Matches(msg, k.JumpTop):
		m.cursor = 0
		m.scrollOffset = 0
	case key.Matches(msg, k.JumpBottom):
		if len(m.labelStats) > 0 {
			m.cursor = len(m.labelStats) - 1
			m.ensureVisible()
		}
	case key.Matches(msg, k.TogglePanel):
		m.focusPanel = (m.focusPanel + 1) % 2
	case key.Matches(msg, k.Enter):
		if m.cursor < len(m.labelStats) {
			m.openDrilldown()
		}
	case key.Matches(msg, k.PageDown):
		m.PageDown()
	case key.Matches(msg, k.PageUp):
		m.PageUp()
	}

	return nil
}

func (m *FlowMatrixModel) updateDrilldown(msg tea.KeyMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.Close):
		if m.inspecting {
			m.EndInspection()
		} else {
			m.showDrilldown = false
		}
	case key.Matches(msg, k.Down):
		if !m.inspecting && m.drilldownCursor < len(m.drilldownIssues)-1 {
			m.drilldownCursor++
			m.ensureDrilldownVisible()
		}
	case key.Matches(msg, k.Up):
		if !m.inspecting && m.drilldownCursor > 0 {
			m.drilldownCursor--
			m.ensureDrilldownVisible()
		}
	case key.Matches(msg, k.PageDown):
		m.PageDown()
	case key.Matches(msg, k.PageUp):
		m.PageUp()
	}
	return nil
}

func (m *FlowMatrixModel) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.labelStats) {
		m.cursor = len(m.labelStats) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

func (m *FlowMatrixModel) ensureVisible() {
	visible := m.visibleRows()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	if m.cursor >= m.scrollOffset+visible {
		m.scrollOffset = m.cursor - visible + 1
	}
}

func (m *FlowMatrixModel) ensureDrilldownVisible() {
	visible := max(0, m.height-2)
	if m.drilldownCursor < m.drilldownScroll {
		m.drilldownScroll = m.drilldownCursor
	}
	if visible > 0 && m.drilldownCursor >= m.drilldownScroll+visible {
		m.drilldownScroll = m.drilldownCursor - visible + 1
	}
}

func (m *FlowMatrixModel) visibleRows() int {
	rows := m.height - 6 // header, footer, borders
	if rows < 3 {
		rows = 3
	}
	return rows
}

func (m *FlowMatrixModel) openDrilldown() {
	if m.cursor >= len(m.labelStats) {
		return
	}
	selectedLabel := m.labelStats[m.cursor].Label
	m.drilldownLabel = selectedLabel
	m.rebuildDrilldown(flowPairKey{}, "", 0)
	m.drilldownScroll = 0
	m.drilldownTitle = fmt.Sprintf("Issues with label: %s", selectedLabel)
	m.showDrilldown = true
	m.inspecting = false
}

func (m *FlowMatrixModel) rebuildDrilldown(preferredPair flowPairKey, preferredID string, fallbackCursor int) {
	lookup := make(map[string]model.Issue, len(m.issues))
	for _, issue := range m.issues {
		lookup[issue.ID] = issue
	}
	seen := make(map[flowPairKey]bool)
	pairs := make([]flowPairKey, 0)
	if m.flow != nil {
		for _, dep := range m.flow.Dependencies {
			if dep.FromLabel != m.drilldownLabel && dep.ToLabel != m.drilldownLabel {
				continue
			}
			for _, pair := range dep.BlockingPairs {
				key := flowPairKey{blocker: pair.BlockerID, blocked: pair.BlockedID}
				if key.blocker == "" || key.blocked == "" || seen[key] {
					continue
				}
				seen[key] = true
				pairs = append(pairs, key)
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].blocker != pairs[j].blocker {
			return pairs[i].blocker < pairs[j].blocker
		}
		return pairs[i].blocked < pairs[j].blocked
	})
	endpoints := make([]flowPairEndpoint, 0, len(pairs)*2)
	for _, pair := range pairs {
		if issue, ok := lookup[pair.blocker]; ok {
			endpoints = append(endpoints, flowPairEndpoint{key: pair, issue: issue, blocker: true})
		}
		if issue, ok := lookup[pair.blocked]; ok {
			endpoints = append(endpoints, flowPairEndpoint{key: pair, issue: issue})
		}
	}
	m.drilldownIssues = endpoints
	m.drilldownCursor = min(fallbackCursor, max(0, len(endpoints)-1))
	if preferredID != "" {
		for i := range endpoints {
			if endpoints[i].key == preferredPair && endpoints[i].issue.ID == preferredID {
				m.drilldownCursor = i
				break
			}
		}
	}
	m.ensureDrilldownVisible()
}

// SelectedLabel returns the currently selected label (for drill-down from parent)
func (m *FlowMatrixModel) SelectedLabel() string {
	if m.cursor < len(m.labelStats) {
		return m.labelStats[m.cursor].Label
	}
	return ""
}

// View renders the flow matrix dashboard
func (m FlowMatrixModel) View() string {
	if !m.ready {
		return m.theme.Text.Body.Render("No cross-label dependencies found")
	}

	if m.showDrilldown {
		return m.renderDrilldown()
	}

	// Calculate panel widths with safety bounds
	leftWidth := m.width * 35 / 100 // 35% for labels list
	minLeftWidth := 25
	minRightWidth := 30
	sepWidth := 3 // border/separator space

	// Ensure we don't exceed total width
	if leftWidth < minLeftWidth {
		leftWidth = minLeftWidth
	}
	rightWidth := m.width - leftWidth - sepWidth
	if rightWidth < minRightWidth {
		rightWidth = minRightWidth
	}

	// If total exceeds width, scale down proportionally
	totalNeeded := leftWidth + rightWidth + sepWidth
	if totalNeeded > m.width && m.width > 0 {
		scale := float64(m.width-sepWidth) / float64(leftWidth+rightWidth)
		leftWidth = int(float64(leftWidth) * scale)
		rightWidth = m.width - leftWidth - sepWidth
		if leftWidth < 10 {
			leftWidth = 10
		}
		if rightWidth < 10 {
			rightWidth = m.width - leftWidth - sepWidth
			if rightWidth < 10 {
				rightWidth = 10
			}
		}
	}

	// Build panels
	leftPanel := m.renderLabelsPanel(leftWidth)
	rightPanel := m.renderDetailPanel(rightWidth)

	// Header
	header := m.renderHeader()

	// Join panels side by side
	leftLines := strings.Split(leftPanel, "\n")
	rightLines := strings.Split(rightPanel, "\n")

	// Normalize heights
	maxLines := len(leftLines)
	if len(rightLines) > maxLines {
		maxLines = len(rightLines)
	}
	for len(leftLines) < maxLines {
		leftLines = append(leftLines, strings.Repeat(" ", leftWidth))
	}
	for len(rightLines) < maxLines {
		rightLines = append(rightLines, strings.Repeat(" ", rightWidth))
	}

	var body strings.Builder
	separator := lipgloss.NewStyle().
		Foreground(m.theme.Border).
		Render("│")

	for i := 0; i < maxLines; i++ {
		body.WriteString(leftLines[i])
		body.WriteString(" ")
		body.WriteString(separator)
		body.WriteString(" ")
		body.WriteString(rightLines[i])
		if i < maxLines-1 {
			body.WriteString("\n")
		}
	}

	// Footer
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, body.String(), footer)
}

func (m FlowMatrixModel) renderHeader() string {
	titleStyle := m.theme.Text.Title.PaddingRight(2)

	statsStyle := m.theme.Text.Metadata

	title := titleStyle.Render("DEPENDENCY FLOW")
	stats := statsStyle.Render(fmt.Sprintf("│ %d labels │ %d cross-label deps │ %d bottlenecks",
		len(m.flow.Labels),
		m.flow.TotalCrossLabelDeps,
		len(m.flow.BottleneckLabels)))

	headerLine := lipgloss.JoinHorizontal(lipgloss.Left, title, stats)

	borderStyle := lipgloss.NewStyle().
		Foreground(m.theme.Border)

	return lipgloss.JoinVertical(lipgloss.Left,
		headerLine,
		borderStyle.Render(strings.Repeat("─", m.width)))
}

func (m FlowMatrixModel) renderLabelsPanel(width int) string {
	var b strings.Builder

	// Panel header
	headerStyle := m.theme.Text.Heading.
		Width(width)

	focusIndicator := " "
	if m.focusPanel == 0 {
		focusIndicator = "▸"
	}
	b.WriteString(headerStyle.Render(focusIndicator + " LABELS (by blocking power)"))
	b.WriteString("\n")

	// Separator
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
	b.WriteString(sepStyle.Render(strings.Repeat("─", width)))
	b.WriteString("\n")

	// Find max for bar scaling
	maxOut := 1
	for _, s := range m.labelStats {
		if s.OutgoingCount > maxOut {
			maxOut = s.OutgoingCount
		}
	}

	// Visible rows
	visible := m.visibleRows()
	start := m.scrollOffset
	end := start + visible
	if end > len(m.labelStats) {
		end = len(m.labelStats)
	}

	// Bar width
	barWidth := width - 20 // space for label name and count
	if barWidth < 5 {
		barWidth = 5
	}

	for i := start; i < end; i++ {
		stat := m.labelStats[i]
		isSelected := i == m.cursor

		// Build the row
		row := m.renderLabelRow(stat, isSelected, barWidth, maxOut, width)
		b.WriteString(row)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining space
	for i := end - start; i < visible; i++ {
		b.WriteString("\n")
		b.WriteString(strings.Repeat(" ", width))
	}

	return b.String()
}

func (m FlowMatrixModel) renderLabelRow(stat labelFlowStats, selected bool, barWidth, maxOut, totalWidth int) string {
	// Label name (truncated if needed, using rune count for UTF-8 safety)
	labelWidth := 12
	label := stat.Label
	labelRunes := []rune(label)
	if len(labelRunes) > labelWidth {
		label = string(labelRunes[:labelWidth-1]) + "…"
	}

	// Color based on bottleneck status
	var labelColor color.Color
	if stat.IsBottleneck {
		labelColor = m.theme.Blocked // Red for bottlenecks
	} else if stat.BottleneckScore > 0.5 {
		labelColor = m.theme.Feature // Orange for high impact
	} else if stat.OutgoingCount > 0 {
		labelColor = m.theme.Task // Yellow for some impact
	} else {
		labelColor = m.theme.Subtext // Gray for no impact
	}

	rowStyle := m.theme.Text.Body
	labelStyle := rowStyle.Foreground(labelColor)
	label = ansi.Truncate(label, labelWidth, "…")
	label += strings.Repeat(" ", max(0, labelWidth-ansi.StringWidth(label)))

	// Bar visualization
	barFilled := 0
	if maxOut > 0 {
		barFilled = stat.OutgoingCount * barWidth / maxOut
	}
	if barFilled > barWidth {
		barFilled = barWidth
	}

	// Use different characters for bar intensity
	bar := ""
	if barFilled > 0 {
		bar = strings.Repeat("█", barFilled)
	}
	barEmpty := strings.Repeat("░", barWidth-barFilled)

	barStyle := lipgloss.NewStyle().Foreground(labelColor)
	emptyStyle := lipgloss.NewStyle().Foreground(m.theme.Border)

	// Count
	countStr := fmt.Sprintf("%3d", stat.OutgoingCount)

	// Assemble row
	row := labelStyle.Render(label) + rowStyle.Render(" ") + barStyle.Render(bar) + emptyStyle.Render(barEmpty) + rowStyle.Render(" "+countStr)

	if selected {
		return renderSelectedRow(m.theme, row, totalWidth)
	}
	return row
}

func (m FlowMatrixModel) renderDetailPanel(width int) string {
	var b strings.Builder

	if m.cursor >= len(m.labelStats) {
		return m.theme.Text.Metadata.Render("Select a label")
	}

	stat := m.labelStats[m.cursor]

	// Panel header
	headerStyle := m.theme.Text.Heading

	b.WriteString(headerStyle.Render(fmt.Sprintf("▸ %s", stat.Label)))
	b.WriteString("\n")

	// Separator
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
	b.WriteString(sepStyle.Render(strings.Repeat("─", width)))
	b.WriteString("\n\n")

	// Stats summary
	summaryStyle := m.theme.Text.Heading
	b.WriteString(summaryStyle.Render("IMPACT SUMMARY"))
	b.WriteString("\n")

	// Blocking power indicator
	scoreBar := m.renderScoreBar(stat.BottleneckScore, 20)
	scoreLabel := "Low"
	scoreColor := m.theme.Open
	if stat.BottleneckScore > 0.7 {
		scoreLabel = "HIGH"
		scoreColor = m.theme.Blocked
	} else if stat.BottleneckScore > 0.3 {
		scoreLabel = "Medium"
		scoreColor = m.theme.Feature
	}
	scoreStyle := m.theme.Text.Body.Foreground(scoreColor)
	b.WriteString(m.theme.Text.Metadata.Render("  Blocking Power: ") + scoreBar + " " + scoreStyle.Render(scoreLabel) + "\n")

	if stat.IsBottleneck {
		bottleneckStyle := m.theme.Text.Body.Foreground(m.theme.Blocked)
		b.WriteString(bottleneckStyle.Render("  " + activeGlyphs.Warning + " BOTTLENECK"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Two-column layout for blocks/blocked by
	halfWidth := max(0, (width-4)/2)
	// Text roles add ANSI sequences, and labels may contain wide graphemes.
	// Clip each column and pad by display cells, never by Go string length.
	columns := func(left, right string) string {
		left = ansi.Truncate(left, halfWidth, "…")
		right = ansi.Truncate(right, max(0, width-halfWidth-2), "…")
		return left + strings.Repeat(" ", max(0, halfWidth-ansi.StringWidth(left))) + "  " + right + "\n"
	}

	// BLOCKS section
	blocksHeader := m.theme.Text.Heading.
		Foreground(m.theme.Blocked).
		Render(fmt.Sprintf("BLOCKS → (%d)", stat.OutgoingCount))

	blockedByHeader := m.theme.Text.Heading.
		Foreground(m.theme.InProgress).
		Render(fmt.Sprintf("← BLOCKED BY (%d)", stat.IncomingCount))

	b.WriteString(columns(blocksHeader, blockedByHeader))

	// List entries
	maxEntries := 6
	outLabels := stat.OutgoingLabels
	inLabels := stat.IncomingLabels

	// Get counts for each label
	outCounts := m.getFlowCounts(stat.Label, outLabels, true)
	inCounts := m.getFlowCounts(stat.Label, inLabels, false)

	for i := 0; i < maxEntries; i++ {
		leftStr := ""
		rightStr := ""

		if i < len(outLabels) {
			count := outCounts[outLabels[i]]
			miniBar := m.miniBar(count, 5)
			leftStr = "  " + miniBar + " " + m.theme.Text.Body.Render(fmt.Sprintf("%s (%d)", outLabels[i], count))
		}
		if i < len(inLabels) {
			count := inCounts[inLabels[i]]
			miniBar := m.miniBar(count, 5)
			rightStr = "  " + miniBar + " " + m.theme.Text.Body.Render(fmt.Sprintf("%s (%d)", inLabels[i], count))
		}

		b.WriteString(columns(leftStr, rightStr))
	}

	if len(outLabels) > maxEntries || len(inLabels) > maxEntries {
		moreStyle := m.theme.Text.Metadata
		leftMore := ""
		rightMore := ""
		if len(outLabels) > maxEntries {
			leftMore = fmt.Sprintf("  +%d more", len(outLabels)-maxEntries)
		}
		if len(inLabels) > maxEntries {
			rightMore = fmt.Sprintf("  +%d more", len(inLabels)-maxEntries)
		}
		b.WriteString(columns(moreStyle.Render(leftMore), moreStyle.Render(rightMore)))
	}

	return strings.TrimSuffix(b.String(), "\n")
}

func (m FlowMatrixModel) getFlowCounts(sourceLabel string, targetLabels []string, outgoing bool) map[string]int {
	counts := make(map[string]int)
	if m.flow == nil {
		return counts
	}

	// Build label index
	labelIndex := make(map[string]int)
	for i, l := range m.flow.Labels {
		labelIndex[l] = i
	}

	sourceIdx, ok := labelIndex[sourceLabel]
	if !ok {
		return counts
	}

	for _, target := range targetLabels {
		targetIdx, ok := labelIndex[target]
		if !ok {
			continue
		}
		if outgoing {
			counts[target] = m.flow.FlowMatrix[sourceIdx][targetIdx]
		} else {
			counts[target] = m.flow.FlowMatrix[targetIdx][sourceIdx]
		}
	}
	return counts
}

func (m FlowMatrixModel) renderScoreBar(score float64, width int) string {
	filled := int(score * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	var barColor color.Color
	if score > 0.7 {
		barColor = m.theme.Blocked
	} else if score > 0.3 {
		barColor = m.theme.Feature
	} else {
		barColor = m.theme.Open
	}

	barStyle := lipgloss.NewStyle().Foreground(barColor)
	emptyStyle := lipgloss.NewStyle().Foreground(m.theme.Border)

	return barStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", width-filled))
}

func (m FlowMatrixModel) miniBar(count, maxWidth int) string {
	if count <= 0 {
		return strings.Repeat("·", maxWidth)
	}

	// Scale linearly for visualization
	filled := count
	if filled > maxWidth {
		filled = maxWidth
	}

	var miniColor color.Color
	if count >= 5 {
		miniColor = m.theme.Blocked
	} else if count >= 2 {
		miniColor = m.theme.Feature
	} else {
		miniColor = m.theme.Task
	}

	miniBarStyle := lipgloss.NewStyle().Foreground(miniColor)
	return miniBarStyle.Render(strings.Repeat("■", filled)) + strings.Repeat("·", maxWidth-filled)
}

func (m FlowMatrixModel) renderFooter() string {
	borderStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
	helpStyle := m.theme.Text.Metadata

	help := "Tab: switch panel"

	return lipgloss.JoinVertical(lipgloss.Left,
		borderStyle.Render(strings.Repeat("─", m.width)),
		helpStyle.Render(help))
}

func (m FlowMatrixModel) renderDrilldown() string {
	if m.inspecting {
		return m.inspectionVP.View()
	}
	width, height := max(0, m.width), max(0, m.height)
	if width == 0 || height == 0 {
		return ""
	}
	lines := make([]string, 0, height)
	pairSet := make(map[flowPairKey]struct{}, len(m.drilldownIssues))
	for _, endpoint := range m.drilldownIssues {
		pairSet[endpoint.key] = struct{}{}
	}
	header := m.theme.Text.Heading.Render(fmt.Sprintf("Blocking relationships for %s (%d)", m.drilldownLabel, len(pairSet)))
	lines = append(lines, ansi.Truncate(header, width, "…"))
	if len(lines) < height {
		borderStyle := lipgloss.NewStyle().Foreground(m.theme.Border)
		lines = append(lines, ansi.Truncate(borderStyle.Render(strings.Repeat("─", width)), width, ""))
	}

	if len(m.drilldownIssues) == 0 {
		if len(lines) < height {
			empty := m.theme.Text.Metadata.Render("No blocking relationships")
			lines = append(lines, ansi.Truncate(empty, width, "…"))
		}
		return strings.Join(lines, "\n")
	}

	visible := max(0, height-len(lines))
	start := m.drilldownScroll
	if start > len(m.drilldownIssues) {
		start = len(m.drilldownIssues)
	}
	end := start + visible
	if end > len(m.drilldownIssues) {
		end = len(m.drilldownIssues)
	}

	for i := start; i < end; i++ {
		endpoint := m.drilldownIssues[i]
		iss := endpoint.issue
		selected := i == m.drilldownCursor
		marker := "  "
		if selected {
			marker = "▸ "
		}
		role := "BLOCKED"
		if endpoint.blocker {
			role = "BLOCKER"
		}
		text := fmt.Sprintf("%sBLOCKER → BLOCKED | %s %s %s [%s] %s", marker,
			GetStatusIcon(string(iss.Status)), iss.ID, role, iss.Status, iss.Title)
		row := ansi.Truncate(text, width, "…")

		if selected {
			row = renderSelectedRow(m.theme, row, width)
		} else {
			row = m.theme.Text.Body.Render(row)
		}

		lines = append(lines, ansi.Truncate(row, width, "…"))
	}

	return strings.Join(lines, "\n")
}

// MoveUp moves the cursor up by one
func (m *FlowMatrixModel) MoveUp() {
	if m.showDrilldown {
		if !m.inspecting && m.drilldownCursor > 0 {
			m.drilldownCursor--
			m.ensureDrilldownVisible()
		}
	} else {
		m.moveCursor(-1)
	}
}

// MoveDown moves the cursor down by one
func (m *FlowMatrixModel) MoveDown() {
	if m.showDrilldown {
		if !m.inspecting && m.drilldownCursor < len(m.drilldownIssues)-1 {
			m.drilldownCursor++
			m.ensureDrilldownVisible()
		}
	} else {
		m.moveCursor(1)
	}
}

// PageDown moves the active flow-matrix cursor by half a viewport.
func (m *FlowMatrixModel) PageDown() {
	pageSize := max(1, m.visibleRows()/2)
	if m.showDrilldown {
		if len(m.drilldownIssues) == 0 {
			return
		}
		m.drilldownCursor = min(m.drilldownCursor+pageSize, len(m.drilldownIssues)-1)
		m.ensureDrilldownVisible()
		return
	}
	m.moveCursor(pageSize)
}

// PageUp moves the active flow-matrix cursor by half a viewport.
func (m *FlowMatrixModel) PageUp() {
	pageSize := max(1, m.visibleRows()/2)
	if m.showDrilldown {
		if len(m.drilldownIssues) == 0 {
			return
		}
		m.drilldownCursor = max(0, m.drilldownCursor-pageSize)
		m.ensureDrilldownVisible()
		return
	}
	m.moveCursor(-pageSize)
}

// TogglePanel switches focus between the labels list and detail panel
func (m *FlowMatrixModel) TogglePanel() {
	m.focusPanel = (m.focusPanel + 1) % 2
}

// OpenDrilldown opens the drill-down view for the selected label
func (m *FlowMatrixModel) OpenDrilldown() {
	m.openDrilldown()
}

// GoToStart moves cursor to the first item
func (m *FlowMatrixModel) GoToStart() {
	if m.showDrilldown {
		if m.inspecting {
			return
		}
		m.drilldownCursor = 0
		m.drilldownScroll = 0
	} else {
		m.cursor = 0
		m.scrollOffset = 0
	}
}

// GoToEnd moves cursor to the last item
func (m *FlowMatrixModel) GoToEnd() {
	if m.showDrilldown {
		if m.inspecting {
			return
		}
		if len(m.drilldownIssues) > 0 {
			m.drilldownCursor = len(m.drilldownIssues) - 1
			m.ensureDrilldownVisible()
		}
	} else {
		if len(m.labelStats) > 0 {
			m.cursor = len(m.labelStats) - 1
			m.ensureVisible()
		}
	}
}

// SelectedDrilldownIssue returns the selected relationship endpoint.
func (m *FlowMatrixModel) SelectedDrilldownIssue() *model.Issue {
	if !m.showDrilldown || m.drilldownCursor >= len(m.drilldownIssues) {
		return nil
	}
	return &m.drilldownIssues[m.drilldownCursor].issue
}
