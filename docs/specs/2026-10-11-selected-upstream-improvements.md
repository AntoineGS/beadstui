# Selected beads_viewer improvements

<!-- Related: bt-k20, bt-k20.1 -->

Status: Approved for implementation. The user approved the written specification,
auto-approved implementation planning, and selected subagents with worktrees and
bead tasks.

## Intent and scope

Implement the six improvements selected by the user from the local
`../beads_viewer` checkout: unified readiness, relationship-level flow
drilldowns, remote clipboard support, deduplicated dependency trees,
asynchronous Cass session lookup, and bounded duplicate detection.

Success means more reliable work recommendations, more useful dependency
inspection, responsive external-tool interactions, and lower duplicate-analysis
memory use. Adapt the behavior to beadstui's Dolt readers, embedded `bd export`
reader, Bubble Tea v2 event loop, and deterministic robot API.

Comparison baseline: beadstui `6dd31a73`, beads_viewer `6c6efa15`. The installed
tracker inspected during design is `bd 1.3.1`. Source references below refer to
those checkouts; upstream code is a reference, not a wholesale replacement.

The six workstreams share one specification but receive separate implementation
tasks and acceptance evidence. Readiness is the cross-cutting foundation. The
other five can be implemented independently; clipboard and Cass both touch the
event loop and require coordinated integration. The implementation plan will
set ordering and file ownership.

## Selected approach

Adapt the selected behavior within existing package boundaries. A bulk upstream
merge would bring incompatible storage, keybinding, and rendering assumptions.
Separate local fixes without shared readiness would leave contradictory answers
across filtering, planning, and recommendations.

Use a small pure readiness component in `pkg/model`, existing analysis entry
points, existing flow/tree views, and message-based TUI commands. Keep writes
routed through `internal/bdexec` and `internal/bdroute`. This work does not add
sprints, migrate Git history, or introduce a new plugin or configuration system.

## 1. Full-source readiness and claimability

### Source context and ownership

Maintain two explicit concepts: the loaded source corpus and the selected
candidate IDs. Preserve the corpus before `--repo`, `--source`, label, BQL,
recipe, and TUI display filters. A filter limits candidate membership, not the
evidence used to determine whether a candidate's prerequisites are satisfied.
An explicitly empty selection stays empty; it never falls back to all issues.

"Full source" means the databases or workspace repositories actually loaded for
this invocation. Do not fetch unrelated databases automatically. A prerequisite
outside that corpus remains unknown unless its resolved state is present in the
loaded evidence. Closed/tombstone rows needed for resolution survive display
filtering, either in the corpus or as explicit resolved-ID evidence.

Resolve cross-project aliases against this corpus before building readiness.
Extend the existing `ResolveExternalDeps` pipeline to retain unresolved external
references rather than dropping them. Keep canonical IDs for known targets and
original reference strings for unknown targets. Resolution must be deterministic,
idempotent, and must not mutate caller-owned issues. Ambiguous target identities
are unknown rather than resolved by iteration order. Unknown graph targets must
not become candidate issues or occupy ranking slots.

Build an immutable `ReadinessIndex` from copied decision inputs: status, type,
assignee, deferral timestamp, dependency target/type, and parent-child adjacency.
The index owns these inputs and is safe to share with analysis and UI snapshots.
Construction and dependency-state propagation are O(issues + edges); ordinary
state/ready lookups are O(1). Precompute open-child presence for claimability.
Expensive graph metrics remain in the existing two-phase analyzer.

### Behavioral contract

| Concept | Definition |
| --- | --- |
| Resolved prerequisite | Target is closed or tombstone. |
| Blocking relationship | Empty legacy type, `blocks`, `conditional-blocks`, or `waits-for`, matching the inspected upstream viewer policy. |
| Informational relationship | `related`, `discovered-from`, and unrecognized nonblank types do not acquire blocking meaning implicitly. |
| Dependency state: satisfied | Every blocking target is resolved and inherited parent dependency state is satisfied. |
| Dependency state: unsatisfied | A known unresolved blocking target exists directly or through parent inheritance, with no unknown dependency evidence. |
| Dependency state: unknown | A required target/parent is missing or ambiguous, or parent inheritance encounters an unresolved hierarchy cycle. Unknown takes precedence over unsatisfied. |
| Ready for planning | Status is open or in_progress, defer_until is absent or at/before evaluation time, and dependency state is satisfied. |
| Claimable recommendation | Ready, status open, no assignee, not an epic, and no immediate child whose status is unresolved. |

Parent-child edges inherit a parent's dependency state, not a requirement that
the parent itself close. A resolved parent terminates inheritance. An open parent
with no blockers does not block a child merely by being open. Inheritance has no
arbitrary depth cutoff. Direct blocking cycles are unsatisfied unless other
unknown evidence takes precedence; unresolved parent cycles and affected
descendants remain unknown.

The blocking variants above are conservative viewer eligibility rules, not an
implementation of the tracker's molecule gate scheduler. Preserve relationship
types in data and explanations; a suggestion does not reserve work or guarantee
that a later `bd` claim succeeds.

Unknown and unsatisfied both withhold readiness. Return sorted, deduplicated
blocker references internally so existing explanation surfaces can identify
missing references and inherited parent gates. The broader status vocabulary
(blocked, deferred, pinned, hooked, review, and custom states) is never treated
as ready solely because no edge blocks it.

Capture one evaluation time per robot request or UI recomputation and pass it
explicitly. At exact deferral expiry an otherwise eligible issue becomes ready.
UI refresh/tick handling reevaluates expiry without requiring an issue edit;
expired readiness caches cannot remain valid just because the source hash stayed
the same. Deferral edits must also invalidate data/content-dependent caches.

### Data and consumer integration

- Add `DeferUntil` to `model.Issue`, JSON handling, cloning, and relevant hashes.
  Read `defer_until` through the embedded/JSONL loader and both Dolt SQL scan
  paths. Update the shared ordered column list and schema-probing projections.
  SQL NULL or a column absent under an existing supported schema-drift path
  maps to no timestamp; preserve the current single-reader error policy.
- Extend dependency validation for the two recognized blocking variants.
  Preserve nonblank custom statuses/relationship types during loading; their
  eligibility follows the explicit rules above.
- Carry corpus/readiness and selection through application context,
  `robotCtx`, analyzer/triage options, TUI filter environment, and immutable
  snapshots. `issuesForSearch` is currently saved after `--repo`; it cannot
  serve as the complete readiness corpus without changing that boundary.
- Use the shared predicate for actionable plans, Ready filters and recipes,
  triage actionable counts, quick wins, capacity/forecast eligibility, and
  Go-computed export eligibility. Where a BQL predicate asks for readiness,
  it receives the same context; ordinary field predicates retain their meaning.
- `robot next`, immediate top picks, and emitted new-claim commands must select
  claimable candidates. General priority/blocker recommendations may still
  discuss nonclaimable work with appropriate explanations. An empty claimable
  set produces the existing no-pick output rather than promoting in-progress
  work. Use the existing command-building/routing facilities for claim actions.
- Manual user-directed claims retain the existing `bd` write path. The stricter
  recommendation predicate is not an unsolicited restriction on manual edits.
- Preserve the footer's mutually exclusive partition: in_progress is InFlight;
  other unresolved rows are Ready only if the readiness predicate passes;
  remaining unresolved rows are in its existing Blocked bucket. Closed and
  tombstone rows are excluded. Document that Blocked here also covers unknown
  and otherwise ineligible work, rather than claiming every row has a known
  blocking edge.
- Go what-if/unblock calculations use the same full-source inputs with a
  per-query completion set. Simulations must not mutate the index, implicitly
  complete missing issues, or count filtered-out candidates as selected gains.
  Shared prerequisites and parent gates still apply after hypothetical closure.
- Keep selected graph-metric scope explicit; preserving readiness context does
  not authorize expanding ranked output beyond selection. Preserve robot field
  names/types and scope envelopes. Any new explanation metadata is additive.

Static browser timeline/WASM modernization is a separate project; this work's
readiness contract covers Go analysis, TUI, robot, and Go-derived export facts.

Primary integration areas: `pkg/model/types.go`, new `pkg/model/readiness.go`,
`internal/datasource/{columns,dolt,global_dolt}.go`, `pkg/loader`,
`pkg/analysis/{external_resolution,graph,plan,triage,triage_context}.go`,
`cmd/bt/{context,root,cobra_robot,robot_ctx,robot_triage}.go`, and
`pkg/ui/{snapshot,background_worker,filterset,footer_triad}.go`.

## 2. Relationship-level flow drilldowns

Build rows from the selected label's actual `CrossLabelFlow.Dependencies` and
`BlockingPairs`. A relationship is included when either endpoint-label side
matches the selected label. Deduplicate by canonical `(blockerID, blockedID)`
and sort by these IDs; unrelated issues sharing the label do not appear.

Each relationship exposes both endpoint IDs, titles, and statuses with explicit
blocker-to-blocked direction. Enter inspects the selected endpoint using the
existing detail renderer. Escape returns to the relationship, then to the flow
matrix. Inspection must work even if that endpoint is absent from the current
issue-list filter, without changing the filter or relying on an invisible list
index. Preserve the originating view, selection, and scroll position.

On refresh rebuild relationships from current flow and source data. Preserve the
selected pair and endpoint by ID if present; otherwise choose the nearest valid
remaining row. If the label disappears, return to the matrix. Empty relationship
sets have a bounded empty state. Missing endpoint records are omitted rather
than dereferenced. Render within narrow terminal dimensions using existing keys
and theme conventions.

Primary areas: `pkg/ui/flow_matrix.go`, `pkg/ui/flow_matrix_keys.go`, and the
existing detail/focus integration. Reference: upstream `pkg/ui/flow_matrix.go`.

## 3. Asynchronous clipboard with remote fallback

Use one clipboard operation for issue IDs, Markdown, history SHAs, and Cass
commands. Capture text at invocation; run helper discovery/execution outside
Update in a `tea.Cmd`; return a typed completion message. A monotonically
increasing request identity prevents older completions overwriting newer copy
feedback. A new copy cancels the previous helper; helper writes are serialized
so an older operation cannot overwrite a newer selection. The event loop checks
request identity before emitting a fallback terminal command. Selection changes
do not change the captured text.

Use appropriate local helpers (`pbcopy`, Windows clipboard helper, Wayland
`wl-copy`, or X11 `xclip`/`xsel`). Respect display availability on Unix. Bound
the helper phase to two seconds using context-aware subprocess execution and
bounded pipe handling. If no helper is usable or a helper fails/times out, try
OSC 52 on the active interactive terminal.

Route the terminal write through the Bubble Tea v2 terminal-output mechanism so
it is serialized with rendering. Never fall back to printing escape sequences
into redirected stdout. Send raw OSC 52 through tmux and screen passthrough
when running under GNU screen without tmux. Cap the base64 payload at 74,994
bytes; oversize fallback returns an actionable copy error. Encoding, helper,
and terminal errors reach the existing status/toast mechanism.

A successful OSC 52 write means the request was sent to the terminal; it cannot
confirm that the terminal accepted or stored the clipboard. Native and fallback
paths share the same asynchronous UI lifecycle. Cancel pending helper work on
shutdown; all resulting messages are harmless after cancellation.

Primary areas: `pkg/ui/model_export.go`, `pkg/ui/cass_session_modal.go`, copy
callers in list/board/history key handlers, and model message handling.
Reference: upstream `pkg/ui/cass_session_modal.go`.

## 4. Deduplicated dependency trees

Compute minimum outgoing-dependency distances before choosing expansion nodes.
Expand each reachable issue once at a canonical shortest-path occurrence;
represent subsequent incoming occurrences as reference rows retaining target
metadata and relationship type. Source dependency order determines ties.

Depth is measured in outgoing edges from the root. Preserve the existing default
depth of three and the explicit unlimited-depth convention. Keep each edge
within the depth boundary visible even if its target is already expanded.
Discover and mark directed cycle-closing edges separately from ordinary shared
references; a diamond is not a cycle. Cycle/reference annotations must survive
title truncation and preserve normal status styling.

Keep beadstui's inverse parent-child leaf display and natural-numeric child
ordering. Inverse leaves do not participate in outgoing expansion or generate
synthetic cycles. Deduplicate repeated inverse-child entries without collapsing
distinct outgoing relationship types. Skip nil dependency entries; missing or
nil target records produce a safe not-found row. Keep existing Lipgloss tree
rendering and its bypass of Glamour code-fence processing.

Primary area: `pkg/ui/helpers.go` and its tests. Reference: upstream
`pkg/ui/helpers.go` shortest-path/reference traversal.

## 5. Cancellable asynchronous Cass lookup

Move Cass availability probing and correlation out of Update into a command with
a three-second overall deadline. Capture issue identity/data, source/workspace,
dataset generation, and request generation. Return an immutable result or error
message; only Update changes modal, focus, or status state.

While pending, navigation and resize continue. Escape or invoking the Cass action
again cancels the lookup. Selection changes, dataset replacement, a conflicting
modal, and shutdown cancel/invalidate it. Accept a completion only when all
captured identities still match; stale completions cannot open a modal or replace
current feedback. Closing the session modal restores its originating view/focus.

Keep successful caching scoped to the correct issue/source snapshot. Failed,
cancelled, or partial lookups remain retryable and are not cached as successful
empty results. A successful empty result displays the existing no-sessions
message. External errors surface through TUI status with wrapped causes. Reuse
the existing Cass integration and existing modal rather than adding another
session viewer. Ensure worker-owned/cached values are safe across cancellation
and subsequent requests.

Primary areas: `pkg/ui/model_export.go`, model state/message and key handling,
`pkg/ui/cass_session_modal.go`, and `pkg/cass` cache/correlation boundaries.
Reference: upstream `cassSessionsLoadedMsg` and its request lifecycle.

## 6. Bounded duplicate detection

Retain the current inverted keyword index, Jaccard scoring, status filtering,
and source-index orientation of every suggestion and action. Inspection during
implementation found the old implementation sorts only by score, leaving ties
dependent on map iteration. For unique IDs, make equal-score selection
deterministic using canonical ID-pair comparison keys, without reversing output
orientation. This corrects the design's original assumption that ID tie ordering
already existed. For unique issue IDs, retain at most the
requested K best candidates in a worst-first heap instead of collecting and
sorting all qualifying pairs. Store compact candidate indices and scores;
compute keyword explanations only for final selected pairs, using their original
source indices.

Fewer than two issues or K <= 0 returns no suggestions. If duplicate issue IDs
occur, preserve existing full-sort semantics on that exceptional input rather
than silently changing which source records explain a suggestion. This is an
input-dependent algorithm choice, not a compatibility API layer.

The full-sort test oracle and paired benchmark must retain the literal old
implementation (apart from the specified nonpositive-K guard). Test untied
results for exact parity; for ties, independently verify scores, source payloads,
and the explicit unique-ID ordering rule rather than asserting an old arbitrary
ordering is stable. Repeated-ID fallback retains its old score-only sort.

For ordinary unique-ID inputs, retained pair storage is O(K) and qualifying-pair
selection is O(P log K), followed by sorting at most K retained pairs. Keyword
posting lists and overlap maps remain necessary; overlap enumeration can still
be quadratic. Do not claim that all memory or runtime becomes O(K).

Primary area: `pkg/analysis/duplicates.go` and existing tests/benchmarks.
Reference: upstream `pkg/analysis/duplicates.go` bounded candidate selection.

## Verification and acceptance

| Workstream | Required evidence |
| --- | --- |
| Readiness | Table-driven states/claimability; deferral boundary and unchanged-data expiry; deep parent chains, cycles, unknown/missing parents, blocking variants, resolved tombstones, and independent what-if queries. |
| Source/selection | Same candidate readiness under repo/source/label/recipe/BQL filtering; hidden open/closed prerequisite fixtures; unresolved and ambiguous external refs retained; empty selections stay empty; scoped outputs contain only selected candidates. |
| Data parity | Identical deferral and relationship inputs through embedded JSONL, project SQL, and global SQL including nullable/schema-probed columns; hashes and clones cover new fields. |
| Robot/TUI | No-pick when only nonclaimable work exists; ready/in-flight/footer distinctions; deterministic ordering and existing robot I/O/envelope tests; full-source refresh rebuilds readiness. |
| Flow | Actual pairs only, multi-label deduplication, inspection outside list filter, Escape return, refresh deletion/reordering, empty and narrow layouts. |
| Clipboard | Helper success/absence/failure/timeout, base64 bounds, tmux/screen variants, no redirected-stdout escapes, stale completion order, shared copy callsites. Terminal delivery checked separately from a unit-level sequence test. |
| Trees | Diamond, shorter-path encounter, true cycles, missing/nil targets, exact depth cutoff, inverse children, natural ordering, status/annotation rendering. |
| Cass | Delayed lookup leaves navigation responsive; cancellation, reselection, source/data refresh, conflicting modal and shutdown reject late results; errors retry; modal return focus. |
| Duplicates | Compare complete suggestions to a full-sort oracle on deterministic random and dense fixtures, ties, thresholds, limits, status filters, repeated IDs, and source-specific explanation keywords. |

Use existing test files/harnesses where practical. New files are justified for
the new readiness component and focused genuinely new functionality. Race tests
cover shared snapshots, Cass caches, and asynchronous action lifecycles. Runtime
validation of native clipboard delivery is platform-specific and must be
reported accurately when unavailable.

Measure duplicate detection with the old full-sort implementation and the new
selection on the same fixture, compiler, and worker. Record repeated `ns/op`,
`B/op`, and `allocs/op` samples for dense 500-issue/K=20 and a representative
sparser input. Require output equivalence and reduced retained-candidate
allocation on the dense case; report timing and sparse-case tradeoffs honestly.
Upstream's benchmark numbers are not beadstui results.

After implementation, run affected package/integration tests, `go test ./...`,
`go build ./...`, `go vet ./...`, and applicable race checks. Keep expensive
benchmark runs focused. Verify each deliverable before closing its issue and
make directed commits with its bead reference. The specification-writing task
is complete when this document is checked and committed for user review; the
parent implementation epic remains open.

## Review and handoff

The user approved this written specification, including withholding readiness
for unknown prerequisites. The implementation plan lives under `docs/plans/`;
the user explicitly auto-approved it and selected subagent execution in an
isolated worktree with six bead-tracked deliverables.
