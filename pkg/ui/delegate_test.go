package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Build a minimal issue item used across delegate tests.
func newTestIssueItem(id string) IssueItem {
	now := time.Now().Add(-2 * time.Hour) // deterministic-ish age string (e.g. "2h")
	return IssueItem{
		Issue: model.Issue{
			ID:        id,
			Title:     "Short title for testing",
			Status:    model.StatusOpen,
			IssueType: model.TypeFeature,
			Priority:  1,
			Assignee:  "alice",
			Labels:    []string{"one", "two"},
			Comments: []*model.Comment{
				{ID: "1", IssueID: id, Author: "bob", Text: "hello", CreatedAt: now},
			},
			CreatedAt: now,
		},
		DiffStatus: DiffStatusNone,
		RepoPrefix: "",
	}
}

func TestIssueDelegate_RenderWorkspaceWithPriorityHints(t *testing.T) {
	item := newTestIssueItem("api-123")
	item.RepoPrefix = "api"         // exercise workspace badge branch
	item.DiffStatus = DiffStatusNew // exercise diff badge branch
	theme := DefaultTheme()

	delegate := IssueDelegate{
		Theme:             theme,
		ShowPriorityHints: true,
		PriorityHints: map[string]*analysis.PriorityRecommendation{
			item.Issue.ID: {IssueID: item.Issue.ID, Direction: "increase"},
		},
		ShowRepoBadges: true,
	}

	items := []list.Item{item}
	l := list.New(items, delegate, 0, 0)
	l.SetWidth(120) // wide enough to render right-side columns

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "api-123") {
		t.Fatalf("render output should omit redundant repo prefix: %q", out)
	}
	if !strings.Contains(out, "123") {
		t.Fatalf("render output missing compact issue id: %q", out)
	}
	if !strings.Contains(out, "↑") {
		t.Fatalf("render output missing priority hint arrow: %q", out)
	}
	if !strings.Contains(out, "[API]") {
		t.Fatalf("render output missing repo badge [API]: %q", out)
	}
	if !strings.Contains(out, activeGlyphs.New) {
		t.Fatalf("render output missing diff badge for new item: %q", out)
	}
	if !strings.Contains(out, activeGlyphs.Comment+"1") {
		t.Fatalf("render output missing comment count badge: %q", out)
	}
}

func TestIssueDelegate_RenderSingleProjectUsesCompactIDWithoutBadge(t *testing.T) {
	item := newTestIssueItem("portfolio-hhg1r.1")
	item.RepoPrefix = "portfolio"
	delegate := IssueDelegate{Theme: DefaultTheme()}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(80)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "portfolio-hhg1r.1") {
		t.Fatalf("render output should omit project prefix: %q", out)
	}
	if !strings.Contains(out, "hhg1r.1") {
		t.Fatalf("render output missing compact issue id: %q", out)
	}
	if strings.Contains(out, "[PORT]") {
		t.Fatalf("single-project output should not contain repo badge: %q", out)
	}
	if got := issueIDForClipboard(item); got != "portfolio-hhg1r.1" {
		t.Fatalf("clipboard ID = %q, want full canonical ID", got)
	}
}

// TestIssueDelegate_RenderAliasesAtlasNamespaceBadge guards bt-z1pzj: the
// beads_global namespace's bare ID-prefix "global" (RepoPrefix is always
// ID-derived, see ExtractRepoPrefix) must render as the "atlas" display
// alias in the workspace-mode repo badge, not the raw "beads_global" or
// "global" spelling.
func TestIssueDelegate_RenderAliasesAtlasNamespaceBadge(t *testing.T) {
	item := newTestIssueItem("global-42")
	item.RepoPrefix = "global"
	delegate := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: true}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(120)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "[GLOB") {
		t.Fatalf("render output should not show raw beads_global badge: %q", out)
	}
	if !strings.Contains(out, "[ATLA]") {
		t.Fatalf("render output missing aliased atlas badge [ATLA]: %q", out)
	}
}

// TestIssueDelegate_RenderDerivedGlobalBadge covers bt-l76b8: when the global
// prefix is derived (here "foo-"), the workspace badge for a bare-"global"
// RepoPrefix row renders the derived label, not the hardcoded "atlas".
func TestIssueDelegate_RenderDerivedGlobalBadge(t *testing.T) {
	model.SetGlobalDisplayName("foo")
	defer model.SetGlobalDisplayName("")

	item := newTestIssueItem("global-42")
	item.RepoPrefix = "global"
	delegate := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: true}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(120)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if !strings.Contains(out, "[FOO]") {
		t.Fatalf("render output missing derived badge [FOO]: %q", out)
	}
	if strings.Contains(out, "[ATLA]") {
		t.Fatalf("render output should not show hardcoded atlas badge: %q", out)
	}
}

func TestIssueDelegate_CompactIDWidthFlowsToTitle(t *testing.T) {
	item := newTestIssueItem("portfolio-hhg1r.1")
	item.Issue.Title = strings.Repeat("Z", 100)
	d := IssueDelegate{Theme: DefaultTheme()}
	full := ansi.Strip(renderDelegateRow(t, d, item, 50))
	item.RepoPrefix = "portfolio"
	compact := ansi.Strip(renderDelegateRow(t, d, item, 50))
	if gain := strings.Count(compact, "Z") - strings.Count(full, "Z"); gain != 10 {
		t.Fatalf("removing portfolio- should return 10 cells to title, got %d:\nfull: %q\ncompact: %q", gain, full, compact)
	}
}

func TestIssueListColumnHeaderUsesCompactIDCell(t *testing.T) {
	for _, workspaceMode := range []bool{false, true} {
		header := issueListColumnHeader(issueListLeftLayout(list.New(nil, IssueDelegate{}, 80, 10), workspaceMode))
		if !strings.Contains(header, "ID TITLE") {
			t.Fatalf("workspaceMode=%t header does not use compact ID cell: %q", workspaceMode, header)
		}
		if workspaceMode != strings.Contains(header, "REPO") {
			t.Fatalf("workspaceMode=%t header repo column mismatch: %q", workspaceMode, header)
		}
	}
}

func TestIssueListHeaderIsQuietAndSingleLine(t *testing.T) {
	for _, workspace := range []bool{false, true} {
		for _, width := range []int{1, 12, 30, 80} {
			m := Model{
				theme: DefaultTheme(),
				list:  list.New(nil, IssueDelegate{}, width, 10),
			}
			m.workspaceMode = workspace
			header := m.splitViewHeader()
			lines := uv.NewStyledString(header).Lines(ansi.GraphemeWidth)
			if len(lines) != 1 || len(lines[0]) != width {
				t.Fatalf("workspace=%t width=%d: header must occupy one full row: %q", workspace, width, header)
			}
			for x, cell := range lines[0] {
				if cell.Style.Bg != nil || cell.Style.Attrs != 0 {
					t.Fatalf("workspace=%t width=%d cell=%d: header must have no fill or bold: %+v", workspace, width, x, cell.Style)
				}
				if cell.Content != " " && !cell.Style.Equal(&uv.Style{Fg: m.theme.Subtext}) {
					t.Fatalf("workspace=%t width=%d cell=%d: column label must use subdued text: %+v", workspace, width, x, cell.Style)
				}
			}
			wantPrefix := "T"
			if workspace {
				wantPrefix = "R"
			}
			if plain := ansi.Strip(header); !strings.HasPrefix(plain, wantPrefix) {
				t.Errorf("workspace=%t width=%d: missing column label: %q", workspace, width, plain)
			}
		}
	}
}

func TestIssueDelegate_NoSelectionGutter(t *testing.T) {
	setGlyphs(t, asciiGlyphs)
	item := newTestIssueItem("api-123")
	item.RepoPrefix = "api"
	for _, workspace := range []bool{false, true} {
		d := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: workspace}
		l := list.New([]list.Item{item, item}, d, 80, 2)
		for _, index := range []int{0, 1} {
			var buf bytes.Buffer
			d.Render(&buf, l, index, item)
			wantPrefix := "* o 1 "
			if workspace {
				wantPrefix = "[API] " + wantPrefix
			}
			if got := ansi.Strip(buf.String()); !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("workspace=%t index=%d row has a selection gutter: %q", workspace, index, got)
			}
		}
		if header := issueListColumnHeader(issueListLeftLayout(l, workspace)); strings.HasPrefix(header, " ") {
			t.Errorf("workspace=%t header still reserves a gutter: %q", workspace, header)
		}
	}
}

func TestIssueDelegate_SelectedRowHasUniformHighlight(t *testing.T) {
	item := newTestIssueItem("api-123")
	item.IsQuickWin = true
	item.RepoPrefix = "api"
	d := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: true, Slots: waitRegistry()}
	for _, width := range []int{12, 50, 80, 120, 160} {
		row := renderDelegateRow(t, d, item, width)
		lines := uv.NewStyledString(row).Lines(ansi.GraphemeWidth)
		if len(lines) != 1 || len(lines[0]) != width {
			t.Fatalf("width=%d: selected row must fill exactly one row, got %q", width, row)
		}
		for x, cell := range lines[0] {
			if cell.Style.Bg == nil || cell.Style.Fg == nil {
				t.Fatalf("width=%d cell=%d: missing selection colors", width, x)
			}
			if !cell.Style.Equal(&uv.Style{Bg: d.Theme.Highlight, Fg: ColorText}) {
				t.Fatalf("width=%d cell=%d: non-uniform selection style: %+v", width, x, cell.Style)
			}
		}
	}
}

func TestIssueDelegate_SelectedRowUsesCustomThemeText(t *testing.T) {
	restoreThemeGlobals(t)
	tf := &ThemeFile{Colors: ThemeColors{
		Text:      &AdaptiveHex{Dark: "#eeeeee", Light: "#222222"},
		Highlight: &AdaptiveHex{Dark: "#333333", Light: "#dddddd"},
	}}
	for _, dark := range []bool{false, true} {
		isDarkBackground = dark
		d := IssueDelegate{Theme: DefaultTheme()}
		ApplyThemeToGlobals(tf)
		ApplyThemeToThemeStruct(&d.Theme, tf)
		want := uv.Style{Fg: lipgloss.Color("#222222"), Bg: lipgloss.Color("#dddddd")}
		if dark {
			want = uv.Style{Fg: lipgloss.Color("#eeeeee"), Bg: lipgloss.Color("#333333")}
		}
		row := renderDelegateRow(t, d, newTestIssueItem("api-123"), 80)
		for x, cell := range uv.NewStyledString(row).Lines(ansi.GraphemeWidth)[0] {
			if !cell.Style.Equal(&want) {
				t.Fatalf("dark=%t cell=%d: selected row ignores custom text/highlight colors: %+v", dark, x, cell.Style)
			}
		}
	}
}

func TestIssueDelegate_QuickWinBoltStaysAtRightEdge(t *testing.T) {
	wideGlyphs := nerdfontGlyphs
	wideGlyphs.Bolt = "⚡"
	for _, tier := range []struct {
		name   string
		glyphs GlyphSet
	}{
		{"nerdfont", nerdfontGlyphs},
		{"ascii", asciiGlyphs},
		{"wide-bolt", wideGlyphs},
	} {
		t.Run(tier.name, func(t *testing.T) {
			setGlyphs(t, tier.glyphs)
			item := newTestIssueItem("api-0hx")
			item.RepoPrefix = "api"
			item.Issue.Title = strings.Repeat("long title words ", 10)
			item.IsQuickWin = true
			item.IsBlocker = true
			item.UnblocksCount = 3
			d := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: true, Slots: waitRegistry()}
			for _, width := range []int{1, 2, 12, 24, 50, 60, 61, 80, 81, 100, 101, 120, 121, 140, 141, 160} {
				for _, selected := range []bool{false, true} {
					l := list.New([]list.Item{item, item}, d, width, 2)
					index := 1
					if selected {
						index = 0
					}
					var buf bytes.Buffer
					d.Render(&buf, l, index, item)
					row := buf.String()
					lines := uv.NewStyledString(row).Lines(ansi.GraphemeWidth)
					rowWidth := 0
					if len(lines) == 1 {
						for _, cell := range lines[0] {
							rowWidth += cell.Width
						}
					}
					if len(lines) != 1 || rowWidth != width {
						t.Errorf("width=%d selected=%t: row must occupy exactly one line: %q", width, selected, row)
					}
					plain := ansi.Strip(row)
					if width < lipgloss.Width(tier.glyphs.Bolt) {
						if plain != strings.Repeat(" ", width) {
							t.Errorf("width=%d: oversized bolt must leave a blank cell: %q", width, plain)
						}
						continue
					}
					if !strings.HasSuffix(plain, tier.glyphs.Bolt) || strings.Count(plain, tier.glyphs.Bolt) != 1 {
						t.Errorf("width=%d selected=%t: quick-win bolt must appear once at the right edge: %q", width, selected, plain)
					}
					if !selected && len(lines) == 1 && rowWidth == width {
						cell := lines[0][len(lines[0])-1]
						if !cell.Style.Equal(&uv.Style{Fg: ThemeFg("#f0c674")}) {
							t.Errorf("width=%d: quick-win bolt must retain its gold style: %+v", width, cell.Style)
						}
					}
				}
			}
		})
	}
}

func TestIssueDelegate_QuickWinDoesNotShiftColumns(t *testing.T) {
	for _, glyphs := range []GlyphSet{nerdfontGlyphs, asciiGlyphs} {
		setGlyphs(t, glyphs)
		item := newTestIssueItem("api-0hx")
		item.RepoPrefix = "api"
		item.Issue.Title = "TITLE " + strings.Repeat("long words ", 20)
		d := IssueDelegate{Theme: DefaultTheme(), ShowRepoBadges: true}
		for _, width := range []int{30, 50, 80, 120, 160} {
			item.IsQuickWin = false
			ordinary := ansi.Strip(renderDelegateRow(t, d, item, width))
			item.IsQuickWin = true
			quickWin := ansi.Strip(renderDelegateRow(t, d, item, width))
			cellWidth := lipgloss.Width(glyphs.Bolt) + 1
			bodyWidth := width - cellWidth
			if ansi.Truncate(ordinary, bodyWidth, "") != ansi.Truncate(quickWin, bodyWidth, "") {
				t.Errorf("width=%d: quick-win flag shifted columns or changed title truncation:\nordinary: %q\nquickwin: %q", width, ordinary, quickWin)
			}
			if !strings.HasSuffix(ordinary, strings.Repeat(" ", cellWidth)) {
				t.Errorf("width=%d: ordinary row must reserve a blank marker cell: %q", width, ordinary)
			}
		}
	}
}

func TestIssueDelegate_PendingClaimKeepsFeedbackOnRight(t *testing.T) {
	setGlyphs(t, asciiGlyphs)
	item := newTestIssueItem("api-123")
	item.RepoPrefix = "api"
	d := IssueDelegate{Theme: DefaultTheme(), PendingClaims: map[string]bool{item.Issue.ID: true}, ClaimSpinner: "|"}
	row := ansi.Strip(renderDelegateRow(t, d, item, 50))
	if !strings.HasPrefix(row, "* o 1 123 Short title for testing") || strings.Index(row, "|") < strings.Index(row, "testing") {
		t.Fatalf("pending feedback should follow the title, not shift it: %q", row)
	}
	d.ClaimSpinner = ""
	if row := renderDelegateRow(t, d, item, 50); !strings.Contains(row, claimSpinnerFrame(0)) {
		t.Fatalf("pending row is missing its fallback spinner: %q", row)
	}
}

func TestIssueDelegate_RenderFallsBackWidthAndNoPanic(t *testing.T) {
	item := newTestIssueItem("TASK-1")
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0) // width defaults to 0 → delegate fallback

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if out == "" {
		t.Fatal("render output should not be empty")
	}
	if !strings.Contains(out, "TASK-1") {
		t.Fatalf("render output missing id after fallback width handling: %q", out)
	}
}

func TestIssueDelegate_RenderUltraWide(t *testing.T) {
	item := newTestIssueItem("WIDE-1")
	// Assignee and Labels require width thresholds >100 and >140
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(160) // Ultra-wide

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if !strings.Contains(out, "@alice") {
		t.Fatalf("ultra-wide output missing assignee @alice: %q", out)
	}
	if !strings.Contains(out, "one,two") { // joined labels
		t.Fatalf("ultra-wide output missing labels 'one,two': %q", out)
	}
}

// Author column renders at width > 120 when Author differs from Assignee.
// Prefix (✎) + 10-char left-padded author ID. bt-aw4h.
func TestIssueDelegate_RenderShowsAuthor(t *testing.T) {
	item := newTestIssueItem("AUTH-1")
	item.Issue.Author = "bt-7d42e" // shorthand session ID
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(140) // > 120 threshold

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if !strings.Contains(out, "bt-7d42e") {
		t.Fatalf("width=140 output should include author 'bt-7d42e': %q", out)
	}
	if !strings.Contains(out, activeGlyphs.Pencil) {
		t.Fatalf("width=140 output should include author prefix ✎: %q", out)
	}
}

// Author == Assignee case: column is suppressed to avoid duplication.
func TestIssueDelegate_RenderSuppressesAuthorWhenSameAsAssignee(t *testing.T) {
	item := newTestIssueItem("SAME-1")
	item.Issue.Author = "alice" // matches Assignee from newTestIssueItem
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(140)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "✎") {
		t.Fatalf("author==assignee should NOT render author column: %q", out)
	}
}

// Author column hidden below width threshold.
func TestIssueDelegate_RenderHidesAuthorAtNarrowWidth(t *testing.T) {
	item := newTestIssueItem("NARR-AUTH-1")
	item.Issue.Author = "bt-7d42e"
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(110) // between Assignee threshold (>100) and Author threshold (>120)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "bt-7d42e") {
		t.Fatalf("width=110 should hide author column: %q", out)
	}
}

// Row rendering must never emit a [0.NN] score badge. The hybrid score is
// now consumed only by the detail-pane Search Scores section (bt-gfxhz.6).
// bt-r3zxj decided row badges are noise for humans; agents continue to
// receive scores via bt robot search JSON.
func TestIssueDelegate_RenderOmitsSearchScoreBadge(t *testing.T) {
	item := newTestIssueItem("NOBADGE-1")
	item.SearchScoreSet = true
	item.SearchScore = 0.48 // well above the former 0.05 threshold
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(120)

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if strings.Contains(out, "[0.48]") {
		t.Fatalf("render output should not contain row score badge: %q", out)
	}
	if strings.Contains(out, "[0.") {
		t.Fatalf("render output contains a [0.NN]-shaped score badge: %q", out)
	}
}

func TestIssueDelegate_RenderNarrow(t *testing.T) {
	item := newTestIssueItem("NARROW-1")
	theme := DefaultTheme()
	delegate := IssueDelegate{Theme: theme}

	l := list.New([]list.Item{item}, delegate, 0, 0)
	l.SetWidth(50) // Very narrow

	var buf bytes.Buffer
	delegate.Render(&buf, l, 0, item)
	out := buf.String()

	if !strings.Contains(out, "NARROW-1") {
		t.Fatalf("narrow output missing id: %q", out)
	}
	// Should NOT contain right-side metadata
	if strings.Contains(out, "@alice") {
		t.Fatalf("narrow output should hide assignee: %q", out)
	}
	if strings.Contains(out, "💬") {
		t.Fatalf("narrow output should hide comments count: %q", out)
	}
}

func renderDelegateRow(t *testing.T, d IssueDelegate, item IssueItem, width int) string {
	t.Helper()
	l := list.New([]list.Item{item}, d, 0, 0)
	l.SetWidth(width)
	var buf bytes.Buffer
	d.Render(&buf, l, 0, item)
	return buf.String()
}

func waitRegistry() *slots.Registry {
	reg := slots.NewRegistry()
	reg.AddBadges(slots.BadgeFunc(func(*model.Issue) []slots.Badge {
		return []slots.Badge{{Text: "WAIT", Tone: slots.ToneWarn}}
	}))
	return reg
}

func TestIssueDelegate_RendersSlotBadges(t *testing.T) {
	item := newTestIssueItem("api-1")
	d := IssueDelegate{Theme: DefaultTheme(), Slots: waitRegistry()}

	if out := renderDelegateRow(t, d, item, 120); !strings.Contains(out, "WAIT") {
		t.Fatalf("width 120 row missing slot badge: %q", out)
	}
	if out := renderDelegateRow(t, d, item, 80); strings.Contains(out, "WAIT") {
		t.Fatalf("width 80 row should hide slot badges: %q", out)
	}
}

func TestIssueDelegate_SlotBadgesNeverWrapNarrowRows(t *testing.T) {
	item := newTestIssueItem("api-1")
	item.Issue.Title = strings.Repeat("long title words ", 10)
	d := IssueDelegate{Theme: DefaultTheme(), Slots: waitRegistry()}

	out := renderDelegateRow(t, d, item, 81)
	if strings.Contains(out, "\n") {
		t.Fatalf("row wrapped: %q", out)
	}
	if w := lipgloss.Width(out); w > 81 {
		t.Fatalf("row width %d exceeds 81", w)
	}
}

func TestIssueDelegate_OverdueBadgeViaRegistry(t *testing.T) {
	item := newTestIssueItem("api-1")
	past := time.Now().Add(-48 * time.Hour)
	item.Issue.DueDate = &past
	reg := slots.NewRegistry()
	registerBuiltinSlots(reg)
	d := IssueDelegate{Theme: DefaultTheme(), Slots: reg}

	if out := renderDelegateRow(t, d, item, 120); !strings.Contains(out, "DUE") {
		t.Fatalf("overdue row missing DUE badge: %q", out)
	}
}

// These cases catch per-item prefix widths and optional left-side indicators:
// neither IDs nor titles may move when a neighboring row has extra metadata.
func TestIssueDelegate_SharedColumnsMoveIndicatorsRight(t *testing.T) {
	setGlyphs(t, asciiGlyphs)
	plain := newTestIssueItem("api-x")
	plain.RepoPrefix = "api"
	plain.Issue.Title = "TITLE plain"
	plain.Issue.Comments = nil
	decorated := newTestIssueItem("api-long.123")
	decorated.RepoPrefix = "api"
	decorated.Issue.Title = "TITLE decorated"
	decorated.GateAwaitType = "human"
	decorated.UnblocksCount = 12
	decorated.IsBlocker = true
	decorated.Issue.IssueType = model.TypeEpic
	decorated.EpicDone, decorated.EpicTotal = 2, 10
	decorated.DiffStatus = DiffStatusNew
	d := IssueDelegate{
		Theme: DefaultTheme(), Slots: waitRegistry(), ShowPriorityHints: true,
		PriorityHints: map[string]*analysis.PriorityRecommendation{
			decorated.Issue.ID: {Direction: "increase"},
		},
		PendingClaims: map[string]bool{decorated.Issue.ID: true}, ClaimSpinner: "|",
	}
	l := list.New([]list.Item{plain, decorated}, d, 180, 10)
	l.Paginator.PerPage = 2
	var rows []string
	for index, item := range []IssueItem{plain, decorated} {
		var buf bytes.Buffer
		d.Render(&buf, l, index, item)
		rows = append(rows, ansi.Strip(buf.String()))
	}
	for index, row := range rows {
		if got := strings.Index(row, "TITLE"); got != 15 {
			t.Errorf("row %d title starts at %d, want 15 after shared 8-cell ID: %q", index, got, row)
		}
		if lipgloss.Width(row) != 180 {
			t.Errorf("row %d width = %d, want 180", index, lipgloss.Width(row))
		}
	}
	for _, indicator := range []string{"↑", "o12", "@", "WAIT", "2/10", "+", "|"} {
		if got := strings.Index(rows[1], indicator); got <= strings.Index(rows[1], "TITLE") {
			t.Errorf("indicator %q must follow title, got %q", indicator, rows[1])
		}
	}
}

func TestIssueDelegate_CommentsDoNotShiftAge(t *testing.T) {
	for _, glyphs := range []GlyphSet{asciiGlyphs, nerdfontGlyphs} {
		setGlyphs(t, glyphs)
		var items []list.Item
		for _, count := range []int{0, 1, 12, 123} {
			item := newTestIssueItem("api-abc")
			item.Issue.UpdatedAt = time.Now().Add(-2 * time.Hour)
			item.Issue.CreatedAt = item.Issue.UpdatedAt
			item.Issue.Comments = make([]*model.Comment, count)
			items = append(items, item)
		}
		d := IssueDelegate{Theme: DefaultTheme()}
		l := list.New(items, d, 90, 12)
		l.Paginator.PerPage = 4
		ageColumn := -1
		for index, item := range items {
			var buf bytes.Buffer
			d.Render(&buf, l, index, item)
			row := ansi.Strip(buf.String())
			at := strings.Index(row, "2h ago")
			if at < 0 {
				t.Fatalf("missing age: %q", row)
			}
			cell := lipgloss.Width(row[:at])
			if ageColumn < 0 {
				ageColumn = cell
			} else if cell != ageColumn {
				t.Errorf("comments=%d age starts at %d, want %d: %q", len(item.(IssueItem).Issue.Comments), cell, ageColumn, row)
			}
		}
	}
}

func TestIssueDelegate_EditedAgeHasNoTilde(t *testing.T) {
	item := newTestIssueItem("api-abc")
	item.Issue.UpdatedAt = time.Now().Add(-time.Hour)
	item.Issue.CreatedAt = item.Issue.UpdatedAt.Add(-time.Hour)
	d := IssueDelegate{Theme: DefaultTheme()}
	l := list.New([]list.Item{item, item}, d, 90, 10)
	var buf bytes.Buffer
	d.Render(&buf, l, 1, item) // nonselected row retains italic styling
	row := buf.String()
	if strings.Contains(ansi.Strip(row), "~") {
		t.Fatalf("edited age must not have tilde: %q", row)
	}
	if !strings.Contains(ansi.Strip(row), "1h ago") {
		t.Fatalf("missing edited age: %q", row)
	}
	italic := false
	for _, cell := range uv.NewStyledString(row).Lines(ansi.GraphemeWidth)[0] {
		if cell.Content == "h" && cell.Style.Attrs&uv.AttrItalic != 0 {
			italic = true
		}
	}
	if !italic {
		t.Fatalf("edited age lost italic styling: %q", row)
	}
}

func TestIssueListHeaderTracksDisplayedIDWidth(t *testing.T) {
	setGlyphs(t, asciiGlyphs)
	d := IssueDelegate{Theme: DefaultTheme()}
	var items []list.Item
	for _, id := range []string{"api-a", "api-12345678", "api-xy"} {
		item := newTestIssueItem(id)
		item.RepoPrefix = "api"
		items = append(items, item)
	}
	l := list.New(items, d, 80, 10)
	l.Paginator.PerPage = 2
	m := Model{theme: DefaultTheme(), list: l}
	if got := strings.Index(ansi.Strip(m.splitViewHeader()), "TITLE"); got != 15 {
		t.Errorf("first page header TITLE starts at %d, want 15", got)
	}
	m.list.Paginator.Page = 1
	if got := strings.Index(ansi.Strip(m.splitViewHeader()), "TITLE"); got != 9 {
		t.Errorf("second page header TITLE starts at %d, want 9", got)
	}
}

func TestIssueDelegate_StyledMetadataUsesDisplayCellPadding(t *testing.T) {
	setGlyphs(t, nerdfontGlyphs)
	var items []list.Item
	for index, assignee := range []string{"李", "alice", ""} {
		item := newTestIssueItem("api-abc")
		item.RepoPrefix = "api"
		item.Issue.Title = "TITLE " + strings.Repeat("words ", 40)
		item.Issue.UpdatedAt = time.Now().Add(-2 * time.Hour)
		item.Issue.CreatedAt = item.Issue.UpdatedAt
		item.Issue.Assignee = assignee
		item.Issue.Author = "王"
		item.Issue.Labels = []string{"LABEL"}
		item.UnblocksCount = []int{1, 123, 0}[index]
		item.IsBlocker = true
		item.Issue.Comments = make([]*model.Comment, []int{1, 123, 0}[index])
		items = append(items, item)
	}
	d := IssueDelegate{Theme: DefaultTheme()}
	l := list.New(items, d, 160, 12)
	l.Paginator.PerPage = 3
	anchors := map[string]int{}
	for index, item := range items {
		var buf bytes.Buffer
		d.Render(&buf, l, index, item)
		row := ansi.Strip(buf.String())
		for _, token := range []string{"TITLE", "2h ago", "王", "LABEL"} {
			at := strings.Index(row, token)
			if at < 0 {
				t.Fatalf("row %d missing %q: %q", index, token, row)
			}
			cell := lipgloss.Width(row[:at])
			if index == 0 {
				anchors[token] = cell
			} else if cell != anchors[token] {
				t.Errorf("row %d %q starts at cell %d, want %d: %q", index, token, cell, anchors[token], row)
			}
		}
	}
}

func TestIssuesPanelMeasuresBadgeColumnsOncePerPage(t *testing.T) {
	var issues []model.Issue
	for _, id := range []string{"api-a", "api-b", "api-c", "api-d", "api-e", "api-f", "api-g", "api-h"} {
		item := newTestIssueItem(id)
		issues = append(issues, item.Issue)
	}
	m := NewModel(issues, nil, "", nil, nil)
	m.list.SetSize(100, 20)
	m.list.Paginator.PerPage = 8
	reg := slots.NewRegistry()
	calls := 0
	reg.AddBadges(slots.BadgeFunc(func(*model.Issue) []slots.Badge {
		calls++
		return []slots.Badge{{Text: "WAIT"}}
	}))
	m.slotRegistry = reg
	m.updateListDelegate()
	out := ansi.Strip(m.renderIssuesPanel(104, 24, true))
	if !strings.Contains(out, "WAIT") {
		t.Fatalf("panel did not render slot badges: %q", out)
	}
	// At most two measurement passes plus one rendering pass over this page.
	if calls > 3*len(issues) {
		t.Fatalf("badge providers called %d times for %d rows; layout must be measured once per page", calls, len(issues))
	}
}
