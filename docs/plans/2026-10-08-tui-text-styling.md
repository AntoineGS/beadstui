# Shared TUI Text Styling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize user-configurable TUI text formatting and make Actionable readable and correctly bounded while navigating.

**Architecture:** Extend the existing layered theme schema with typed text roles. Resolve and cache role styles from each Theme's own palette, propagate them through one theme-refresh path, and migrate standard text renderers while preserving domain-specific colors. Actionable shares one measured body layout between rendering and visibility calculations.

**Tech Stack:** Go, Bubble Tea v2, Lipgloss v2, yaml.v3, existing ANSI/display-cell helpers.

**Spec:** `docs/specs/2026-10-08-tui-text-styling.md`

## Global Constraints

- "This is an extension of the existing theme system, not a second configuration system or a redesign of the application's layout."
- "Partial overrides inherit unspecified values, and explicit `false` disables a previously enabled text attribute."
- "Explicit foreground choices are respected, even if a user deliberately chooses a low-contrast pair."
- "No TUI renderer introduces new hex literals."
- "Cosmetic configuration must not prevent startup. Unknown role names are ignored."
- "Malformed YAML retains the existing file-level fallback behavior."
- "No raw printing is added to the TUI rendering path."
- "Do not rebuild execution plans, reset cursors, clear filters, or change modal state merely to update styling."
- "Robot output and data loading are unchanged."
- "The vendored btop corpus is not edited and inherits the central role defaults."
- Go version remains `1.25.8` from `go.mod`; add no dependencies.
- Use existing files, no deletion, no scripts to rewrite code, no backwards-compatibility shims.
- Issue tracking is waived by the user; do not run recovery, install the binary, or push code.
- After each code-changing task run `go build ./...` and `go vet ./...`.
- Start implementation in an isolated worktree using `using-git-worktrees`; keep all unrelated changes intact.
- Do not dispatch agents until the user chooses an execution method that authorizes them. Reviews in this planning session are inline.

## Review Focus

Each condition below gets an executable regression in the owning task:

1. A typo or wrong YAML field type alongside valid overrides must discard only the bad field, not the whole valid role (Task 1).
2. Changing from a customized theme to an uncustomized theme must not leak stale attributes, palette colors, or cached subview styles (Tasks 2-3).
3. A terminal resize to one row with a recommendation and empty tracks must still show a selected issue when one exists (Task 4).
4. A selected row containing styled IDs, badge resets, and wide Unicode text must have a continuous highlight and respect the width budget (Tasks 4-5).
5. A user setting `bold: false` or `italic: false` must not have those preferences silently reapplied by popup, help, footer, or view renderers (Tasks 5-8).

## File and Interface Map

Edit files in place. Do not create a separate theme framework or move existing
large view files merely to reorganize them.

| Unit | Existing files | Responsibility |
|---|---|---|
| Role configuration | `pkg/ui/theme_loader.go`, `pkg/ui/defaults/theme.yaml`, `pkg/ui/theme_loader_test.go`, `pkg/ui/theme_persist_test.go` | Typed schema, tolerant field parsing, field-level merging, persistence contract |
| Role resolution | `pkg/ui/theme.go`, `pkg/ui/theme_loader.go`, `pkg/ui/styles.go`, `pkg/ui/theme_test.go`, `pkg/ui/styles_test.go` | Complete per-Theme palette, contrast and cached role styles |
| Theme lifecycle | `pkg/ui/model.go`, `pkg/ui/settings_modal.go`, `pkg/ui/theme_test.go`, `pkg/ui/settings_modal_test.go`, `pkg/ui/embedded_refresh_test.go` | One update path, retained theme/style refresh, no state reset |
| Actionable | `pkg/ui/actionable.go`, `pkg/ui/actionable_test.go` | Role adoption, measured chrome/body layout, safe selection and scrolling |
| Shared chrome/list/details | `pkg/ui/panel.go`, `pkg/ui/delegate.go`, `pkg/ui/model_view.go`, `pkg/ui/helpers.go`, `pkg/ui/model_filter.go`, `pkg/ui/peek_strip.go`, `pkg/ui/slot_render.go` and corresponding existing tests | Roles in reusable rendering paths and main list/details |
| Structural views | `pkg/ui/board.go`, `pkg/ui/graph.go`, `pkg/ui/tree.go`, `pkg/ui/epics_tree.go`, `pkg/ui/epic_card.go` and existing tests | Roles in board, graph, tree, and epics text |
| Analysis/history views | `pkg/ui/history.go`, `pkg/ui/insights.go`, `pkg/ui/model_alerts.go`, `pkg/ui/model_alerts_header.go`, `pkg/ui/label_dashboard.go`, `pkg/ui/velocity_comparison.go`, `pkg/ui/flow_matrix.go`, `pkg/ui/memories.go` and existing tests | Roles in ordinary view text; preserve metric/domain encodings |
| Supporting surfaces | `pkg/ui/model_footer.go`, `pkg/ui/footer_lens.go`, `pkg/ui/shortcuts_sidebar.go`, `pkg/ui/context_help.go`, `pkg/ui/tutorial.go`, `pkg/ui/tutorial_components.go`, `pkg/ui/settings_modal.go`, `pkg/ui/field_edit.go`, `pkg/ui/longform_edit.go`, `pkg/ui/claim.go`, `pkg/ui/update_modal.go`, `pkg/ui/bql_modal.go`, `pkg/ui/agent_prompt_modal.go`, `pkg/ui/cass_session_modal.go` and existing tests | Roles in footer, help, modal bodies, editable widgets |
| Final coverage/docs | `pkg/ui/render_harness_test.go`, `docs/specs/2026-10-08-tui-text-styling.md`, `pkg/ui/defaults/theme.yaml` | Whole-surface fixture coverage, documented exceptions and config examples |

The inventory command below currently finds 43 production files. Some contain
only domain-specific styling and will not need edits. Picker models use shared
popup helpers; their retained theme copies still need refreshing in Task 3.

```bash
rg -l 'lipgloss.NewStyle|\.theme\.Base|t\.Base' pkg/ui -g '*.go' -g '!**/*_test.go'
```

### Interfaces introduced by Tasks 1-3

```go
// theme_loader.go
type TextRoleConfig struct {
    Foreground string `yaml:"foreground"`
    Background string `yaml:"background"`
    Bold       *bool  `yaml:"bold"`
    Italic     *bool  `yaml:"italic"`
    Underline  *bool  `yaml:"underline"`
}
type TextRoleConfigs struct {
    Title    TextRoleConfig `yaml:"title"`
    Heading  TextRoleConfig `yaml:"heading"`
    Body     TextRoleConfig `yaml:"body"`
    Metadata TextRoleConfig `yaml:"metadata"`
    Badge    TextRoleConfig `yaml:"badge"`
    Selected TextRoleConfig `yaml:"selected"`
    Callout  TextRoleConfig `yaml:"callout"`
}
// Add Text TextRoleConfigs `yaml:"text"` to ThemeFile.
func (c *TextRoleConfig) UnmarshalYAML(n *yaml.Node) error
func defaultTextRoleConfigs() TextRoleConfigs
func validTextColorReference(value string, background bool) bool
func mergeTextRole(base *TextRoleConfig, overlay TextRoleConfig)
func mergeTextRoles(base *TextRoleConfigs, overlay TextRoleConfigs)

// theme.go / styles.go
type TextStyles struct {
    Title, Heading, Body, Metadata, Badge, Selected, Callout lipgloss.Style
}
// Add Text TextStyles to Theme, plus the missing scalar palette fields:
// Bg, BgDark, BgSubtle, BgHighlight, TextColor, TextSecondary, BgContrast.
func (t Theme) textPaletteColor(token string) color.Color
func (t *Theme) rebuildTextStyles(config TextRoleConfigs)
func autoTextForeground(background color.Color) color.Color
func textContrastRatio(foreground, background color.Color) float64
var ActiveTextStyles TextStyles // Existing global-only render helpers.

// model.go
func (m *Model) applyThemeConfig(tf *ThemeFile)
func (m *Model) refreshThemeConsumers()
```

Do not change these names independently in downstream tasks. Existing
`DefaultTheme`, `ApplyThemeToGlobals`, and `ApplyThemeToThemeStruct` remain the
actual constructors/application entry points; they use the same new role builder.
Rebuild existing `Base`, `Header`, and `Selected` styles while they have real
consumers; remove unused duplicates once migration eliminates their consumers.
Do not preserve obsolete fields solely as compatibility aliases.

## Implementation Tasks

### Task 1: Add typed role configuration and layered overrides

**Files:** Modify `pkg/ui/theme_loader.go`, `pkg/ui/defaults/theme.yaml`.
Test in `pkg/ui/theme_loader_test.go`, `pkg/ui/theme_persist_test.go`.

**Interfaces:** Consumes existing `ThemeFile`, `mergeTheme`, `loadThemeWith`,
`loadThemeFile`, and `SaveSelectedTheme`. Produces the configuration types and
helpers declared above. This task does not change rendering yet.

- [ ] **Step 1: Add parsing and partial-merge regression tests.** Add yaml.v3 to
  the existing test imports. Use a local boolean helper or the existing test
  `hptr` helper; do not add production helpers solely for tests.

```go
func TestTextRolesFieldOverrides(t *testing.T) {
    var overlay ThemeFile
    err := yaml.Unmarshal([]byte(`text:
  title: {foreground: auto, background: primary, bold: false}
  metadata: {italic: false, underline: true}
  nonexistent: {bold: true}
`), &overlay)
    if err != nil { t.Fatal(err) }
    base := &ThemeFile{Text: defaultTextRoleConfigs()}
    mergeTheme(base, &overlay)
    if base.Text.Title.Bold == nil || *base.Text.Title.Bold {
        t.Fatal("explicit false did not disable title bold")
    }
    if base.Text.Metadata.Underline == nil || !*base.Text.Metadata.Underline {
        t.Fatal("valid underline override was lost")
    }
    if base.Text.Body.Foreground != "text" {
        t.Fatal("unspecified body role did not inherit")
    }
}

func TestTextRolesInvalidFieldsAreIndependent(t *testing.T) {
    for _, invalid := range []string{
        "foreground: typo", "foreground: [text]", "background: auto",
        "bold: absolutely", "italic: [false]", "underline: {bad: true}",
    } {
        t.Run(invalid, func(t *testing.T) {
            var overlay ThemeFile
            input := "text:\n  title:\n    " + invalid + "\n    background: secondary\n"
            // Avoid duplicate background keys in the background case.
            if strings.HasPrefix(invalid, "background:") {
                input = "text:\n  title:\n    " + invalid + "\n    italic: true\n"
            }
            if err := yaml.Unmarshal([]byte(input), &overlay); err != nil { t.Fatal(err) }
            base := &ThemeFile{Text: defaultTextRoleConfigs()}
            mergeTheme(base, &overlay)
            if strings.HasPrefix(invalid, "background:") {
                if base.Text.Title.Background != "primary" || !*base.Text.Title.Italic {
                    t.Fatal("bad field erased a valid neighbor or inherited background")
                }
            } else if base.Text.Title.Background != "secondary" {
                t.Fatal("bad field erased the valid background")
            }
        })
    }
}
```

- [ ] **Step 2: Run the new tests and record the failure.**

```bash
go test ./pkg/ui -run 'TestTextRoles' -count=1
```

Expected: undefined role types/helpers before implementation. After adding the
types, confirm that explicit false/invalid-field behavior fails without the
merge/parser logic; do not treat compilation alone as the regression proof.

- [ ] **Step 3: Implement the schema, tolerant role parser, and merge helpers.**
  Use individual tagged fields in `TextRoleConfigs`. Implement
  `UnmarshalYAML` by walking a mapping node: accept scalar string references
  only when valid for that field; accept boolean attributes only when their
  node tag is `!!bool`. Ignore invalid values, unknown fields, null values,
  and non-mapping role nodes. Decode accepted nodes into locals and wrap any
  unexpected decode error with `fmt.Errorf("decode text role: %w", err)`.
  Syntactically malformed YAML still returns the existing file-level failure.
  Check node kind/tag before decoding so a bad attribute does not invalidate
  the surrounding file.

```go
func mergeTextRole(base *TextRoleConfig, overlay TextRoleConfig) {
    if validTextColorReference(overlay.Foreground, false) {
        base.Foreground = overlay.Foreground
    }
    if validTextColorReference(overlay.Background, true) {
        base.Background = overlay.Background
    }
    for _, pair := range []struct{ dst **bool; src *bool }{
        {&base.Bold, overlay.Bold}, {&base.Italic, overlay.Italic},
        {&base.Underline, overlay.Underline},
    } {
        if pair.src != nil {
            value := *pair.src
            *pair.dst = &value
        }
    }
}
```

Call `mergeTextRoles(&base.Text, overlay.Text)` from `mergeTheme`. Validation
also runs in merging so programmatically constructed configs are safe. Empty
references do not overwrite inherited values. Valid scalar token references
are exactly the spec's list, plus foreground `auto` and background `none`.
Default roles are exactly the spec's table. Write these defaults into the
embedded YAML and the shared Go fallback helper, documenting the mirror.

- [ ] **Step 4: Add full-stack precedence and persistence tests.** In
  `TestTextRolesLayerPrecedence`, use `t.Chdir(t.TempDir())`,
  `withThemeConfigHome(t)`, and `t.Setenv("BT_THEME", "dracula")`; write a user
  file with title `bold: false` and metadata `italic: true`, and a project
  `.bt/theme.yaml` with metadata `italic: false, underline: true`. Assert all
  four final fields and repeat with `LoadThemeNamed("matcha-dark-sea")`.
  No test in this group uses `t.Parallel` or the real home/config directory.

```go
func TestTextRolesLayerPrecedence(t *testing.T) {
    t.Chdir(t.TempDir())
    user := withThemeConfigHome(t)
    t.Setenv("BT_THEME", "dracula")
    if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil { t.Fatal(err) }
    if err := os.MkdirAll(".bt", 0o755); err != nil { t.Fatal(err) }
    if err := os.WriteFile(user, []byte("text:\n  title: {bold: false}\n  metadata: {italic: true}\n"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(".bt/theme.yaml", []byte("text:\n  metadata: {italic: false, underline: true}\n"), 0o644); err != nil { t.Fatal(err) }
    for _, tf := range []*ThemeFile{LoadTheme(), LoadThemeNamed("matcha-dark-sea")} {
        if *tf.Text.Title.Bold || *tf.Text.Metadata.Italic || !*tf.Text.Metadata.Underline {
            t.Fatal("named/user/project precedence lost false or inherited fields")
        }
        if tf.Text.Body.Foreground != "text" { t.Fatal("embedded role was lost") }
    }
}

func TestSaveThemePreservesTextRoles(t *testing.T) {
    path := withThemeConfigHome(t)
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
    original := "# my text preferences\ntext:\n  metadata:\n    # supporting text is plain\n    italic: false\n    foreground: text\ntheme: dracula\n"
    if err := os.WriteFile(path, []byte(original), 0o644); err != nil { t.Fatal(err) }
    if err := SaveSelectedTheme("loam"); err != nil { t.Fatal(err) }
    saved, err := os.ReadFile(path)
    if err != nil { t.Fatal(err) }
    for _, want := range []string{"my text preferences", "supporting text is plain", "italic: false", "foreground: text"} {
        if !strings.Contains(string(saved), want) { t.Fatalf("lost %q", want) }
    }
    if strings.Index(string(saved), "text:") > strings.Index(string(saved), "theme:") {
        t.Fatal("save reordered text and theme keys")
    }
    var parsed ThemeFile
    if err := yaml.Unmarshal(saved, &parsed); err != nil { t.Fatal(err) }
    if parsed.Text.Metadata.Italic == nil || *parsed.Text.Metadata.Italic || parsed.Text.Metadata.Foreground != "text" {
        t.Fatal("saved role changed")
    }
}
```

- [ ] **Step 5: Verify and commit this deliverable.**

```bash
go test ./pkg/ui -run 'TestTextRoles|TestMergeTheme|TestLoadTheme|TestSaveTheme' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/theme_loader.go pkg/ui/defaults/theme.yaml pkg/ui/theme_loader_test.go pkg/ui/theme_persist_test.go -m "feat(tui): add configurable semantic text roles"
```

### Task 2: Resolve and cache readable styles from each Theme's own palette

**Files:** Modify `pkg/ui/theme.go`, `pkg/ui/theme_loader.go`, `pkg/ui/styles.go`.
Test in `pkg/ui/theme_test.go`, `pkg/ui/styles_test.go`.

**Interfaces:** Consumes `TextRoleConfigs` and defaults from Task 1. Produces
`Theme.Text`, the complete scalar palette fields, `textPaletteColor`,
`rebuildTextStyles`, contrast helpers, and `ActiveTextStyles`.

- [ ] **Step 1: Add stale-base and auto-contrast regressions.**

```go
func TestTextStylesUseOwnPalette(t *testing.T) {
    restoreThemeGlobals(t)
    previous := isDarkBackground
    isDarkBackground = true
    t.Cleanup(func() { isDarkBackground = previous })
    a := DefaultTheme()
    ApplyThemeToThemeStruct(&a, &ThemeFile{Colors: ThemeColors{
        Text: &AdaptiveHex{Dark: "#123456"},
        Primary: &AdaptiveHex{Dark: "#ffccdd"},
    }})
    ApplyThemeToGlobals(&ThemeFile{Colors: ThemeColors{
        Text: &AdaptiveHex{Dark: "#abcdef"},
    }})
    if a.Text.Body.GetForeground() != lipgloss.Color("#123456") {
        t.Fatal("body style borrowed the global palette")
    }
    if a.Base.GetForeground() != a.Text.Body.GetForeground() {
        t.Fatal("base text stayed on fallback colors")
    }
    if ratio := textContrastRatio(a.Text.Title.GetForeground(), a.Text.Title.GetBackground()); ratio < 4.5 {
        t.Fatalf("filled title contrast %.2f < 4.5", ratio)
    }
}

func TestAutoTextForeground(t *testing.T) {
    for _, bg := range []string{"#000000", "#ffffff", "#777777", "#bd93f9", "#ffb8d1"} {
        color := lipgloss.Color(bg)
        fg := autoTextForeground(color)
        if ratio := textContrastRatio(fg, color); ratio < 4.5 {
            t.Fatalf("background %s: contrast %.2f", bg, ratio)
        }
    }
}
```

- [ ] **Step 2: Run and observe failure.**

```bash
go test ./pkg/ui -run 'TestTextStyles|TestAutoTextForeground' -count=1
```

Expected: missing style interface or stale/low-contrast style assertions.

- [ ] **Step 3: Implement the palette fields and role builder.** Resolve all
  scalar palette fields through `Theme` defaults and the loader. Build fresh
  Lipgloss styles instead of mutating an old theme's attributes. Start from
  `defaultTextRoleConfigs()` each time, then merge the supplied config. `none`
  produces `lipgloss.NoColor{}` as the role background. `auto` uses that role's
  concrete background or `t.Bg` when its background is `none`.

```go
// styles.go: luminance for an opaque RGB palette color.
channel := func(v uint32) float64 {
    x := float64(v) / 65535
    if x <= 0.04045 { return x / 12.92 }
    return math.Pow((x+0.055)/1.055, 2.4)
}
// For each color: L = .2126*channel(r) + .7152*channel(g) + .0722*channel(b).
// Contrast: (max(L1,L2)+.05)/(min(L1,L2)+.05).
// autoTextForeground selects the higher-contrast black/white color.
// Handle nil/NoColor at the builder boundary using the Theme's concrete Bg;
// never dereference nil or use an invisible foreground.
```

`DefaultTheme` and `ApplyThemeToThemeStruct` invoke `rebuildTextStyles`. Rebuild
existing base/header/selection and precomputed delegate styles from Theme
fields only. `ApplyThemeToGlobals` updates `ActiveTextStyles` through a local
Theme built from the same config, never by invoking a helper that reads the
last active global palette. Keep the module's existing limited-terminal
color-profile helpers and do not make new terminal capability probes.

- [ ] **Step 4: Extend tests over named palettes and attribute clearing.**
  Iterate `dracula`, `matcha-dark-sea`, `paper`, and `bt:greyscale` through
  `LoadThemeNamed` for both `isDarkBackground` values, isolating the user/home
  and project directory. Check all four filled roles for >=4.5 contrast and
  their resolved backgrounds. Test an explicit foreground `secondary` is not
  auto-corrected, every role's false attributes stay false, and rebuilding a
  customized theme from a config without attributes clears prior attributes.
  In `styles_test.go`, test black/white ratios (21:1), equal-color ratios
  (1:1), and representative intermediate ratios using a tolerance.

- [ ] **Step 5: Verify and commit.**

```bash
go test ./pkg/ui -run 'TestTextStyles|TestAutoTextForeground|TestDefaultTheme|TestTheme|TestApplyTheme|TestTextContrast' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/theme.go pkg/ui/theme_loader.go pkg/ui/styles.go pkg/ui/theme_test.go pkg/ui/styles_test.go -m "fix(tui): resolve readable text roles from the active palette"
```

### Task 3: Unify theme refresh without resetting retained UI state

**Files:** Modify `pkg/ui/model.go`, `pkg/ui/settings_modal.go`.
Test in `pkg/ui/theme_test.go`, `pkg/ui/settings_modal_test.go`,
`pkg/ui/embedded_refresh_test.go`.

**Interfaces:** Consumes resolved `Theme.Text` from Task 2. Produces
`Model.applyThemeConfig(*ThemeFile)` and `Model.refreshThemeConsumers()`.
Existing `applyThemeLive(name string)` becomes a loader plus this shared path.

- [ ] **Step 1: Pin the retained-view refresh contract.**

```go
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
    m.applyThemeLive("dracula")
    if m.actionableView.SelectedIssueID() != "second" { t.Fatal("selection reset") }
    if m.actionableView.theme.Text.Body.GetForeground() != m.theme.Text.Body.GetForeground() {
        t.Fatal("retained Actionable theme is stale")
    }
    if m.renderer.width != 80 { t.Fatal("theme refresh changed markdown wrap width") }
}
```

- [ ] **Step 2: Run the test before implementing refresh.**

```bash
go test ./pkg/ui -run TestThemeRefreshRetainsActionableState -count=1
```

Expected: stale Actionable theme. The test uses only fixtures; it must not need
the missing beads database.

- [ ] **Step 3: Implement and route the shared update path.** Reset global
  palette defaults with `resolveColors`, apply the config to globals, create a
  fresh `DefaultTheme`, apply the config to it, then refresh consumers. Startup
  uses the same theme construction sequence before component initialization;
  startup's initialized components receive that Theme and can use the refresh
  helper after Model construction. The background-color handler sets
  `isDarkBackground` before calling `applyThemeConfig(LoadTheme())`.

```go
func (m *Model) applyThemeConfig(tf *ThemeFile) {
    resolveColors()
    ApplyThemeToGlobals(tf)
    m.theme = DefaultTheme()
    ApplyThemeToThemeStruct(&m.theme, tf)
    m.refreshThemeConsumers()
}
```

`refreshThemeConsumers` explicitly updates these retained Theme copies:
`board`, `labelDashboard`, `velocityComparison`, `shortcutsSidebar`,
`graphView`, `tree`, `insightsPanel`, `flowMatrix`, `actionableView`,
`historyView`, `memories`, `recipePicker`, `bqlQuery`, `labelPicker`,
`repoPicker`, `agentPromptModal`, `tutorialModel`, `cassModal`, `updateModal`,
`fieldSelect`, `fieldPicker`, `fieldInput`, and `longformEdit`.
Use existing `SetTheme` methods for `settingsModal`, `settingsMenu`, and
`epicsTree`. Same-package assignment is enough for simple Theme copies; do
not invent a interface/registry merely to set fields.

Also rebuild the list delegate with `updateListDelegate`, set the filter/input
widget role styles, recreate the markdown renderer using its current positive
`width` (not a hardcoded 80), and invalidate/repaint text-only caches, including
`epicsViewText` and current viewport content, preserving viewport scroll.
Locate any other cached rendered strings/style fields in the listed models
and rebuild only their presentation cache. For a background-mode change,
recreating the markdown renderer is necessary to update its `isDark` flag.
Never call issue-loading, graph-analysis, or plan-building functions here.

- [ ] **Step 4: Add the no-reset and stale-style matrix tests.** Set distinctive
  colors/attributes and use a table of closures reading each retained model's
  role styles; require every one to equal the main Theme after applying two
  different configs. Preserve list index/filter, actionable track/item/offset,
  sidebar scroll, field-input text, textarea draft/cursor, active modal, and
  cached epics content's selected issue. Send `tea.BackgroundColorMsg` through
  `Update`; require consumers and `renderer.IsDarkMode()` to agree. Verify
  canceling the theme picker restores both role styles and palette fields.
  Extend the existing `TestThemeSwapMidSession_NoRace` rather than creating a
  second async theme-storm harness.

```go
func TestThemeRefreshAllRetainedConsumers(t *testing.T) {
    restoreThemeGlobals(t)
    withThemeConfigHome(t)
    m := settingsTestModel(t)
    tf := &ThemeFile{Text: TextRoleConfigs{Body: TextRoleConfig{
        Foreground: "danger", Underline: hptr(true),
    }}}
    m.applyThemeConfig(tf)
    consumers := []Theme{
        m.board.theme, m.graphView.theme, m.tree.theme, m.actionableView.theme,
        m.historyView.theme, m.memories.theme, m.labelDashboard.theme,
        m.velocityComparison.theme, m.flowMatrix.theme, m.insightsPanel.theme,
        m.shortcutsSidebar.theme, m.recipePicker.theme, m.repoPicker.theme,
        m.labelPicker.theme, m.bqlQuery.theme, m.agentPromptModal.theme,
        m.tutorialModel.theme, m.cassModal.theme, m.updateModal.theme,
        m.fieldSelect.theme, m.fieldPicker.theme, m.fieldInput.theme,
        m.longformEdit.theme, m.settingsModal.theme, m.settingsMenu.theme,
        m.epicsTree.theme,
    }
    for i, theme := range consumers {
        if theme.Text.Body.GetForeground() != m.theme.Text.Body.GetForeground() || !theme.Text.Body.GetUnderline() {
            t.Fatalf("consumer %d did not refresh", i)
        }
    }
    m.applyThemeConfig(&ThemeFile{})
    if m.actionableView.theme.Text.Body.GetUnderline() {
        t.Fatal("old role attributes leaked into the next theme")
    }
}
```

- [ ] **Step 5: Verify and commit.**

```bash
go test ./pkg/ui -run 'TestThemeRefresh|TestThemeSwap|TestSettings|TestEmbedded' -count=1
go test ./pkg/ui -race -run 'TestThemeRefresh|TestThemeSwapMidSession' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/model.go pkg/ui/settings_modal.go pkg/ui/theme_test.go pkg/ui/settings_modal_test.go pkg/ui/embedded_refresh_test.go -m "fix(tui): refresh retained text styles on theme changes"
```

### Task 4: Make Actionable readable, bounded, and correctly scrollable

**Files:** Modify `pkg/ui/actionable.go`, `pkg/ui/actionable_test.go`.

**Interfaces:** Consumes `Theme.Text`. Introduces internal
`actionableLayout{chrome []string, body []string, selectedStart int,
selectedEnd int, bodyHeight int}` and
`func (m *ActionableModel) layout() actionableLayout`.
Also add `func (m *ActionableModel) normalizeSelection()` for guarded movement
and layout entry; it selects the nearest valid nonempty track or leaves a
documented no-selection state when all tracks are empty.
Selection bounds are body-line indices; `selectedEnd` is exclusive and -1
start/end means there is no selectable item.

- [ ] **Step 1: Add visibility and bounds regressions.**

```go
func TestActionableSelectedItemVisibleAfterNavigation(t *testing.T) {
    plan := analysis.ExecutionPlan{Summary: analysis.PlanSummary{
        HighestImpact: "issue-00", ImpactReason: "Unblocks work", UnblocksCount: 2,
    }}
    for i := 0; i < 24; i++ {
        plan.Tracks = append(plan.Tracks, analysis.ExecutionTrack{
            TrackID: fmt.Sprintf("track-%02d", i), Reason: "Single actionable item",
            Items: []analysis.PlanItem{{ID: fmt.Sprintf("issue-%02d", i), Title: "Readable title", UnblocksIDs: []string{"next"}}},
        })
    }
    m := NewActionableModel(plan, DefaultTheme())
    m.SetSize(64, 9)
    for i := 0; i < 24; i++ {
        plain := ansi.Strip(m.Render())
        if !strings.Contains(plain, m.SelectedIssueID()) {
            t.Fatalf("selected item %s not visible:\n%s", m.SelectedIssueID(), plain)
        }
        if lipgloss.Height(plain) > 9 { t.Fatal("height exceeded") }
        for _, line := range strings.Split(plain, "\n") {
            if ansi.StringWidth(line) > 64 { t.Fatal("width exceeded") }
        }
        m.MoveDown()
    }
}
```

Add table cases for `(width,height)` values `(-1,8)`, `(0,8)`, `(8,0)`,
`(1,1)`, `(12,1)`, `(20,2)`, `(40,6)`, and `(80,20)`. For invalid sizes
expect empty output; for positive sizes assert bounds. Use a single selected
issue ID `x` to assert selection is visible at width 12/height 1.

- [ ] **Step 2: Run before changing the layout.**

```bash
go test ./pkg/ui -run 'TestActionableSelectedItemVisible|TestActionableBounds' -count=1
```

Expected: selection disappears during navigation and negative/tiny sizes do
not meet the bounds contract.

- [ ] **Step 3: Replace independent estimates with the shared layout.** Build
  single-line chrome and body rows with role styles. Include both track
  header/divider and selected unblock-detail rows in `body`. Reserve at least
  one body row when an item exists, dropping spacers, then recommendation,
  then title to fit. Clamp/normalize selection to a nonempty track;
  `SelectedIssueID`, MoveUp/Down, and page movement must skip empty tracks and
  guard both negative and upper bounds. `SetSize` rechecks visibility.

```go
// Core visibility rule, using measured layout:
if l.selectedStart >= 0 && l.bodyHeight > 0 {
    m.scrollOffset = min(m.scrollOffset, l.selectedStart)
    end := l.selectedEnd
    if end-l.selectedStart > l.bodyHeight { end = l.selectedStart + 1 }
    m.scrollOffset = max(m.scrollOffset, end-l.bodyHeight)
}
m.scrollOffset = min(max(0, m.scrollOffset), max(0, len(l.body)-l.bodyHeight))
```

Construct ordinary selected-row text without nested conflicting styles and
render it once through `t.Text.Selected.Width(width)`. Unselected ID/title
use Metadata/Body. Keep independently colored domain chips only if they
retain self-contained foreground/background pairs. Recommendation uses
Callout; title uses Title; track labels use Badge; reasons/unblock detail use
Metadata. Dividers remain border-semantic rather than text.

Allocate title columns as `max(0, width-prefixCells-suffixCells)`. Use
`ansi.StringWidth` and `ansi.Truncate`, and sanitize single-row text with
`popupChromeLine` to prevent embedded newlines/tabs/control styling from
creating unexpected rows. Fit the prefix itself before allocating remaining
title space. Never use a negative Width, Padding, slice, or Repeat argument.
Use measured body capacity for page navigation rather than raw terminal height.

- [ ] **Step 4: Add contrast, Unicode, and resizing tests.** Verify rendered
  title/badge styles match the configured roles with distinct sentinel colors
  and false attributes. Fixtures include long IDs/titles, `界面 e\u0301`,
  multiline titles, very long recommendation text, leading/middle/trailing
  empty tracks, no tracks, and all-empty tracks. Exercise MoveUp/Down and
  PageUp/Down after shrinking from 20 rows to 1 and growing again. Use
  `SelectedIssueID()` plus stripped visible text for navigation assertions,
  rather than only checking internal offsets. Assert wide/combining title text
  uses the available cells and does not lose 20 arbitrary columns.

- [ ] **Step 5: Verify and commit.**

```bash
go test ./pkg/ui -run TestActionable -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/actionable.go pkg/ui/actionable_test.go -m "fix(tui): make actionable text readable and scrolling accurate"
```

### Task 5: Adopt roles in shared panels, popup menus, issues, and details

**Files:** Modify `pkg/ui/panel.go`, `pkg/ui/delegate.go`,
`pkg/ui/model_view.go`, `pkg/ui/helpers.go`, `pkg/ui/model_filter.go`,
`pkg/ui/peek_strip.go`, `pkg/ui/slot_render.go`.
Test in `pkg/ui/panel_test.go`, `pkg/ui/delegate_test.go`,
`pkg/ui/detail_sections_test.go`, `pkg/ui/filter_preservation_test.go`,
`pkg/ui/peek_strip_test.go`, `pkg/ui/slot_render_test.go`.

**Interfaces:** Consumes `Theme.Text` and `ActiveTextStyles`.
`RenderTitledPanel` keeps its signature; its default text style uses the
active Heading role. Add `TitleStyle *lipgloss.Style` to `PanelOpts` so a
popup can pass its own Theme's Heading role without reading globals.
Explicit semantic `TitleColor` still takes precedence for domain-colored
panels, but no helper unconditionally reapplies Bold/Italic.

- [ ] **Step 1: Add a helper-renderer role override regression.**

```go
func TestPopupMenuRespectsTextAttributes(t *testing.T) {
    theme := DefaultTheme()
    theme.Text.Body = lipgloss.NewStyle().Underline(true)
    theme.Text.Metadata = lipgloss.NewStyle().Italic(false).Underline(true)
    theme.Text.Selected = lipgloss.NewStyle().Bold(false).Underline(true)
    entries := []PopupMenuEntry{{Label: "Choice", Detail: "Explanation", Selected: true}}
    layout := MeasurePopupMenu(entries, PopupMenuOpts{})
    got := strings.Join(RenderPopupMenu(entries, layout, theme, 40), "\n")
    want := theme.Text.Selected.Render("Choice")
    if !strings.Contains(got, want) { t.Fatal("selected menu label ignored its role") }
    if strings.Contains(got, "\x1b[3m") { t.Fatal("helper forced italic metadata") }
}
```

Add panel title tests with a supplied
Heading style where bold is false and underline is true, for focused and
unfocused panels. Keep the existing width/focus/ANSI-title tests.

- [ ] **Step 2: Run the new tests before migration.**

```bash
go test ./pkg/ui -run 'TestPopupMenuRespectsTextAttributes|TestPanelTextRole' -count=1
```

Expected: menu labels force bold and details force italic; panel title ignores
configured attributes.

- [ ] **Step 3: Migrate shared recipes, then their immediate consumers.**
  Map popup footer/detail hints to Metadata, labels to Body/Heading, selected
  entries to Selected, view title bars to Title, list column headings to
  Heading, IDs/supporting detail values to Metadata, issue titles to Body.
  Reuse `opts.Theme.Text.Heading` via `PanelOpts.TitleStyle` in `RenderPopup`.
  Retain border weight/color and popup geometry. Component padding/alignment
  is applied after the text role, without changing attributes.

```go
headerStyle := m.theme.Text.Heading.Width(m.list.Width())
titleStyle := t.Text.Body
idStyle := t.Text.Metadata
// Preserve the existing full-row selection mechanism in IssueDelegate:
if isSelected {
    row = t.Text.Selected.Width(width).MaxWidth(width).Render(ansi.Strip(row))
}
// The existing strip approach is permitted for the list's classic coherent
// selection; it must keep chip labels/glyphs intact even without chip colors.
```

Do not change slot providers' badge meanings or add new row badges outside
`pkg/ui/slots`. Domain-specific disabled/wisp, search-match, diff, and status
color treatments can remain explicit, but start from configured attributes
and document their semantic exceptions. A wisp must not force italic when
the user explicitly disabled it; distinguish the state with its marker and
existing semantic dim-color treatment instead.

- [ ] **Step 4: Add selected-row and detail regressions.** In
  `delegate_test.go`, render a selected issue with a long ID, a wide title,
  triage/gate/slot badges, and a false-bold Selected role. Verify width,
  complete selected background, rendered false-bold role, and marker retention.
  In `detail_sections_test.go`, use a sentinel Heading/Metadata role and
  assert non-markdown field labels/values use it, while dependency/status
  styling remains semantic. Re-run filter-preservation tests so text migration
  cannot reintroduce search-row/header shifts.

- [ ] **Step 5: Verify and commit the shared surfaces.**

```bash
go test ./pkg/ui -run 'TestPopup|TestPanel|TestDelegate|TestDetail|TestFilter|TestPeek|TestSlot' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/panel.go pkg/ui/delegate.go pkg/ui/model_view.go pkg/ui/helpers.go pkg/ui/model_filter.go pkg/ui/peek_strip.go pkg/ui/slot_render.go pkg/ui/panel_test.go pkg/ui/delegate_test.go pkg/ui/detail_sections_test.go pkg/ui/filter_preservation_test.go pkg/ui/peek_strip_test.go pkg/ui/slot_render_test.go -m "refactor(tui): use shared text roles in panels and issue surfaces"
```

### Task 6: Adopt roles in board, graph, tree, and epics

**Files:** Modify `pkg/ui/board.go`, `pkg/ui/graph.go`, `pkg/ui/tree.go`,
`pkg/ui/epics_tree.go`, `pkg/ui/epic_card.go`.
Test in `pkg/ui/board_test.go`, `pkg/ui/graph_internal_test.go`,
`pkg/ui/tree_test.go`, `pkg/ui/epics_tree_test.go`, `pkg/ui/epic_card_test.go`.

**Interfaces:** Consumes the role styles and shared panel helpers. No new
cross-task API. External `ui_test` fixtures use exported `Theme.Text`.

- [ ] **Step 1: Add an ordinary-text override test for each view.** In the
  external board test use `ui.Theme`; in internal tests use `Theme`. Apply
  Body/Metadata styles with unique sentinel colors and Underline true, Heading
  Bold false, and Selected Bold false. Include a fixture with two issues so
  one ordinary row is unselected and reveals the Body role.

```go
// board_test.go, package ui_test
func TestBoardBodyTextRole(t *testing.T) {
    theme := ui.DefaultTheme()
    theme.Text.Body = lipgloss.NewStyle().Foreground(lipgloss.Color("#123456")).Underline(true)
    issues := []model.Issue{
        {ID: "one", Title: "Selected", Status: model.StatusOpen},
        {ID: "two", Title: "Ordinary body", Status: model.StatusOpen},
    }
    board := ui.NewBoardModel(issues, theme)
    out := board.View(120, 30)
    if !strings.Contains(out, theme.Text.Body.Render("Ordinary body")) {
        t.Fatal("board body still uses a local recipe")
    }
}
```

Add imports to the existing test file, not another board test variant. For
graph/tree/epics/card, add the same sentinel-role check to existing populated
fixtures and their own View/render method tests; keep their navigation checks.

- [ ] **Step 2: Run only the new role tests and verify failures.**

```bash
go test ./pkg/ui -run 'TestBoardBodyTextRole|TestGraphTextRole|TestTreeTextRole|TestEpicsTextRole|TestEpicCardTextRole' -count=1
```

- [ ] **Step 3: Migrate ordinary text without changing structure.** Replace
  title bars with Title, border/column/section text with Heading, ordinary
  titles with Body, IDs/reasons/age/hints with Metadata, general labels with
  Badge, and selection with Selected. Pass per-model Heading styles to shared
  panels. Preserve graph node/edge colors, board status-column identity,
  progress geometry, folded-tree markers, and priority/status chips.

```go
titleStyle := t.Text.Body
metadataStyle := t.Text.Metadata
headingStyle := t.Text.Heading
if selected { titleStyle = t.Text.Selected }
// Width, borders, spacing and tree connectors stay in each view's layout.
```

- [ ] **Step 4: Verify false attributes and existing behavior.** Add one
  unselected/selected false-bold assertion per view. Ensure progress and
  state chip labels survive. Run graph ASCII goldens; update a golden only
  when a reviewed text/layout change requires it, not to mask a regression.

- [ ] **Step 5: Run the view tests, build/vet, and commit exactly the files
  changed in this task.**

```bash
go test ./pkg/ui -run 'TestBoard|TestGraph|TestTree|TestEpics|TestEpicCard' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/board.go pkg/ui/graph.go pkg/ui/tree.go pkg/ui/epics_tree.go pkg/ui/epic_card.go pkg/ui/board_test.go pkg/ui/graph_internal_test.go pkg/ui/tree_test.go pkg/ui/epics_tree_test.go pkg/ui/epic_card_test.go -m "refactor(tui): share text formatting across structural views"
```

### Task 7: Adopt roles in analysis, alerts, history, and supporting dashboards

**Files:** Modify `pkg/ui/history.go`, `pkg/ui/insights.go`,
`pkg/ui/model_alerts.go`, `pkg/ui/model_alerts_header.go`,
`pkg/ui/label_dashboard.go`, `pkg/ui/velocity_comparison.go`,
`pkg/ui/flow_matrix.go`, `pkg/ui/memories.go`.
Test in `pkg/ui/history_test.go`, `pkg/ui/insights_test.go`,
`pkg/ui/model_alerts_test.go`, `pkg/ui/model_alerts_header_test.go`,
`pkg/ui/label_dashboard_test.go`, `pkg/ui/velocity_comparison_test.go`,
`pkg/ui/flow_matrix_test.go`, `pkg/ui/memories_test.go`.

**Interfaces:** Consumes cached roles/shared panel styles. No data model,
correlation, analysis, or historical data API changes.

- [ ] **Step 1: Add title/heading/body/metadata override tests to each existing
  view fixture.** The following dashboard test is the minimum executable
  example; use populated fixture rows for body/metadata checks in history,
  insights, alerts, flow, velocity, and memories.

```go
func TestLabelDashboardHeadingTextRole(t *testing.T) {
    theme := DefaultTheme()
    theme.Text.Title = lipgloss.NewStyle().Bold(false)
    theme.Text.Heading = lipgloss.NewStyle().Underline(true).Bold(false)
    m := NewLabelDashboardModel(theme)
    m.SetSize(120, 20)
    m.SetData([]analysis.LabelHealth{{Label: "example", Health: 90,
        HealthLevel: analysis.HealthLevelHealthy}})
    out := m.View()
    header := strings.Split(out, "\n")[0]
    if header != theme.Text.Heading.Render(ansi.Strip(header)) {
        t.Fatal("dashboard column heading ignored its configured role")
    }
}
```

For every fixture assert the specific label/title's styled substring, not only
that an unrelated glyph anywhere has underline. In `history_test.go` use
`createTestHistoryReport`, not a new database/git fixture.

- [ ] **Step 2: Run new override tests and record failures.**

```bash
go test ./pkg/ui -run 'Test.*TextRole' -count=1
```

Only the newly introduced tests for this task should fail; earlier role tests
must remain green.

- [ ] **Step 3: Replace ordinary text recipes in these views.** Keep metric
  score colors, severity labels, trend arrows, heatmap gradients, correlation
  confidence, and commit/diff meaning as semantic exceptions. Remove hardcoded
  italic from ordinary reasons/relative times/hints unless represented by the
  configured role. Header backgrounds use the paired Title role rather than
  body-on-accent styling. Read all styles through the view's own Theme.

```go
reasonStyle := t.Text.Metadata
headingStyle := t.Text.Heading
rowStyle := t.Text.Body
if selected { rowStyle = t.Text.Selected }
// Example domain exception: a warning marker retains t.Warning, but the
// adjacent explanation inherits Metadata's attributes and foreground.
```

- [ ] **Step 4: Verify semantic indicators and attribute preferences.** Add
  tests with plain/false attributes and require explanation/time text to remain
  plain, while severity/trend labels and graph-derived numbers remain present.
  Re-run history focus/selection/mouse tests and alerts header/tabs tests;
  styling changes must not alter hit-testing, paging, or selection retention.

- [ ] **Step 5: Verify and commit.**

```bash
go test ./pkg/ui -run 'TestHistory|TestInsights|TestAlert|TestNotifications|TestLabelDashboard|TestVelocity|TestFlow|TestMemories|TestModalTab' -count=1
go build ./...
go vet ./...
git diff --check
git commit --only pkg/ui/history.go pkg/ui/insights.go pkg/ui/model_alerts.go pkg/ui/model_alerts_header.go pkg/ui/label_dashboard.go pkg/ui/velocity_comparison.go pkg/ui/flow_matrix.go pkg/ui/memories.go pkg/ui/history_test.go pkg/ui/insights_test.go pkg/ui/model_alerts_test.go pkg/ui/model_alerts_header_test.go pkg/ui/label_dashboard_test.go pkg/ui/velocity_comparison_test.go pkg/ui/flow_matrix_test.go pkg/ui/memories_test.go -m "refactor(tui): share text formatting across analysis and history views"
```

### Task 8: Adopt roles in footer, help, settings, and modal bodies

**Files:** Modify `pkg/ui/model_footer.go`, `pkg/ui/footer_lens.go`,
`pkg/ui/shortcuts_sidebar.go`, `pkg/ui/context_help.go`, `pkg/ui/tutorial.go`,
`pkg/ui/tutorial_components.go`, `pkg/ui/settings_modal.go`,
`pkg/ui/field_edit.go`, `pkg/ui/longform_edit.go`, `pkg/ui/claim.go`,
`pkg/ui/update_modal.go`, `pkg/ui/bql_modal.go`,
`pkg/ui/agent_prompt_modal.go`, `pkg/ui/cass_session_modal.go`.
Test in `pkg/ui/model_footer_test.go`, `pkg/ui/footer_lens_test.go`,
`pkg/ui/shortcuts_sidebar_test.go`, `pkg/ui/context_help_test.go`,
`pkg/ui/tutorial_test.go`, `pkg/ui/settings_modal_test.go`,
`pkg/ui/field_edit_test.go`, `pkg/ui/longform_edit_test.go`,
`pkg/ui/claim_test.go`, `pkg/ui/update_modal_test.go`,
`pkg/ui/agent_prompt_modal_test.go`, `pkg/ui/cass_session_modal_test.go`,
`pkg/ui/coverage_extra_test.go` (for the existing BQL modal fixture coverage).

**Interfaces:** Consumes shared text roles and popup helpers. Refresh editable
widget styles in the shared lifecycle path from Task 3; do not recreate the
textinput/textarea model or lose buffer/cursor state.

- [ ] **Step 1: Add a tutorial/help plain-text override regression and modal
  label assertions.**

```go
func TestTutorialElementsRespectTextRoles(t *testing.T) {
    theme := DefaultTheme()
    theme.Text.Body = lipgloss.NewStyle().Underline(true)
    theme.Text.Heading = lipgloss.NewStyle().Bold(false)
    theme.Text.Metadata = lipgloss.NewStyle().Italic(false)
    out := (Paragraph{Text: "Body text"}).Render(theme, 40)
    if out != theme.Text.Body.Render(ansi.Strip(out)) {
        t.Fatal("paragraph ignored body role")
    }
    heading := (Section{Title: "Section text"}).Render(theme, 40)
    if strings.Contains(heading, "\x1b[1m") { t.Fatal("section forced bold") }
}
```

Add equivalent specific-label sentinel assertions to populated sidebar,
footer, settings, field-edit, claim, update, agent prompt, Cass, and BQL modal
fixtures. Footer tests must use their existing centered-total probes and
unchanged dimensions. No title/body formatting test needs to invoke `bd`.

- [ ] **Step 2: Run new tests and verify they fail before changing recipes.**

```bash
go test ./pkg/ui -run 'TestTutorialElementsRespectTextRoles|Test.*TextRole' -count=1
```

- [ ] **Step 3: Migrate ordinary supporting text and widget styles.** Footer
  labels/hints use Metadata, ordinary values use Body, grouping labels use
  Heading, general workspace labels use Badge, selections use Selected, and
  informational callouts use Callout. Tutorial diagrams, code samples,
  warning severity, and progress indicators retain their semantic systems.

```go
labelStyle := t.Text.Heading
bodyStyle := t.Text.Body
hintStyle := t.Text.Metadata
// Textinput and textarea text/prompt styles start from the corresponding
// cached role. Keep their geometry, value, selection and cursor intact.
```

Shared popup helpers already style picker menus; inspect
`recipe_picker.go`, `repo_picker.go`, `label_picker.go`, and
`settings_menu.go` for remaining local overrides. `recipe_picker.go` has a
local italic hint that must become Metadata. Include that file and its
existing test in this task's commit if changed. Do not create setters for
models whose simple same-package Theme assignment already satisfies Task 3.

- [ ] **Step 4: Verify disabled/false attributes and unsaved drafts.** A user
  disabling all attributes must get plain hints and section labels, not
  hardcoded italics/bold. Changing palettes with an open field input/textarea
  must preserve the exact unsaved content and cursor, update styles, and leave
  save/cancel/dirty-guard behavior unchanged. Re-run popup bounds and modal
  mouse tests to prevent styling from changing geometry.

- [ ] **Step 5: Verify and commit only this task's changed files.**

```bash
go test ./pkg/ui -run 'TestFooter|TestShortcuts|TestContextHelp|TestTutorial|TestSettings|TestField|TestLongform|TestClaim|TestUpdate|TestAgentPrompt|TestCass|TestBQL|TestModalMouse|Test.*TextRole' -count=1
go build ./...
go vet ./...
git diff --check
```

Use `git commit --only` with the exact changed paths from the task's Files
list (and `recipe_picker.go`/`recipe_picker_test.go` if its local hint changed).
Commit message: `refactor(tui): share text formatting across supporting surfaces`.

### Task 9: Close the migration inventory, document configuration, and verify

**Files:** Modify `pkg/ui/render_harness_test.go`,
`pkg/ui/theme_loader_test.go`, `docs/specs/2026-10-08-tui-text-styling.md`,
`pkg/ui/defaults/theme.yaml`.
Any production edit here belongs in its owning earlier task with that task's
regression tests; this task is a coverage/documentation gate, not a new broad
refactor.

**Interfaces:** Consumes all adopted roles. Extend the existing optional
render-dump harness; do not create another parallel harness file.

- [ ] **Step 1: Repeat the production styling inventory and classify each
  remaining local recipe.**

```bash
rg -n 'lipgloss.NewStyle|\.Bold\(true\)|\.Italic\(true\)|\.Faint\(true\)|\.Foreground\(' pkg/ui -g '*.go' -g '!**/*_test.go'
```

For every remaining standard title/heading/body/metadata/badge/selection or
callout recipe, return to Tasks 5-8 and migrate it with a failing test.
Remaining semantic exceptions include `visuals.go` and `braille.go` chart
palettes, `epic_progress.go` progress meaning, status/priority/gate helpers,
graph geometry/edges, search-match/diff state, and Glamour syntax/markdown.
Record specific exception functions and reasons in a small table appended to
the living spec; do not claim every `NewStyle` should disappear. Search
`pkg/ui/slots` as well for ordinary section headings versus semantic badges.

- [ ] **Step 2: Extend the render harness with a many-track Actionable fixture
  and role overrides.** Reuse the issues fixture and existing `BT_RENDER_DUMP`
  gating; add 24 tracks, long Unicode titles, a recommendation, a non-first
  selection, and unblock details. Render dracula and a light/monochrome
  palette, at 120x40 and 40x12, using temporary isolated theme files. Store
  outputs through the harness's existing dump helper under `_tmp/render`.
  Require no browser, database, screenshot service, or new tool installation.

```go
// Add this closure beside enterActionable in TestRenderDump.
readableActionable := func(name string) func(*Model) {
    return func(m *Model) {
        m.applyThemeLive(name)
        plan := analysis.ExecutionPlan{Summary: analysis.PlanSummary{
            HighestImpact: "issue-00", ImpactReason: "Unblocks work", UnblocksCount: 2,
        }}
        for i := 0; i < 24; i++ {
            plan.Tracks = append(plan.Tracks, analysis.ExecutionTrack{
                TrackID: fmt.Sprintf("track-%02d", i), Reason: "Independent work stream",
                Items: []analysis.PlanItem{{ID: fmt.Sprintf("issue-%02d", i),
                    Title: "界面 e\u0301 - a long actionable title for width checks",
                    UnblocksIDs: []string{"next-a", "next-b"}}},
            })
        }
        m.actionableView = NewActionableModel(plan, m.theme)
        m.actionableView.SetSize(m.width, max(1, m.height-1))
        for i := 0; i < 15; i++ { m.actionableView.MoveDown() }
        enterActionable(m)
    }
}
// Add these entries to the existing scenario table:
// {"actionable_readable_dracula_120x40", 120, 40, readableActionable("dracula")},
// {"actionable_readable_dracula_40x12", 40, 12, readableActionable("dracula")},
// {"actionable_readable_paper_120x40", 120, 40, readableActionable("paper")},
// {"actionable_readable_greyscale_40x12", 40, 12, readableActionable("bt:greyscale")},
```

Isolate/restore theme globals in the harness and apply the scenario's light/dark
mode before theme loading. The ordinary automated assertions remain in
`actionable_test.go`; dumping is supplemental visual inspection only.

- [ ] **Step 3: Add configuration documentation and verify examples.** Ensure
  the embedded YAML documents each role, supported palette references,
  `auto`/`none`, explicit false booleans, layer precedence, and the terminal
  font-size limitation. The spec retains its reference example and gains the
  actual migrated-surface/semantic-exception table. Add a loader test that
  unmarshals the exact documented example and checks its resulting roles,
  rather than letting documentation drift from the schema.

- [ ] **Step 4: Run the final verification commands below and review their
  full output.** A baseline failure must be reproduced on the unchanged
  base before being called pre-existing. Never change unrelated fixtures to
  force this branch green. For any failure, load `systematic-debugging` before
  proposing a fix. Use available render dumps to inspect readable title/badge
  pairs and selected rows, but do not assert the user's screenshot is fixed
  solely from stripped text.

- [ ] **Step 5: Review and commit coverage/documentation changes.** Invoke
  `requesting-code-review` and `jev.review_select` with the actual whole-branch
  diff, spec requirements, and user-selected delegation policy. Perform all
  mandatory correctness review; the selector is not a correctness verdict.
  Before commits and completion claims use `verification-before-completion`.
  Commit with `git commit --only` naming only the actual changed coverage/doc
  paths. Commit message: `docs(tui): document shared text role configuration`.

## Final Verification and Handoff

Run from the implementation worktree. No bare `bt` invocation is permitted.

```bash
go test ./pkg/ui -count=1
go test ./... -count=1
go test ./pkg/ui -race -run 'TestThemeRefresh|TestThemeSwapMidSession|TestActionable' -count=1
go build ./...
go vet ./...
git diff --check
```

Optional visual fixture dump, after ordinary assertions pass:

```bash
BT_RENDER_DUMP=1 go test ./pkg/ui -run TestRenderDump -count=1
```

Report changed surfaces, remaining intentional semantic exceptions, commands
that passed/failed, and any visual-verification limitations. Do not install or
push. Follow `finishing-a-development-branch` for integration choices after the
implementation tests/review pass.

### Spec coverage map (planning self-review)

These checks confirm plan coverage, not implementation completion:

- [x] Config roles, booleans, invalid fields, overlays, persistence: Task 1.
- [x] Per-Theme style isolation, complete base palette, contrast, fallback: Task 2.
- [x] Startup/live/background-mode propagation and state preservation: Task 3.
- [x] Actionable readability, measured scrolling, bounded Unicode output: Task 4.
- [x] Panel/list/details/popup text adoption and coherent selection: Task 5.
- [x] Board/graph/tree/epics text adoption: Task 6.
- [x] Analysis/history/dashboard/alert/memories text adoption: Task 7.
- [x] Footer/sidebar/help/settings/modals/widget text adoption: Task 8.
- [x] Exhaustive renderer inventory, exceptions, documentation, final tests: Task 9.

Self-review checked interface names, existing files, configuration examples,
all five Review Focus conditions, contrast math, and state/semantic preservation.
There are no known spec-coverage gaps. No implementation tests have run because
this change only adds planning documentation.

### Execution choice

Await user review of this plan and execution-method selection before touching
product code. Recommend **Native** execution: these tasks share theme interfaces
and global-state test constraints, so keeping one implementer's context avoids
repeated setup and conflicting migrations. Use a fresh whole-branch reviewer at
the end only after the chosen execution method authorizes that delegation.

**Subagent-driven** remains an option: execute tasks sequentially with fresh
implementer/reviewer gates; parallelize only independent view-migration work
after the configuration, lifecycle, and shared-helper interfaces are stable.
