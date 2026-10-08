package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// LabelPickerModel provides a fuzzy search popup for quick label filtering
type LabelPickerModel struct {
	allLabels     []string
	labelCounts   map[string]int // count of issues per label
	filtered      []string
	input         textinput.Model
	selectedIndex int
	activeLabels  map[string]bool // currently applied label filters (shown with indicator)
	selected      map[string]bool // labels toggled in this session (space to toggle)
	width         int
	height        int
	popupSize     *PopupSize
	theme         Theme
	// searchFocused gates whether typed characters route to the text input or
	// are interpreted as navigation/no-op. The picker opens with searchFocused
	// false (bt-wnda); pressing "/" focuses the search bar, Esc inside search
	// blurs it without closing the modal.
	searchFocused bool
	// openedWithFilter records whether SetActiveLabels was called with a
	// non-empty slice when the modal opened. Used by the Enter handler to
	// distinguish "user came in with filters and explicitly deselected
	// everything to clear them" from "user opened cold and pressed Enter
	// on a label to filter by it" -- both produce SelectedLabels()==nil
	// but mean opposite things.
	openedWithFilter bool
}

// NewLabelPickerModel creates a new label picker with fuzzy search
// labels should be pre-sorted by count descending (from LabelExtractionResult.TopLabels)
func NewLabelPickerModel(labels []string, counts map[string]int, theme Theme) LabelPickerModel {
	// Sort labels by count descending
	sorted := sortLabelsByCountDesc(labels, counts)

	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 50
	ti.SetWidth(30)
	// Search starts blurred (bt-wnda): the user lands on the labels list.
	ti.Blur()

	return LabelPickerModel{
		allLabels:     sorted,
		labelCounts:   counts,
		filtered:      sorted,
		input:         ti,
		selectedIndex: 0,
		theme:         theme,
		searchFocused: false,
	}
}

// FocusSearch routes typed characters to the search input.
func (m *LabelPickerModel) FocusSearch() {
	m.searchFocused = true
	m.input.Focus()
}

// BlurSearch returns focus to the labels list. The search query buffer is
// preserved so the user can resume editing without retyping.
func (m *LabelPickerModel) BlurSearch() {
	m.searchFocused = false
	m.input.Blur()
}

// IsSearchFocused reports whether the text input owns keyboard focus.
func (m *LabelPickerModel) IsSearchFocused() bool {
	return m.searchFocused
}

// sortLabelsByCountDesc sorts labels by count descending, then alphabetically for ties
func sortLabelsByCountDesc(labels []string, counts map[string]int) []string {
	sorted := make([]string, len(labels))
	copy(sorted, labels)
	sort.Slice(sorted, func(i, j int) bool {
		ci := counts[sorted[i]]
		cj := counts[sorted[j]]
		if ci != cj {
			return ci > cj // descending by count
		}
		return sorted[i] < sorted[j] // alphabetically for ties
	})
	return sorted
}

// SetSize updates the picker dimensions
func (m *LabelPickerModel) SetSize(width, height int) {
	m.popupSize = &PopupSize{width, height}
	m.width = width
	m.height = height
}

// SetLabels updates the available labels with their counts
func (m *LabelPickerModel) SetLabels(labels []string, counts map[string]int) {
	m.labelCounts = counts
	m.allLabels = sortLabelsByCountDesc(labels, counts)
	m.filterLabels()
}

// SetActiveLabels sets the currently applied label filters so they can be indicated.
func (m *LabelPickerModel) SetActiveLabels(labels []string) {
	m.activeLabels = make(map[string]bool, len(labels))
	for _, l := range labels {
		m.activeLabels[l] = true
	}
	// Pre-select active labels so enter preserves the current filter
	m.selected = make(map[string]bool, len(labels))
	for _, l := range labels {
		m.selected[l] = true
	}
	m.openedWithFilter = len(labels) > 0
}

// OpenedWithFilter reports whether the picker was opened with active labels
// already applied. Lets the Enter handler distinguish "deselected everything
// to clear the filter" from "no selection, apply cursor's label as a
// shortcut".
func (m *LabelPickerModel) OpenedWithFilter() bool {
	return m.openedWithFilter
}

// ToggleSelected toggles the label under the cursor.
func (m *LabelPickerModel) ToggleSelected() {
	if len(m.filtered) == 0 || m.selectedIndex >= len(m.filtered) {
		return
	}
	label := m.filtered[m.selectedIndex]
	if m.selected == nil {
		m.selected = make(map[string]bool)
	}
	if m.selected[label] {
		delete(m.selected, label)
	} else {
		m.selected[label] = true
	}
}

// SelectedLabels returns the labels that have been toggled on.
func (m *LabelPickerModel) SelectedLabels() []string {
	var labels []string
	// Return in display order (allLabels order) for deterministic output
	for _, l := range m.allLabels {
		if m.selected[l] {
			labels = append(labels, l)
		}
	}
	return labels
}

// HasSelections returns true if any labels are toggled.
func (m *LabelPickerModel) HasSelections() bool {
	return len(m.selected) > 0
}

// MoveUp moves selection up, wrapping to the bottom.
func (m *LabelPickerModel) MoveUp() {
	if len(m.filtered) == 0 {
		return
	}
	if m.selectedIndex > 0 {
		m.selectedIndex--
	} else {
		m.selectedIndex = len(m.filtered) - 1
	}
}

// MoveDown moves selection down, wrapping to the top.
func (m *LabelPickerModel) MoveDown() {
	if len(m.filtered) == 0 {
		return
	}
	if m.selectedIndex < len(m.filtered)-1 {
		m.selectedIndex++
	} else {
		m.selectedIndex = 0
	}
}

// PageDown moves selection to the bottom of the next page.
func (m *LabelPickerModel) PageDown() {
	if len(m.filtered) == 0 {
		return
	}
	pageSize := m.visibleCount()
	currentPageStart := (m.selectedIndex / pageSize) * pageSize
	target := currentPageStart + pageSize + pageSize - 1 // bottom of next page
	if target >= len(m.filtered) {
		target = len(m.filtered) - 1
	}
	m.selectedIndex = target
}

// PageUp moves selection to the top of the previous page.
func (m *LabelPickerModel) PageUp() {
	if len(m.filtered) == 0 {
		return
	}
	pageSize := m.visibleCount()
	currentPageStart := (m.selectedIndex / pageSize) * pageSize
	target := currentPageStart - pageSize // top of previous page
	if target < 0 {
		target = 0
	}
	m.selectedIndex = target
}

// labelPickerMaxVisible caps the number of label rows shown at once. With 440
// real-world labels (bt-wnda dogfood data) we want substantially more than the
// previous 10-row cap, but a hard ceiling keeps the modal from filling the
// entire screen on tall terminals.
const labelPickerMaxVisible = 30

// visibleCount consumes the shared body budget after search/page rows.
func (m *LabelPickerModel) visibleCount() int {
	return min(labelPickerMaxVisible, max(1, m.popupLayout().BodyHeight-4))
}

// SelectedLabel returns the currently selected label
func (m *LabelPickerModel) SelectedLabel() string {
	if len(m.filtered) == 0 || m.selectedIndex >= len(m.filtered) {
		return ""
	}
	return m.filtered[m.selectedIndex]
}

// SetCursor moves the cursor to the given index within the filtered list.
// Out-of-bounds indices are clamped. Used by the mouse click handler to
// select the row under the pointer (bt-wnda).
func (m *LabelPickerModel) SetCursor(idx int) {
	if len(m.filtered) == 0 {
		m.selectedIndex = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.filtered) {
		idx = len(m.filtered) - 1
	}
	m.selectedIndex = idx
}

// ItemAtPanelY maps a Y coordinate relative to the picker's top border to
// the filtered-list index currently rendered there. Returns (-1, false) for
// non-row regions (chrome, input, blanks, footer, etc.). The window math
// mirrors View()'s page-aligned slice (start, end).
func (m *LabelPickerModel) ItemAtPanelY(my int) (int, bool) {
	l := m.popupLayout()
	if l.Compact || l.Height == 0 {
		return -1, false
	}
	maxVisible := m.visibleCount()
	if maxVisible <= 0 {
		return -1, false
	}
	relRow := my - l.BodyY - 2
	if relRow < 0 || relRow >= maxVisible {
		return -1, false
	}
	if len(m.filtered) == 0 {
		return -1, false
	}
	start := (m.selectedIndex / maxVisible) * maxVisible
	idx := start + relRow
	if idx >= len(m.filtered) {
		return -1, false
	}
	return idx, true
}

// IsSearchRow reports whether the given panel-relative Y is the search
// input row. Used by mouse routing to focus the search input on click.
func (m *LabelPickerModel) IsSearchRow(my int) bool {
	l := m.popupLayout()
	return !l.Compact && l.Height > 0 && my == l.BodyY
}

// UpdateInput processes a key message for the text input
func (m *LabelPickerModel) UpdateInput(msg interface{}) {
	m.input, _ = m.input.Update(msg)
	m.filterLabels()
}

// Reset clears the input and resets selection.
// If active label filters are set, the cursor moves to the first one.
func (m *LabelPickerModel) Reset() {
	m.input.SetValue("")
	// Re-open in navigation mode (bt-wnda): every Reset must restore the
	// blurred search, otherwise reopening the modal after a previous search
	// session leaves focus stuck on the input.
	m.searchFocused = false
	m.input.Blur()
	m.filterLabels()
	// Position cursor on the first active label if any are set
	if len(m.activeLabels) > 0 {
		for i, label := range m.filtered {
			if m.activeLabels[label] {
				m.selectedIndex = i
				return
			}
		}
	}
}

// filterLabels filters the labels based on current input using fuzzy matching.
// Selected (toggled) labels are always pinned at the top of the list so they
// remain visible and accessible even when they don't match the search query.
func (m *LabelPickerModel) filterLabels() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))

	// Always allocate a fresh slice - never reuse m.allLabels' backing array
	var result []string

	if query == "" {
		// No search: pin selected at top, then the rest in count order
		if len(m.selected) > 0 {
			seen := make(map[string]bool, len(m.selected))
			for _, l := range m.allLabels {
				if m.selected[l] {
					result = append(result, l)
					seen[l] = true
				}
			}
			for _, l := range m.allLabels {
				if !seen[l] {
					result = append(result, l)
				}
			}
		} else {
			result = make([]string, len(m.allLabels))
			copy(result, m.allLabels)
		}
		m.filtered = result
		m.selectedIndex = 0
		return
	}

	type scored struct {
		label string
		score int
	}

	var matches []scored
	for _, label := range m.allLabels {
		if score := fuzzyScore(label, query); score > 0 {
			matches = append(matches, scored{label, score})
		}
	}

	// Sort by score (higher is better), then alphabetically
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].label < matches[j].label
	})

	// Pin selected labels at the top (in their original sort order), then matches
	seen := make(map[string]bool, len(matches)+len(m.selected))
	if len(m.selected) > 0 {
		for _, l := range m.allLabels {
			if m.selected[l] {
				result = append(result, l)
				seen[l] = true
			}
		}
	}
	for _, match := range matches {
		if !seen[match.label] {
			result = append(result, match.label)
		}
	}

	m.filtered = result

	// Keep selection in bounds
	if m.selectedIndex >= len(m.filtered) {
		m.selectedIndex = len(m.filtered) - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

// fuzzyScore returns a score for how well query matches label (0 = no match)
// Uses fzf-style scoring: consecutive matches, word boundary bonuses
func fuzzyScore(label, query string) int {
	label = strings.ToLower(label)
	query = strings.ToLower(query)

	// Exact match gets highest score
	if label == query {
		return 1000
	}

	// Prefix match gets high score
	if strings.HasPrefix(label, query) {
		return 500 + len(query)
	}

	// Contains match
	if strings.Contains(label, query) {
		return 200 + len(query)
	}

	// Fuzzy subsequence match
	li, qi := 0, 0
	score := 0
	consecutive := 0
	lastMatchIdx := -1

	for li < len(label) && qi < len(query) {
		if label[li] == query[qi] {
			qi++
			matchScore := 10

			// Bonus for consecutive matches
			if lastMatchIdx == li-1 {
				consecutive++
				matchScore += consecutive * 5
			} else {
				consecutive = 0
			}

			// Bonus for word boundary match
			if li == 0 || !unicode.IsLetter(rune(label[li-1])) {
				matchScore += 15
			}

			score += matchScore
			lastMatchIdx = li
		}
		li++
	}

	// Only count as match if all query chars were found
	if qi == len(query) {
		return score
	}
	return 0
}

func (m *LabelPickerModel) popupEntries() []PopupMenuEntry {
	entries := make([]PopupMenuEntry, len(m.filtered))
	for i, label := range m.filtered {
		marker := "•"
		if m.selected[label] {
			marker = activeGlyphs.Success
		}
		entries[i] = PopupMenuEntry{Label: label, Marker: marker, Suffix: fmt.Sprintf(" (%d)", m.labelCounts[label]), Selected: i == m.selectedIndex}
	}
	return entries
}

func (m *LabelPickerModel) popupOpts() PopupOpts {
	return PopupOpts{Title: "Filter by Label", Theme: m.theme, Available: m.popupSize, Footer: []string{"space toggle / search ←/→ page enter apply esc back", "space / ←/→ enter esc"}}
}

func (m *LabelPickerModel) popupLayout() PopupLayout {
	return measureSearchPopup(m.popupEntries(), labelPickerMaxVisible, m.popupOpts())
}

// Dimensions returns the modal's outer box (width, height) in cells. The
// click handler uses this to compute the panel's centered start row/col.
func (m *LabelPickerModel) Dimensions() (int, int) {
	l := m.popupLayout()
	return l.Width, l.Height
}

// View renders the label picker overlay
func (m *LabelPickerModel) View() string {
	l := m.popupLayout()
	m.input.SetWidth(max(1, l.BodyWidth-lipgloss.Width(m.input.Prompt)))
	return renderSearchPopup(m.popupEntries(), m.selectedIndex, labelPickerMaxVisible, len(m.selected), m.input.View(), "No matching labels", "labels", m.popupOpts(), l)
}

// InputValue returns the current input value
func (m *LabelPickerModel) InputValue() string {
	return m.input.Value()
}

// FilteredCount returns the number of filtered labels
func (m *LabelPickerModel) FilteredCount() int {
	return len(m.filtered)
}

// itoa is a simple int to string helper
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
