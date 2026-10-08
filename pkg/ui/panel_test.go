package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func assertPopupBounds(t *testing.T, out string, width, height int) {
	t.Helper()
	if out == "" {
		return
	}
	rows := strings.Split(out, "\n")
	if len(rows) > height {
		t.Fatalf("popup has %d rows, available %d:\n%s", len(rows), height, ansi.Strip(out))
	}
	firstWidth := ansi.StringWidth(rows[0])
	for i, row := range rows {
		if w := ansi.StringWidth(row); w > width || w != firstWidth {
			t.Errorf("row %d width=%d, first=%d, available=%d", i, w, firstWidth, width)
		}
	}
}

func popupFindRow(t *testing.T, out, text string) (int, string) {
	t.Helper()
	for i, row := range strings.Split(ansi.Strip(out), "\n") {
		if strings.Contains(row, text) {
			return i, row
		}
	}
	t.Fatalf("missing %q in popup:\n%s", text, ansi.Strip(out))
	return -1, ""
}

func TestPopupFrame_FitsAndKeepsFooter(t *testing.T) {
	opts := PopupOpts{Title: "Status", Theme: DefaultTheme(), Available: &PopupSize{Width: 36, Height: 10}, Width: 60, Height: 20, Footer: []string{"j/k move  enter commit  esc back  * current", "j/k enter esc"}}
	out := RenderPopup([]string{"Open", "In Progress", "Blocked"}, opts)
	assertPopupBounds(t, out, 36, 10)
	y, row := popupFindRow(t, out, "j/k enter esc")
	if y >= len(strings.Split(out, "\n"))-1 {
		t.Fatal("footer must be inside bottom border")
	}
	x := ansi.StringWidth(row[:strings.Index(row, "j/k enter esc")])
	if d := x - (ansi.StringWidth(row) - x - len("j/k enter esc")); d < -1 || d > 1 {
		t.Fatalf("footer not centered: %q", row)
	}
}

func TestPopupFrame_Budgets(t *testing.T) {
	for _, size := range []PopupSize{{120, 32}, {48, 16}, {24, 8}, {8, 3}, {1, 1}, {0, 10}, {10, 0}, {-1, 10}, {10, -1}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			opts := PopupOpts{Title: "Test", Theme: DefaultTheme(), Available: &size, Width: 100, Height: 30, Footer: []string{"enter confirm esc cancel", "enter esc"}}
			body := []string{"界面\nsecond", "third"}
			out := RenderPopup(body, opts)
			layout := MeasurePopup(body, opts)
			if size.Width <= 0 || size.Height <= 0 {
				if out != "" {
					t.Fatalf("zero budget rendered %q", out)
				}
				return
			}
			if out == "" {
				t.Fatal("positive budget rendered nothing")
			}
			assertPopupBounds(t, out, size.Width, size.Height)
			if layout.Width != ansi.StringWidth(strings.Split(out, "\n")[0]) || layout.Height != len(strings.Split(out, "\n")) {
				t.Fatalf("layout disagrees with rendering: %+v", layout)
			}
			if size.Width <= 8 && strings.Contains(out, "╭") {
				t.Fatalf("tiny budget requires fallback: %q", out)
			}
		})
	}
}

func TestPopupFrame_DefaultAndBodyOrigin(t *testing.T) {
	opts := PopupOpts{Title: "Test", Theme: DefaultTheme(), Width: 30}
	body := []string{"first\nsecond", "third"}
	out := RenderPopup(body, opts)
	assertPopupBounds(t, out, 80, 24)
	layout := MeasurePopup(body, opts)
	y, row := popupFindRow(t, out, "first")
	if y != 2 || ansi.StringWidth(row[:strings.Index(row, "first")]) != 3 || layout.BodyX != 3 || layout.BodyY != 2 || layout.BodyHeight != 3 || layout.FooterY != -1 {
		t.Fatalf("wrong body origin/layout: %+v, row %q", layout, row)
	}
	popupFindRow(t, out, "second")
	popupFindRow(t, out, "third")
	assertPopupBounds(t, RenderPopup([]string{strings.Repeat("a", 200)}, opts), 80, 24)
}

func TestPopupFrame_WrapsCompactHintAndBoundsTitle(t *testing.T) {
	opts := PopupOpts{Title: strings.Repeat("界", 30), RightLabel: "(123)", Theme: DefaultTheme(), Available: &PopupSize{12, 12}, Width: 12, Footer: []string{"enter esc"}}
	out := RenderPopup([]string{"body"}, opts)
	assertPopupBounds(t, out, 12, 12)
	popupFindRow(t, out, "(123)")
	popupFindRow(t, out, "enter")
	popupFindRow(t, out, "esc")
	for _, candidates := range [][]string{nil, {}, {""}} {
		if lines := popupFooterLines(8, candidates); len(lines) != 0 {
			t.Fatalf("empty footer produced %v", lines)
		}
	}
}

func TestPopupFrame_MinimumUsefulBody(t *testing.T) {
	opts := PopupOpts{Theme: DefaultTheme(), Available: &PopupSize{40, 7}, MinBodyRows: 5, Footer: []string{"enter esc"}}
	layout := MeasurePopup([]string{"search", "", "item", "", "page"}, opts)
	if !layout.Compact || layout.BodyHeight != 0 || layout.FooterY != -1 {
		t.Fatalf("unusable body exposed as normal layout: %+v", layout)
	}
	assertPopupBounds(t, RenderPopup([]string{"search", "", "item", "", "page"}, opts), 40, 7)
}

func TestPopupFrame_WindowKeepsBreathingRoomWhenUsefulBodyFits(t *testing.T) {
	opts := PopupOpts{Theme: DefaultTheme(), Available: &PopupSize{40, 20}, Height: 15, MinBodyRows: 5, Footer: []string{"enter esc"}}
	l := MeasurePopup(make([]string, 30), opts)
	if l.PadY != 1 || l.BodyY != 2 || l.BodyHeight != 9 {
		t.Fatalf("window lost breathing room despite useful body fitting: %+v", l)
	}
}

func TestPopupFrame_AccentOverride(t *testing.T) {
	theme := DefaultTheme()
	opts := PopupOpts{Theme: theme, Title: "Accent", Width: 30}
	base := RenderPopup([]string{"body"}, opts)
	opts.Accent = lipgloss.Color("#ff0000")
	out := RenderPopup([]string{"body"}, opts)
	if out == base {
		t.Fatal("accent override did not change frame styling")
	}
	if ansi.Strip(out) != ansi.Strip(base) {
		t.Fatal("accent override changed geometry")
	}
}

func TestPopupMenu_ColumnsSurvivePageAndMarkerChanges(t *testing.T) {
	entries := []PopupMenuEntry{{Label: "Open", Shortcut: "s", Marker: "*", Selected: true}, {Label: "In Progress"}, {Label: "界面", Shortcut: "A", Marker: "✓"}}
	layout := MeasurePopupMenu(entries, PopupMenuOpts{Shortcuts: true, Markers: true})
	var columns []int
	for _, page := range [][]PopupMenuEntry{entries[:1], entries[1:]} {
		for _, row := range RenderPopupMenu(page, layout, DefaultTheme(), 30) {
			plain := ansi.Strip(row)
			for _, label := range []string{"Open", "In Progress", "界面"} {
				if index := strings.Index(plain, label); index >= 0 {
					columns = append(columns, ansi.StringWidth(plain[:index]))
				}
			}
			if ansi.StringWidth(row) != 30 {
				t.Errorf("row width=%d, want 30", ansi.StringWidth(row))
			}
		}
	}
	if len(columns) != 3 || columns[0] != columns[1] || columns[1] != columns[2] {
		t.Fatalf("label columns=%v", columns)
	}
	if columns[0] != 13 {
		t.Fatalf("centered label column=%d, want 13", columns[0])
	}
}

func TestPopupMenu_StyledWideContentKeepsGeometry(t *testing.T) {
	entries := []PopupMenuEntry{{Label: "\x1b[1m界面\x1b[0m", Marker: "✓", Suffix: " (4)", Selected: true}, {Label: "Plain", Detail: "a long detail that must occupy only one row"}}
	layout := MeasurePopupMenu(entries, PopupMenuOpts{Markers: true})
	rows := RenderPopupMenu(entries, layout, DefaultTheme(), 16)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for _, row := range rows {
		if ansi.StringWidth(row) != 16 {
			t.Fatalf("row width=%d, want 16", ansi.StringWidth(row))
		}
	}
	first, second := ansi.Strip(rows[0]), ansi.Strip(rows[1])
	if x, y := ansi.StringWidth(first[:strings.Index(first, "界面")]), ansi.StringWidth(second[:strings.Index(second, "Plain")]); x != y {
		t.Fatalf("label columns %d != %d", x, y)
	}
	if detail := ansi.Strip(rows[2]); !strings.HasPrefix(detail, "     a long") {
		t.Fatalf("detail not under label: %q", detail)
	}
}

func TestPopupMenu_OptionalColumnsAndNarrowRows(t *testing.T) {
	entries := []PopupMenuEntry{{Label: "Label", Selected: true}, {Label: "Other"}}
	for _, tc := range []struct {
		opts  PopupMenuOpts
		wantX int
	}{{PopupMenuOpts{}, 2}, {PopupMenuOpts{Shortcuts: true}, 5}, {PopupMenuOpts{Markers: true}, 5}, {PopupMenuOpts{Shortcuts: true, Markers: true}, 8}} {
		layout := MeasurePopupMenu(entries, tc.opts)
		if layout.LabelX != tc.wantX {
			t.Fatalf("column=%d, want %d", layout.LabelX, tc.wantX)
		}
		for _, width := range []int{0, 1, 4, 20} {
			rows := RenderPopupMenu(entries, layout, DefaultTheme(), width)
			if width == 0 && len(rows) != 0 {
				t.Fatal("zero budget rendered menu")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) != width {
					t.Fatalf("row width=%d, want %d", ansi.StringWidth(row), width)
				}
			}
		}
	}
	entries[0].Marker = "界"
	layout := MeasurePopupMenu(entries, PopupMenuOpts{Markers: true})
	before := RenderPopupMenu(entries, layout, DefaultTheme(), 20)
	entries[0].Selected = false
	entries[1].Selected = true
	after := RenderPopupMenu(entries, layout, DefaultTheme(), 20)
	if strings.Index(ansi.Strip(before[0]), "Label") != strings.Index(ansi.Strip(after[0]), "Label") {
		t.Fatal("selection moved label origin")
	}
	if PopupMenuEntryRows(PopupMenuEntry{}) != 1 || PopupMenuEntryRows(PopupMenuEntry{Detail: "detail"}) != 2 {
		t.Fatal("wrong menu row counts")
	}
}

func TestPopupMenu_MultilineDetailStillOccupiesOneRow(t *testing.T) {
	entries := []PopupMenuEntry{{Label: "Name", Detail: "first\nsecond"}}
	layout := MeasurePopupMenu(entries, PopupMenuOpts{})
	rows := RenderPopupMenu(entries, layout, DefaultTheme(), 24)
	if len(rows) != 2 || strings.Contains(rows[1], "\n") || ansi.StringWidth(rows[1]) != 24 {
		t.Fatalf("multiline detail broke row geometry: %q", rows)
	}
	if !strings.Contains(ansi.Strip(rows[1]), "first second") {
		t.Fatal("detail text lost")
	}
}

func TestRenderTitledPanel_Basic(t *testing.T) {
	content := "hello"

	result := RenderTitledPanel(content, PanelOpts{
		Title: "Test",
		Width: 20,
	})

	if !strings.Contains(result, "Test") {
		t.Error("panel should contain title")
	}
	if !strings.Contains(result, "╭") {
		t.Error("panel should have top-left corner")
	}
	if !strings.Contains(result, "╯") {
		t.Error("panel should have bottom-right corner")
	}
	if !strings.Contains(result, "hello") {
		t.Error("panel should contain content")
	}
}

func TestRenderTitledPanel_NoTitle(t *testing.T) {
	result := RenderTitledPanel("content", PanelOpts{
		Width: 20,
	})

	// Should have full horizontal line on top (no title text)
	if !strings.Contains(result, "╭") {
		t.Error("no-title panel should still have border")
	}
	if !strings.Contains(result, "content") {
		t.Error("should contain content")
	}
}

func TestRenderTitledPanel_Focused(t *testing.T) {

	unfocused := RenderTitledPanel("a", PanelOpts{
		Title: "Panel",
		Width: 20,
	})
	focused := RenderTitledPanel("a", PanelOpts{
		Title:   "Panel",
		Width:   20,
		Focused: true,
	})

	// Both should have titles but different styling (hard to test colors in unit test)
	if !strings.Contains(unfocused, "Panel") {
		t.Error("unfocused should have title")
	}
	if !strings.Contains(focused, "Panel") {
		t.Error("focused should have title")
	}
}

func TestRenderTitledPanel_Height(t *testing.T) {
	result := RenderTitledPanel("line1\nline2", PanelOpts{
		Title:  "H",
		Width:  20,
		Height: 5, // top border + 3 content lines + bottom border
	})

	lines := strings.Split(result, "\n")
	// Should have 5 lines: top border, 3 content, bottom border
	// (result ends with bottom border, no trailing newline from split)
	if len(lines) < 5 {
		t.Errorf("expected at least 5 lines for height=5, got %d", len(lines))
	}
}

func TestRenderTitledPanel_TitleTruncation(t *testing.T) {
	result := RenderTitledPanel("x", PanelOpts{
		Title: "This Is A Very Long Title That Should Be Truncated",
		Width: 20,
	})

	if !strings.Contains(result, "…") {
		t.Error("long title should be truncated with ellipsis")
	}
}

func TestRenderTitledPanel_MinWidth(t *testing.T) {
	// Should not panic with very small width
	result := RenderTitledPanel("x", PanelOpts{
		Title: "T",
		Width: 2,
	})
	if result == "" {
		t.Error("should produce output even with small width")
	}
}

func TestRenderTitledPanel_Variants(t *testing.T) {

	normal := RenderTitledPanel("x", PanelOpts{
		Title:   "N",
		Width:   20,
		Variant: BorderNormal,
	})
	thick := RenderTitledPanel("x", PanelOpts{
		Title:   "T",
		Width:   20,
		Variant: BorderThick,
	})
	double := RenderTitledPanel("x", PanelOpts{
		Title:   "D",
		Width:   20,
		Variant: BorderDouble,
	})

	if !strings.Contains(normal, "╭") {
		t.Error("normal variant should use ╭")
	}
	if !strings.Contains(thick, "┏") {
		t.Error("thick variant should use ┏")
	}
	if !strings.Contains(double, "╔") {
		t.Error("double variant should use ╔")
	}
}

func TestRenderTitledPanel_ColorOverrides(t *testing.T) {

	customBorder := lipgloss.Color("#ff0000")
	customTitle := lipgloss.Color("#00ff00")

	// With overrides, the panel should still render correctly regardless of Focused
	result := RenderTitledPanel("content", PanelOpts{
		Title:       "Custom",
		Width:       20,
		Focused:     false,
		BorderColor: customBorder,
		TitleColor:  customTitle,
	})

	if !strings.Contains(result, "Custom") {
		t.Error("panel with color overrides should contain title")
	}
	if !strings.Contains(result, "content") {
		t.Error("panel with color overrides should contain content")
	}
	if !strings.Contains(result, "╭") {
		t.Error("panel with color overrides should have border")
	}

	// Overrides should work with focused too
	focusedResult := RenderTitledPanel("x", PanelOpts{
		Title:       "F",
		Width:       20,
		Focused:     true,
		BorderColor: customBorder,
		TitleColor:  customTitle,
	})
	if !strings.Contains(focusedResult, "F") {
		t.Error("focused panel with overrides should contain title")
	}
}

func TestRenderTitledPanel_PartialOverrides(t *testing.T) {

	// Only override border color, let title use default
	customBorder := lipgloss.Color("#ff0000")
	result := RenderTitledPanel("x", PanelOpts{
		Title:       "Partial",
		Width:       20,
		BorderColor: customBorder,
	})
	if !strings.Contains(result, "Partial") {
		t.Error("partial override should still render title")
	}

	// Only override title color, let border use default
	customTitle := lipgloss.Color("#00ff00")
	result2 := RenderTitledPanel("x", PanelOpts{
		Title:      "Partial2",
		Width:      20,
		TitleColor: customTitle,
	})
	if !strings.Contains(result2, "Partial2") {
		t.Error("partial title override should still render title")
	}
}

// TestTimeTravelPromptUniformRowWidth is a regression guard for bt-rhfo:
// the time-travel modal's title and content rows must all match the panel
// width. The original "⏱️  Time-Travel Mode" title and "⏱️  Revision: "
// textinput prompt under-reported their cell width via runewidth (VS16
// emoji presentation), which the terminal renders 1 cell wider — pushing
// the top border and the input row's right border out of alignment.
func TestTimeTravelPromptUniformRowWidth(t *testing.T) {
	m := seedModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = updated.(Model)

	view := m.renderTimeTravelPrompt()
	rows := strings.Split(view, "\n")
	if len(rows) == 0 {
		t.Fatal("time-travel prompt rendered no rows")
	}
	want := lipgloss.Width(rows[0])
	for i, r := range rows {
		if got := lipgloss.Width(r); got != want {
			t.Errorf("row %d width = %d, want %d (top border); row=%q",
				i, got, want, r)
		}
	}
}

func TestRenderTitledPanel_RightLabel(t *testing.T) {
	// RightLabel renders on the top border with corner stability preserved.
	result := RenderTitledPanel("body", PanelOpts{
		Title:      "Alerts!",
		RightLabel: "(219)",
		Width:      40,
	})
	if !strings.Contains(result, "Alerts!") {
		t.Errorf("right-label render should still show title; got:\n%s", result)
	}
	if !strings.Contains(result, "(219)") {
		t.Errorf("right-label should appear in output; got:\n%s", result)
	}
	// Top border row is first line; both title and label should be there.
	firstLine := strings.SplitN(result, "\n", 2)[0]
	if !strings.Contains(firstLine, "Alerts!") || !strings.Contains(firstLine, "(219)") {
		t.Errorf("title and right-label both expected on top border; got:\n%s", firstLine)
	}
	// Width of the top line equals Width (corner stability).
	if w := lipgloss.Width(firstLine); w != 40 {
		t.Errorf("top border width expected 40 (opts.Width), got %d; line=%q", w, firstLine)
	}

	// Empty RightLabel is a no-op (backwards compat).
	plain := RenderTitledPanel("body", PanelOpts{Title: "Alerts!", Width: 40})
	if strings.Contains(plain, "(") {
		t.Errorf("empty RightLabel should not introduce parens; got:\n%s", plain)
	}
}

// TestRenderTitledPanel_UniformRowWidthWithStyledContent is a regression
// guard for bt-rhfo: when content has per-line ANSI scopes (every line
// styled differently), every rendered row must end at the panel's right
// border, not at the row's natural width. Pre-fix the recipe modal's name
// row ended at column 50 while the description row (different style) ended
// at column 28, leaving the right border ragged and the bg visible through
// the modal interior.
func TestRenderTitledPanel_UniformRowWidthWithStyledContent(t *testing.T) {
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Italic(true)
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#0000ff")).Bold(true)

	content := strings.Join([]string{
		"  " + red.Render("name a"),
		"  " + green.Render("desc a"),
		"",
		"  " + blue.Render("name b longer"),
		"  " + green.Render("desc b"),
	}, "\n")

	const panelWidth = 40
	result := RenderTitledPanel(content, PanelOpts{
		Title: "Test",
		Width: panelWidth,
	})

	rows := strings.Split(result, "\n")
	for i, r := range rows {
		w := lipgloss.Width(r)
		if w != panelWidth {
			t.Errorf("row %d width = %d, want %d (panel.Width); row=%q",
				i, w, panelWidth, r)
		}
	}
}

// TestRenderTitledPanel_StyledRowOverWidthClampsToInnerWidth is the bt-l22b
// root-cause regression guard: when a content row is styled AND exceeds
// innerWidth, the truncate branch must produce a row whose ansi.StringWidth
// is exactly innerWidth. Pre-fix, panel.go used runewidth.Truncate which
// counts SGR escape bytes as visible cells and over-truncates, returning a
// row whose ansi.StringWidth is below innerWidth. The compositor then sees
// fg rows of inconsistent widths and drifts the modal's right border across
// rows - the dimension-sensitive shape that bit Notifications at 117-124+
// and Project Filter at 77-78 / 83-84 / 163-175 even after the defensive
// bg/fg padding landed in OverlayCenter{,DimBackdrop}.
//
// The fix replaces runewidth.Truncate with ansi.Truncate so SGR codes are
// preserved and visible-cell counting is correct. This test exercises the
// truncate branch with styled content well over innerWidth.
func TestRenderTitledPanel_StyledRowOverWidthClampsToInnerWidth(t *testing.T) {
	italic := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#aaaaaa"))
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00"))

	// Each row's visible content well exceeds innerWidth (28) so every row
	// hits the truncate branch. ANSI styling on every row exercises the
	// runewidth-counts-SGR-as-visible bug.
	content := strings.Join([]string{
		italic.Render(strings.Repeat("italic text overflow ", 5)),
		bold.Render(strings.Repeat("bold text overflow ", 5)),
		"plain text " + strings.Repeat("padding ", 8),
		italic.Render("short styled row that fits"), // exercises the < innerWidth branch alongside
	}, "\n")

	const panelWidth = 30
	result := RenderTitledPanel(content, PanelOpts{
		Title: "T",
		Width: panelWidth,
	})

	rows := strings.Split(result, "\n")
	for i, r := range rows {
		w := ansi.StringWidth(r)
		if w != panelWidth {
			t.Errorf("row %d ansi.StringWidth = %d, want %d (panel.Width); row=%q",
				i, w, panelWidth, r)
		}
	}
}

// TestTruncateRunesHelper_StyledInputANSIAware is the bt-l22b second-pass
// regression guard. truncateRunesHelper is called from 20+ sites across
// the TUI (alerts/notifications rows, board cards, history entries, etc.)
// to truncate row content before it reaches RenderTitledPanel. Pre-fix it
// used runewidth.* internally, which counts SGR escape bytes as visible
// cells and over-truncates styled input - leaving rows shorter than the
// caller expected. The L8 panel.go fix did not reach this layer because
// the over-truncation happens upstream of RenderTitledPanel.
//
// Post-fix: helpers.go uses ansi.* internally, so SGR bytes are excluded
// from the cell count and the truncated row's ansi.StringWidth matches
// what the caller asked for.
func TestTruncateRunesHelper_StyledInputANSIAware(t *testing.T) {
	italic := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#aaaaaa"))
	bold := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffaa00"))

	cases := []struct {
		name     string
		input    string
		maxWidth int
		wantW    int // expected ansi.StringWidth of the output
	}{
		{
			name:     "plain text under width passes through",
			input:    "short",
			maxWidth: 20,
			wantW:    5,
		},
		{
			name:     "styled text under width passes through",
			input:    italic.Render("short"),
			maxWidth: 20,
			wantW:    5,
		},
		{
			name:     "plain text over width truncates to width including suffix",
			input:    "this is a long plain string",
			maxWidth: 10,
			wantW:    10, // 9 chars + "…"
		},
		{
			name:     "styled text over width truncates to width including suffix",
			input:    italic.Render("this is a long italic string"),
			maxWidth: 10,
			wantW:    10, // 9 visible chars + "…", SGR bytes not counted
		},
		{
			name:     "bold styled text over width truncates to width including suffix",
			input:    bold.Render(strings.Repeat("bold ", 8)),
			maxWidth: 15,
			wantW:    15,
		},
		{
			name:     "styled text at exact width passes through",
			input:    italic.Render("exact10chr"),
			maxWidth: 10,
			wantW:    10,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := truncateRunesHelper(tc.input, tc.maxWidth, "…")
			if w := ansi.StringWidth(out); w != tc.wantW {
				t.Errorf("ansi.StringWidth = %d, want %d; out=%q", w, tc.wantW, out)
			}
		})
	}
}

// TestRenderTitledPanel_RightLabelOnly covers the bt-fxbl variant where the
// caller wants the label rendered ONLY on the right (no left title). Used
// by the Issues panel in renderSplitView so the title doesn't compete
// visually with the column header right below it.
func TestRenderTitledPanel_RightLabelOnly(t *testing.T) {
	result := RenderTitledPanel("body", PanelOpts{
		RightLabel: "Issues",
		Width:      40,
	})
	firstLine := strings.SplitN(result, "\n", 2)[0]
	if !strings.Contains(firstLine, "Issues") {
		t.Errorf("right-label-only panel should show label on top border; got:\n%s", firstLine)
	}
	// Width should still equal opts.Width (corner stability).
	if w := lipgloss.Width(firstLine); w != 40 {
		t.Errorf("top border width expected 40 (opts.Width), got %d; line=%q", w, firstLine)
	}
	// Sanity: the label appears in the top border (ANSI styling may split
	// "Issues" from the trailing space). Above we already asserted "Issues"
	// is present and the line width matches opts.Width.
}
