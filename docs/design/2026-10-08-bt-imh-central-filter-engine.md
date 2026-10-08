# Central filter engine (bt-imh)

<!-- Related: bt-imh, bt-obw -->

## Problem

Which issues are "in view" is decided in seven places in `pkg/ui`, and they
disagree. Displays and counts therefore describe different sets. The reported
case: project `cdapi`, status `open`, label `tests` lists 2 issues while the
label picker shows `tests (3)`, because bt-obw scoped picker counts to the
project only.

Current deciders:

| Path | Applies | Misses |
|---|---|---|
| `applyFilter` + `matchesCurrentFilter` (`model_filter.go`) | scope, status/ready, labels, wisps | - |
| `applyRecipe` (`model_filter.go`) | scope, inline copy of recipe matching | labels, wisps |
| `applyBQL` (`model_filter.go`) | scope, BQL | labels, wisps |
| `filteredIssuesForActiveView` (`model_filter.go`) | scope, wisps, status/recipe/BQL | labels under recipe/BQL |
| `handleSnapshotReady` non-recipe branch (`model_update_data.go`) | inline status switch | `in_progress`, `blocked`, `deferred` |
| `workspacePrefilter` callers (tree, label picker, insights/triage, label health, attention, flow, epics) | scope | status, labels, recipe, BQL |
| raw `m.data.issues` readers (alerts, board hint total, attention text, label drilldown, export) | nothing | everything |

The snapshot fast-path condition ("can board/graph reuse the precomputed
snapshot?") is copied four times (`model_filter.go:439,455,545,550`,
`model_update_data.go:307,312`); the graph copies never check project scope.
Recipe matching exists twice (`applyRecipe` inline, `issueMatchesRecipe` in
`snapshot.go`).

## Goal

One definition of the active issue set. Every display and every number in the
TUI derives from it, so a mismatch like the one above cannot be written without
a test failing.

Out of scope: robot mode (`--label`, `--recipe`, `--source` keep their own
filtering); the `/` fuzzy search, which stays a list-only refinement on top of
the visible set.

## The engine: `pkg/ui/filterset.go`

```go
type FilterDim uint8

const (
	DimScope   FilterDim = 1 << iota // workspace project selection (w)
	DimPrimary                       // exactly one of: status | ready | recipe | BQL
	DimLabels                        // l picker; OR across selected labels
	DimWisps                         // ephemeral visibility
)

type FilterSpec struct {
	Workspace bool            // Repos only applies in workspace mode
	Repos     map[string]bool // nil = all projects
	Status    string          // "all", "open", "in_progress", "blocked", "deferred", "closed", "ready"
	Recipe    *recipe.Recipe  // set => Status ignored
	BQL       *bql.Query      // set => Status and Recipe ignored
	Labels    []string
	ShowWisps bool
}

type FilterEnv struct {
	IssueMap map[string]*model.Issue // ready / actionable blocker lookups
	BQL      *bql.MemoryExecutor
	BQLOpts  bql.ExecuteOpts
	Stats    *analysis.GraphStats // recipe sort order
}

func (s FilterSpec) Apply(issues []model.Issue, env FilterEnv) []model.Issue
func (s FilterSpec) Without(d FilterDim) FilterSpec
func (s FilterSpec) IsUnfiltered() bool
func (s FilterSpec) Key() string
```

`Apply` order:

1. Per-issue, cheapest first: scope (`IssueRepoKey`, empty key passes, as
   today), wisps, labels (`matchesLabelFilter`).
2. Primary, per-issue: status/ready via the existing `matchesCurrentFilter`
   status switch moved here, or recipe via `issueMatchesRecipe` (the only
   recipe matcher; `applyRecipe`'s inline copy is deleted).
3. Primary, set-level: BQL executes over the survivors of step 1. Today BQL
   receives the scope-filtered set; it now also gets labels and wisps applied.
4. Order: recipe sort (`sortIssuesByRecipe`), BQL `ORDER BY`, else input
   order. The list's sort mode (`s`) is presentation and stays outside the
   engine.

`Without(d)` clears the given dimensions (`DimPrimary` resets Status to
`"all"` and drops Recipe/BQL; `DimLabels` drops Labels; `DimScope` sets Repos
nil; `DimWisps` sets ShowWisps true). `IsUnfiltered()` is true when no
dimension narrows anything (scope nil or not workspace, Status `"all"`, no
recipe, no BQL, no labels). Wisps are not part of it: snapshots include wisps,
so hidden wisps are caught by the snapshot length check below. `Key()` replaces `Model.filterKey()` and feeds
the bt-qc3 "new filter starts at row 0" check.

The model builds the spec from its existing UI state (`m.filter.currentFilter`,
`labelFilter`, `activeRecipe`, `activeBQLExpr`, `m.activeRepos`,
`m.workspaceMode`, `m.showWisps`) in one `m.filterSpec()` method. Those fields
stay as they are; the spec is derived, not a second source of truth.

The legacy `label:X` form of `currentFilter` is folded into `Labels` by
`m.filterSpec()` and has no separate path in the engine.

## The visible set on the model

- New field `visible []model.Issue` and method `refreshVisible()`, which sets
  it to `m.filterSpec().Apply(m.data.issues, m.filterEnv())`.
- Computed eagerly inside `Update`, never lazily: `View()` gets the model by
  value, so a cache filled there is lost.
- `refreshVisible()` runs from exactly two triggers: a filter change (via
  `applyFilter`) and new data. New data means every place that assigns or
  reorders `m.data.issues`: `handleSnapshotReady`, `replaceIssues`
  (`model.go`), the file-change reload (`model_update_data.go`), and the phase-2
  re-sort (`model_update_analysis.go`). Each calls `applyFilter()`.

### One apply path

`applyFilter`, `applyRecipe` and `applyBQL` collapse into one `applyFilter()`:

1. `refreshVisible()`.
2. Build list items from `visible` with one `buildIssueItem(issue)` helper (the
   `IssueItem` construction is currently copied in `applyFilter`,
   `applyRecipe`, `applyBQL`, `replaceIssues` and the reload path). When the
   snapshot carries prebuilt `ListItems`, reuse them by ID instead of rebuilding.
3. Apply the list sort mode only when the primary is a status (today's
   behavior; recipe and BQL keep their own order).
4. `setListItems`, then refresh board, graph, tree, epics and any open analysis
   surface from `visible`.

`applyRecipe(r)` and `applyBQL(q, s)` remain as small setters that update
filter state and call `applyFilter()`. `reapplyActiveFilter()` becomes
`applyFilter()` and is removed. The project-scope safety net inside
`setListItems` is removed: every caller now passes items built from `visible`.
`refreshBoardAndGraphForCurrentFilter` and `filteredIssuesForActiveView` read
`visible` instead of recomputing. `workspacePrefilter` is deleted.

The snapshot fast path becomes one helper used by board and graph:

```go
func (m *Model) canUseSnapshotFor(spec FilterSpec) bool
```

true when `len(m.visible) == len(m.data.snapshot.Issues)` (as today; this is
what excludes hidden wisps) and either `spec.IsUnfiltered()` or the only active
dimension is a recipe that matches `m.data.snapshot.RecipeName`/`RecipeHash`.

## Surfaces

| Surface | Source after the change |
|---|---|
| List, board, graph | `visible` (snapshot reuse only via `canUseSnapshotFor`) |
| Tree | `visible`; an issue whose parent is filtered out becomes a root |
| Epics overview | `spec.Without(DimPrimary).Apply(...)`. The epics view has its own active/all/completed mode and its progress bars count closed children; this preserves the existing design noted in `epics_view.go` |
| Label picker counts | `spec.Without(DimLabels).Apply(...)`; applied labels absent from that set stay listed with count 0 (bt-obw behavior kept) |
| Insights, triage badges, priority hints, actionable plan, label health, attention (scores and text), flow matrix, label drilldown subgraph and cross-flows | `visible`. Recomputed when the view opens, and by `applyFilter()` step 4 while it is shown (the existing priority-hints pattern), so filter changes and data reloads both refresh it |
| Alerts modal and badge counts | drift computed as today; an issue-linked alert is shown and counted only if its issue is in `visible` (replaces the scope check in `passesAlertBaseScope`). Alerts without an issue ID are unaffected |
| Board hint `[open:2/N]` | N = `spec.Without(DimPrimary \| DimLabels)` count, i.e. issues in the project scope |
| Footer issue count and triad | unchanged; already derived from list items, which now come from `visible` |
| Markdown export | `visible` |

### Deliberate full-data readers

These read `m.data.issues` or `m.data.issueMap` on purpose. Each carries a
one-line comment naming this section.

- Graph metrics (PageRank, critical path, blocker sets, "ready" blocker
  checks): a blocker can sit outside the filter.
- Epic progress (`epicProgress`, epic cards): a property of the epic, not a
  count of the view.
- Status header "corpus N issues (tier)": describes what is loaded.
- `computeAlerts`: compares against a whole-corpus baseline; only display is
  filtered.
- ID lookups: details pane, dependency navigation, plugin host sync, semantic
  index build, time-travel snapshot, events diff.

## Tests

1. **Engine unit tests** (`filterset_test.go`): each dimension alone, every
   pair, `Without` for each dimension, `IsUnfiltered`, BQL running after
   labels/wisps, recipe order preserved, legacy `label:X` folding.
2. **Static guard** modeled on `TestNoRawListSetItems`: scans non-test
   `pkg/ui/*.go` and fails on `m.data.issues` reads outside an allowlist of
   functions (data loading/assignment, `refreshVisible`, and the deliberate
   full-data readers above).
3. **Consistency test**: for a matrix of specs (scope x status/ready/recipe/BQL
   x labels x wisps) over a two-project fixture, list items, board total,
   graph nodes, tree nodes and analysis inputs all equal `visible`; epics and
   picker counts equal their `Without` sets.
4. **Reported repro**: project scoped, status `open`, label `tests` gives a
   list of 2 and a picker reading `tests (2)`.
5. Existing activeRepos tests (`workspace_filter_test.go`,
   `filter_preservation_test.go`, `snapshot_events_test.go`) keep passing;
   ones that assert the old project-only analysis scope are updated to the
   full filter.

## Behavior changes users will notice

- Label picker counts reflect status/recipe/BQL and project scope.
- The label filter and wisps toggle now narrow recipe and BQL results.
- Tree, insights, label health, attention, flow, triage badges and alerts
  follow status, label, recipe and BQL, not just the project.
- Board hint denominator is the project's issue count, not the corpus.
- Markdown export writes what is shown.
