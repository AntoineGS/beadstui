# Shared Popup Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans for native execution, or superpowers:subagent-driven-development if the owner selects task-by-task delegation. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every popup a reusable frame and every popup menu stable, aligned selection/shortcut/marker/label columns.

**Architecture:** Add stateless popup layout/frame and menu-row helpers to `pkg/ui/panel.go`, above the existing titled-panel renderer. Migrate callers without moving interaction state or commands into the helpers. Keep the current modal dispatch and backdrop compositor.

**Tech Stack:** Go 1.25.8+, Charm Bracelet v2, `github.com/charmbracelet/x/ansi`, existing Go tests.

**Spec:** `docs/design/popup-rendering.md` (approved by the owner).

## Global Constraints

- "Keep `RenderTitledPanel` and the existing compositor underneath them."
- "Without compatibility wrappers or parallel rendering paths."
- "Two cells of horizontal padding and one blank row above and below content when the available size permits."
- "The footer uses the theme's secondary color, italic styling, and centered placement."
- "No body mode centers individual menu rows."
- "Unknown dimensions use a shared 80-column, 24-row default terminal budget."
- "A known zero-sized available budget produces no frame content."
- "All widths are terminal cells measured with ANSI-aware helpers, not byte lengths."
- "All shortcuts and navigation keys, including uppercase accelerators" remain unchanged.
- "Ordinary board cards, panes, robot output, and non-modal overlays do not change."
- "Do not change the user's registry, repair Dolt, or alter theme loading to make a popup-rendering change appear green."
- Respect `AGENTS.md`: no file deletion, destructive git, script-based code changes, new dependency, install, or push. Edit existing production/test files. This plan and the approved design are the only new documentation artifacts for this refactor.
- Keep new row badges/detail sections/BQL fields in `pkg/ui/slots` if any are requested separately. This refactor adds none: reuse existing indicators and specialized content.

## Review Focus

1. **Marker changes across pages:** current/multi-select markers must not move the label column; measure the entire filtered collection before slicing. Tasks 2 and 5 test this.
2. **Styled/wide text at a narrow edge:** glyphs and ANSI resets must not displace the frame or swallow side padding. Tasks 1, 2, and 5 test this.
3. **Resize during editing:** typed input, selection, dirty drafts, and back navigation must survive; the footer must remain visible. Tasks 3 and 8 test this.
4. **Mouse coordinates after shared padding:** a click must activate the visually clicked row, not its neighbor or chrome. Tasks 5 and 6 test this from rendered coordinates.
5. **Specialized bodies and safety cues:** status pills, nested previews, warning accents, and update actions must remain meaningful. Tasks 6 and 7 test these; ordinary detail-pane epic progress remains unchanged.

---

## File Responsibilities and Execution Rules

| Files | Responsibility |
|---|---|
| `pkg/ui/panel.go`, `pkg/ui/panel_test.go` | Stateless popup budget, frame, menu columns, and reusable test assertions |
| `pkg/ui/field_edit.go`, `pkg/ui/field_edit_test.go`, `pkg/ui/longform_edit.go`, `pkg/ui/longform_edit_test.go` | Edit hub/pickers and editors; retain writes and guards |
| `pkg/ui/settings_menu.go`, `pkg/ui/settings_modal_test.go`, `pkg/ui/recipe_picker.go`, `pkg/ui/recipe_picker_test.go` | Launcher and recipe entries with descriptions |
| `pkg/ui/repo_picker.go`, `pkg/ui/repo_picker_test.go`, `pkg/ui/label_picker.go`, `pkg/ui/label_picker_test.go` | Filtered menu projection, paging, search, and mouse geometry |
| `pkg/ui/model_view.go`, `pkg/ui/context_help_test.go`, `pkg/ui/claim.go`, `pkg/ui/claim_test.go` | Help, time-travel, quit, and claim frames |
| `pkg/ui/settings_modal.go`, `pkg/ui/model_alerts.go`, `pkg/ui/model_alerts_header.go`, their existing tests | Specialized multi-column/report bodies and their layout budgets |
| Agent/session/update modal files and existing tests; `pkg/ui/epic_card.go`, `pkg/ui/epic_card_test.go` | Rich bodies inside the shared frame; selectable headers reuse menu rows |
| `pkg/ui/bql_modal.go`, `pkg/ui/model_update_input.go`, `pkg/ui/model_update_analysis.go`, `pkg/ui/model_export.go` | BQL host-preserving frame and popup size propagation |
| `pkg/ui/modal_overlay_test.go`, `pkg/ui/modal_mouse_test.go`, `docs/design/tui-modal-compositing.md` | Cross-modal geometry, mouse integration, and canonical authoring instructions |

Line numbers below refer to base `36cfe9e6`; locate the named methods after earlier tasks shift them. Work in the existing `fix/aligned-edit-popup` worktree. Read the actual current `AGENTS.md` and approved design before execution. Investigate unfamiliar changes; do not overwrite them.

Before the first code task, retry `bd search "popup"` and `bd list --status=in_progress`. If tracking works, read both `.beads/conventions/reference.md` and `.beads/conventions/labels.md`, then create/claim an `area:tui,ux` task with the approved design and acceptance checks. If the `bt` database is still absent, disclose that limitation and preserve the previously approved no-database-repair boundary; do not fabricate an ID.

After **every code task**, run the specified focused tests, `go build ./...`, `go vet ./...`, and `git diff --check`. Review its deliverable, resolve important findings, then commit only its named paths with `git commit --only <paths>`. Include the real bead ID if one is available. Never push automatically, including to the fork.

TDD applies to every task: write the regression, run it red, make the smallest implementation change, and run it green. Do not manufacture a failure by breaking a dependency. For new APIs, an initial undefined-symbol failure is expected; once types exist, verify assertions actually detect wrong geometry/columns rather than merely compiling.

## Shared Interfaces

Introduce these types in `pkg/ui/panel.go`; no new production file is needed.

```go
type PopupSize struct {
	Width, Height int
}

type PopupOpts struct {
	Title, RightLabel string
	Theme             Theme
	Available         *PopupSize // nil: unknown, use 80x24; non-nil: actual budget
	Width, Height     int        // preferred OUTER dimensions; 0: content-sized
	MinBodyRows        int        // minimum useful specialized body; 0: one row
	Accent            color.Color // nil: Theme.Primary
	Footer            []string    // raw hints, detailed to compact
}

type PopupLayout struct {
	Width, Height         int
	BodyWidth, BodyHeight int
	BodyX, BodyY          int // panel-relative coordinates, including border
	FooterY               int // first footer row; -1 if absent
	PadX, PadY            int
	Footer                []string // fitted/wrapped raw lines
	Compact               bool
}

func MeasurePopup(body []string, opts PopupOpts) PopupLayout
func RenderPopup(body []string, opts PopupOpts) string
func popupFooterLines(width int, candidates []string) []string
func popupBodyLines(body []string) []string
func popupAvailableSize(size *PopupSize) PopupSize

type PopupMenuEntry struct {
	Label, Shortcut, Marker, Detail, Suffix string
	Selected                               bool
}

type PopupMenuOpts struct {
	Shortcuts, Markers bool // explicitly reserve these columns
}

type PopupMenuLayout struct {
	Width, LabelX, ShortcutWidth, MarkerWidth int
	Shortcuts, Markers                       bool
}

func MeasurePopupMenu(entries []PopupMenuEntry, opts PopupMenuOpts) PopupMenuLayout
func RenderPopupMenu(entries []PopupMenuEntry, layout PopupMenuLayout, theme Theme, width int) []string
func PopupMenuEntryRows(entry PopupMenuEntry) int
```

`RenderPopup` left-aligns its body; menu rows themselves supply the centered, fixed-width menu block. This keeps search inputs/page counts/forms left-aligned while centering only the actual menu entries. There is no independent per-row centering mode.

Every standalone popup model receives an immutable `popupSize *PopupSize` field. Its existing `SetSize(w,h)` assigns a new `&PopupSize{Width:w, Height:h}` before doing widget-specific sizing. Constructors leave it nil, so unset size and a known zero size are distinct. Do not mutate a pointed-to size shared by a copied Bubble Tea model. Models whose renderers belong to `Model` construct an available budget from `m.width` and `max(0,m.height-1)`.

Preferred dimensions remain a caller policy (e.g. Options is wider than the edit hub); the shared helpers alone own border/padding/footer deductions and final caps. Existing public `Dimensions`, `ItemAtPanelY`, and `IsSearchRow` methods remain; their implementation must derive coordinates from the same popup layout used by `View`.

## Task 1: Shared Popup Layout and Frame

**Files:** Modify `pkg/ui/panel.go` and `pkg/ui/panel_test.go`.

**Interfaces:** Consumes `Theme`, `RenderTitledPanel`, and ANSI functions. Produces `PopupSize`, `PopupOpts`, `PopupLayout`, `MeasurePopup`, `RenderPopup`, `popupFooterLines`, `popupBodyLines`, `popupAvailableSize`; test-only `assertPopupBounds` and `popupFindRow`.

- [ ] **Step 1: Add reusable behavioral assertions and a red frame test.** These utilities live only in `panel_test.go` and are available to other tests in package `ui`.

```go
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
	opts := PopupOpts{
		Title: "Status", Theme: DefaultTheme(),
		Available: &PopupSize{Width: 36, Height: 10}, Width: 60, Height: 20,
		Footer: []string{"j/k move  enter commit  esc back  * current", "j/k enter esc"},
	}
	out := RenderPopup([]string{"Open", "In Progress", "Blocked"}, opts)
	assertPopupBounds(t, out, 36, 10)
	footerRow, _ := popupFindRow(t, out, "j/k enter esc")
	if footerRow >= len(strings.Split(out, "\n"))-1 {
		t.Fatal("footer must be inside the bottom border")
	}
}
```

Add literal table cases for budgets `(120,32)`, `(48,16)`, `(24,8)`, `(8,3)`, `(1,1)`, `(0,10)`, `(10,0)`, and negative dimensions. Zero/negative known dimensions produce `""`; `(8,3)` and `(1,1)` produce a bounded plain fallback. Also test nil budget uses at most `80x24`, multiline body strings are counted correctly, empty footer candidates are safe, a compact hint wraps without losing `enter`/`esc`, long title/right label stay inside the border, and `Accent` overrides default colors. Existing color capability is controlled by package `TermProfile`; use `restoreThemeGlobals(t)` if a test needs to change it, rather than assuming Lipgloss v2 has a global profile setter.

```go
func TestPopupFrame_ZeroAndTinyBudgets(t *testing.T) {
	for _, size := range []PopupSize{
		{Width: 0, Height: 10}, {Width: 10, Height: 0},
		{Width: -1, Height: 10}, {Width: 1, Height: 1}, {Width: 8, Height: 3},
	} {
		opts := PopupOpts{Title: "Test", Theme: DefaultTheme(), Available: &size, Footer: []string{"enter esc"}}
		out := RenderPopup([]string{"body"}, opts)
		if size.Width <= 0 || size.Height <= 0 {
			if out != "" {
				t.Fatalf("budget %+v must produce no content, got %q", size, out)
			}
			continue
		}
		assertPopupBounds(t, out, size.Width, size.Height)
		if out == "" || strings.Contains(out, "╭") {
			t.Fatalf("budget %+v needs a bounded plain fallback, got %q", size, out)
		}
	}
}
```

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run '^TestPopupFrame' -count=1`. Verify undefined new symbols or the exact size/footer assertion failures, not unrelated live tests.

- [ ] **Step 3: Implement the layout calculations and frame.** Use these concrete normalization/footer operations:

```go
func popupAvailableSize(size *PopupSize) PopupSize {
	if size == nil {
		return PopupSize{Width: 80, Height: 24}
	}
	return PopupSize{Width: max(0,size.Width), Height: max(0,size.Height)}
}

func popupBodyLines(body []string) []string {
	var lines []string
	for _, block := range body {
		lines = append(lines, strings.Split(block, "\n")...)
	}
	return lines
}

func popupFooterLines(width int, candidates []string) []string {
	if width <= 0 || len(candidates) == 0 {
		return nil
	}
	for _, hint := range candidates {
		if ansi.StringWidth(hint) <= width {
			if hint == "" {
				return nil
			}
			return []string{hint}
		}
	}
	last := candidates[len(candidates)-1]
	if last == "" {
		return nil
	}
	return strings.Split(ansi.Wrap(last, width, ""), "\n")
}
```

`MeasurePopup` must apply this algorithm in order, with no caller-specific constants:

1. Use `80x24` for nil `Available`; clamp explicit dimensions to at least zero and return an empty layout if either is zero.
2. Flatten body lines. Measure body/title/right-label widths in cells. For content sizing, choose footer candidates against the maximum available body width before measuring the chosen footer. Otherwise a long full hint would force a needlessly wide panel when a compact hint fits.
3. Use preferred outer width if positive, else widest content plus borders/padding; cap to available width. Use normal two-cell side padding, reduce to one below 16 columns and zero below 10 columns. Subtract borders/padding once to get `BodyWidth`.
4. Fit/wrap footer against `BodyWidth`. Natural height is body rows + two borders + two breathing rows + footer rows + one separator if a footer exists. Cap preferred/content-sized height to the available height.
5. Remove vertical breathing rows first if they crowd out body or footer; remove the separator next. Keep at least one body row and all footer rows when a normal frame is possible. Set `Compact` if outer width is below 8 or those minimum rows cannot fit.
6. Fill `BodyX=1+PadX`, `BodyY=1+PadY`, `BodyHeight`, and `FooterY` from the actual chosen chrome, not hardcoded offsets. In compact mode set `Width=min(chosenWidth,ansi.StringWidth("Terminal too small"))`, `Height=1`, body sizes/padding to zero, footer lines to nil, and `FooterY=-1`. This makes layout dimensions match the actual one-line fallback, including mouse centering.

For callers with search/header/paging chrome, the minimum in step 5 is
`max(1,opts.MinBodyRows)`. If that minimum cannot fit, the shared frame chooses
the compact fallback; callers must not create their own fallback renderers.
Add a literal test with `MinBodyRows:5` and a short budget to prove body chrome
is not misreported as clickable item rows.

The body and footer portions of `RenderPopup` follow this assembly shape:

```go
layout := MeasurePopup(body, opts)
if layout.Width == 0 || layout.Height == 0 {
	return ""
}
if layout.Compact {
	return ansi.Truncate("Terminal too small", layout.Width, "")
}
flat := popupBodyLines(body)
inner := make([]string, layout.Height-2)
pad := strings.Repeat(" ", layout.PadX)
for i := 0; i < layout.BodyHeight; i++ {
	line := ""
	if i < len(flat) {
		line = ansi.Truncate(flat[i], layout.BodyWidth, "")
	}
	inner[layout.BodyY-1+i] = pad + line
}
hintStyle := lipgloss.NewStyle().Foreground(opts.Theme.Secondary).Italic(true)
for i, hint := range layout.Footer {
	inner[layout.FooterY-1+i] = pad + centerLine(hintStyle.Render(hint), layout.BodyWidth)
}
accent := opts.Accent
if accent == nil {
	accent = opts.Theme.Primary
}
```

Pass the joined `inner` slice to `RenderTitledPanel` with `Width/Height` from the layout, `Focused:true`, both color overrides set to `accent`, and `CenterTitle: opts.RightLabel==""`. Before passing a right label, truncate the label and title to the available border budget so the lower renderer's right-label branch cannot overflow; preserve a count before spending remaining cells on the title. Titles/right labels are plain text, so strip ANSI before truncating them. No new popup defaults belong in `RenderTitledPanel` itself.

- [ ] **Step 4: Run green and the existing panel tests.** `go test ./pkg/ui -run 'Test(PopupFrame|RenderTitledPanel|OverlayCenter)' -count=1`. Assert the footer's horizontal margins differ by at most one cell and forms' first content cell is `BodyX`.
- [ ] **Step 5: Run task gates, review, and commit.** `fix(tui): centralize popup frame and size budgets`.

## Task 2: Shared Fixed-Column Menu Rows

**Files:** Modify `pkg/ui/panel.go` and `pkg/ui/panel_test.go`.

**Interfaces:** Consumes Task 1's cell-aware body budgets. Produces `PopupMenuEntry`, `PopupMenuOpts`, `PopupMenuLayout`, `MeasurePopupMenu`, `RenderPopupMenu`, and `PopupMenuEntryRows`.

- [ ] **Step 1: Write red column/page tests.** Use the same measured full menu for both visible pages:

```go
func TestPopupMenu_ColumnsSurvivePageAndMarkerChanges(t *testing.T) {
	entries := []PopupMenuEntry{
		{Label: "Open", Shortcut: "s", Marker: "*", Selected: true},
		{Label: "In Progress", Shortcut: "", Marker: ""},
		{Label: "界面", Shortcut: "A", Marker: "✓"},
	}
	menu := MeasurePopupMenu(entries, PopupMenuOpts{Shortcuts: true, Markers: true})
	first := RenderPopupMenu(entries[:1], menu, DefaultTheme(), 30)
	second := RenderPopupMenu(entries[1:], menu, DefaultTheme(), 30)
	columns := []int{}
	for _, page := range [][]string{first, second} {
		for _, row := range page {
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
		t.Fatalf("label columns = %v", columns)
	}
}
```

Add cases for all-empty enabled columns, disabled columns, styled labels/suffixes, a two-cell marker, detail indentation/truncation, zero usable width, long unbroken labels, and selection changing without changing the block origin. Check actual centering margins, not just equal output widths.

```go
func TestPopupMenu_StyledWideContentKeepsGeometry(t *testing.T) {
	entries := []PopupMenuEntry{
		{Label: "\x1b[1m界面\x1b[0m", Marker: "✓", Suffix: " (4)", Selected: true},
		{Label: "Plain", Marker: "", Detail: "a long detail that must occupy only one row"},
	}
	menu := MeasurePopupMenu(entries, PopupMenuOpts{Markers: true})
	rows := RenderPopupMenu(entries,menu,DefaultTheme(),16)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want two labels and one detail",len(rows))
	}
	for _, row := range rows {
		if ansi.StringWidth(row) != 16 {
			t.Fatalf("styled/wide row has width %d, want 16",ansi.StringWidth(row))
		}
	}
	first, second := ansi.Strip(rows[0]), ansi.Strip(rows[1])
	firstX := ansi.StringWidth(first[:strings.Index(first,"界面")])
	secondX := ansi.StringWidth(second[:strings.Index(second,"Plain")])
	if firstX != secondX {
		t.Fatalf("wide/ANSI label columns differ: %d vs %d",firstX,secondX)
	}
}
```

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run '^TestPopupMenu' -count=1`.
- [ ] **Step 3: Implement full-menu measurement and rendering.** Use these column calculations:

```go
layout := PopupMenuLayout{Shortcuts: opts.Shortcuts, Markers: opts.Markers, LabelX: 2}
if opts.Shortcuts {
	layout.ShortcutWidth = 1
	for _, entry := range entries {
		layout.ShortcutWidth = max(layout.ShortcutWidth, ansi.StringWidth(entry.Shortcut))
	}
	layout.LabelX += layout.ShortcutWidth + 2
}
if opts.Markers {
	layout.MarkerWidth = 2
	for _, entry := range entries {
		layout.MarkerWidth = max(layout.MarkerWidth, ansi.StringWidth(entry.Marker))
	}
	layout.LabelX += layout.MarkerWidth + 1
}
layout.Width = layout.LabelX
for _, entry := range entries {
	layout.Width = max(layout.Width, layout.LabelX+ansi.StringWidth(entry.Label+entry.Suffix))
	layout.Width = max(layout.Width, layout.LabelX+ansi.StringWidth(entry.Detail))
}
```

In `RenderPopupMenu`, return no rows for `width<=0`. Reserve `blockWidth=min(layout.Width,width)` and one shared left gap `(width-blockWidth)/2`. Build each row from the two-cell `> `/blank cursor, padded enabled columns, and label/suffix. Use primary/bold for cursor/selected label and theme base foreground for idle labels; style shortcuts with primary/bold and markers/metadata with secondary unless their strings already carry specialized ANSI spans. Truncate the assembled block to `blockWidth`, pad it to that width, then add the shared left gap and trailing spaces to exactly `width`. Do not recalculate widths per page. Detail lines have `layout.LabelX` leading cells inside the block, secondary/italic text, and one row truncated to the remaining width.

```go
func PopupMenuEntryRows(entry PopupMenuEntry) int {
	if entry.Detail != "" {
		return 2
	}
	return 1
}
```

All `strings.Repeat` arguments must be nonnegative. No map iteration may determine entry order. Helpers neither modify entries nor infer selection/current values.

- [ ] **Step 4: Run green.** `go test ./pkg/ui -run '^TestPopup(Menu|Frame)' -count=1`. Mutate the tests mentally: measuring only the visible page or dropping empty marker padding must fail them.
- [ ] **Step 5: Run task gates, review, and commit.** `fix(tui): reuse aligned popup menu rows`.

## Task 3: Edit Hub, Status/Priority Pickers, and Editors

**Files:** Modify `pkg/ui/field_edit.go:58-419`, `pkg/ui/field_edit_test.go`, `pkg/ui/longform_edit.go:195-273`, `pkg/ui/longform_edit_test.go`.

**Interfaces:** Consumes `MeasurePopup`, `RenderPopup`, and all Task 2 APIs. Produces migrated edit views, `popupSize` fields, updated `SetSize` sizing, and existing `panelDims()` backed by shared layout. Keep `SelectedField`, `Selected`, `Value`, `Update`, dispatch, write functions, and guard semantics unchanged.

- [ ] **Step 1: Write red submenu tests that separate current value from cursor.**

```go
func TestFieldPickerView_AlignedCurrentAndCursor(t *testing.T) {
	for _, width := range []int{48, 80, 120} {
		picker := NewStatusPickerModal(model.StatusOpen, DefaultTheme())
		picker.SetSize(width, 24)
		picker.MoveDown()
		out := picker.View()
		_, openRow := popupFindRow(t, out, "Open")
		_, progressRow := popupFindRow(t, out, "In Progress")
		openCol := ansi.StringWidth(openRow[:strings.Index(openRow, "Open")])
		progressCol := ansi.StringWidth(progressRow[:strings.Index(progressRow, "In Progress")])
		if openCol != progressCol {
			t.Fatalf("Open column=%d, In Progress column=%d", openCol, progressCol)
		}
		if !strings.Contains(openRow, "*") || strings.Contains(openRow, ">") || !strings.Contains(progressRow, ">") {
			t.Fatalf("current/cursor markers lost:\n%s", ansi.Strip(out))
		}
		assertPopupBounds(t, out, width, 24)
	}
}
```

Add the priority equivalent with current `P2` and cursor `P3`, and edit hub column coverage after migration. Add input/editor tests using `SetSize(32,12)` and `SetSize(80,24)`: label/input left edges align, `enter`/`esc` or `ctrl+s`/`esc` footer remains, long validation messages do not overflow, and reapplying size leaves input/textarea value, `original`, `escArmed`, and cursor selection unchanged. This editor fixture pins the buffer and guard, not only its rendered dimensions:

```go
func TestLongformPopup_SizePreservesDraftAndGuard(t *testing.T) {
	m := NewLongformEditModal("description", "Description", "original", DefaultTheme())
	m.textarea.SetValue("edited draft")
	m.escArmed = true
	m.SetSize(32,12)
	if m.textarea.Value() != "edited draft" || m.original != "original" || !m.escArmed {
		t.Fatal("sizing changed the draft, baseline, or discard guard")
	}
	assertPopupBounds(t, m.View(),32,12)
	popupFindRow(t,m.View(),"ctrl+s")
	popupFindRow(t,m.View(),"esc")
}
```

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(FieldPickerView|FieldSelectView|FieldInput.*Popup|Longform.*Popup)' -count=1`.
- [ ] **Step 3: Replace hub/picker rendering with entry projection and shared rendering.** For the status/priority picker, this is the concrete projection:

```go
entries := make([]PopupMenuEntry, len(m.options))
for i, option := range m.options {
	marker := ""
	if option.Value == m.current {
		marker = "*"
	}
	entries[i] = PopupMenuEntry{Label: option.Label, Marker: marker, Selected: i == m.cursor}
}
menu := MeasurePopupMenu(entries, PopupMenuOpts{Markers: true})
opts := PopupOpts{
	Title: m.title, Theme: m.theme, Available: m.popupSize,
	Footer: []string{"j/k move  enter commit  esc back  * current", "j/k enter commit esc back"},
}
shape := make([]string, len(entries))
if len(shape) > 0 {
	shape[0] = strings.Repeat(" ", menu.Width)
}
layout := MeasurePopup(shape, opts)
visible := min(len(entries),layout.BodyHeight)
start := min(max(0,m.cursor-visible+1), max(0,len(entries)-visible))
rows := RenderPopupMenu(entries[start:start+visible], menu, m.theme, layout.BodyWidth)
return RenderPopup(rows, opts)
```

For the hub, project `fieldEditEntries` into `{Label:e.Label, Shortcut:e.Key, Selected:i==m.cursor}`, reserve shortcuts, and use `"j/k move  enter select  esc cancel"` / `"j/k enter esc"` footer variants. Use the same measured full-menu shape and selected-row window when terminal height cannot show all entries. Add a regression with the cursor on Acceptance Criteria at `36x10` to prove it is visible rather than silently clipped. Delete the functions' duplicated padding/style loops and remove `renderFieldModalLines` after its final consumer migrates; removing a function is allowed, deleting a file is not.

For input/editor bodies use shared left-aligned frames. In `SetSize`, calculate layout from preferred dimensions and footer, set input width to `max(1,layout.BodyWidth)`, and textarea width/height to `max(1,layout.BodyWidth)` / `max(1,layout.BodyHeight-1)` (the one row is its label). Preserve `panelDims`' callers by returning the shared layout's outer dimensions, not a compatibility renderer. If layout is compact/empty, render the fallback but retain the buffer.

- [ ] **Step 4: Run green and interaction tests.** `go test ./pkg/ui -run 'Test(Field|RequestFieldEdit|Longform|LongformFieldSpecs)' -count=1`. Existing executor stubs must still observe the same argv and pending-write behavior.
- [ ] **Step 5: Run task gates, review, and commit.** `fix(tui): share edit popup and submenu layouts`.

## Task 4: Settings Launcher and Recipe Menu

**Files:** Modify `pkg/ui/settings_menu.go:89-137`, `pkg/ui/settings_modal_test.go`, `pkg/ui/recipe_picker.go:75-230`, `pkg/ui/recipe_picker_test.go`.

**Interfaces:** Consumes Task 1/2 APIs. Produces migrated menu views and `popupSize` capture in their existing `SetSize` methods. Navigation methods are unchanged.

- [ ] **Step 1: Add red description/selection tests.** Use two recipes with deliberately unequal names/descriptions and a small height; check the selected last item and footer remain visible after scrolling.

```go
func TestRecipePopup_SelectedRowAndFooterFit(t *testing.T) {
	items := []recipe.Recipe{
		{Name: "Short", Description: "one"},
		{Name: "A much longer name", Description: strings.Repeat("description ", 8)},
		{Name: "Last", Description: "three"},
	}
	menu := NewRecipePickerModel(items, DefaultTheme())
	menu.SetSize(40, 12)
	menu.MoveDown()
	menu.MoveDown()
	out := menu.View()
	assertPopupBounds(t, out, 40, 12)
	_, last := popupFindRow(t, out, "Last")
	if !strings.Contains(last, ">") {
		t.Fatal("last visible recipe lacks selected-row cue")
	}
	popupFindRow(t, out, "esc")
}
```

Add launcher tests through `settingsTestModel`: all three entry labels share a column, details start under labels, and Enter/Esc keep existing behavior. Use literal rendering expectations, not assertions that source text calls a helper.

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(RecipePopup|SettingsLauncherPopup)' -count=1`.
- [ ] **Step 3: Replace manual menu prefixes/description padding with shared entries.** Recipe projection:

```go
entries := make([]PopupMenuEntry, len(m.recipes))
for i, item := range m.recipes {
	entries[i] = PopupMenuEntry{Label: item.Name, Detail: item.Description, Selected: i == m.selectedIndex}
}
menu := MeasurePopupMenu(entries, PopupMenuOpts{})
available := popupAvailableSize(m.popupSize)
opts := PopupOpts{
	Title: "Select Recipe", Theme: m.theme, Available: m.popupSize, Width: 50,
	Height: available.Height * 7 / 10,
	Footer: []string{"j/k: navigate  enter: apply  esc: cancel", "j/k enter esc"},
}
shapeRows := 2 // scroll indicators
for _, entry := range entries {
	shapeRows += PopupMenuEntryRows(entry)
}
shape := make([]string, shapeRows)
shape[0] = strings.Repeat(" ", menu.Width)
opts.MinBodyRows = 3 // two indicators and at least one selectable row
if len(entries) > 0 {
	opts.MinBodyRows = 2 + PopupMenuEntryRows(entries[m.selectedIndex])
}
layout := MeasurePopup(shape, opts)
```

Reserve the existing two scroll-indicator rows within `layout.BodyHeight`. Determine the visible recipe window by summing `PopupMenuEntryRows` until the remaining budget is exhausted, keeping the selected item in view. Keep blank indicator rows when not scrolled. Empty recipes use one "No recipes available" body row between indicators; guard entry indexing and selected-recipe access accordingly. For the launcher, entries are `{Label:e.label, Detail:e.desc, Selected:i==s.selected}`; render all three if they fit, otherwise window around selection using the same row-count budget. Final frame is `RenderPopup(body,opts)` with no per-caller outer padding/footer styling.

- [ ] **Step 4: Run green and existing key tests.** `go test ./pkg/ui -run 'Test(Recipe|SettingsLauncherPopup|SettingsMenu)' -count=1`. Do not edit failing palette-load tests in this task.
- [ ] **Step 5: Run task gates, review, and commit.** `refactor(tui): share launcher and recipe menu rendering`.

## Task 5: Label/Repository Menus and Mouse Geometry

**Files:** Modify `pkg/ui/repo_picker.go:344-600`, `pkg/ui/label_picker.go:239-681`, their existing tests, and picker sections of `pkg/ui/modal_mouse_test.go` / `pkg/ui/mouse_click_test.go` if coordinates are hardcoded there.

**Interfaces:** Consumes Task 1/2 APIs. Produces shared-layout-backed `Dimensions() (int,int)`, `ItemAtPanelY(int) (int,bool)`, `IsSearchRow(int) bool`, and existing visible-count calculations. Add private `popupOpts() PopupOpts` and `popupLayout() PopupLayout` on both picker types so rendering and hit-testing share exact geometry, not duplicated estimates.

- [ ] **Step 1: Add red layout/render/hit-test agreement tests.** In `repo_picker_test.go`:

```go
func TestRepoPopup_RenderedRowMatchesHitTest(t *testing.T) {
	m := NewRepoPickerModel([]string{"short", "界面-project", "last-project"}, DefaultTheme())
	m.SetSize(48, 16)
	m.selectedIndex = 2
	out := m.View()
	assertPopupBounds(t, out, 48, 16)
	w, h := m.Dimensions()
	if w != ansi.StringWidth(strings.Split(out, "\n")[0]) || h != len(strings.Split(out, "\n")) {
		t.Fatalf("Dimensions=%dx%d do not match rendered popup", w, h)
	}
	y, _ := popupFindRow(t, out, "last-project")
	index, ok := m.ItemAtPanelY(y)
	if !ok || index != 2 {
		t.Fatalf("rendered last-project at y=%d maps to (%d,%v)", y, index, ok)
	}
	footerY, _ := popupFindRow(t, out, "enter")
	if _, ok := m.ItemAtPanelY(footerY); ok {
		t.Fatal("footer is incorrectly clickable as a project")
	}
}
```

Add the label equivalent with literal counts `{ "alpha": 12, "界面": 4, "last": 1 }`. Add page tests with more rows than the height budget, including empty/no-match filtered lists. Assert filtering and toggling keep raw repository keys (including the atlas alias), counts/multi-select markers stay visible, search focus is unchanged, and the selected row stays within the rendered item window.

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(RepoPopup|LabelPopup)' -count=1`.
- [ ] **Step 3: Add entry projections and shared geometry.** Repository entries use raw keys for state and `model.DisplayRepoName(repo)` only for labels:

```go
entries := make([]PopupMenuEntry, len(m.filtered))
for i, repo := range m.filtered {
	marker := "•"
	if m.selected[repo] {
		marker = activeGlyphs.Success
	}
	entries[i] = PopupMenuEntry{
		Label: model.DisplayRepoName(repo), Marker: marker, Selected: i == m.selectedIndex,
	}
}
menu := MeasurePopupMenu(entries, PopupMenuOpts{Markers: true})
```

Label entries use the label unchanged and `Suffix:fmt.Sprintf(" (%d)",m.labelCounts[label])`. Measure the complete filtered entries once per view/layout calculation, then slice pages. The preferred outer width may retain the existing 80%-of-terminal policy, but use cell width instead of `len` and let `MeasurePopup` enforce final caps.

Body row accounting is explicit: search row + one blank + item window + one blank + page/count row. Footer/chrome rows belong only to `MeasurePopup`. Set `MinBodyRows:5` in picker options and `maxVisible=min(existingMaxVisible,max(1,layout.BodyHeight-4))` only for a noncompact layout; otherwise let `RenderPopup` produce its compact fallback and disable all row/search hit-testing.

Preserve the existing height policies: repository menus size down to `max(1,len(filtered))` entries; label menus keep a fixed visible window across search states. Both keep the existing 30-item tall-terminal caps. `popupLayout` uses a representative body shape of item rows plus the four non-item rows: filtered count (capped at 30, at least one) for repos, 30 slots for labels. Measure natural dimensions first, then cap preferred height at 75% of `popupAvailableSize(m.popupSize).Height`; if that soft target would be compact but the full available height can fit the minimum useful body, retry with the natural/full-budget cap. Only shared layout deducts chrome. The resulting effective options must be reused by `View`, not recalculated from a smaller current page.

Pad unused item slots to retain vertical stability. `popupLayout` must not call `View` or `visibleCount`, avoiding recursion; `visibleCount` consumes that layout instead. Add a label regression that filters to one/no matches and asserts unchanged rendered height at the same terminal size. The label equivalent of the rendered-hit-test fixture sets `SetCursor(2)` before rendering so the final label is on the visible page.

Implement `ItemAtPanelY` from `layout.BodyY+2` (search + blank), the actual item window size, and the page start. Search row is `layout.BodyY`. `Dimensions` returns the actual layout dimensions, including compact fallback dimensions. Existing constants for old offsets must not continue driving mouse routing. Remove unused old constants/functions, not their files.

Footer variants retain toggle/search/page/apply/back on normal terminals and essential navigation/confirm/cancel on narrow ones. Each picker uses `RenderPopup` for its final body/footer; neither prepends its own horizontal padding nor draws an independent border.

- [ ] **Step 4: Run green and mouse/filter suites.** `go test ./pkg/ui -run 'Test(Repo|LabelPicker|LabelPopup|Workspace.*Filter|.*Picker.*Mouse|.*Picker.*Click)' -count=1`. Existing alias/search/multi-select tests must pass unchanged except deliberate geometric expectations.
- [ ] **Step 5: Run task gates, review, and commit.** `refactor(tui): unify filter popups and hit-test geometry`.

## Task 6: Confirmation, Help, Options, and Alert Frames

**Files:** Modify `pkg/ui/claim.go:455-518`, `pkg/ui/model_view.go` (`renderQuitConfirm`, `renderHelpMini`, `renderHelpOverlay`, `helpOverlayAvailBody`, `renderTimeTravelPrompt`), `pkg/ui/settings_modal.go:236-328`, `pkg/ui/model_alerts.go`, `pkg/ui/model_alerts_header.go` layout methods; existing `claim_test.go`, `context_help_test.go`, `settings_modal_test.go`, `model_alerts_test.go`, `modal_mouse_test.go`, and `modal_overlay_test.go`.

**Interfaces:** Consumes Task 1 frame/layout APIs. Keeps public/model render entry points unchanged. Alert item offsets and help available-body calculations consume `PopupLayout.BodyY/BodyHeight`; warning/count title data remains explicit.

- [ ] **Step 1: Add red frame tests using real existing models.** In `modal_overlay_test.go`:

```go
func TestConfirmationPopups_ShareBoundsAndKeepWarnings(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 40, 12)
	m.claimTargetID = "zz-target"
	m.claimTargetTitle = strings.Repeat("long title ", 8)
	for _, out := range []string{m.renderQuitConfirm(), m.renderClaimConfirm(), m.renderTimeTravelPrompt()} {
		assertPopupBounds(t, out, 40, 11)
		popupFindRow(t, out, "esc")
	}
}
```

Add Options tests showing two columns do not overlap at `48x16`; help mini/full tests retain their existing `;` and `Esc` footer wording and do not hide it; alerts/notifications keep right-hand counts and warning accent. For mouse integration, derive a target item's screen Y from its unique rendered fixture text plus the compositor start row, then send an actual `tea.MouseClickMsg`; assert the matching issue/event cursor and existing double-click behavior. Keep backdrop/chrome clicks as no-ops.

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(ConfirmationPopups|OptionsPopup|HelpPopup|AlertsPopup)' -count=1`.
- [ ] **Step 3: Move outer chrome into `RenderPopup`; keep specialized bodies.** Quit confirmation's frame becomes:

```go
opts := PopupOpts{
	Title: "Quit?", Theme: m.theme, Accent: m.theme.Blocked,
	Available: &PopupSize{Width: m.width, Height: max(0,m.height-1)},
	Footer: []string{"esc / y to quit; any other key cancels", "esc/y quit; other cancel"},
}
return RenderPopup([]string{"Quit beadstui?"}, opts)
```

Do not change the quit handler's keys. Claim body remains the claim ID/title/prediction and moves its confirmation hints to raw footer variants. Time travel retains subtitle, input, and example strings while using preferred width `64` and shared widget width.

Options retains tabs and its two-column content; use `layout.BodyWidth` to split columns and `layout.BodyHeight` to choose rows. Remove its outer leading padding and separately styled footer. Preserve theme-preview/revert handlers.

For help, keep body generation and mini/full selection, but calculate the available body from the shared layout. Pass `"; per-view  -  Esc / q to close"` and its scrolling variant as footer input; do not accidentally include `"shortcuts"` in the footer. A full sheet and the mini both use `RenderPopup` exactly once.

For alerts/notifications, split footer construction out of `renderAlertsTab`/`renderNotificationsTab` so it is not counted twice. Their body width/height and `alertsItemsChromeRows()` must use the shared layout's body origin. Preserve active tab, existing adaptive hint candidates, report header, counts, selected details, day separators, dismissal state, and pagination. `renderAlertsPanel` passes `RightLabel` and the appropriate primary/blocked `Accent`, not a second panel wrapper.

- [ ] **Step 4: Run green and existing interaction suites.** `go test ./pkg/ui -run 'Test(Claim|RequestClaim|Quit|.*Help|.*TimeTravel|.*Alerts|.*Notifications|OptionsPopup|SettingsCancel|SettingsKeep)' -count=1`. Also run modal mouse tests and the preexisting styled-frame/backdrop tests. Report the known palette failure if a wider selector includes it; do not modify its expectation.
- [ ] **Step 5: Run task gates, review, and commit.** `refactor(tui): share confirmation and report popup frames`.

## Task 7: Agent, Session, Update, Epic, and BQL Dialogs

**Files:** Modify `pkg/ui/agent_prompt_modal.go`, `pkg/ui/cass_session_modal.go`, `pkg/ui/update_modal.go`, `pkg/ui/epic_card.go`, `pkg/ui/bql_modal.go`, and their existing test files, plus `pkg/ui/epic_progress_test.go` for the ordinary-view guard. Add BQL frame tests to `pkg/ui/modal_overlay_test.go` because there is no existing `bql_modal_test.go`; do not proliferate test files.

**Interfaces:** Consumes shared frame/menu APIs. Produces migrated outer frames and session/epic selectable header projections. Preserve existing constructor/method signatures and BQL host placement.

- [ ] **Step 1: Write red bounded-rich-body tests.** In `modal_overlay_test.go`:

```go
func TestLegacyDialogPopups_FitNarrowTerminal(t *testing.T) {
	theme := DefaultTheme()
	agent := NewAgentPromptModal("/test/AGENTS.md", "AGENTS.md", theme)
	agent.SetSize(42, 18)
	update := NewUpdateModal("v1.0.0", "", theme)
	update.SetSize(42, 18)
	for _, out := range []string{agent.View(), update.View()} {
		assertPopupBounds(t, out, 42, 18)
	}
}
```

Add fixture tests in the existing session/epic files: unequal session agent names preserve selected header alignment and copy feedback; epic children retain status/priority pill text and selected cursor after windowing. Before changing epic popup construction, record an unchanged literal detail-pane `buildEpicProgressANSI(epic,issues,-1,width)` expectation in `epic_progress_test.go`; it is the ordinary-view regression guard. Add BQL `SetSize(32,12)` tests for preserved query/error/history hints and existing top placement. Update-state tests cover confirm, downloading, verifying, installing, success, and error with their existing actions untouched.

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(LegacyDialogPopups|SessionPopup|EpicPopup|BQLPopup)' -count=1`.
- [ ] **Step 3: Replace outer Lipgloss borders and duplicated titles/footers.** Each legacy dialog captures actual terminal availability separately from preferred panel width in `SetSize`; do not keep the old minimum widths that exceed narrow terminals. Build the existing rich body, then use this shape:

```go
opts := PopupOpts{
	Title: "Enhance AI Agent Integration?", Theme: m.theme,
	Available: m.popupSize, Width: min(70,m.width),
	Footer: []string{"left/right select  enter confirm  esc cancel", "left/right enter esc"},
}
layout := MeasurePopup(nil, opts)
previewWidth := max(1,layout.BodyWidth-2)
```

Size the preview/snippet inner boxes from `previewWidth` and the remaining body rows; keep their specialized internal borders. For short agent/update dialogs, retain the existing action labels while compacting visual button padding rather than dropping an action or changing bindings. Footer text must match the actual existing handler; verify Enter/Esc semantics separately for each state rather than copying the sample hints blindly.

Session-card headers use `PopupMenuEntry{Label:sessionInfo, Shortcut:fmt.Sprintf("[%d]",i+1), Selected:i==m.selected}` with reserved shortcuts; the rest of each card stays specialized. Build complete entries before windowing so cursor movement does not shift headers.

For epic popup children, call existing `buildEpicProgressANSI` with selected index `-1` so it does not add a second cursor. Split its summary/body into rows; remove only the known leading two blank cursor cells from the popup projection with `ansi.TruncateLeft(row,2,"")`, then pass the still-styled row as `PopupMenuEntry.Label` with the popup's selected flag. Do not change the shared progress renderer or register new slot providers. Derive visible children from the shared body budget after preserving summary and indicators.

BQL uses the shared frame with its query/error body and raw hint variants. Keep its existing top-pad host placement in `BQLQueryModal.View` and `Model.View`'s dispatch; do not add a dimmed overlay or change query execution/history.

- [ ] **Step 4: Run green and existing state/action suites.** `go test ./pkg/ui -run 'Test(AgentPrompt|NewAgentPrompt|CassSession|NewCassSession|UpdateModal|NewUpdateModal|EpicCard|EpicProgress|LegacyDialogPopups|SessionPopup|EpicPopup|BQLPopup)' -count=1`.
- [ ] **Step 5: Run task gates, review, and commit.** `refactor(tui): share rich dialog popup chrome`.

## Task 8: Resize Propagation and Canonical Popup Guidance

**Files:** Modify `pkg/ui/model_update_input.go:2219-2255`, `pkg/ui/model_update_analysis.go:491-499`, `pkg/ui/model_export.go` popup-open methods, `pkg/ui/modal_overlay_test.go`, `pkg/ui/longform_edit_test.go`, `docs/design/tui-modal-compositing.md`, and the status line of `docs/design/popup-rendering.md` when the whole implementation passes review.

**Interfaces:** Consumes migrated `SetSize` methods and shared popup layouts. Produces private `func (m *Model) resizeActivePopup(width,height int)` in `model_update_input.go`; it only forwards size, never commands or actions. Existing modal dispatch stays intact.

- [ ] **Step 1: Add red resize tests through the actual update path.**

```go
func TestEditPopupResize_PreservesCursorAndFits(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m.fieldSelect.MoveDown()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 36, Height: 12})
	got := updated.(Model)
	if got.fieldSelect.SelectedField() != "priority" || got.activeModal != ModalFieldSelect {
		t.Fatal("resize changed edit selection or modal")
	}
	assertPopupBounds(t, got.fieldSelect.View(), 36, 11)
	popupFindRow(t, got.fieldSelect.View(), "esc")
}
```

Add input/longform resize tests with a dirty typed buffer and armed discard guard; assert buffer, original baseline, and modal remain unchanged while widget sizes and footer fit. Add resize-open tests for agent/update/session popups, and zero/very-short size messages that must not panic or render beyond the body budget.

- [ ] **Step 2: Run red.** `go test ./pkg/ui -run 'Test(EditPopupResize|.*Popup.*Resize|.*Popup.*Zero)' -count=1`.
- [ ] **Step 3: Forward the real budget without the ordinary-view five-row floor.** Add this call in the cheap resize phase:

```go
m.resizeActivePopup(max(0,m.width), max(0,m.height-1))
```

The method switches only on the existing active modal enum and forwards to the existing popup `SetSize` methods for field select/picker/input, longform, recipe, settings/menu, repo/label, agent, cass, update, and BQL. Do not reuse the ordinary-view five-row floor as a popup budget; it can exceed the real available rows. Adjust unconditional repo/label sizing so it also gets the real popup budget. Model-owned help/claim/quit/time-travel/alerts/epic rendering already calculates current availability directly.

```go
func (m *Model) resizeActivePopup(width,height int) {
	switch m.activeModal {
	case ModalFieldSelect:
		m.fieldSelect.SetSize(width,height)
	case ModalFieldPicker:
		m.fieldPicker.SetSize(width,height)
	case ModalFieldInput:
		m.fieldInput.SetSize(width,height)
	case ModalLongformEdit:
		m.longformEdit.SetSize(width,height)
	case ModalRecipePicker:
		m.recipePicker.SetSize(width,height)
	case ModalSettings:
		m.settingsModal.SetSize(width,height)
	case ModalSettingsMenu:
		m.settingsMenu.SetSize(width,height)
	case ModalRepoPicker:
		m.repoPicker.SetSize(width,height)
	case ModalLabelPicker:
		m.labelPicker.SetSize(width,height)
	case ModalAgentPrompt:
		m.agentPromptModal.SetSize(width,height)
	case ModalCassSession:
		m.cassModal.SetSize(width,height)
	case ModalUpdate:
		m.updateModal.SetSize(width,height)
	case ModalBQLQuery:
		m.bqlQuery.SetSize(width,height)
	}
}
```

At agent prompt open in `handleAgentFileCheck`, call `SetSize(m.width,max(0,m.height-1))` once the new modal is created if `m.ready`. Session/update open methods receive the same body budget. Do not rebuild text buffers or re-run external commands on resize; the debounced ordinary-view rendering path remains unchanged.

- [ ] **Step 4: Update the canonical modal instructions in the same task.** Replace the old first authoring step with: calculate shared `PopupLayout`, build raw body/footer, use `RenderPopup`; use `MeasurePopupMenu`/`RenderPopupMenu` for selectable menu rows. Explain nil vs explicit size, outer vs body dimensions, shared mouse origins, and that `Model.View` alone overlays popups with `OverlayCenterDimBackdrop`. Keep nonmodal `RenderTitledPanel`/`OverlayCenter` guidance. Link `docs/design/popup-rendering.md` rather than duplicating its full spec.
- [ ] **Step 5: Run green, race checks, task gates, and commit.** `go test ./pkg/ui -run 'Test(.*Popup|Modal|.*Resize|.*Mouse)' -count=1`; then `go test -race ./pkg/ui -run 'Test(PopupFrame|PopupMenu|EditPopupResize)' -count=1`. Commit `fix(tui): keep shared popups consistent on resize`. Keep the design's status in progress until whole-branch review and acceptance in Task 9.

## Task 9: Safe Full Verification and Whole-Branch Review

**Files:** No new production changes by default. Fix only findings within this refactor in their existing files, with a new failing regression before each fix. Update the design status after final acceptance.

**Interfaces:** Consumes all Task 1-8 deliverables. Produces verified acceptance coverage and a final change summary; no release/install/push.

- [ ] **Step 1: Audit migration coverage against the design tables.** Search popup call sites, inspect each remaining `Border(...)`/`RenderTitledPanel(...)`, and distinguish nested boxes/ordinary panels from outer popup frames. Do not add a source-grep test: rendering/interaction tests establish behavior; this manual audit establishes there is no second outer-popup renderer. Confirm menu entry order remains deterministic and shared measurements do not perform I/O.

```bash
git diff --check
git diff 36cfe9e6 -- pkg/ui docs/design
```

- [ ] **Step 2: Establish safe suite isolation before running the full suite.** Preserve real Go build/module caches so a temporary home does not download everything again; run tests with a temporary user/config home and registry. Creating scratch directories is allowed; do not delete them automatically. These commands change only the child test process's environment, not source code:

```bash
cache_path=$(go env GOCACHE)
module_cache_path=$(go env GOMODCACHE)
mkdir -p _tmp
test_home=$(mktemp -d "$PWD/_tmp/popup-test-home.XXXXXX")
mkdir -p "$test_home/.bt" "$test_home/config" "$test_home/cache" "$test_home/test-registry"
env HOME="$test_home" XDG_CONFIG_HOME="$test_home/config" \
    XDG_CACHE_HOME="$test_home/cache" GOCACHE="$cache_path" \
    GOMODCACHE="$module_cache_path" \
    BT_PROJECTS_REGISTRY_PATH="$test_home/test-registry/projects.json" \
    BT_TEST_MODE=1 BT_NO_BROWSER=1 \
    go test ./... -timeout=5m
```

Record the result; do not treat environment isolation as a reason to hide failed tests. The explicit test registry is deliberately different from the disposable home's default `.bt/projects.json`; otherwise a correctly isolated override would itself trip the default-registry guard. Test guards may still report changes to that disposable default if a test ignores the override. Do not copy/restore the real registry. The live Dolt dogfood test can still fail because the checkout's `bt` database is missing; disclose it explicitly.

- [ ] **Step 3: Run fresh final gates and focused regression/race tests.**

```bash
go build ./...
go vet ./...
go test ./pkg/ui -run 'Test(PopupFrame|PopupMenu|FieldPickerView|FieldSelectView|RecipePopup|SettingsLauncherPopup|RepoPopup|LabelPopup|ConfirmationPopups|OptionsPopup|HelpPopup|AlertsPopup|LegacyDialogPopups|SessionPopup|EpicPopup|BQLPopup|EditPopupResize)' -count=1
go test -race ./pkg/ui -run 'Test(PopupFrame|PopupMenu|EditPopupResize)' -count=1
git diff --check
```

Previously observed UI failures, each reproduced on unchanged `main`: `TestMemoriesLoadCmd_DogfoodLiveProject`, `TestBtThemeEnvSelectsNative`, `TestSettingsCyclesThemeLive`, `TestThemeSwapMidSession_NoRace`. The earlier CLI `TestMain` registry guard also failed. Compare new failures against a safely isolated unchanged-base run if attribution is unclear; do not repair unrelated environment/data/theme code.

- [ ] **Step 4: Perform whole-branch correctness review.** Use `jev.review_select` at this boundary with branch scope base `36cfe9e6`, the approved design, the actual execution method's delegation policy, and mandatory correctness coverage. Inspect selected relevant dimensions (architecture, tests, accessibility, documentation, resize performance). Native execution uses one fresh reviewer for the whole branch; subagent-driven execution also has its task reviews. Fix important findings before claiming completion; minor findings must be either addressed or reported.
- [ ] **Step 5: Finalize documentation and tracking, then commit only directed files.** Mark `docs/design/popup-rendering.md` implemented only if the design's acceptance checks and migrated call-site audit are satisfied. Close a real bead with the repository's Summary/Change/Files/Verify/Risk format if tracking is available; otherwise report the missing database. No invented bead reference. Leave any unrelated `.beads.gate.lock` untouched.
- [ ] **Step 6: Report outcome with evidence.** Name commit(s), migrated popup families, focused/full/race/build/vet results, all remaining failures, and the unchanged keyboard/data behavior. Do not claim the whole suite passed if it did not. Do not run `go install` or push without the owner's approval.

## Plan Self-Review and Handoff

- Frame defaults, right labels, accent, bounded dimensions, unknown/zero sizes, multiline body and footer reserve: Task 1.
- Shared cursor/shortcut/marker/detail columns, ANSI/wide glyphs and paging stability: Task 2.
- Edit menus, current-value separation, input/editor sizing and guards: Task 3.
- Launcher and recipe descriptions, windowing and selected-row visibility: Task 4.
- Label/repository search, counts, toggles, paging and mouse geometry: Task 5.
- Confirmations, time travel, help, Options and alerts/notifications: Task 6.
- Rich legacy prompts, session/epic headers, nested boxes and BQL host: Task 7.
- Actual resize budgets, buffer preservation, unchanged compositor and authoring guide: Task 8.
- Safe full suite, ordinary-view exclusions, whole-branch review, tracking, commits and no push/install: Task 9.

Execution has not started. Recommended execution is **Native**: most migrations depend on the same layout and menu interfaces and touch shared model/view files, so one implementer avoids interface drift and overlapping edits. A fresh whole-branch reviewer still checks the final result. The owner must review this plan and choose Native or Subagent-driven before code changes.
