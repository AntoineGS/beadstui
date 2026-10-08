# Scope-aware repo column

Status: Approved for implementation.

## Intent

Remove redundant source information from the issue list when the user has
selected exactly one project. Reclaim the repo indicator's width for issue
titles, while keeping source information visible when the scope can include
multiple projects.

The user calls this the "repo column." In the current implementation it is a
leading repo badge in each issue-list row, rendered by
`pkg/ui/delegate.go`. This spec covers that row element, not the Kanban board's
status, priority, or type columns.

## Current behavior

- `IssueDelegate.WorkspaceMode` controls whether repo badges are rendered.
  Workspace/global mode therefore shows them even with one project selected.
- The badge uses `IssueItem.RepoPrefix`, derived from the issue ID. For example,
  `se-123` has prefix `se`.
- The project picker filters using `activeRepos` and `model.RepoKey`, which
  prefers `SourceRepo` (the source database name) and falls back to the ID
  prefix. A database named `statsengine` can therefore contain `se-*` issues.
- These names are not independent task classifications. A single selected
  project establishes source context without repeating its prefix on every row.

## Visibility rule

Repo visibility is determined by project scope, not by the projects represented
in the current search results or by the number of available databases.

| Mode and committed project scope | Repo indicator |
| --- | --- |
| Non-workspace, local-project mode | Hidden, as today |
| Workspace/global mode, exactly one enabled project | Hidden |
| Workspace/global mode, two or more enabled projects | Shown, as today |
| Workspace/global mode, all projects (`activeRepos == nil`) | Shown, as today |
| Workspace/global mode, non-nil selection with zero enabled projects | Normal workspace rendering; no single-project suppression |

An enabled project is a map entry whose value is `true`; entries with `false`
do not count. A single enabled project counts as single-project scope even if
it currently has no issues or is no longer available after a reload.

All-project scope remains all-project scope even if only one database is loaded
or secondary filters leave results from only one project. This keeps the layout
stable while searching, filtering by status or label, and receiving new issues.
Rows without a renderable repo prefix retain their existing behavior.

## Layout and interaction

1. In single-project scope, omit both the badge and its following separator.
   Reserve no blank repo gutter or hidden-column padding.
2. Remove the badge and separator from the fixed-width budget. The recovered
   cells go through the existing title-width and truncation calculation; other
   column thresholds and layout policies do not change.
3. Preserve existing compact ID rendering and full canonical IDs in task data,
   details, and clipboard operations. Do not change the issue's source
   information or alter ID formatting as part of hiding the repo indicator.
4. Preserve existing project-scope indications in the UI and the project's
   selection in the picker. Hiding the row indicator does not hide scope.
5. Hide the `REPO` column header under the same rule as the row badges.
   Apply the rule to all rows uniformly in both full-width and split issue-list
   layouts. Preserve selection and scroll position under existing filter rules.
6. Update visibility on the next render after a committed scope change:
   project-picker confirmation, the current-project/all-project toggle, or
   programmatic startup scope selection. Moving the picker cursor or changing
   uncommitted checkmarks does not change the underlying list layout.
7. Background data refreshes, recipe changes, search, status/label filters, and
   terminal resizing must not restore a suppressed badge or hide a required one.

Different or legacy ID prefixes inside one database do not make the selection
multi-project. Source filtering continues to use the existing canonical key.
Details retain their existing source display.

## Implementation boundaries

Separate the row's repo-display policy from workspace mode. Do not set
`workspaceMode` to false to hide badges: workspace mode also controls filtering,
footer behavior, history context, and other features.

Derive a single display decision from the current workspace flag and committed
project selection, and propagate it to the list delegate whenever that selection
changes or the delegate is rebuilt. Rendering and width accounting must use the
same decision.

Relevant existing integration points:

- `pkg/ui/delegate.go`: badge rendering and title-width budget.
- `pkg/ui/model_view.go`: shared issue-list column header.
- `pkg/ui/semantic_search.go`: `updateListDelegate`.
- `pkg/ui/model_filter.go`: filtering and list refresh paths.
- `pkg/ui/model_modes.go`: `EnableWorkspaceMode` and `SetActiveRepos`.
- `pkg/ui/repo_picker_keys.go`: committed picker selection.
- `pkg/ui/model_update_input.go`: current-project/all-project toggle.
- `cmd/bt/root.go`: startup project selection through model setters.

No database migration, source-field mutation, settings persistence, new CLI
flag, or user preference is required. Do not rename "repo" or "project," change
badge labels/aliases, alter filtering semantics, or modify other views and
robot/export output as part of this change.

## Acceptance and verification

- Single-project rows contain no repo badge or leftover repo spacing; task IDs
  and all other row elements remain present.
- At a fixed width with a sufficiently long title and unchanged other badges,
  single-project rows gain the badge's measured width plus its separator in
  the title budget, except where existing layout clamps apply.
- Multi-project and all-project rows keep their current repo indicators and
  width accounting, including existing global-namespace display aliases.
- Test local mode, nil selection, empty selection, one enabled project, two
  enabled projects, and one enabled entry alongside a false entry.
- Test a database/prefix mismatch such as `statsengine` / `se-123`: scope is
  determined by the database key, not by the displayed prefix.
- Test single-to-multiple-to-all-to-single transitions via picker confirmation
  and the project-scope toggle, plus initial scope set through `SetActiveRepos`.
- Test refresh and delegate rebuild with single-project scope, search narrowing
  a multi-project scope to one project's results, and empty-result selections.
- Test narrow and wide list widths, split layout, and resizing. Rendered lines
  must fit their width under the existing rendering contract.
- Keep existing workspace-filter and delegate-rendering tests passing. Before
  declaring an implementation complete, run `go test ./pkg/ui/...`,
  `go test ./...`, `go build ./...`, and `go vet ./...`.

## Alternatives considered

Always showing the repo indicator preserves today's layout but repeats context
and wastes title space in single-project scope. Hiding it whenever the current
results happen to contain one project causes layout changes during unrelated
filters and refreshes. The selected rule follows explicit project scope instead.
