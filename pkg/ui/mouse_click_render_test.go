package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// Tests that verify splitViewListChromeHeight() matches the Y where the first
// list item actually renders in the full View() output. The existing
// TestHandleMouseClick_RowMathMatchesChrome asks the implementation for the
// expected Y and then clicks there — so the formula and the assertion move
// together. These tests render the view, find the item's real Y by scanning
// the rendered bytes, and compare. Catches drift in bubbles-list phantom
// behavior, panel chrome, or pill rendering that the self-consistent test
// would miss (bt-ej61).

func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !isCSITerm(s[j]) {
				j++
			}
			i = j
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func isCSITerm(b byte) bool { return b >= 0x40 && b <= 0x7e }

func findRenderedItemY(rendered, needle string) int {
	return findRenderedItemYInPane(rendered, needle, 0)
}

// findRenderedItemYInPane scans a joined two-pane render but bounds the search
// to the leftmost paneWidth runes of each (ANSI-stripped) line. Pass 0 to
// search the whole line. Use a positive paneWidth in split-view tests to avoid
// matching content that bled in from the detail pane on the right (bt-2cvx
// added ID rendering near the top of the detail pane, which used to never
// contain "bd-xxx" needles before). Box-drawing and emoji glyphs are
// multi-byte, so we slice runes — not bytes — to keep the bound aligned with
// visual columns and avoid cutting characters in half.
func findRenderedItemYInPane(rendered, needle string, paneWidth int) int {
	for i, line := range strings.Split(rendered, "\n") {
		stripped := stripANSI(line)
		if paneWidth > 0 {
			if runes := []rune(stripped); len(runes) > paneWidth {
				stripped = string(runes[:paneWidth])
			}
		}
		if strings.Contains(stripped, needle) {
			return i
		}
	}
	return -1
}

func mouseTestModel(n int, w, h, listW, listH int) Model {
	var issues []model.Issue
	for i := 0; i < n; i++ {
		issues = append(issues, model.Issue{
			ID:     "bd-x" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
			Title:  "title",
			Status: model.StatusOpen,
		})
	}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = w
	m.height = h
	m.mode = ViewList
	m.isSplitView = true
	m.list.SetSize(listW, listH)
	m.focused = focusList
	m.ready = true
	return m
}

// Default split view: no pill, no filtering, wide pane.
func TestMouseClick_FormulaMatchesRender_Default(t *testing.T) {
	m := mouseTestModel(3, 200, 40, 60, 30)
	formulaY := m.splitViewListChromeHeight()
	actualY := findRenderedItemY(m.View().Content, "xaa title")
	if formulaY != actualY {
		t.Errorf("chrome height drifted: formula=%d actual=%d", formulaY, actualY)
	}
}

// Workspace mode (REPO badges on each row): header layout doesn't change row count.
func TestMouseClick_FormulaMatchesRender_WorkspaceMode(t *testing.T) {
	m := mouseTestModel(3, 200, 40, 60, 30)
	m.workspaceMode = true
	formulaY := m.splitViewListChromeHeight()
	actualY := findRenderedItemY(m.View().Content, "xaa")
	if formulaY != actualY {
		t.Errorf("chrome height drifted in workspace mode: formula=%d actual=%d", formulaY, actualY)
	}
}

// FilterApplied state: bottom search must not shift the header or item rows.
func TestMouseClick_FormulaMatchesRender_WithPill(t *testing.T) {
	m := mouseTestModel(3, 200, 40, 60, 30)
	m.list.SetFilterText("xaa")
	formulaY := m.splitViewListChromeHeight()
	actualY := findRenderedItemY(m.View().Content, "xaa title")
	if formulaY != actualY {
		t.Errorf("chrome height drifted with pill: formula=%d actual=%d", formulaY, actualY)
	}
}

// Narrow list pane: the literal header text "  TYPE PRI STATUS      ID
// ...  TITLE" (~55 chars) would wrap to a 2nd row before bt-i138. Formula and
// render must still agree when the pane is narrower than the raw header.
// Uses WindowSizeMsg so both list + viewport are sized through the real path,
// then forces a narrow split ratio that puts listInnerWidth well under the
// raw header length.
func TestMouseClick_FormulaMatchesRender_NarrowPane(t *testing.T) {
	m := mouseTestModel(3, 120, 40, 60, 30)
	// Drive through real sizing so viewport width is set.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	// Shrink the list pane (below the ~55-char raw header — pre-fix wrapped).
	m.splitPaneRatio = 0.25
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(Model)
	if m.list.Width() > 40 {
		t.Fatalf("precondition: listInnerWidth=%d, expected narrow (< raw header length)", m.list.Width())
	}
	formulaY := m.splitViewListChromeHeight()
	// Bound search to the list pane's column range — the detail pane's
	// identity strip now contains the bead ID near its top (bt-2cvx) and
	// would otherwise be matched first at narrow split ratios.
	actualY := findRenderedItemYInPane(m.View().Content, "xaa", m.list.Width())
	if formulaY != actualY {
		t.Errorf("chrome height drifted at narrow pane (listW=%d): formula=%d actual=%d",
			m.list.Width(), formulaY, actualY)
	}
}

// Paginated list, still on page 0: click on rendered Y+2 must select index 2.
func TestMouseClick_ResolvesThirdRowOnFirstPage(t *testing.T) {
	m := mouseTestModel(200, 200, 40, 60, 10)
	firstY := findRenderedItemY(m.View().Content, "xaa")
	clicked, _ := m.handleMouseClick(tea.MouseClickMsg{
		X: 10, Y: firstY + 2, Button: tea.MouseLeft,
	})
	if got := clicked.list.Index(); got != 2 {
		t.Errorf("click at rendered Y=%d expected index 2, got %d", firstY+2, got)
	}
}

// Scrolled past page 0: click on first visible rendered row must select the
// first-visible index (page * perPage), not index 0.
func TestMouseClick_ResolvesFirstRowAcrossPages(t *testing.T) {
	m := mouseTestModel(200, 200, 40, 60, 10)
	m.list.Select(25)
	content := m.View().Content
	expected := m.list.Paginator.Page * m.list.Paginator.PerPage
	firstItem := m.list.Items()[expected].(IssueItem)
	needle := CompactIssueID(firstItem.Issue.ID, firstItem.RepoPrefix)
	firstY := findRenderedItemY(content, needle)
	clicked, _ := m.handleMouseClick(tea.MouseClickMsg{
		X: 10, Y: firstY, Button: tea.MouseLeft,
	})
	if got := clicked.list.Index(); got != expected {
		t.Errorf("click at page-%d first visible Y=%d: expected index %d, got %d",
			m.list.Paginator.Page, firstY, expected, got)
	}
}

func TestIssuesSearchAtBottomOnlyWhileActive(t *testing.T) {
	for _, width := range []int{80, 120, 200} {
		for _, fullscreen := range []bool{false, true} {
			for _, tc := range []struct {
				name    string
				state   list.FilterState
				query   string
				visible bool
			}{
				{"idle", list.Unfiltered, "", false},
				{"editing empty", list.Filtering, "", true},
				{"editing", list.Filtering, "title", true},
				{"applied", list.FilterApplied, "title", true},
				{"applied empty", list.FilterApplied, "", false},
				{"no matches", list.FilterApplied, "absent", true},
			} {
				t.Run(fmt.Sprintf("%d/fullscreen=%t/%s", width, fullscreen, tc.name), func(t *testing.T) {
					m := mouseTestModel(3, width, 30, 60, 20)
					if width == 120 {
						m.splitPaneRatio = 0.1 // Pagination must not wrap in a narrow split pane.
					}
					m.list.SetFilterText(tc.query)
					m.list.SetFilterState(tc.state)
					if fullscreen {
						m.fullscreen = fullscreenIssues
					}
					updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
					m = updated.(Model)
					panel := m.renderIssuesPanel(m.list.Width()+4, 29, true)
					searchY := findRenderedItemY(panel, "Search:")
					wantY := -1
					wantHeight := 25 // 29-row panel minus borders, header, pagination.
					if tc.visible {
						wantY = 26 // Directly above pagination (27) and bottom border (28).
						wantHeight--
					}
					if fullscreen {
						wantHeight -= 4 // Selection peek strip.
					}
					if searchY != wantY {
						t.Errorf("search row Y=%d, want %d", searchY, wantY)
					}
					if m.list.Height() != wantHeight {
						t.Errorf("list height=%d, want %d", m.list.Height(), wantHeight)
					}
					if y := findRenderedItemY(panel, "T S P"); y != 1 {
						t.Errorf("column header Y=%d, want 1", y)
					}
					if y := findRenderedItemY(panel, "Page "); y != 27 {
						t.Errorf("pagination Y=%d, want 27", y)
					}
					if searchY >= 0 {
						m.focused = focusDetail
						got, _ := m.handleMouseClick(tea.MouseClickMsg{X: m.list.Width() - 1, Y: searchY, Button: tea.MouseLeft})
						if got.list.FilterState() != list.Filtering || got.focused != focusList {
							t.Errorf("click on bottom search did not focus editing: state=%v focus=%v", got.list.FilterState(), got.focused)
						}
					}
				})
			}
		}
	}
}

func TestIssuesPaginationUsesSearchMatches(t *testing.T) {
	for _, tc := range []struct {
		query string
		index int
		want  string
	}{
		{"xaa", 0, "Page 1/1 (1-1 of 1)"},
		{"absent", 0, "Page 1/1 (0-0 of 0)"},
		{"title", 0, "Page 1/3 (1-24 of 60)"},
		{"title", 30, "Page 2/3 (25-48 of 60)"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			m := mouseTestModel(60, 200, 30, 60, 20)
			m.list.SetFilterText(tc.query)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
			m = updated.(Model)
			m.list.Select(tc.index)
			panel := stripANSI(m.renderIssuesPanel(m.list.Width()+4, 29, true))
			if !strings.Contains(panel, tc.want) {
				t.Errorf("pagination does not describe visible matches; want %q in\n%s", tc.want, panel)
			}
		})
	}
}

func TestIssuesSearchResizesOnKeyboardTransitions(t *testing.T) {
	for _, width := range []int{80, 200} {
		m := NewModel([]model.Issue{{ID: "bd-xaa", Title: "title", Status: model.StatusOpen,
			Description: strings.Repeat("A paragraph of detail content.\n\n", 100)}}, nil, "", nil, nil)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		m = updated.(Model)
		m.updateViewportContent()
		m.viewport.SetYOffset(5)
		beforeOffset := m.viewport.YOffset()
		if beforeOffset != 5 {
			t.Fatalf("precondition: expected scrollable details, offset=%d", beforeOffset)
		}
		for _, step := range []struct {
			key        tea.KeyPressMsg
			wantHeight int
		}{
			{tea.KeyPressMsg{Code: '/'}, 24},
			{tea.KeyPressMsg{Code: tea.KeyEnter}, 25},
			{tea.KeyPressMsg{Code: '/'}, 24},
			{tea.KeyPressMsg{Code: tea.KeyEscape}, 25},
			{tea.KeyPressMsg{Code: '/'}, 24},
			{tea.KeyPressMsg{Code: 't', Text: "t"}, 24},
			{tea.KeyPressMsg{Code: tea.KeyEnter}, 24},
			{tea.KeyPressMsg{Code: tea.KeyEscape}, 25},
		} {
			updated, _ = m.Update(step.key)
			m = updated.(Model)
			if m.list.Height() != step.wantHeight {
				t.Errorf("width=%d key=%s: list height=%d, want %d", width, step.key.String(), m.list.Height(), step.wantHeight)
			}
			if m.viewport.YOffset() != beforeOffset {
				t.Errorf("filter visibility change reset detail scroll")
			}
		}
	}
}
