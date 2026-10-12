# Selected Upstream Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver all six approved improvements with deterministic analysis and responsive TUI interactions.

**Architecture:** Add immutable source-wide readiness independently of selected candidates; adapt existing TUI and analysis paths rather than importing upstream subsystems. Run external clipboard/Cass work through commands and typed messages. Keep six independently reviewed deliverables on one isolated integration branch.

**Tech Stack:** Go 1.26.0, Charm Bracelet v2, Dolt/MySQL, embedded bd export, existing Go test/race/benchmark harnesses.

**Spec:** `docs/specs/2026-10-11-selected-upstream-improvements.md`

**Approval:** User approved the specification, auto-approved this plan, and requested subagents, worktrees, and bead tasks. No additional plan approval is required.

## Global Constraints

- Preserve the corpus before `--repo`, `--source`, label, BQL, recipe, and TUI display filters.
- An explicitly empty selection stays empty; it never falls back to all issues.
- Unknown and unsatisfied both withhold readiness.
- Capture one evaluation time per robot request or UI recomputation and pass it explicitly.
- Keep writes routed through `internal/bdexec` and `internal/bdroute`.
- Preserve robot field names/types and scope envelopes. Any new explanation metadata is additive.
- Never fall back to printing escape sequences into redirected stdout.
- Bound the helper phase to two seconds; cap the base64 OSC 52 payload at 74,994 bytes.
- Cass availability probing and correlation share a three-second overall deadline.
- No file deletion, destructive git, script-based source edits, dependency changes, or go install.
- Use `go build ./...` and `go vet ./...` after every task's code changes.
- Read AGENTS.md and the bead conventions; use `bd -C /home/antoinegs/gits/beadstui` for shared task state. Close each delivery bead before its directed commit.
- Upstream reference checkout is `/home/antoinegs/gits/beads_viewer`; do not edit it.

## Review Focus

- Empty selected ID sets: filters must not silently expand to the full corpus (Task 1).
- Time passing with unchanged issue bytes: defer_until expiry must become visible without a database edit (Task 1).
- A flow endpoint absent from the filtered list: detail inspection must not select an unrelated visible issue (Task 2).
- Two external requests completing in reverse order: old clipboard/Cass work must not change the newest state (Tasks 3 and 5).
- Duplicate IDs with different descriptions: final suggestion explanations must use the original source pair (Task 6).

## File structure and execution ownership

The readiness component and asynchronous action lifecycles justify focused new
production files. Existing rendering, query, and analysis files are edited in
place. Extend existing test files when they already own the behavior. Tasks run
serially in `.claude/worktrees/upstream-improvements` on
`feat/upstream-improvements` to avoid concurrent edits to the model, key handlers,
and analysis wiring. Each task gets a fresh implementer and task-scoped reviewer.

| Task | Bead | Responsibility |
| --- | --- | --- |
| 1 | bt-k20.2 | Pure readiness model, data parity, context preservation, all Go/TUI/robot consumers |
| 2 | bt-k20.3 | Flow pair rows, independent endpoint inspection, return/refresh behavior |
| 3 | bt-k20.4 | Shared asynchronous clipboard operation and all copy callsites |
| 4 | bt-k20.5 | Shortest-path dependency expansion and annotations |
| 5 | bt-k20.6 | Cancellable Cass request lifecycle and retryable caches |
| 6 | bt-k20.7 | Bounded duplicate candidates, oracle tests, paired benchmark |

No task creates a new BQL field or browser algorithm. Existing readiness-related
consumers receive consistent context; browser timeline/WASM is outside scope.

### Task 1: Unified full-source readiness and claimability

**Files:**
- Create: `pkg/model/readiness.go`, `pkg/model/readiness_test.go`.
- Modify: `pkg/model/types.go`, relevant existing model/loader tests,
  `internal/datasource/columns.go`, `internal/datasource/dolt.go`,
  `internal/datasource/global_dolt.go`, their existing tests.
- Modify: `pkg/analysis/external_resolution.go`, `graph.go`, `plan.go`,
  `triage.go`, `triage_context.go`, `cache.go` (ComputeDataHash), and existing
  readiness-related forecasting/scoring tests.
- Modify: `cmd/bt/context.go`, `root.go`, `cobra_robot.go`, `robot_ctx.go`,
  `robot_triage.go`, `robot_analysis.go`, `robot_forecast.go`, existing recipe
  filtering and Go-export analysis callsites.
- Modify: `pkg/ui/snapshot.go`, `background_worker.go`, `filterset.go`,
  `footer_triad.go`, `model_filter.go`, and model reload/tick integration.
- Test: existing analysis, datasource, robot, filter/snapshot, and footer tests;
  add targeted integration tests in their owning packages when necessary.

**Interfaces:**
- Consumes: `[]model.Issue`, existing external alias resolver, explicit selected IDs.
- Produces the following pure model API (names/types are the cross-task contract):

```go
type DependencyState string
const (
    DependenciesSatisfied DependencyState = "satisfied"
    DependenciesUnsatisfied DependencyState = "unsatisfied"
    DependenciesUnknown DependencyState = "unknown"
)
func NewReadinessIndex(issues []Issue) *ReadinessIndex
func (r *ReadinessIndex) DependencyState(id string) DependencyState
func (r *ReadinessIndex) Ready(id string, now time.Time) bool
func (r *ReadinessIndex) Claimable(id string, now time.Time) bool
func (r *ReadinessIndex) Blockers(id string) []string
func (r *ReadinessIndex) ReadyAfter(id string, now time.Time, completed map[string]bool) bool
```

- Add `DeferUntil *time.Time` with JSON tag `defer_until,omitempty`.
- Analyzer/triage options carry the immutable full-source readiness authority,
  selected IDs, and evaluation time. Preserve the default NewAnalyzer entry point
  by deriving authority from its input for genuinely unscoped callers; scoped
  callers explicitly provide full context rather than a second competing policy.

- [ ] **Step 1: Pin model eligibility with failing tests.** Include this core case
  and table-driven combinations for direct/parent blockers, cycles, depth > 50,
  claimable leaf policy, null/expired/future deferral and all supported statuses:

```go
func TestReadinessMissingBlockerIsUnknown(t *testing.T) {
    r := NewReadinessIndex([]Issue{{ID: "bt-a", Status: StatusOpen,
        Dependencies: []*Dependency{{DependsOnID: "bt-missing", Type: DepBlocks}}}})
    if got := r.DependencyState("bt-a"); got != DependenciesUnknown {
        t.Fatalf("state = %s", got)
    }
    if r.Ready("bt-a", time.Unix(0, 0)) { t.Fatal("missing blocker became ready") }
}
```

- [ ] **Step 2: Run the focused model tests and record the pre-implementation failure.**
  Run `go test ./pkg/model -run Readiness -count=1`.
- [ ] **Step 3: Implement immutable copied inputs, dependency propagation, and deferral.**
  Derive dependency state once; unknown dominates unsatisfied. For parent edges,
  propagate parent state in topological order and mark remaining hierarchy cycles
  unknown. Closed parents terminate inheritance. Copy pointers/dependencies so
  caller mutation cannot alter the index. Use one per-query memo for ReadyAfter.

```go
func (r *ReadinessIndex) Ready(id string, now time.Time) bool {
    issue, ok := r.issues[id]
    return ok && (issue.Status == StatusOpen || issue.Status == StatusInProgress) &&
        (issue.DeferUntil == nil || !issue.DeferUntil.After(now)) &&
        r.DependencyState(id) == DependenciesSatisfied
}
```

- [ ] **Step 4: Add and run loader/hash/context regression tests before wiring.**
  Use the same exported JSON timestamp in embedded and SQL reader fixtures.
  Verify changing only DeferUntil changes relevant hashes and survives Clone.
  Add resolver cases for unknown/malformed/ambiguous aliases and idempotence.
  Add `--repo`/source/label/recipe/BQL tests where A is selected and B is a hidden
  blocker; closed B satisfies A, open B withholds it, no B yields unknown.
- [ ] **Step 5: Wire full corpus, time, and candidate selection end-to-end.**
  Capture corpus before root repo filtering; preserve alias evidence; update SQL
  column order and scan destinations together. Carry full authority through every
  scoped Analyzer/ComputeTriage construction. Use Ready for plans/counts and
  Claimable for immediate picks and emitted new-claim commands. Preserve manual
  claims and graph metric scope. Keep empty selection distinct from unset scope.

```go
// Selection limits output, while authority retains hidden prerequisites.
for _, issue := range selectedIssues {
    if readiness.Ready(issue.ID, evaluatedAt) { actionable = append(actionable, issue) }
}
```

- [ ] **Step 6: Exercise expiry and no-pick behavior in UI/robot tests.** A fixed
  clock at expiry must refresh readiness without source edits; in_progress stays
  InFlight in the footer but is not a new-claim pick. Simulating A closed cannot
  unblock C if another hidden/missing prerequisite remains. Preserve sorted IDs.
- [ ] **Step 7: Run affected suites and required gates.**
  `go test ./pkg/model ./pkg/loader ./internal/datasource ./pkg/analysis ./pkg/ui ./cmd/bt`,
  targeted `go test -race` for snapshot/time lifecycle tests,
  `go build ./...`, and `go vet ./...`.
- [ ] **Step 8: Document Ready versus Claimable and conservative unknown results**
  in the existing robot docs; close bt-k20.2 with evidence and make directed
  `feat(analysis): unify full-source readiness (bt-k20.2)` commit(s).

### Task 2: Relationship-level flow drilldowns

**Files:** Modify `pkg/ui/flow_matrix.go`, `flow_matrix_keys.go`,
`flow_matrix_test.go`, and existing model detail/focus integration/tests.

**Interfaces:**
- Consumes: `analysis.CrossLabelFlow.Dependencies[].BlockingPairs`, current full
  issue lookup and existing detail renderer.
- Produces: canonical pair/endpoint selection stored by ID; detail inspection
  independent of list indices. Existing SelectedDrilldownIssue remains usable.

- [ ] **Step 1: Add failing pair and hidden-endpoint tests.** Build a fixture with
  A blocking B across labels plus unrelated same-label C. Opening the label must
  contain A and B but not C, and a second label on A must not duplicate the pair.
  Set the active list filter to only C; inspecting B still renders B and Escape
  restores the pair and original filter.

```go
for _, row := range m.drilldownIssues {
    if row.ID == "bt-unrelated" { t.Fatal("unrelated label member in flow pairs") }
}
```

- [ ] **Step 2: Run `go test ./pkg/ui -run FlowMatrix -count=1` to capture failure.**
- [ ] **Step 3: Build deterministic pairs and a bounded detail-return state.**

```go
type flowPairKey struct { blocker, blocked string }
seen := make(map[flowPairKey]bool)
// For matching FromLabel or ToLabel, append each BlockingPairs key once.
// Sort by blocker, then blocked; render two inspectable endpoint rows per pair.
```

  Store detail target separately from list selection. Escape unwinds inspection,
  drilldown, then matrix. Rebuild and restore selection by pair ID and endpoint
  on refresh; clamp deleted selections and close a vanished-label drilldown.
- [ ] **Step 4: Add refresh/deletion/empty/narrow-terminal regression cases** and
  run `go test ./pkg/ui -run 'FlowMatrix|FilterPreservation' -count=1`, then the
  full UI suite, build, and vet.
- [ ] **Step 5: Close bt-k20.3 with evidence and commit directed paths** using
  `feat(tui): inspect cross-label blocking pairs (bt-k20.3)`.

### Task 3: Asynchronous clipboard and remote fallback

**Files:** Create focused `pkg/ui/clipboard.go` and `clipboard_test.go` for the
new lifecycle. Modify `model_export.go`, `cass_session_modal.go`, `list_keys.go`,
`board_keys.go`, `history_keys.go`, model input/message/shutdown handling, and
existing copy tests to execute commands and completion messages.

**Interfaces:**
- Consumes: captured text, success label, request identity, TUI output capability.
- Produces: one model-level copy request path and typed results, reused by Cass.
  Keep test seams injectable without replacing production safety checks.

```go
type clipboardResultMsg struct {
    requestID uint64
    err error
    // Fallback sequence is emitted only after Update validates request identity.
    sequence string
}
```

- [ ] **Step 1: Add failing sequence/helper/lifecycle tests.** Test raw tmux,
  screen passthrough, display-less discovery, helper timeout and failure,
  base64 payload limits, no redirected output, and reversed request completion.

```go
// A delayed request A must not replace request B's clipboard or feedback.
// Use channels for helper entry/release and explicit result delivery, not sleeps.
if strings.Contains(capturedStdout, "\x1b]52;") {
    t.Fatal("terminal clipboard sequence leaked to redirected stdout")
}
```

- [ ] **Step 2: Run `go test ./pkg/ui -run Clipboard -count=1` to capture failure.**
- [ ] **Step 3: Implement command-owned, cancellable, serialized helper work.**
  Use a shared controller with mutex-protected request/cancellation state to
  serialize writes across value copies of Model. On a new copy, cancel the old
  helper and skip obsolete queued requests. Local helpers use CommandContext
  and bounded pipe waits. Fall back after the two-second helper phase only for
  the still-current request; cancellation itself must not trigger stale fallback.
  Send terminal output using Bubble Tea v2's serialized output API only when an
  interactive terminal is available. Wrap errors and report status via messages.
- [ ] **Step 4: Route every copy action through this path** including Cass modal
  requests returned to parent Update. Replace synchronous clipboard tests with
  command/result tests that prove the copied payload, not merely no panic.
- [ ] **Step 5: Run full UI tests, targeted clipboard race checks, build and vet.**
  Record native terminal delivery as unverified if the environment lacks it.
- [ ] **Step 6: Close bt-k20.4 and commit directed paths** using
  `feat(tui): add asynchronous remote clipboard support (bt-k20.4)`.

### Task 4: Deduplicated dependency trees

**Files:** Modify `pkg/ui/helpers.go` and `pkg/ui/helpers_test.go`.

**Interfaces:**
- Consumes: existing `BuildDependencyTree(rootID, issueMap, maxDepth)`.
- Produces: same API and existing node fields, plus `Reference bool` and
  `Cycle bool` annotations; rendering preserves styled statuses/inverse children.

- [ ] **Step 1: Add failing diamond/shortest-path tests and assert metadata.**
  Use A -> B,C; B -> D; C -> D and a longer-first path to D. Only one D expands,
  D's other occurrence has Reference=true and Cycle=false, and its descendants
  remain visible if reachable within the shortest-path cutoff.

```go
if repeated.Title != "Shared prerequisite" || !repeated.Reference || repeated.Cycle {
    t.Fatalf("bad shared target: %+v", repeated)
}
```

- [ ] **Step 2: Run `go test ./pkg/ui -run 'DependencyTree' -count=1` to capture failure.**
- [ ] **Step 3: Implement breadth-first minimum depths, canonical expansion, and
  separate cycle-edge discovery.** Preserve every outgoing relationship within
  depth, mark duplicate targets as references, and mark DFS backedges as cycles.
  Inverse children remain naturally sorted leaves, never traversal edges.
  Deduplicate inverse child entries without merging different outgoing types.
  Keep nil/missing rows safe and annotations separate from truncated titles.
- [ ] **Step 4: Extend cycle/nil/depth/unlimited/inverse-child rendering tests.**
  Run dependency tests, full UI suite, build and vet.
- [ ] **Step 5: Close bt-k20.5 and commit directed paths** using
  `feat(tui): deduplicate dependency tree expansion (bt-k20.5)`.

### Task 5: Asynchronous cancellable Cass lookup

**Files:** Create focused `pkg/ui/cass_lookup.go` and lifecycle tests if the
existing export file would become overloaded. Modify `model_export.go`, model
state/update/key/shutdown handlers, `cass_session_modal.go`, and relevant
`pkg/cass` correlation/cache files and tests.

**Interfaces:**
- Consumes: captured issue/source/dataset identity, existing cass correlator and
  modal; Task 3's clipboard request path for copying commands.
- Produces: `cassSessionsLoadedMsg` with request generation, source/data identity,
  result/error; model owns pending cancellation and originating focus.

- [ ] **Step 1: Add failing asynchronous lifecycle tests with a controlled fake.**
  Delay the fake correlator on a channel and prove Update still handles resize
  and navigation. Deliver a result after Escape, reselection, dataset reload,
  conflicting modal, and a newer request; none may open the old modal.

```go
// Deliver A after B was requested; B remains authoritative.
m, _ = m.Update(oldCompletion)
if m.focused == focusCassModal { t.Fatal("stale lookup opened a modal") }
```

- [ ] **Step 2: Run `go test ./pkg/ui ./pkg/cass -run Cass -count=1` and capture failure.**
- [ ] **Step 3: Move detection and correlation into one three-second command.**
  Copy request data, use context propagation through process helpers, and return
  immutable results. Update alone owns modal/status/focus changes. Cancel on all
  specified invalidation paths; retry partial/failing results; scope caches to
  the captured corpus/source and protect shared state.
- [ ] **Step 4: Wire loading feedback, second-invocation cancellation, successful
  empty/error handling, and originating focus restoration.** Keep Cass modal
  clipboard events integrated with Task 3 rather than running helpers directly.
- [ ] **Step 5: Run full UI/Cass suites, targeted race checks, build and vet.**
- [ ] **Step 6: Close bt-k20.6 and commit directed paths** using
  `feat(tui): make Cass session lookup cancellable (bt-k20.6)`.

### Task 6: Bounded duplicate detection with paired measurements

**Files:** Modify `pkg/analysis/duplicates.go`, `duplicates_test.go`, and the
existing duplicates benchmark owner (add a focused benchmark file only if none).

**Interfaces:**
- Consumes/produces the existing `DetectDuplicates([]model.Issue, DuplicateConfig) []Suggestion`.
- Preserve full explanation contents, source-oriented action/issue fields, and
  filters. Per the recorded implementation ruling, unique-ID ties use canonical
  ID comparison keys without reorienting output; old ties were unspecified.

- [ ] **Step 1: Add a test-only full-sort oracle from the current implementation.**
  Cover dense/sparse fixtures, deterministic random input, repeated IDs with
  different descriptions, ties, statuses and K <= 0, 1, 20 and above pair count.

```go
got := DetectDuplicates(issues, cfg)
want := detectDuplicatesFullSortOracle(issues, cfg)
if !reflect.DeepEqual(got, want) { t.Fatalf("suggestions differ:\ngot %#v\nwant %#v", got, want) }
```

- [ ] **Step 2: Run the regression cases, including a failing nonpositive-K case,
  and capture baseline dense/sparse benchmark samples before modifying production.**
- [ ] **Step 3: Implement compact top-K retention for unique IDs.** Candidate
  fields hold source indices and score. Comparator preserves descending score
  then canonical IDs; heap root is worst retained. Generate common keywords only
  after sorting retained candidates. Repeated-ID input uses the exact full-sort
  path and original source indices. K <= 0 returns no suggestions in both paths.
- [ ] **Step 4: Run oracle parity tests and paired benchmark subcases.**
  Use dense 500 issues/K=20 and a representative sparse fixture, `-benchmem`
  and repeated samples (`-count=5`); report old/new ns/op, B/op and allocs/op.
  Verify dense allocation reduction and record any sparse tradeoff.
- [ ] **Step 5: Run full analysis suite, build and vet.**
- [ ] **Step 6: Close bt-k20.7 with measurements and commit directed paths** using
  `perf(analysis): bound duplicate candidate selection (bt-k20.7)`.

## Integration verification and delivery

- [ ] Independently review each task for spec compliance and code quality before
  starting the next one. Fix and re-review important findings through subagents.
- [ ] After the six tasks, run `go test ./...`, `go build ./...`, `go vet ./...`,
  and focused race tests covering the introduced concurrent paths.
- [ ] Run whole-branch review against `622f3637`; include task reports and any
  baseline failures. Fix confirmed integration defects through a subagent.
- [ ] Update plan completion boxes, record verification evidence in bt-k20,
  close completed beads before directed commits, and sync beads from the primary
  checkout. Keep the feature worktree/branch available for user-directed merge;
  do not push code or delete worktrees/scratch without authorization.
