# Shared popup rendering

Status: implemented and reviewed; live-Dolt verification remains environment-blocked.

## Purpose

Popup menus should look like one component family. Their titles, borders,
padding, selection indicators, aligned columns, and footer hints should not
depend on which menu is open. Forms and richer dialogs should share that
frame without losing their content layout or keyboard behavior.

The owner approved this scope on 2026-10-07. This document records the design
for review before an implementation plan is written.

## Original problem

`pkg/ui/field_edit.go` shares `renderFieldModalLines` between the edit hub,
status/priority pickers, and single-line inputs. That helper centers every
line separately. Labels of different lengths therefore start in different
columns, even though callers reserve spaces for cursor and current markers.

Commit `703c3ec6` pads only the hub rows to equal visible width. The status
picker still shows the same defect. Moving that workaround into each caller
would leave duplicated layout decisions rather than a reusable component.

Other popups also duplicate padding, title placement, footer styling, and size
calculations. Some use `RenderTitledPanel`; agent, session, and update dialogs
construct their own outer Lipgloss borders.

Two useful shared layers already exist in `pkg/ui/panel.go`:

- `RenderTitledPanel` draws ANSI-aware bordered panels. It is also used by
  ordinary views and must not acquire popup-only defaults.
- `OverlayCenterDimBackdrop` places a popup over the current view. Rendering
  the frame and positioning it remain separate responsibilities.

## Decision

Add popup-specific frame/layout and menu-row helpers in `pkg/ui/panel.go`.
Keep `RenderTitledPanel` and the existing compositor underneath them. Migrate
popup callers to these helpers; remove duplicated popup layout code directly,
without compatibility wrappers or parallel rendering paths.

This is preferable to extending the hub-only workaround because both the
alignment defect and inconsistent chrome are shared concerns. A universal
stateful dialog model is unnecessary: the existing Bubble Tea models already
own navigation, filtering, input, validation, and actions.

## Popup frame contract

The frame accepts a title, styled body lines, footer hint variants, the current
theme, available terminal dimensions, and optional preferred outer dimensions.
An optional right-hand title label and semantic accent support counts and
warning dialogs. These are visual options, not new interaction modes.

Shared defaults:

- Rounded border, focused title styling, and the active theme's primary
  border/title color.
- Centered title. A right-hand label uses the existing titled-panel layout
  with a left-hand title and right-hand label so neither overlaps.
- Two cells of horizontal padding and one blank row above and below content
  when the available size permits.
- One blank separator between body and footer. The footer uses the theme's
  secondary color, italic styling, and centered placement.
- Menu bodies are left-aligned within a block that is centered as a whole.
  Forms, editors, and multi-column content are left-aligned within the usable
  body area. No body mode centers individual menu rows.
- Warning accents remain explicit. Quit/claim warnings, current-value markers,
  validation errors, and destructive-action cues retain their meaning.

The frame returns only its bordered content. It does not call `Place`, invoke
the compositor, change model state, run commands, or print output.

### Size and overflow

All widths are terminal cells measured with ANSI-aware helpers, not byte
lengths. The outer width and height include borders and padding. Preferred
dimensions are capped to the caller's available budget; content-sized popups
measure title, body, and the fitting footer before applying that cap.

Expose a shared layout calculation so callers can obtain usable body width
and height before sizing text inputs, textareas, columns, or scrolling lists.
Do not maintain a separate hand-written border/padding subtraction per popup.

The layout reserves footer space before assigning the remaining body height.
Scrollable callers retain their existing offset/selection logic and apply
the returned body budget. A frame must not silently clip the selected row
or the footer because a caller filled all available rows with body content.

Footer variants are ordered from detailed to compact. Select the first that
fits, using the existing `fitModalHint` approach where appropriate. Compact
menu hints retain navigation, confirm, and cancel/back instructions. If no
variant fits, wrap the smallest variant and account for the extra rows. At
extreme sizes where chrome and essential hints cannot fit, render a bounded
"Terminal too small" message, truncated to the available cells, rather than
drawing outside the terminal. Existing cancel/back bindings remain active.

For long body lines, truncate with ANSI-aware functions while preserving the
frame's side padding. For tiny dimensions, reduce padding before usable
content; width/height calculations must remain nonnegative. Unknown dimensions
use a shared 80-column, 24-row default terminal budget, not per-modal arbitrary
minima. A known zero-sized available budget produces no frame content.

## Menu-row contract

A menu entry supplies its label and selected state, with optional shortcut,
marker, detail text, and trailing metadata. Models continue to decide which
entry is selected and what a marker means.

The renderer owns:

- A two-cell cursor column, using `> ` for the selected entry and spaces for
  other entries. Selection also uses the primary color and bold text, so it
  is not indicated only through color.
- Optional shortcut and marker columns, explicitly enabled for the menu.
  Enabled columns are reserved in every row, even where their value is empty.
- Column widths based on the widest value across the whole filtered menu,
  before slicing to the visible page. Marker glyphs and styled values are
  measured in terminal cells. Scrolling must not change the label origin.
- A stable label column, shared selected/idle styling, and trailing metadata
  that cannot shift the label start.
- Detail lines indented to the label column and truncated to the usable width.
  Each detail occupies one row so callers can budget item heights reliably.
- Equal-width rendered rows so centering the menu block cannot recenter
  shorter labels. Cursor motion must not change the block's horizontal origin.

The helper produces body lines only. It does not select entries, scroll,
filter, bind keys, infer current values, or execute actions. Callers with
pagination calculate the shared column layout over their filtered entries,
then pass that layout and the visible entries to the row renderer. They keep
their existing scroll indicators and offset policy.

## Migration scope

Every actual popup menu uses both the shared frame and shared menu-row
renderer:

| Surface | Existing implementation |
|---|---|
| Edit field hub, status, priority | `pkg/ui/field_edit.go` |
| Settings launcher | `pkg/ui/settings_menu.go` |
| Recipe picker | `pkg/ui/recipe_picker.go` |
| Repository picker | `pkg/ui/repo_picker.go` |
| Label picker | `pkg/ui/label_picker.go` |

Other popups share the frame but retain specialized bodies:

| Surface | Existing implementation |
|---|---|
| Title/assignee input | `pkg/ui/field_edit.go` |
| Long-form field editor | `pkg/ui/longform_edit.go` |
| Claim confirmation | `pkg/ui/claim.go` |
| Quit confirmation, time-travel prompt, help | `pkg/ui/model_view.go` |
| Options screen | `pkg/ui/settings_modal.go` |
| Agent integration prompt | `pkg/ui/agent_prompt_modal.go` |
| Related session cards | `pkg/ui/cass_session_modal.go` |
| Update dialog | `pkg/ui/update_modal.go` |
| Alerts/notifications | `pkg/ui/model_alerts.go` |
| Epic focus popup | `pkg/ui/epic_card.go` |

The BQL query dialog in `pkg/ui/bql_modal.go` also adopts the frame, but keeps
its current host/placement behavior. This change does not turn full-page
surfaces into overlays. Tutorial pages and label-analysis detail screens are
not popup menus and are outside this migration.

Nested preview/snippet boxes, editor widgets, rich help content, option
columns, and update progress displays remain specialized content. Ordinary
board cards, panes, robot output, and non-modal overlays do not change.
Where a richer popup includes a selectable list, such as session-card headers
or epic children, its selection prefixes and columns also use the menu-row
helper while the surrounding card/tree content stays specialized.

Keep the existing `Model.View` modal dispatch and backdrop compositor. Frame
consistency does not require rewriting the Elm update loop or introducing a
second modal registry.

## Behavior that must remain unchanged

- All shortcuts and navigation keys, including uppercase accelerators.
- Status/priority current-value markers and separate cursor selection.
- Label/repository multi-selection, search, counts, paging, and scroll hints.
- Confirmation/commit semantics, pending-write machinery, and status fences.
- Text input, validation, editor shortcuts, draft retention, and discard guards.
- Options theme preview and help descriptions.
- Clipboard, agent prompt, update, and session actions.
- `BT_NO_BROWSER` / `BT_TEST_MODE` safety gates.

Visual alignment and chrome are the only deliberate behavior changes.

## Verification and acceptance

Write regression tests before changing implementation. Extend
`pkg/ui/panel_test.go` for shared contracts and the existing modal test files
for migrated callers; do not create replacement implementations or broad
snapshot-only tests.

Required checks:

1. Status and priority rows keep cursor, marker, and label columns aligned
   when selection moves away from the current value. Retain hub coverage.
2. Shared menu rows handle unequal labels, missing shortcuts/markers, styled
   text, and wide Unicode glyphs without shifting columns.
3. Menu blocks are centered as units; changing selection does not move them.
4. All frame rows have matching visible widths. Panels and essential footers
   fit normal, narrow, short, and extreme terminal budgets.
5. Forms/editors remain left-aligned and receive the correct content budget.
6. Each migrated popup uses shared chrome and preserves its interaction tests.
   Cover warning accents and title/right-label layouts explicitly.
7. Existing backdrop, mouse hit-testing, scrolling, and resize tests remain
   applicable. Update geometry-dependent tests only to match deliberate layout
   changes, not to conceal behavioral regressions.

Run `go build ./...`, `go vet ./...`, focused popup tests, and the full suite.
Run focused race tests for shared rendering/resize paths where supported.
Review the diff for remaining independent outer popup borders and duplicated
menu alignment logic. Update `docs/design/tui-modal-compositing.md` with the
new canonical helpers when implementation lands.

## Known environment constraints

Bead tracking currently cannot open the configured `bt` database on Dolt at
`127.0.0.1:3308`. Record this limitation rather than creating or repairing a
database as part of a visual refactor. The design is not associated with a
fabricated bead ID.

The previous full-suite run had four UI failures that also reproduce on
unchanged `main`: `TestMemoriesLoadCmd_DogfoodLiveProject`,
`TestBtThemeEnvSelectsNative`, `TestSettingsCyclesThemeLive`, and
`TestThemeSwapMidSession_NoRace`. The CLI suite's registry guard also detected
a change to the real `~/.bt/projects.json` during that run. Do not blindly
rerun that suite against the live registry; establish safe test isolation
before another full run and disclose any remaining failures.

These are verification constraints, not additions to the implementation
scope. Do not change the user's registry, repair Dolt, or alter theme loading
to make a popup-rendering change appear green.

## Implementation validation

The migration is implemented in `pkg/ui/panel.go` and all active popup families
listed above. Ordinary panels and the existing compositor remain unchanged.
The fresh whole-branch review's correctness findings now have regressions for
back navigation after resize, essential input/action visibility at short
heights, and grapheme/multiline chrome geometry. Full-menu normalization also
reuses an immutable replacer with a no-control-character fast path; the
1,000-entry measurement regression reports zero allocations after warmup.

Build, vet, focused popup tests, focused race tests, and the isolated UI suite
with only the known live-Dolt test excluded pass. The unfiltered full suite,
run with a disposable home/config/registry and browser suppression, fails only
`TestMemoriesLoadCmd_DogfoodLiveProject` because the live `bt` database is
missing. The isolated CLI registry guard and prior theme tests pass. A first
run additionally failed five datasource discovery tests because global
`BT_TEST_MODE=1` disables the behavior they test; rerunning without that global
flag resolved those failures without changing production safety gates.

The reviewer identified an existing notification mouse mismatch when a
selected system headline suppresses its summary-expansion row. That predicate
mismatch is unchanged from the base and remains separate follow-up work.
Real clipboard, installation, and terminal/screen-reader side effects were
not exercised. Nothing was installed or pushed.
