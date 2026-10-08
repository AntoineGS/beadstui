package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	// Check a few known colors are set (not nil)
	if theme.Primary == nil {
		t.Error("DefaultTheme Primary color is nil")
	}
	if theme.Open == nil {
		t.Error("DefaultTheme Open color is nil")
	}
}

func TestThemeRefreshRetainsActionableState(t *testing.T) {
	restoreThemeGlobals(t)
	withThemeConfigHome(t)
	t.Setenv("BT_THEME", "")
	m := settingsTestModel(t)
	plan := analysis.ExecutionPlan{Tracks: []analysis.ExecutionTrack{
		{TrackID: "track-A", Items: []analysis.PlanItem{{ID: "first"}, {ID: "second"}}},
	}}
	m.actionableView = NewActionableModel(plan, m.theme)
	m.actionableView.SetSize(80, 12)
	m.actionableView.MoveDown()
	// The current split-pane renderer is not necessarily 80 columns wide.
	m.renderer.SetWidth(47)
	m.applyThemeLive("dracula")
	if m.actionableView.SelectedIssueID() != "second" {
		t.Fatal("selection reset")
	}
	if m.actionableView.theme.Text.Body.GetForeground() != m.theme.Text.Body.GetForeground() {
		t.Fatal("retained Actionable theme is stale")
	}
	if m.renderer.width != 47 {
		t.Fatalf("theme refresh changed markdown wrap width: %d", m.renderer.width)
	}
}

func assertRetainedThemes(t *testing.T, m *Model) {
	t.Helper()
	consumers := map[string]*Theme{
		"board": &m.board.theme, "labels": &m.labelDashboard.theme,
		"velocity": &m.velocityComparison.theme, "sidebar": &m.shortcutsSidebar.theme,
		"graph": &m.graphView.theme, "tree": &m.tree.theme,
		"insights": &m.insightsPanel.theme, "flow": &m.flowMatrix.theme,
		"actionable": &m.actionableView.theme, "history": &m.historyView.theme,
		"memories": &m.memories.theme, "recipe": &m.recipePicker.theme,
		"bql": &m.bqlQuery.theme, "labels picker": &m.labelPicker.theme,
		"repo": &m.repoPicker.theme, "agent": &m.agentPromptModal.theme,
		"tutorial": &m.tutorialModel.theme, "cass": &m.cassModal.theme,
		"update": &m.updateModal.theme, "field select": &m.fieldSelect.theme,
		"field picker": &m.fieldPicker.theme, "field input": &m.fieldInput.theme,
		"longform": &m.longformEdit.theme, "settings": &m.settingsModal.theme,
		"settings menu": &m.settingsMenu.theme, "epics": &m.epicsTree.theme,
	}
	for name, theme := range consumers {
		if !reflect.DeepEqual(theme.Text, m.theme.Text) || theme.Primary != m.theme.Primary || theme.Bg != m.theme.Bg {
			t.Errorf("%s retained stale styles/palette", name)
		}
	}
}

func TestThemeRefreshAllRetainedConsumers(t *testing.T) {
	restoreThemeGlobals(t)
	withThemeConfigHome(t)
	t.Setenv("BT_THEME", "")
	m := epicsTestModel(epicsFixture())
	assertRetainedThemes(t, &m) // Startup must initialize even unopened models.
	pickerInputs := map[string]*textinput.Model{
		"label": &m.labelPicker.input,
		"repo":  &m.repoPicker.input,
	}
	assertPickerStyles := func() {
		t.Helper()
		for name, input := range pickerInputs {
			styles := input.Styles()
			for stateName, state := range map[string]textinput.StyleState{"focused": styles.Focused, "blurred": styles.Blurred} {
				if !reflect.DeepEqual(state.Text, m.theme.Text.Body) || !reflect.DeepEqual(state.Prompt, m.theme.Text.Heading) || !reflect.DeepEqual(state.Placeholder, m.theme.Text.Metadata) || !reflect.DeepEqual(state.Suggestion, m.theme.Text.Metadata) {
					t.Errorf("%s picker %s widget styles stale", name, stateName)
				}
			}
		}
	}
	assertPickerStyles() // These retained search widgets also need startup roles.
	// Repository selection is initialized lazily; use a real opened picker for
	// cursor/focus retention rather than focusing its zero-value startup input.
	m.repoPicker = NewRepoPickerModel([]string{"repo"}, m.theme)
	m.labelPicker.input.SetValue("label search")
	m.labelPicker.input.SetCursor(3)
	m.labelPicker.input.Focus()
	m.repoPicker.input.SetValue("repo search")
	m.repoPicker.input.SetCursor(5)
	m.repoPicker.input.Blur()
	m.list.Select(1)
	m.list.SetFilterText("ep")
	index, filterState := m.list.Index(), m.list.FilterState()
	m.fieldInput = NewFieldInputModal("title", "Title", "typed text", m.theme)
	m.fieldInput.input.SetCursor(3)
	m.longformEdit = NewLongformEditModal("description", "Description", "draft\nsecond line", m.theme)
	m.longformEdit.SetSize(80, 20)
	m.longformEdit.textarea.SetCursorColumn(3)
	row, col := m.longformEdit.textarea.Line(), m.longformEdit.textarea.Column()
	m.openModal(ModalLongformEdit)
	m.shortcutsSidebar.scrollOffset = 4
	m.actionableView = NewActionableModel(analysis.ExecutionPlan{Tracks: []analysis.ExecutionTrack{
		{TrackID: "track-A", Items: []analysis.PlanItem{{ID: "first"}, {ID: "second"}}},
	}}, m.theme)
	m.actionableView.MoveDown()
	m.actionableView.scrollOffset = 2
	m.epicsTree.moveCursor(1)
	m.epicsViewText = m.epicsTree.View()
	epicsCursor := m.epicsTree.cursor
	m.viewport.SetHeight(3)
	m.updateViewportContent()
	m.viewport.SetYOffset(2)
	offset := m.viewport.YOffset()
	for i, tf := range []*ThemeFile{
		{Text: TextRoleConfigs{Body: TextRoleConfig{Foreground: "danger", Underline: hptr(true)}, Metadata: TextRoleConfig{Italic: hptr(true)}}},
		{Text: TextRoleConfigs{Body: TextRoleConfig{Foreground: "success", Underline: hptr(false)}}},
	} {
		if i == 1 {
			m.labelPicker.input.Blur()
			m.repoPicker.input.Focus()
		}
		m.applyThemeConfig(tf)
		assertRetainedThemes(t, &m)
		assertPickerStyles()
		if m.labelPicker.input.Value() != "label search" || m.labelPicker.input.Position() != 3 || m.labelPicker.input.Focused() != (i == 0) || m.repoPicker.input.Value() != "repo search" || m.repoPicker.input.Position() != 5 || m.repoPicker.input.Focused() != (i == 1) {
			t.Fatal("picker search text/cursor/focus reset")
		}
		if m.list.Index() != index || m.list.FilterState() != filterState || m.list.FilterInput.Value() != "ep" {
			t.Fatal("list selection/filter reset")
		}
		if m.actionableView.SelectedIssueID() != "second" || m.actionableView.scrollOffset != 2 || m.shortcutsSidebar.scrollOffset != 4 {
			t.Fatal("retained navigation reset")
		}
		if m.fieldInput.input.Value() != "typed text" || m.fieldInput.input.Position() != 3 || m.longformEdit.textarea.Value() != "draft\nsecond line" || m.longformEdit.textarea.Line() != row || m.longformEdit.textarea.Column() != col {
			t.Fatal("edit draft/cursor reset")
		}
		if m.activeModal != ModalLongformEdit || m.viewport.YOffset() != offset || m.epicsTree.cursor != epicsCursor {
			t.Fatal("modal/viewport/epics selection reset")
		}
		if m.epicsViewText != m.epicsTree.View() || !strings.Contains(m.epicsViewText, "ep1") {
			t.Fatal("epics presentation cache not repainted")
		}
		if !reflect.DeepEqual(m.fieldInput.input.Styles().Focused.Text, m.theme.Text.Body) || !reflect.DeepEqual(m.longformEdit.textarea.Styles().Focused.Text, m.theme.Text.Body) || !reflect.DeepEqual(m.list.Styles.Filter.Focused.Text, m.theme.Text.Body) {
			t.Fatal("widget styles stale")
		}
	}
}

func TestThemeRefreshBackgroundMode(t *testing.T) {
	restoreThemeGlobals(t)
	withThemeConfigHome(t)
	t.Setenv("BT_THEME", "dracula")
	m := settingsTestModel(t)
	m.renderer.SetWidth(43)
	for _, dark := range []bool{false, true} {
		background := lipgloss.Color("#ffffff")
		if dark {
			background = lipgloss.Color("#000000")
		}
		updated, _ := m.Update(tea.BackgroundColorMsg{Color: background})
		m = updated.(Model)
		assertRetainedThemes(t, &m)
		for _, r := range []*MarkdownRenderer{m.renderer, m.insightsPanel.mdRenderer, m.tutorialModel.markdownRenderer} {
			if r == nil || r.IsDarkMode() != dark || !reflect.DeepEqual(r.theme.Text, m.theme.Text) {
				t.Fatal("markdown renderer stale after background change")
			}
		}
		if m.renderer.width != 43 {
			t.Fatal("background update changed wrap width")
		}
	}
}

func TestThemeRefreshPresentationCachesPreservesPendingNavigation(t *testing.T) {
	restoreThemeGlobals(t)
	withThemeConfigHome(t)
	m := NewModel([]model.Issue{{ID: "cache", Title: "Cached issue", Status: model.StatusOpen, Description: strings.Repeat("paragraph\n\n", 40)}}, nil, "", nil, nil)
	m.viewport.SetHeight(3)
	m.updateViewportContent()
	m.viewport.SetYOffset(3)
	m.board.renderDetailPanel(50, 12)
	m.board.detailVP.SetYOffset(2)
	m.viewport.SetContent(strings.Repeat("stale\n", 100))
	m.board.detailVP.SetContent(strings.Repeat("stale\n", 100))
	m.insightsPanel.detailContent = "stale"
	m.pendingCommentScroll = time.Unix(100, 0)
	pending := m.pendingCommentScroll
	m.applyThemeLive("dracula")
	if m.pendingCommentScroll != pending {
		t.Fatal("cosmetic refresh consumed pending comment navigation")
	}
	if m.viewport.YOffset() != 3 || m.board.detailVP.YOffset() != 2 {
		t.Fatal("cached detail repaint reset scroll")
	}
	if strings.Contains(m.viewport.View(), "stale") || strings.Contains(m.board.detailVP.View(), "stale") || m.insightsPanel.detailContent == "stale" {
		t.Fatal("detail presentation cache not repainted")
	}
}

func TestTextStylesUseOwnPalette(t *testing.T) {
	restoreThemeGlobals(t)
	previous := isDarkBackground
	isDarkBackground = true
	t.Cleanup(func() { isDarkBackground = previous })
	a := DefaultTheme()
	ApplyThemeToThemeStruct(&a, &ThemeFile{Colors: ThemeColors{
		Text: &AdaptiveHex{Dark: "#123456"}, Primary: &AdaptiveHex{Dark: "#ffccdd"},
	}})
	ApplyThemeToGlobals(&ThemeFile{Colors: ThemeColors{Text: &AdaptiveHex{Dark: "#abcdef"}}})
	if a.Text.Body.GetForeground() != lipgloss.Color("#123456") {
		t.Fatal("body style borrowed the global palette")
	}
	if a.Base.GetForeground() != a.Text.Body.GetForeground() {
		t.Fatal("base text stayed on fallback colors")
	}
	if ratio := textContrastRatio(a.Text.Title.GetForeground(), a.Text.Title.GetBackground()); ratio < 4.5 {
		t.Fatalf("filled title contrast %.2f < 4.5", ratio)
	}
	if ActiveTextStyles.Body.GetForeground() != lipgloss.Color("#abcdef") {
		t.Fatal("active body did not use applied config")
	}
}

func TestAutoTextForeground(t *testing.T) {
	for _, bg := range []string{"#000000", "#ffffff", "#777777", "#bd93f9", "#ffb8d1"} {
		c := lipgloss.Color(bg)
		want := lipgloss.Color("#000000")
		if textContrastRatio(lipgloss.Color("#ffffff"), c) > textContrastRatio(want, c) {
			want = lipgloss.Color("#ffffff")
		}
		if got := autoTextForeground(c); got != want {
			t.Fatalf("background %s: foreground %v, want maximum-contrast %v", bg, got, want)
		}
		if ratio := textContrastRatio(autoTextForeground(c), c); ratio < 4.5 {
			t.Fatalf("background %s: contrast %.2f", bg, ratio)
		}
	}
}

func TestTextStylesNamedPalettes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BT_THEME", "")
	t.Chdir(t.TempDir())
	restoreThemeGlobals(t)
	for _, dark := range []bool{true, false} {
		isDarkBackground = dark
		for _, name := range []string{"dracula", "matcha-dark-sea", "paper", "bt:greyscale"} {
			t.Run(fmt.Sprintf("%s/dark=%t", name, dark), func(t *testing.T) {
				theme := DefaultTheme()
				ApplyThemeToThemeStruct(&theme, LoadThemeNamed(name))
				for _, pair := range []struct {
					style lipgloss.Style
					bg    color.Color
				}{
					{theme.Text.Title, theme.Primary}, {theme.Text.Badge, theme.Secondary},
					{theme.Text.Selected, theme.Highlight}, {theme.Text.Callout, theme.BgHighlight},
				} {
					if pair.style.GetBackground() != pair.bg {
						t.Fatal("incorrect role background")
					}
					if ratio := textContrastRatio(pair.style.GetForeground(), pair.bg); ratio < 4.5 {
						t.Fatalf("contrast %.2f < 4.5", ratio)
					}
					black := textContrastRatio(lipgloss.Color("#000000"), pair.bg)
					white := textContrastRatio(lipgloss.Color("#ffffff"), pair.bg)
					if got := textContrastRatio(pair.style.GetForeground(), pair.bg); got != max(black, white) {
						t.Fatalf("contrast %v, want maximum %v", got, max(black, white))
					}
				}
			})
		}
	}
}

func TestTextStylesAttributesAndExplicitForeground(t *testing.T) {
	theme := DefaultTheme()
	no, yes := false, true
	off := TextRoleConfig{Bold: &no, Italic: &no, Underline: &no}
	ApplyThemeToThemeStruct(&theme, &ThemeFile{Text: TextRoleConfigs{
		Title: off, Heading: off, Body: off, Metadata: off, Badge: off, Selected: off, Callout: off,
	}})
	for _, style := range []lipgloss.Style{theme.Text.Title, theme.Text.Heading, theme.Text.Body,
		theme.Text.Metadata, theme.Text.Badge, theme.Text.Selected, theme.Text.Callout} {
		if style.GetBold() || style.GetItalic() || style.GetUnderline() || style.GetFaint() {
			t.Fatal("explicit false attribute not respected")
		}
	}
	ApplyThemeToThemeStruct(&theme, &ThemeFile{Text: TextRoleConfigs{
		Title: TextRoleConfig{Foreground: "secondary", Italic: &yes, Underline: &yes},
		Body:  TextRoleConfig{Bold: &yes, Italic: &yes, Underline: &yes},
	}})
	if theme.Text.Title.GetForeground() != theme.Secondary {
		t.Fatal("explicit foreground auto-corrected")
	}
	ApplyThemeToThemeStruct(&theme, &ThemeFile{})
	if theme.Text.Title.GetItalic() || theme.Text.Title.GetUnderline() || !theme.Text.Title.GetBold() {
		t.Fatal("title did not reset to default attributes")
	}
	if theme.Text.Body.GetBold() || theme.Text.Body.GetItalic() || theme.Text.Body.GetUnderline() {
		t.Fatal("body retained previous attributes")
	}
	if _, ok := theme.Text.Body.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatal("none background not preserved")
	}
	ApplyThemeToThemeStruct(&theme, &ThemeFile{Text: TextRoleConfigs{Body: TextRoleConfig{Foreground: "auto"}}})
	if textContrastRatio(theme.Text.Body.GetForeground(), theme.Bg) < 4.5 {
		t.Fatal("auto with none ignored theme background")
	}
}

func TestTextStylesPaletteTokens(t *testing.T) {
	theme := DefaultTheme()
	for token, want := range map[string]color.Color{
		"bg": theme.Bg, "bg_dark": theme.BgDark, "bg_subtle": theme.BgSubtle,
		"bg_highlight": theme.BgHighlight, "text": theme.TextColor, "subtext": theme.Subtext,
		"muted": theme.Muted, "primary": theme.Primary, "secondary": theme.Secondary,
		"info": theme.Info, "success": theme.Success, "warning": theme.Warning, "danger": theme.Danger,
		"text_secondary": theme.TextSecondary, "bg_contrast": theme.BgContrast,
		"border": theme.Border, "highlight": theme.Highlight,
	} {
		if want == nil || theme.textPaletteColor(token) != want {
			t.Fatalf("token %s not resolved", token)
		}
	}
	for _, bg := range []color.Color{nil, lipgloss.NoColor{}} {
		partial := Theme{Bg: bg}
		partial.rebuildTextStyles(TextRoleConfigs{})
		if partial.Text.Body.GetForeground() == nil {
			t.Fatal("partial theme has no foreground")
		}
		if _, invisible := partial.Text.Body.GetForeground().(lipgloss.NoColor); invisible {
			t.Fatal("partial theme has invisible foreground")
		}
	}
}

func TestTextStylesLoadedScalarPalette(t *testing.T) {
	previous := isDarkBackground
	t.Cleanup(func() { isDarkBackground = previous })
	hex := &AdaptiveHex{Light: "#135790", Dark: "#246801"}
	for _, dark := range []bool{false, true} {
		isDarkBackground = dark
		theme := DefaultTheme()
		ApplyThemeToThemeStruct(&theme, &ThemeFile{Colors: ThemeColors{
			Bg: hex, BgDark: hex, BgSubtle: hex, BgHighlight: hex,
			Text: hex, TextSecondary: hex, BgContrast: hex,
		}})
		want := resolveColor(hex.Light, hex.Dark)
		for _, token := range []string{"bg", "bg_dark", "bg_subtle", "bg_highlight", "text", "text_secondary", "bg_contrast"} {
			if theme.textPaletteColor(token) != want {
				t.Fatalf("loaded %s did not resolve in dark=%t", token, dark)
			}
		}
		if theme.Base.GetForeground() != want || theme.Text.Body.GetForeground() != want {
			t.Fatal("body/base did not rebuild from loaded text")
		}
	}
}

func TestGetStatusColor(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		status string
		want   color.Color
	}{
		{"open", theme.Open},
		{"in_progress", theme.InProgress},
		{"blocked", theme.Blocked},
		{"closed", theme.Closed},
		{"unknown", theme.Subtext},
		{"", theme.Subtext},
	}

	for _, tt := range tests {
		got := theme.GetStatusColor(tt.status)
		if got != tt.want {
			t.Errorf("GetStatusColor(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestGetTypeColor(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		typ      string
		wantIcon string
		wantCol  color.Color
	}{
		{"bug", activeGlyphs.TypeBug, theme.Bug},
		{"feature", activeGlyphs.TypeFeature, theme.Feature},
		{"task", activeGlyphs.TypeTask, theme.Task},
		{"epic", activeGlyphs.TypeEpic, theme.Epic},
		{"chore", activeGlyphs.TypeChore, theme.Chore},
		{"unknown", activeGlyphs.TypeDefault, theme.Subtext},
	}

	for _, tt := range tests {
		icon, col := GetTypeIcon(tt.typ), theme.GetTypeColor(tt.typ)
		if icon != tt.wantIcon {
			t.Errorf("GetTypeIcon(%q) icon = %q, want %q", tt.typ, icon, tt.wantIcon)
		}
		if col != tt.wantCol {
			t.Errorf("GetTypeIcon(%q) color = %v, want %v", tt.typ, col, tt.wantCol)
		}
	}
}

// -- Color profile detection tests (bd-2rih) --

func TestColorProfile_Detection(t *testing.T) {
	// TermProfile is set at init(); just verify it's a valid value
	valid := map[colorprofile.Profile]bool{
		colorprofile.Unknown:   true,
		colorprofile.NoTTY:     true,
		colorprofile.ASCII:     true,
		colorprofile.ANSI:      true,
		colorprofile.ANSI256:   true,
		colorprofile.TrueColor: true,
	}
	if !valid[TermProfile] {
		t.Errorf("TermProfile has unexpected value: %d", TermProfile)
	}
}

func TestThemeBg_TrueColor(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.TrueColor

	got := ThemeBg("#282A36")
	if _, ok := got.(lipgloss.NoColor); ok {
		t.Error("ThemeBg should return hex color in TrueColor mode, got NoColor")
	}
}

func TestThemeBg_ANSI(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.ANSI

	got := ThemeBg("#282A36")
	if _, ok := got.(lipgloss.NoColor); !ok {
		t.Errorf("ThemeBg should return NoColor in ANSI mode, got %T", got)
	}
}

func TestThemeBg_ANSI256(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.ANSI256

	got := ThemeBg("#282A36")
	if _, ok := got.(lipgloss.NoColor); !ok {
		t.Errorf("ThemeBg should return NoColor in ANSI256 mode (only TrueColor gets hex bg), got %T", got)
	}
}

func TestThemeFg_TrueColor(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.TrueColor

	got := ThemeFg("#FF6B6B")
	if _, ok := got.(lipgloss.ANSIColor); ok {
		t.Error("ThemeFg should return hex color in TrueColor mode, got ANSIColor")
	}
}

func TestThemeFg_ANSI256(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.ANSI256

	got := ThemeFg("#FF6B6B")
	if _, ok := got.(lipgloss.ANSIColor); ok {
		t.Error("ThemeFg should return hex color in ANSI256 mode, got ANSIColor")
	}
}

func TestThemeFg_ANSI(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.ANSI

	got := ThemeFg("#FF6B6B")
	ansiColor, ok := got.(lipgloss.ANSIColor)
	if !ok {
		t.Errorf("ThemeFg should return ANSIColor in ANSI mode, got %T", got)
	} else if ansiColor != 7 {
		t.Errorf("ThemeFg should return ANSI white (7) in ANSI mode, got %d", ansiColor)
	}
}

func TestThemeFg_NoTTY(t *testing.T) {
	saved := TermProfile
	defer func() { TermProfile = saved }()

	TermProfile = colorprofile.NoTTY

	got := ThemeFg("#FF6B6B")
	if _, ok := got.(lipgloss.ANSIColor); !ok {
		t.Errorf("ThemeFg should return ANSIColor in NoTTY mode, got %T", got)
	}
}

// -- Live theme swap (bt-1n0b1) --

// restoreThemeGlobals puts the mutable theme globals back after a test that
// repaints them. Color*, PanelStyle/FocusedPanelStyle and isDarkBackground are
// package-level and shared with every other test in this package, so leaving a
// foreign palette applied would silently repaint later golden tests -- the
// mutable-global coupling bt-zq6z identified.
//
// The restore reproduces NewModel's own startup sequence rather than snapshotting
// all ~60 vars, which lands on the same well-defined state NewModel leaves.
func restoreThemeGlobals(t *testing.T) {
	t.Helper()
	savedDark := isDarkBackground
	t.Cleanup(func() {
		isDarkBackground = savedDark
		resolveColors()
		ApplyThemeToGlobals(LoadTheme())
	})
}

// TestThemeSwapMidSession_NoRace repaints the entire palette from inside Update,
// mid-session, while the real Bubble Tea event loop is running and async cmds
// are in flight. It is the empirical half of bt-1n0b1.
//
// Why this is the right shape: Bubble Tea calls Update and View sequentially on
// one goroutine (vendor/charm.land/bubbletea/v2/tea.go:853 and :869 are
// consecutive statements in eventLoop), so a swap performed in Update cannot
// race the renderer by construction. What CAN race it is a tea.Cmd, because
// every cmd runs on its own goroutine (tea.go:702-714), or a worker goroutine.
// Those are what this test puts under the race detector.
//
// The swap path exercised here (tea.BackgroundColorMsg, model.go:1892) is the
// one already shipping, and it is the same sequence a theme picker keypress
// would run: resolveColors -> DefaultTheme -> LoadTheme -> ApplyThemeToGlobals
// -> ApplyThemeToThemeStruct.
//
// Run with: go test ./pkg/ui -race -run TestThemeSwapMidSession
func TestThemeSwapMidSession_NoRace(t *testing.T) {
	restoreThemeGlobals(t)

	// Two vendored btop palettes. They must resolve to different primaries,
	// otherwise "the swap took effect" would not be observable and this test
	// would pass vacuously.
	const themeA, themeB = "dracula", "ayu"

	isDarkBackground = true
	primaryOf := func(name string) color.Color {
		t.Setenv("BT_THEME", name)
		ApplyThemeToGlobals(LoadTheme())
		return ColorPrimary
	}
	wantA, wantB := primaryOf(themeA), primaryOf(themeB)
	if wantA == wantB {
		t.Fatalf("fixture palettes %q and %q both resolve primary to %v; pick two that differ",
			themeA, themeB, wantA)
	}

	t.Setenv("BT_THEME", themeA)
	tm := teatest.NewTestModel(t, NewModel(claimTestIssues(), nil, "", nil, nil),
		teatest.WithInitialTermSize(120, 32))

	// Swap only once the session is genuinely running, so the repaints land
	// among live cmds rather than during startup.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("zz-"))
	}, teatest.WithDuration(8*time.Second))

	// Storm phase: alternate palettes while driving input, so View runs and new
	// cmds spawn between repaints. BT_THEME is read by LoadTheme on each swap,
	// so every iteration rewrites all ~60 Color* vars to a different palette.
	// No value assertions here -- tm.Send is async, so which palette is live at
	// any instant is deliberately nondeterministic. The race detector is the
	// assertion.
	for i := range 10 {
		name := themeA
		if i%2 == 1 {
			name = themeB
		}
		t.Setenv("BT_THEME", name)
		// Exercise a retained view between repaints, not just the main list.
		tm.Send(tea.KeyPressMsg{Code: 'a', Text: "a"})
		tm.Send(tea.BackgroundColorMsg{Color: lipgloss.Color("#1d1f21")})
		tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
		tm.Send(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
		tm.Send(tea.KeyPressMsg{Code: tea.KeyUp})
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	}

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(15*time.Second))

	// Settle phase, on the test goroutine once the loop is done, so the
	// assertions are deterministic. Reading the globals here is itself part of
	// the check: a leaked cmd goroutine still touching them would race this.
	t.Setenv("BT_THEME", themeB)
	isDarkBackground = true
	resolveColors()
	ApplyThemeToGlobals(LoadTheme())

	if ColorPrimary != wantB {
		t.Errorf("after swap to %q, ColorPrimary = %v, want %v", themeB, ColorPrimary, wantB)
	}
	// The package-level styles derived from the tokens must be rebuilt by the
	// swap, not snapshotted at init -- a stale style here is a correctness bug
	// distinct from any race.
	if got := FocusedPanelStyle.GetBorderTopForeground(); got != ColorPrimary {
		t.Errorf("FocusedPanelStyle border = %v, want ColorPrimary %v (style not rebuilt on swap)", got, ColorPrimary)
	}
	if got := PanelStyle.GetBorderTopForeground(); got != ColorBgHighlight {
		t.Errorf("PanelStyle border = %v, want ColorBgHighlight %v (style not rebuilt on swap)", got, ColorBgHighlight)
	}
}
