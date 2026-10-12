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
