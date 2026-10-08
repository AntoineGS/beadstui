# Shared TUI text styling and Actionable readability

Date: 2026-10-08

Status: written spec approved by the user; implementation plan awaiting review.

Issue tracking is explicitly waived for this work because the configured beads
database is unavailable.

## Intent and acceptance criteria

The user wants Actionable to be readable and wants one user-configurable place
to define titles, headers, body text, and related formatting across the TUI.
This is an extension of the existing theme system, not a second configuration
system or a redesign of the application's layout.

The change is accepted when:

1. Standard TUI text uses centrally resolved semantic roles rather than local
   foreground, background, and text-attribute recipes.
2. Both `~/.config/bt/theme.yaml` and `.bt/theme.yaml` support per-role overrides.
3. Partial overrides inherit unspecified values, and explicit `false` disables
   a previously enabled text attribute.
4. Default filled titles, badges, callouts, and selected rows have readable
   foreground/background pairs in both dark and light modes.
5. Switching palettes or receiving a terminal background-mode change refreshes
   the main model, delegates, and retained submodels without resetting selection.
6. Actionable fits its assigned dimensions, displays its selected item while
   navigating and resizing, and shows readable headers, badges, and item text.
7. Existing status, priority, and other domain-specific visual meanings remain
   recognizable. Robot output and data loading are unchanged.

## Current problems

Color configuration already exists in `pkg/ui/defaults/theme.yaml`, with named
palettes and user/project overlays loaded by `pkg/ui/theme_loader.go`. Shared
formatting is incomplete: views create their own combinations of colors, bold,
italics, and backgrounds.

Specific evidence:

- `pkg/ui/actionable.go` places `Theme.Base` foreground on `Primary` and
  `Secondary` filled backgrounds. Those colors were not selected as readable
  pairs, producing the pale-on-pale title and track badges in the user's image.
- Actionable mixes its retained `Theme` with package-level `Color*` variables.
- `ApplyThemeToThemeStruct` updates palette fields and some delegate styles, but
  does not rebuild `Base`, `Header`, or `Selected` from the loaded palette.
- `applyThemeLive` refreshes the list delegate and markdown renderer, but not
  Actionable's retained theme copy. The background-color message handler has a
  separate refresh path.
- Actionable's visibility calculation counts one track-header line while its
  renderer emits a header and divider, and does not count the recommendation
  banner. Rendering slices the complete view with that inaccurate offset.
- Width calculations reserve an arbitrary 20 columns from titles, truncate by
  runes rather than terminal cells, and allow minimum title widths that exceed
  the actual space available.

## Configuration contract

Add a top-level `text:` mapping alongside `theme:`, `mono:`, and `colors:` in the
existing `ThemeFile` schema. Supported role names are fixed and typed:

| Role | Intended use | Default foreground | Default background | Default attributes |
|---|---|---|---|---|
| `title` | View titles and modal titles | `auto` | `primary` | bold |
| `heading` | Section, panel, and column headings | `primary` | `none` | bold |
| `body` | Main prose, issue titles, ordinary values | `text` | `none` | plain |
| `metadata` | IDs, secondary labels, supporting explanations | `subtext` | `none` | plain |
| `badge` | General-purpose filled labels, including track labels | `auto` | `secondary` | bold |
| `selected` | Focused list/track rows | `auto` | `highlight` | bold |
| `callout` | Recommendation and informational callout text | `auto` | `bg_highlight` | bold |

Role fields are `foreground`, `background`, `bold`, `italic`, and `underline`.
The three attributes are optional booleans, not truthy-only merge values.
Unspecified attributes inherit the lower layer. Plain defaults mean all three
attributes are false; no role defaults to faint text.

Color fields reference existing scalar palette tokens:

`bg`, `bg_dark`, `bg_subtle`, `bg_highlight`, `text`, `subtext`, `muted`,
`primary`, `secondary`, `info`, `success`, `warning`, `danger`, `text_secondary`,
`bg_contrast`, `border`, and `highlight`.

`none` is supported for backgrounds and means no explicit background color.
`auto` is supported for foregrounds and means automatic contrast selection.
An omitted or empty color field inherits its lower-layer value. Per-role hex
colors, arbitrary nested role definitions, and role inheritance graphs are not
part of this change: custom light/dark colors remain in the existing `colors:`
mapping and can be referenced by roles.

For example:

```yaml
theme: dracula
text:
  title:
    foreground: auto
    background: primary
    bold: true
  metadata:
    foreground: text
    italic: false
  selected:
    background: bg_highlight
    underline: true
```

Merge role fields individually in the existing order: embedded defaults, named
palette, user overrides, project overrides. Palette selection via `BT_THEME`
continues to select the named palette without bypassing user/project role
overrides. Named bt-native palettes may supply role defaults; the vendored btop
corpus is not edited and inherits the central role defaults.

Palette picker persistence continues editing only the `theme:` key. It must
preserve `text:`, color overrides, comments, and key order.

## Style resolution and theme lifecycle

Resolve palette colors first, then build a typed set of role styles once from
the resulting palette and role configuration. `Theme` owns those resolved
styles; renderers do not parse YAML or calculate contrast each frame.

Style building belongs in the existing theme/style files:

- `pkg/ui/theme_loader.go`: schema, field-level merges, palette-token lookup,
  configuration application.
- `pkg/ui/theme.go`: complete theme palette fields, role-style ownership, and a
  shared style-rebuild entry point used by fallback and loaded themes.
- `pkg/ui/styles.go`: shared style/contrast utilities and access for existing
  global-only rendering helpers where passing `Theme` is not appropriate.
- `pkg/ui/defaults/theme.yaml`: documented default role definitions.

Both `DefaultTheme` and loaded themes use the same role builder. A `Theme` built
from one config must not silently acquire styles from another theme's mutable
package globals. Resolve the text and background palette fields into the
`Theme` itself, rather than leaving its base style on fallback colors.

Automatic contrast chooses black or white, whichever has the higher WCAG
relative-luminance contrast against the resolved background. For the existing
opaque RGB palette, the chosen pair must meet at least 4.5:1. With a `none`
background, compare against the resolved `bg` token. This is a guarantee for
the configured RGB pair, not a claim about terminal transparency, wallpaper,
or the terminal's final color quantization.

Explicit foreground choices are respected, even if a user deliberately chooses
a low-contrast pair. Automatic foreground is the safe default, not a forced
rewrite of user preferences. No TUI renderer introduces new hex literals.

Keep formatting separate from layout: padding, alignment, borders, and available
width remain with the component. A component can adapt a role's layout, but
must not override its configured bold/italic/underline or replace its colors
without a domain-semantic reason. Filled title styling applies to title bars;
panel-border headings use `heading` so a title bar background does not paint a
whole border.

Centralize the theme-update propagation used by startup, live palette changes,
and `tea.BackgroundColorMsg`. Every retained theme copy or cached text style
that can appear after the update is refreshed. Rebuild the list delegate using
its existing state-aware helper, and refresh the markdown renderer and active
or retained submodels. Do not rebuild execution plans, reset cursors, clear
filters, or change modal state merely to update styling.

## TUI adoption boundaries

Adopt the roles across standard application text, not just Actionable:

- Shared panel and popup helpers, modal titles and headings.
- Issues list titles, ordinary row text, metadata, and selection.
- Detail labels and ordinary non-markdown values.
- Main view titles, section headings, general badges, and supporting text in
  board, graph, history, insights, alerts, and dashboards.
- Footer/sidebar labels, help text, settings text, and ordinary modal content.

Implementation must inventory production render sites in `pkg/ui`, classify
them by role, and migrate standard text sites. Adding a central role type while
leaving equivalent formatting recipes spread through the main views does not
satisfy this spec. Specialized domain-rendering sites may retain their own
semantic colors; annotate non-obvious exceptions rather than replacing every
`lipgloss.NewStyle` indiscriminately.

Status/priority badges, graph edges, charts, heatmaps, diff highlighting, and
syntax/markdown rendering retain their semantic color systems. General badge
formatting can share the role's attributes while retaining a semantic badge's
own matched foreground/background pair. Markdown continues using Glamour; this
work ensures it receives the refreshed theme, not a new markdown styling
language.

Selection styling applies coherently to the entire ordinary text portion of a
row. Nested ID/title styles must not reset the selection background and leave
patches of unselected cells. Independently colored semantic chips can preserve
their own filled color pairs. Preserve existing focus, cursor, disabled-state,
and error/status indicators instead of making every row visually identical.

## Actionable rendering and navigation

Use shared roles for its title, track badges/headings, explanations, IDs, issue
titles, recommendation, selected rows, and unblock details. Eliminate its
direct dependency on package-global body text colors.

Render headers and optional recommendation into a measured, fixed chrome
region. Scroll only the track content, using the remaining height. For nonempty
selectable content, reserve at least one body row: reduce chrome by dropping
spacer lines, then the recommendation, then the title when necessary. A
one-row view therefore shows the selected issue rather than only a title. The
complete output must still fit within the assigned dimensions. On usable sizes,
the view title and recommendation stay visible while scrolling.

Build the track-body lines and the selected item's line span through a shared
layout calculation. That calculation includes track dividers, inter-track
spacing, and the selected item's optional unblock-detail line. Rendering and
visibility checks use that same result, rather than maintaining different
estimates of row positions. Navigation and resize clamp offsets and keep the
selected item's first row visible; keep its detail line visible too whenever
the body viewport is tall enough.

Compute title space from actual display-cell widths of the cursor, connectors,
priority icon, ID, padding, and any unblock-count suffix. Truncate using the
existing ANSI-aware/cell-aware facilities. Do not reserve arbitrary unused
columns or use a minimum title width larger than the remaining space. Clip
long headers, recommendations, IDs, and details deliberately rather than
allowing Lipgloss width settings to wrap them unexpectedly. Full issue text
remains available through the existing issue-detail navigation.

For zero/negative dimensions return an empty view. Tiny positive dimensions
must not panic, exceed their bounds, or create negative padding/repeat counts.
Empty plans and empty tracks must not produce invalid selection indices.
These guards serve rendering/navigation safety; planning semantics do not
change.

## Failure handling

Cosmetic configuration must not prevent startup. Unknown role names are ignored.
Unknown color references or invalid field values fall back to the valid
lower-layer/default value for that field; they must not erase other valid role
overrides. Malformed YAML retains the existing file-level fallback behavior.
No raw printing is added to the TUI rendering path.

Represent field presence distinctly from its value so `bold: false` merges
correctly. Validate role fields before applying them to a lower layer. Reuse
the existing parsing/error conventions; do not introduce a second config file
or a new background config watcher.

## Verification

Add regression tests before implementation changes:

- Parse every supported role and field, including explicit false attributes.
- Verify partial merges and named/user/project precedence for roles.
- Verify invalid role fields fall back independently of valid fields.
- Verify default and loaded themes rebuild all text roles from their own
  palette, independent of the last globally applied theme.
- Check auto-contrast pairs in dark and light modes, including representative
  dracula, matcha-dark-sea, light, and monochrome palettes.
- Verify palette switching preserves role overrides and refreshes retained
  Actionable state without changing the selected issue.
- Verify background-mode changes use the same refresh path.
- Verify saving a selected theme preserves the `text:` mapping and comments.
- Verify representative panel, list, modal, and footer text responds to role
  overrides while status/priority indicators keep their meanings.
- Verify Actionable with an empty plan, empty tracks, long text, Unicode
  wide/combining characters, optional recommendation, and unblock detail.
- Verify bounded output and selected-item visibility while moving across many
  tracks, paging, and resizing, including tiny positive sizes.

Run focused theme/Actionable tests, the full Go test suite, `go build ./...`, and
`go vet ./...`. Run targeted race tests where theme lifecycle tests touch shared
state. Review the final diff for remaining duplicated standard text recipes.
Manual rendering inspection of representative palettes is supplementary to
automated tests, not a replacement for them.

## Non-goals

- Font family, point size, line height, or terminal wallpaper controls; these
  belong to the terminal emulator.
- A new visual settings editor, live YAML file watching, or theme-export UI.
- A generic CSS-like style engine, arbitrary style inheritance, or a separate
  configuration subsystem.
- Editing the vendored btop themes.
- Rewriting markdown/syntax styling, chart palettes, or data-layer behavior.
- Issue-tracker recovery, installation of the binary, or pushing code.

## Implementation handoff

The written spec must be approved before an implementation plan is created.
The plan must identify the rendering-site inventory and migration work,
regression tests, shared theme refresh, and Actionable layout changes. The user
then reviews the plan and chooses an execution method before implementation.
