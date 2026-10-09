package ui

import (
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestEpicProgressTextRoleCells(t *testing.T) {
	old := ActiveTextStyles
	t.Cleanup(func() { ActiveTextStyles = old })
	ActiveTextStyles.Metadata = lipgloss.NewStyle().Foreground(lipgloss.Color("#654321")).Background(lipgloss.Color("#456789")).Bold(false).Italic(true).Underline(false)
	ActiveTextStyles.Body = lipgloss.NewStyle().Foreground(lipgloss.Color("#123456")).Background(lipgloss.Color("#345678")).Bold(false).Underline(true)
	ActiveTextStyles.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color("#abcdef")).Background(lipgloss.Color("#234567")).Bold(false).Italic(false).Underline(true)
	all := epicProgressFixture()
	assertSpan := func(row, text string, style lipgloss.Style) {
		t.Helper()
		plain := ansi.Strip(row)
		idx := strings.Index(plain, text)
		if idx < 0 {
			t.Fatalf("missing span %q in %q", text, plain)
		}
		start := ansi.StringWidth(plain[:idx])
		cells := uv.NewStyledString(row).Lines(ansi.GraphemeWidth)[0]
		want := uv.NewStyledString(style.Render(text)).Lines(ansi.GraphemeWidth)[0]
		for i, expected := range want {
			if !cells[start+i].Style.Equal(&expected.Style) {
				t.Errorf("span %q cell %d: style=%+v want=%+v", text, i, cells[start+i].Style, expected.Style)
				break
			}
		}
	}
	{
		rows := strings.Split(buildEpicProgressANSI(all[0], all, 80), "\n")
		assertSpan(rows[0], "1 / 3 children complete (33%)", ActiveTextStyles.Metadata)
		for i, child := range all[1:] {
			row := rows[i+2]
			idStyle, titleStyle := ActiveTextStyles.Metadata, ActiveTextStyles.Body
			if child.Status.IsClosed() {
				idStyle, titleStyle = idStyle.Faint(true), titleStyle.Faint(true)
			}
			if strings.Contains(row, "▸") {
				t.Errorf("detail embed row has a cursor: %q", ansi.Strip(row))
			}
			assertSpan(row, child.ID, idStyle)
			assertSpan(row, child.Title, titleStyle)
			assertSpan(row, " — ", titleStyle)
			// The pills must keep their complete domain-semantic styles.
			statusPill, prioPill := RenderStatusBadge(string(child.Status)), RenderPriorityBadge(child.Priority)
			for _, pill := range []string{statusPill, prioPill} {
				plain := ansi.Strip(row)
				start := ansi.StringWidth(plain[:strings.Index(plain, ansi.Strip(pill))])
				cells := uv.NewStyledString(row).Lines(ansi.GraphemeWidth)[0]
				for j, want := range uv.NewStyledString(pill).Lines(ansi.GraphemeWidth)[0] {
					if !cells[start+j].Style.Equal(&want.Style) {
						t.Errorf("pill %q cell %d lost semantic style", ansi.Strip(pill), j)
					}
				}
			}
		}
	}
	for _, width := range []int{30, 40, 80} {
		out := buildEpicProgressANSI(all[0], all, width)
		for _, row := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(row); w > width {
				t.Errorf("width %d: row exceeds budget: %d", width, w)
			}
		}
	}
	childless := model.Issue{ID: "empty", IssueType: model.TypeEpic}
	if got := buildEpicProgressANSI(childless, []model.Issue{childless}, 30); got != "" {
		t.Fatalf("childless epic rendered %q", got)
	}
}

func TestEpicProgress_OrdinaryViewBaseline(t *testing.T) {
	all := epicProgressFixture()
	want := "1 / 3 children complete (33%)\n                             \n  DONE P1 ep.1 — first child \n  PROG P0 ep.2 — second child\n  OPEN P2 ep.10 — tenth child"
	if got := ansi.Strip(buildEpicProgressANSI(all[0], all, 80)); got != want {
		t.Fatalf("ordinary epic renderer changed:\ngot %q\nwant %q", got, want)
	}
}

// pcDep builds a parent_child dependency edge (child depends-on parent).
func pcDep(child, parent string) []*model.Dependency {
	return []*model.Dependency{{IssueID: child, DependsOnID: parent, Type: model.DepParentChild}}
}

func epicProgressFixture() []model.Issue {
	return []model.Issue{
		{ID: "ep", IssueType: model.TypeEpic, Status: model.StatusOpen, Title: "The Epic"},
		{ID: "ep.1", Title: "first child", Status: model.StatusClosed, Priority: 1, Dependencies: pcDep("ep.1", "ep")},
		{ID: "ep.2", Title: "second child", Status: model.StatusInProgress, Priority: 0, Dependencies: pcDep("ep.2", "ep")},
		{ID: "ep.10", Title: "tenth child", Status: model.StatusOpen, Priority: 2, Dependencies: pcDep("ep.10", "ep")},
	}
}

func TestBuildEpicProgressANSI(t *testing.T) {
	all := epicProgressFixture()
	epic := all[0]

	out := buildEpicProgressANSI(epic, all, 0)

	// It must be lipgloss (ANSI SGR), not markdown — the whole point of
	// bt-gfxhz.3 / the renderSection ANSI track (bt-x5xc4).
	if !strings.Contains(out, "\x1b") {
		t.Errorf("output has no ESC byte; expected lipgloss-styled ANSI, got %q", out)
	}
	// Parity: the old per-status markdown styling is gone.
	if strings.Contains(out, "~~") {
		t.Errorf("output contains markdown strikethrough literal ~~: %q", out)
	}
	if strings.Contains(out, "**") {
		t.Errorf("output contains markdown bold literal **: %q", out)
	}

	// Summary: 1 of 3 closed = 33%.
	if !strings.Contains(out, "1 / 3 children complete (33%)") {
		t.Errorf("summary line missing/incorrect: %q", out)
	}

	// Natural-numeric order: .1 before .2 before .10 (not Dolt load order).
	plain := ansi.Strip(out)
	i1 := strings.Index(plain, "ep.1 ")
	i2 := strings.Index(plain, "ep.2")
	i10 := strings.Index(plain, "ep.10")
	if !(i1 >= 0 && i2 > i1 && i10 > i2) {
		t.Errorf("children not in natural order (.1<.2<.10): idx ep.1=%d ep.2=%d ep.10=%d", i1, i2, i10)
	}

	// Childless epic -> empty so callers skip the section + heading.
	childless := model.Issue{ID: "lonely", IssueType: model.TypeEpic}
	if got := buildEpicProgressANSI(childless, []model.Issue{childless}, 0); got != "" {
		t.Errorf("childless epic should render empty, got %q", got)
	}
}

func TestBuildEpicProgressANSI_TitleTruncation(t *testing.T) {
	all := []model.Issue{
		{ID: "ep", IssueType: model.TypeEpic, Status: model.StatusOpen},
		{ID: "ep.1", Title: "this is a very long child title that should be truncated to fit", Status: model.StatusOpen, Priority: 2, Dependencies: pcDep("ep.1", "ep")},
	}
	// Narrow width forces truncation; the ellipsis proves the budget applied.
	out := buildEpicProgressANSI(all[0], all, 40)
	if !strings.Contains(out, "…") {
		t.Errorf("expected ellipsis from title truncation at width 40, got %q", out)
	}
}
