# Task 2 implementation report

Bead: bt-k20.3
Base: 1ee90f34636833c21a4e066c2f694ee23eac1d96

## Implementation

- Flow drilldowns now use actual `CrossLabelFlow.Dependencies[].BlockingPairs`, match either label side, deduplicate canonical blocker/blocked IDs, and sort pairs deterministically.
- Each pair produces independently selectable blocker and blocked endpoint rows with status, ID, title, and explicit `BLOCKER -> BLOCKED` direction. Unrelated issues sharing a label are excluded; missing endpoint records are omitted safely.
- Relationship scope respects active repository scope but excludes status/label and transient list text filters. Endpoint details resolve from that same scoped source set, leaving the selected list item and filter untouched.
- Enter inspects the selected endpoint using the existing detail renderer. Escape unwinds inspection, then relationship drilldown, then matrix. Refresh retains label/pair/endpoint identity, clamps removed endpoints, closes drilldown when its label disappears, and displays a bounded empty state.
- Narrow-width rendering truncates endpoint rows to available display cells.

## Test evidence

- TDD RED: `go test ./pkg/ui -run FlowMatrix -count=1` failed before implementation because the old drilldown included the unrelated label member and refresh did not retain a relationship endpoint.
- `go test ./pkg/ui -run 'FlowMatrix|FilterPreservation' -count=1` passed.
- `go test ./pkg/ui -count=1` passed.
- `go test ./...` passed all packages, including `pkg/ui` and `tests/e2e`.
- `go build ./...` passed (silent success).
- `go vet ./...` passed (silent success).
- `git diff --check` passed.

Regression coverage includes actual pair selection, multi-label deduplication, unrelated-member exclusion, filtered-out endpoint detail inspection with list selection unchanged, detail Escape return, selection retention across refresh, empty/deleted endpoints, vanished labels, and narrow dimensions. Existing cross-repository flow scope coverage also passes.

## Self-review and concerns

- Pair IDs, not row indices, identify selection during refresh. If an endpoint disappears, the cursor clamps to the nearest available endpoint and stale detail inspection is unwound.
- The flow view continues to honor active repository scope; status/label and transient text filtering do not erase relationships needed for inspection. This preserves the existing repository-scope behavior while making list search independent.
- The existing issue-detail renderer is reused by rendering the selected full-source endpoint into the viewport without selecting or mutating a list row.
- The worktree had a pre-existing unrelated `go.mod` change promoting `github.com/charmbracelet/ultraviolet` from indirect to direct. It was left untouched and excluded from the directed commit.
- Native interactive visual verification was not performed; narrow layout is covered by UI tests.

## Review round 1 fix report

Fix base: `2e886f7109a6bb7a1ed123c15bcdd0f3063986ce`.

- Replaced the frozen inspection string with an independent Bubbles viewport. Keyboard scrolling and mouse-wheel scrolling route to it while inspecting; resize adjusts its dimensions and re-renders the detail content.
- Saved the originating list-detail viewport while inspecting and restored its content/offset on return. The final Escape is marked consumed so Bubbles does not clear an applied search; the selected list item and filter remain unchanged.
- Drilldown header, separator, empty state, endpoint rows, and row count are all bounded by nonnegative dimensions. Tiny/zero-width and zero-height surfaces produce bounded output.
- Added end-to-end event-route coverage through `Model.Update` for inspection, page scrolling, mouse wheel, resize, all three Escape returns, applied-search/selection retention, and original detail-scroll retention. Added per-line display-width and total-height assertions for populated, empty, and tiny drilldowns.

Round 1 RED evidence: before the fix, `go test ./pkg/ui -run TestFlowMatrixEndpointInspectionIgnoresListFilter -count=1` failed because inspection reset the originating detail offset (`got 0 want 7`).

Round 1 verification:

- `go test ./pkg/ui -run 'TestFlowMatrix(EndpointInspectionIgnoresListFilter|DrilldownBoundsEveryLineAndRow|DrilldownRefreshRetainsPairAndEndpoint)' -count=1` passed.
- `go test ./pkg/ui -run 'FlowMatrix|FilterPreservation' -count=1` passed.
- `go test ./pkg/ui -count=1` passed.
- `go build ./...` passed (silent success).
- `go vet ./...` passed (silent success).
- `git diff --check` passed.

`go.mod` remains an unrelated pre-existing change and is excluded from the fix commit. No ignored report artifacts were force-added in this round.
