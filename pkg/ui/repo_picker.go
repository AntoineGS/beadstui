package ui

import (
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/model"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

// RepoPickerModel represents the repository filter picker overlay (workspace mode).
type RepoPickerModel struct {
	repos    []string        // canonical full list (original enumeration order)
	filtered []string        // display/nav list: atlas-pinned, search-narrowed
	input    textinput.Model // search box (Wave 2, bt-9lpib core)
	// searchFocused gates whether typed characters route to the text input or
	// are interpreted as navigation. The picker opens with searchFocused false
	// (mirrors the label picker, bt-wnda): the user lands on the project list;
	// "/" focuses the search bar, Esc inside search blurs it without closing.
	searchFocused bool
	selectedIndex int
	selected      map[string]bool // repo -> selected
	width         int
	height        int
	theme         Theme
	popupSize     *PopupSize
}

// NewRepoPickerModel creates a new repo picker. By default, all repos are selected.
func NewRepoPickerModel(repos []string, theme Theme) RepoPickerModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 50
	ti.SetWidth(30)
	// The View renders its own styled "> " prompt, so clear the textinput's
	// built-in one to avoid a doubled ">".
	ti.Prompt = ""
	// Search starts blurred (bt-wnda parity): the user lands on the list.
	ti.Blur()

	m := RepoPickerModel{
		repos:         append([]string(nil), repos...),
		input:         ti,
		selectedIndex: 0,
		selected:      make(map[string]bool, len(repos)),
		theme:         theme,
	}
	for _, r := range m.repos {
		m.selected[r] = true
	}
	m.filterRepos()
	return m
}

// SetSize updates the picker dimensions.
func (m *RepoPickerModel) SetSize(width, height int) {
	m.popupSize = &PopupSize{width, height}
	m.width = width
	m.height = height
}

// FocusSearch routes typed characters to the search input.
func (m *RepoPickerModel) FocusSearch() {
	m.searchFocused = true
	m.input.Focus()
}

// BlurSearch returns focus to the project list. The search query buffer is
// preserved so the user can resume editing without retyping.
func (m *RepoPickerModel) BlurSearch() {
	m.searchFocused = false
	m.input.Blur()
}

// IsSearchFocused reports whether the text input owns keyboard focus.
func (m *RepoPickerModel) IsSearchFocused() bool {
	return m.searchFocused
}

// UpdateInput processes a key message for the text input, then re-filters.
func (m *RepoPickerModel) UpdateInput(msg interface{}) {
	m.input, _ = m.input.Update(msg)
	m.filterRepos()
}

// InputValue returns the current search query.
func (m *RepoPickerModel) InputValue() string {
	return m.input.Value()
}

// atlasFirst returns repos reordered so any beads_global/global namespace key
// sorts to the front (bt-z1pzj: first-class pinned row), preserving the
// relative order of every other repo. Display-only reordering; m.selected keys
// and the active-repo filter stay on the raw spelling, so selection/filtering
// logic is untouched.
func atlasFirst(repos []string) []string {
	pinned := make([]string, 0, 1)
	rest := make([]string, 0, len(repos))
	for _, r := range repos {
		if model.IsAtlasNamespace(r) {
			pinned = append(pinned, r)
		} else {
			rest = append(rest, r)
		}
	}
	return append(pinned, rest...)
}

// repoMatches reports whether repo's display name contains query (already
// lower-cased and trimmed). Matches on DisplayRepoName so a user searching for
// "atlas" finds the beads_global namespace by the label they actually see.
func repoMatches(repo, query string) bool {
	return strings.Contains(strings.ToLower(model.DisplayRepoName(repo)), query)
}

// filterRepos rebuilds m.filtered from the current search query. Empty query =
// the full list; otherwise a case-insensitive substring narrow. The atlas
// namespace is always pinned to the top of whatever survives the filter.
func (m *RepoPickerModel) filterRepos() {
	query := strings.ToLower(strings.TrimSpace(m.input.Value()))

	var result []string
	if query == "" {
		result = append(result, m.repos...)
	} else {
		for _, r := range m.repos {
			if repoMatches(r, query) {
				result = append(result, r)
			}
		}
	}

	m.filtered = atlasFirst(result)

	// Keep the cursor in bounds after the list changes size.
	if m.selectedIndex >= len(m.filtered) {
		m.selectedIndex = len(m.filtered) - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

// SetActiveRepos initializes selection from the currently active repo filter (nil = all).
// Cursor moves to the first selected project (or stays at top if all/none).
func (m *RepoPickerModel) SetActiveRepos(active map[string]bool) {
	if len(m.repos) == 0 {
		m.selected = map[string]bool{}
		return
	}

	m.selected = make(map[string]bool, len(m.repos))
	if active == nil || len(active) <= 1 {
		// All-projects or single-project mode: open with nothing checked for quick-pick.
		// Multi-project groups (2+) preserve their checkmarks for add/remove.
		m.selectedIndex = 0
		return
	}

	firstSelected := -1
	for i, r := range m.filtered {
		if active[r] {
			m.selected[r] = true
			if firstSelected == -1 {
				firstSelected = i
			}
		}
	}
	// Selection is keyed on the raw repo name regardless of display position,
	// so also mark any active repo not currently in the filtered view.
	for r := range active {
		if _, ok := m.selected[r]; !ok {
			for _, rr := range m.repos {
				if rr == r {
					m.selected[r] = true
				}
			}
		}
	}
	if firstSelected >= 0 {
		m.selectedIndex = firstSelected
	} else {
		m.selectedIndex = 0
	}
}

// MoveUp moves selection up, wrapping to the bottom.
func (m *RepoPickerModel) MoveUp() {
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
func (m *RepoPickerModel) MoveDown() {
	if len(m.filtered) == 0 {
		return
	}
	if m.selectedIndex < len(m.filtered)-1 {
		m.selectedIndex++
	} else {
		m.selectedIndex = 0
	}
}

// PageDown moves selection to the bottom of the next page (bt-6ltx9).
func (m *RepoPickerModel) PageDown() {
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

// PageUp moves selection to the top of the previous page (bt-6ltx9).
func (m *RepoPickerModel) PageUp() {
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

// ToggleSelected toggles the selected state of the current repo.
func (m *RepoPickerModel) ToggleSelected() {
	if len(m.filtered) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.filtered) {
		return
	}
	r := m.filtered[m.selectedIndex]
	m.selected[r] = !m.selected[r]
}

// AnySelected returns true if at least one repo is selected.
func (m *RepoPickerModel) AnySelected() bool {
	for _, r := range m.repos {
		if m.selected[r] {
			return true
		}
	}
	return false
}

// NoneSelected returns true if no repos are selected.
func (m *RepoPickerModel) NoneSelected() bool {
	return !m.AnySelected()
}

// AllSelected returns true if every repo is selected.
func (m *RepoPickerModel) AllSelected() bool {
	for _, r := range m.repos {
		if !m.selected[r] {
			return false
		}
	}
	return len(m.repos) > 0
}

// ToggleAll deselects all if any are selected, otherwise selects all.
func (m *RepoPickerModel) ToggleAll() {
	if m.AnySelected() {
		m.DeselectAll()
	} else {
		m.SelectAll()
	}
}

// SelectAll selects all repos.
func (m *RepoPickerModel) SelectAll() {
	for _, r := range m.repos {
		m.selected[r] = true
	}
}

// DeselectAll deselects all repos.
func (m *RepoPickerModel) DeselectAll() {
	for _, r := range m.repos {
		m.selected[r] = false
	}
}

// CursorRepo returns the repo name under the cursor.
func (m *RepoPickerModel) CursorRepo() string {
	if len(m.filtered) == 0 || m.selectedIndex < 0 || m.selectedIndex >= len(m.filtered) {
		return ""
	}
	return m.filtered[m.selectedIndex]
}

// SelectedRepos returns the selected repos as a map (repo -> true). Selection
// spans the full repo set, not just the currently filtered view, so a search
// that hides a checked project does not drop it from the applied filter.
func (m RepoPickerModel) SelectedRepos() map[string]bool {
	out := make(map[string]bool)
	for _, r := range m.repos {
		if m.selected[r] {
			out[r] = true
		}
	}
	return out
}

// SetCursor moves the cursor to the given index. Out-of-bounds indices are
// clamped. Used by the mouse click handler (bt-hpsq).
func (m *RepoPickerModel) SetCursor(idx int) {
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

// repoPickerMaxVisible caps the number of repo rows shown at once. With many
// projects in workspace mode the modal would otherwise grow without bound and
// overflow the terminal on smaller windows.
const repoPickerMaxVisible = 30

// visibleCount returns how many repo rows fit in the modal at the current
// terminal size. Mirrors the label picker pattern (bt-vr2h): aim for ~75%
// of bg, fall back to whatever fits, cap at repoPickerMaxVisible (30) on
// tall terminals, and never exceed the filtered count so the modal stays
// compact for a short project list (paging engages only on genuine overflow,
// which is precisely the bt-6ltx9 scenario).
func (m *RepoPickerModel) visibleCount() int {
	return min(repoPickerMaxVisible, min(max(1, len(m.filtered)), max(1, m.popupLayout().BodyHeight-4)))
}

// popupEntries uses display aliases without changing raw selection keys.
func (m *RepoPickerModel) popupEntries() []PopupMenuEntry {
	entries := make([]PopupMenuEntry, len(m.filtered))
	for i, repo := range m.filtered {
		marker := "•"
		if m.selected[repo] {
			marker = activeGlyphs.Success
		}
		entries[i] = PopupMenuEntry{Label: model.DisplayRepoName(repo), Marker: marker, Selected: i == m.selectedIndex}
	}
	return entries
}

func (m *RepoPickerModel) popupOpts() PopupOpts {
	return PopupOpts{Title: "Project Filter", Theme: m.theme, Available: m.popupSize, Footer: []string{pickerFooter, "space toggle / search ←/→ page enter apply esc back", "space / ←/→ enter esc"}}
}

func (m *RepoPickerModel) popupLayout() PopupLayout {
	return measureSearchPopup(m.popupEntries(), min(repoPickerMaxVisible, max(1, len(m.filtered))), m.popupOpts())
}

// Dimensions returns the modal's outer box (width, height) in cells, used by
// the mouse click handler to compute the centered panel start row/col.
func (m *RepoPickerModel) Dimensions() (int, int) {
	l := m.popupLayout()
	return l.Width, l.Height
}

// ItemAtPanelY maps a Y coordinate relative to the picker's top border to
// a filtered-list index. Returns (-1, false) for non-row regions (chrome,
// input, blanks, page indicator, footer). Accounts for page-aligned
// scrolling when len(m.filtered) exceeds visibleCount() (bt-vr2h).
func (m *RepoPickerModel) ItemAtPanelY(my int) (int, bool) {
	l := m.popupLayout()
	if len(m.filtered) == 0 || l.Compact || l.Height == 0 {
		return -1, false
	}
	maxVisible := m.visibleCount()
	relRow := my - l.BodyY - 2
	if relRow < 0 || relRow >= maxVisible {
		return -1, false
	}
	start := (m.selectedIndex / maxVisible) * maxVisible
	idx := start + relRow
	if idx >= len(m.filtered) {
		return -1, false
	}
	return idx, true
}

// IsSearchRow reports whether the given panel-relative Y is the search input
// row. Used by mouse routing to focus the search input on click.
func (m *RepoPickerModel) IsSearchRow(my int) bool {
	l := m.popupLayout()
	return !l.Compact && l.Height > 0 && my == l.BodyY
}

// footer hint text (no padding - added during render). Mirrors the label
// picker footer convention: select-all ("a") lives in the ; sidebar / ?
// overlay rather than the footer, keeping the line short enough to render
// without truncation on typical terminals.
const pickerFooter = "toggle: space search: / page: ←/→ • apply: enter esc: back"

// View renders the repo picker overlay.
func (m *RepoPickerModel) View() string {
	l := m.popupLayout()
	m.input.SetWidth(max(1, l.BodyWidth-lipgloss.Width(m.input.Prompt)))
	empty := "No projects available."
	if strings.TrimSpace(m.input.Value()) != "" {
		empty = "No matching projects"
	}
	return renderSearchPopup(m.popupEntries(), m.selectedIndex, min(repoPickerMaxVisible, max(1, len(m.filtered))), 0, m.input.View(), empty, "projects", m.popupOpts(), l)
}
