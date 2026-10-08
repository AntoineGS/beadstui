package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/correlation"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/recipe"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// setListItems sets list items while preserving any active Bubbles filter
// (bt-nzsy).
//
//   - Bubbles filter (the `/` search): list.Model.SetItems clears the internal
//     filteredItems slice when a filter is active but does not re-run the match,
//     so downstream renders show "No items." until the user mutates the filter
//     text to trigger a re-match. SetFilterText synchronously re-runs the
//     filter against the new items, restoring search persistence across
//     background refreshes.
//   - Every caller passes items built from the visible set (bt-imh), so this
//     function no longer filters.
//
// All refresh paths that replace list items MUST go through this wrapper.
// A guard test (TestNoRawListSetItems) fails if m.list.SetItems is called
// directly outside this function.
func (m *Model) setListItems(items []list.Item) {
	// The footer's actionable triad is scoped to exactly what the list shows.
	// setListItems is the single chokepoint for list contents, so computing the
	// triad here keeps it in lockstep with TotalItems (= len(list items)) and
	// reflective of the active scope + filters rather than the global corpus —
	// the generalization of bt-gcuv, extended by bt-p8y2f to match pkg/analysis
	// semantics (see footer_triad.go). The footer is the only reader of
	// m.ac.count*, so this is their single source of truth.
	triad := m.lensTriad(items)
	m.ac.countReady, m.ac.countInFlight, m.ac.countBlocked = triad.Ready, triad.InFlight, triad.Blocked

	prevState := m.list.FilterState()
	prevValue := m.list.FilterValue()
	m.list.SetItems(items)
	if prevState == list.Filtering || prevState == list.FilterApplied {
		m.list.SetFilterText(prevValue)
		m.list.SetFilterState(prevState)
	}

	// A new filter starts at the first row; a refresh under the same filter
	// keeps the selection (bt-qc3).
	if key := m.cursorKey(); key != m.filter.appliedKey {
		m.filter.appliedKey = key
		m.list.Select(0)
	}
}

// filterSpec derives the engine's spec from the model's filter state.
func (m *Model) filterSpec() FilterSpec {
	spec := FilterSpec{
		Workspace: m.workspaceMode,
		Repos:     m.activeRepos,
		Status:    m.filter.currentFilter,
		ShowWisps: m.showWisps,
	}
	if m.filter.labelFilter != "" {
		spec.Labels = strings.Split(m.filter.labelFilter, ",")
	}
	switch {
	case m.filter.activeBQLExpr != nil && strings.HasPrefix(m.filter.currentFilter, "bql:"):
		spec.BQL = m.filter.activeBQLExpr
		spec.BQLText = strings.TrimPrefix(m.filter.currentFilter, "bql:")
	case m.filter.activeRecipe != nil:
		spec.Recipe = m.filter.activeRecipe
	}
	return spec
}

func (m *Model) filterEnv() FilterEnv {
	return FilterEnv{
		IssueMap: m.data.issueMap,
		BQL:      m.filter.bqlEngine,
		BQLOpts:  m.bqlExecuteOpts(),
		Stats:    m.data.analysis,
	}
}

// applySpec evaluates a variant of the active filter (facet counts, scope
// totals) against the full corpus.
func (m *Model) applySpec(spec FilterSpec) []model.Issue {
	return spec.Apply(m.data.issues, m.filterEnv())
}

// refreshVisible recomputes the visible set from the active filter.
func (m *Model) refreshVisible() {
	spec := m.filterSpec()
	m.filter.visible = m.applySpec(spec)
	m.filter.visibleIDs = make(map[string]struct{}, len(m.filter.visible))
	for i := range m.filter.visible {
		m.filter.visibleIDs[m.filter.visible[i].ID] = struct{}{}
	}
	m.filter.visibleKey = spec.Key()
	m.filter.scopeCount = len(m.applySpec(spec.Without(DimPrimary | DimLabels)))
}

// visibleIssues is the visible set in list order.
func (m *Model) visibleIssues() []model.Issue {
	return m.filter.visible
}

// cursorKey identifies a filter for bt-qc3 (a new filter starts at the first
// row). Toggling wisps is a refresh, not a new filter.
func (m *Model) cursorKey() string {
	return m.filterSpec().Without(DimWisps).Key()
}

// canUseSnapshot reports whether the precomputed board/graph/tree in the
// snapshot shows exactly the visible set. The count check catches hidden
// wisps, which snapshots include.
func (m *Model) canUseSnapshot(spec FilterSpec, visibleCount int) bool {
	s := m.data.snapshot
	if s == nil || visibleCount != len(s.Issues) {
		return false
	}
	if spec.Recipe != nil && spec.BQL == nil {
		return spec.Without(DimPrimary).IsUnfiltered() &&
			s.RecipeName == spec.Recipe.Name && s.RecipeHash == recipeFingerprint(spec.Recipe)
	}
	return spec.IsUnfiltered()
}

// buildIssueItem is the one place list items are built from issues.
func (m *Model) buildIssueItem(issue model.Issue, epics map[string]epicProgressCount) IssueItem {
	item := IssueItem{
		Issue:      issue,
		DiffStatus: m.getDiffStatus(issue.ID),
		RepoPrefix: ExtractRepoPrefix(issue.ID),
	}
	if m.data.analysis != nil {
		item.GraphScore = m.data.analysis.GetPageRankScore(issue.ID)
		item.Impact = m.data.analysis.GetCriticalPathScore(issue.ID)
	}
	item.TriageScore = m.ac.triageScores[issue.ID]
	if reasons, ok := m.ac.triageReasons[issue.ID]; ok {
		item.TriageReason = reasons.Primary
		item.TriageReasons = reasons.All
	}
	item.IsQuickWin = m.ac.quickWinSet[issue.ID]
	item.IsBlocker = m.ac.blockerSet[issue.ID]
	item.UnblocksCount = len(m.ac.unblocksMap[issue.ID])
	if issue.IssueType == model.TypeEpic {
		c := epics[issue.ID]
		item.EpicDone, item.EpicTotal = c.done, c.total
	}
	item.GateAwaitType = gateAwaitFromBlockers(issue, m.data.issueMap)
	return item
}

// ensureTriageForVisible ranks triage over the visible set: list-row badges,
// insights top picks and recommendations. It recomputes once per filter key
// and data load; force skips the Phase 2 wait (opening insights).
func (m *Model) ensureTriageForVisible(force bool) {
	if m.ac.triage != nil && m.ac.triageKey == m.filter.visibleKey {
		return
	}
	if m.ac.triageWaitPhase2 && !force {
		return
	}
	triage := analysis.ComputeTriageWithOptions(m.filter.visible, analysis.TriageOptions{WaitForPhase2: true})
	m.setTriage(&triage)
	m.ac.triageKey = m.filter.visibleKey
}

// setTriage installs a triage result as the list-row badge maps.
func (m *Model) setTriage(t *analysis.TriageResult) {
	scores := make(map[string]float64, len(t.Recommendations))
	reasons := make(map[string]analysis.TriageReasons, len(t.Recommendations))
	unblocks := make(map[string][]string, len(t.Recommendations))
	quickWins := make(map[string]bool, len(t.QuickWins))
	blockers := make(map[string]bool, len(t.BlockersToClear))
	for _, rec := range t.Recommendations {
		scores[rec.ID] = rec.Score
		if len(rec.Reasons) > 0 {
			reasons[rec.ID] = analysis.TriageReasons{Primary: rec.Reasons[0], All: rec.Reasons, ActionHint: rec.Action}
		}
		unblocks[rec.ID] = rec.UnblocksIDs
	}
	for _, qw := range t.QuickWins {
		quickWins[qw.ID] = true
	}
	for _, bl := range t.BlockersToClear {
		blockers[bl.ID] = true
	}
	m.ac.triage = t
	m.ac.triageScores, m.ac.triageReasons, m.ac.unblocksMap = scores, reasons, unblocks
	m.ac.quickWinSet, m.ac.blockerSet = quickWins, blockers
}

func triageDataHash(t *analysis.TriageResult) string {
	return fmt.Sprintf("v%s@%s#%d", t.Meta.Version, t.Meta.GeneratedAt.Format("15:04:05"), t.Meta.IssueCount)
}

// visibleInsights drops hidden issues from the graph-metric rankings. The
// metrics themselves stay computed over the full graph (spec exceptions);
// ClusterDensity and Velocity are whole-graph values and pass through.
func (m *Model) visibleInsights(ins analysis.Insights) analysis.Insights {
	ids := m.filter.visibleIDs
	if ids == nil || len(m.filter.visible) == len(m.data.issueMap) {
		return ins
	}
	keepItems := func(items []analysis.InsightItem) []analysis.InsightItem {
		out := items[:0:0]
		for _, it := range items {
			if _, ok := ids[it.ID]; ok {
				out = append(out, it)
			}
		}
		return out
	}
	keepIDs := func(in []string) []string {
		out := in[:0:0]
		for _, id := range in {
			if _, ok := ids[id]; ok {
				out = append(out, id)
			}
		}
		return out
	}
	ins.Bottlenecks = keepItems(ins.Bottlenecks)
	ins.Keystones = keepItems(ins.Keystones)
	ins.Influencers = keepItems(ins.Influencers)
	ins.Hubs = keepItems(ins.Hubs)
	ins.Authorities = keepItems(ins.Authorities)
	ins.Cores = keepItems(ins.Cores)
	ins.Slack = keepItems(ins.Slack)
	ins.Articulation = keepIDs(ins.Articulation)
	ins.Orphans = keepIDs(ins.Orphans)
	// A cycle is shown only when every member is visible: listing a partial
	// cycle would show hidden issues or invent a cycle that isn't there.
	var cycles [][]string
	for _, c := range ins.Cycles {
		if len(keepIDs(c)) == len(c) {
			cycles = append(cycles, c)
		}
	}
	ins.Cycles = cycles
	return ins
}

// refreshLabelDashboard computes label health over the visible set.
func (m *Model) refreshLabelDashboard() {
	if !m.labelHealthCached {
		cfg := analysis.DefaultLabelHealthConfig()
		m.labelHealthCache = analysis.ComputeAllLabelHealth(m.filter.visible, cfg, time.Now().UTC(), m.data.analysis)
		m.labelHealthCached = true
	}
	m.labelDashboard.SetData(m.labelHealthCache.Labels)
	m.labelDashboard.SetSize(m.width, m.height-1)
	m.setStatus(fmt.Sprintf("Labels: %d total • critical %d • warning %d", m.labelHealthCache.TotalLabels, m.labelHealthCache.CriticalCount, m.labelHealthCache.WarningCount))
}

// refreshAttentionView computes attention scores and text over the visible set.
func (m *Model) refreshAttentionView() {
	if !m.attentionCached {
		cfg := analysis.DefaultLabelHealthConfig()
		m.attentionCache = analysis.ComputeLabelAttentionScores(m.filter.visible, cfg, time.Now().UTC())
		m.attentionCached = true
	}
	attText, _ := ComputeAttentionView(m.filter.visible, max(40, m.width-4))
	m.insightsPanel = NewInsightsModel(analysis.Insights{}, m.data.issueMap, m.theme)
	m.insightsPanel.labelAttention = m.attentionCache.Labels
	m.insightsPanel.extraText = attText
	m.insightsPanel.SetSize(m.width, max(3, m.height-2))
}

// refreshFlowMatrix computes cross-label flow over the visible set.
func (m *Model) refreshFlowMatrix() {
	cfg := analysis.DefaultLabelHealthConfig()
	flow := analysis.ComputeCrossLabelFlow(m.filter.visible, cfg)
	m.flowMatrix = NewFlowMatrixModel(m.theme)
	m.flowMatrix.SetData(&flow, m.filter.visible)
	m.flowMatrix.SetSize(m.width, max(3, m.height-2))
}

// refreshActionableView builds the execution plan from the visible set.
func (m *Model) refreshActionableView() {
	plan := analysis.NewAnalyzer(m.filter.visible).GetExecutionPlan()
	m.actionableView = NewActionableModel(plan, m.theme)
	m.actionableView.SetSize(m.width, m.height-2)
}

// refreshAnalysisViews recomputes the open analysis view after a filter change.
func (m *Model) refreshAnalysisViews() {
	switch m.mode {
	case ViewActionable:
		m.refreshActionableView()
	case ViewLabelDashboard:
		m.refreshLabelDashboard()
	case ViewAttention:
		m.refreshAttentionView()
	case ViewFlowMatrix:
		m.refreshFlowMatrix()
	case ViewInsights:
		// Rebuilding the panel resets its cursor; keep the user's pane and row.
		panel := m.insightsPanel.FocusedPanel()
		row := m.insightsPanel.SelectedIndexFor(panel)
		m.openInsightsView()
		m.insightsPanel.RestoreCursor(panel, row)
	}
}

// lensTriad (footer_triad.go) is the bt-p8y2f successor to the former
// classifyItemCounts: it partitions whatever items the list currently holds
// into the footer's ready/in-flight/blocked triad, resolved against
// pkg/analysis's graph-based actionable definition rather than an inline
// per-item dependency walk.

// getDiffStatus returns the diff status for an issue if time-travel mode is active
func (m Model) getDiffStatus(id string) DiffStatus {
	if !m.timeTravelMode {
		return DiffStatusNone
	}
	if m.newIssueIDs[id] {
		return DiffStatusNew
	}
	if m.closedIssueIDs[id] {
		return DiffStatusClosed
	}
	if m.modifiedIssueIDs[id] {
		return DiffStatusModified
	}
	return DiffStatusNone
}

// hasActiveFilters returns true if any filter is currently applied
// (status filter, label filter, recipe filter, or fuzzy search)
func (m *Model) hasActiveFilters() bool {
	// Check status filter
	if m.filter.currentFilter != defaultStatusFilter {
		return true
	}
	// Check label filter
	if m.filter.labelFilter != "" {
		return true
	}
	// Check sort mode
	if m.filter.sortMode != SortDefault {
		return true
	}
	// Check if fuzzy search filter is active
	if m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied {
		return true
	}
	return false
}

// selectIssueByID places the list cursor on the issue with the given ID,
// safely respecting Bubbles filter state. If the issue is in the visible
// (filtered) view, select it by its visible index. If the issue exists
// but a filter currently hides it, reset the filter first so the jump
// lands on the intended row. Returns true if the selection was made.
//
// This fixes the "Select(unfilteredIndex) on narrowed filter" crash class
// (bt-nzsy) in user-initiated jumps from the alerts + notifications modal:
// the old code iterated m.list.Items() (unfiltered) and called Select(i),
// which drove Paginator.Page past TotalPages-1 when the filter narrowed
// the visible set to fewer items than the unfiltered index.
func (m *Model) selectIssueByID(issueID string) bool {
	if issueID == "" {
		return false
	}
	selectVisible := func() bool {
		for i, it := range m.list.VisibleItems() {
			if item, ok := it.(IssueItem); ok && item.Issue.ID == issueID {
				m.list.Select(i)
				return true
			}
		}
		return false
	}
	if selectVisible() {
		return true
	}
	// Not in the visible set. If a filter is active, clear it and retry —
	// the user's intent when jumping from a notification/alert is "take me
	// there," which outranks preserving an incompatible filter.
	if m.list.FilterState() != list.Unfiltered {
		m.list.ResetFilter()
		if selectVisible() {
			return true
		}
	}
	// Hidden only by the default open filter (bt-fx1): epics, alerts and
	// notifications still reach closed beads. An explicit filter is kept.
	if m.filter.currentFilter == defaultStatusFilter && m.filter.activeRecipe == nil {
		if issue, ok := m.data.issueMap[issueID]; ok && issue != nil && isClosedLikeStatus(issue.Status) {
			m.filter.currentFilter = "all"
			m.applyFilter()
			return selectVisible()
		}
	}
	return false
}

// focusDetailAfterJump puts focus on the detail pane after a jump from the
// alerts/notifications modal (bt-46p6.10 dogfood). In split view, focus flips
// to the detail pane alongside the list. In single-pane view, the detail
// overlay opens and scrolls to top. Caller is responsible for having already
// placed the list cursor on the target issue.
func (m *Model) focusDetailAfterJump() {
	m.mode = ViewList
	if m.isSplitView {
		m.focused = focusDetail
	} else {
		m.showDetails = true
		m.focused = focusDetail
		m.viewport.GotoTop()
	}
	m.updateViewportContent()
}

// commitFilterIfTyping transitions the Bubbles list filter from Filtering
// (input field accepting keystrokes) to FilterApplied (filter committed, no
// active input) when focus is no longer on the list. Without this, global
// hotkeys gated on FilterState != Filtering stay blocked even though no one
// is typing in the input - locking the user into mouse-only navigation.
// No-op when the filter is already FilterApplied or Unfiltered (bt-ocmw).
//
// When the typed buffer is empty, ResetFilter back to Unfiltered instead of
// committing — an applied-empty filter renders as "No items" in Bubbles even
// when the underlying list is populated, which is misleading after the user
// clicks the search row (bt-49nn) and clicks out without typing (bt-5q51).
func (m *Model) commitFilterIfTyping() {
	if m.list.FilterState() != list.Filtering {
		return
	}
	if strings.TrimSpace(m.list.FilterInput.Value()) == "" {
		m.list.ResetFilter()
		return
	}
	m.list.SetFilterState(list.FilterApplied)
}

// defaultStatusFilter is the status filter bt starts with and returns to when
// filters are cleared (bt-fx1): closed beads stay one o press away.
const defaultStatusFilter = "open"

// clearAllFilters resets all filters to their default state
func (m *Model) clearAllFilters() {
	m.filter.currentFilter = defaultStatusFilter
	m.filter.labelFilter = ""
	m.filter.sortMode = SortDefault
	m.setActiveRecipe(nil)       // Clear any active recipe filter
	m.filter.activeBQLExpr = nil // Clear BQL state
	// Reset the fuzzy search filter by resetting the filter state
	m.list.ResetFilter()
	m.applyFilter()
}

func (m *Model) setActiveRecipe(r *recipe.Recipe) {
	m.filter.activeRecipe = r
	if m.data.backgroundWorker != nil {
		m.data.backgroundWorker.SetRecipe(r)
	}
}

func (m *Model) refreshBoardAndGraphForCurrentFilter() {
	if m.mode != ViewBoard && m.mode != ViewGraph {
		return
	}

	m.refreshBoardAndGraph(m.filter.visible)
}

// refreshBoardAndGraph feeds the board and graph from the visible set,
// reusing the snapshot's precomputed layout only when it shows the same set.
func (m *Model) refreshBoardAndGraph(issues []model.Issue) {
	snap := m.canUseSnapshot(m.filterSpec(), len(issues))
	if snap && m.data.snapshot.BoardState != nil {
		m.board.SetSnapshot(m.data.snapshot)
	} else {
		m.board.SetIssues(issues)
	}
	if snap && m.data.snapshot.GraphLayout != nil {
		m.graphView.SetSnapshot(m.data.snapshot)
	} else {
		var ins analysis.Insights
		if m.data.analysis != nil {
			ins = m.data.analysis.GenerateInsights(len(issues))
		}
		m.graphView.SetIssues(issues, &ins)
	}
}

// rebuildTreeForCurrentFilter rebuilds the tree view from the visible set
// (bt-imh). A child whose parent is filtered out becomes a root. No-op outside
// the tree view.
func (m *Model) rebuildTreeForCurrentFilter() {
	if m.mode != ViewTree {
		return
	}
	if m.canUseSnapshot(m.filterSpec(), len(m.filter.visible)) {
		m.tree.BuildFromSnapshot(m.data.snapshot)
		return
	}
	// Build resets the cursor; keep the selection like BuildFromSnapshot does.
	selectedID := ""
	if sel := m.tree.SelectedIssue(); sel != nil {
		selectedID = sel.ID
	}
	m.tree.Build(m.filter.visible)
	if selectedID != "" {
		m.tree.SelectByID(selectedID)
	}
}

// toggleWisps flips ephemeral (wisp) issue visibility, re-applies the active
// filter, and posts a status message. Shared by the single-project `w`
// binding and the workspace/global-mode `ctrl+w` binding (bt-9kdo, bt-8jds).
func (m *Model) toggleWisps() {
	m.showWisps = !m.showWisps
	m.applyFilter()
	if m.showWisps {
		m.setStatus("wisps: visible")
	} else {
		m.setStatus("wisps: hidden")
	}
}

// applyFilter is the single apply path (bt-imh): it refreshes the visible set
// and feeds every surface from it. Recipes and BQL go through it too.
func (m *Model) applyFilter() {
	prevKey := m.filter.visibleKey
	m.refreshVisible()
	keyChanged := m.filter.visibleKey != prevKey
	if keyChanged {
		m.labelHealthCached = false
		m.attentionCached = false
		m.labelDrilldownCache = make(map[string][]model.Issue)
	}
	m.ensureTriageForVisible(false)

	issues := append([]model.Issue(nil), m.filter.visible...)
	epics := epicProgressIndex(m.data.issues) // epic progress counts all children
	items := make([]list.Item, len(issues))
	for i := range issues {
		items[i] = m.buildIssueItem(issues[i], epics)
	}
	// Recipes and BQL keep their own order; the s sort mode applies otherwise.
	if spec := m.filterSpec(); spec.Recipe == nil && spec.BQL == nil {
		m.sortFilteredItems(items, issues)
	}
	m.filter.visible = issues

	m.setListItems(items)
	m.updateSemanticIDs(items)
	m.refreshBoardAndGraph(issues)
	m.rebuildTreeForCurrentFilter()
	m.refreshEpicsForCurrentFilter()
	// Priority hints are computed from the visible set (bt-gcuv); only pay
	// the recompute cost while the feature is visible.
	if m.ac.showPriorityHints {
		m.recomputePriorityHints()
	}
	if keyChanged {
		m.refreshAnalysisViews()
	}

	if len(items) > 0 && m.list.Index() >= len(items) {
		m.list.Select(0)
	}
	m.updateViewportContent()
}

// refreshListItemsPhase2 updates visible items with Phase 2 scores and triage data
// without rebuilding the filtered set.
func (m *Model) refreshListItemsPhase2() {
	items := m.list.Items()
	if len(items) == 0 {
		return
	}

	// Capture selection by item ID (not index). After setListItems runs, the
	// Bubbles paginator is reset to Page 0 and VisibleItems may be filtered;
	// restoring via index would drive Page out of bounds and panic during render
	// (bt-nzsy follow-up).
	var selectedID string
	if sel := m.list.SelectedItem(); sel != nil {
		if it, ok := sel.(IssueItem); ok {
			selectedID = it.Issue.ID
		}
	}

	for i := range items {
		item, ok := items[i].(IssueItem)
		if !ok {
			continue
		}
		issueID := item.Issue.ID
		if m.data.analysis != nil {
			item.GraphScore = m.data.analysis.GetPageRankScore(issueID)
			item.Impact = m.data.analysis.GetCriticalPathScore(issueID)
		}
		item.TriageScore = m.ac.triageScores[issueID]
		if reasons, exists := m.ac.triageReasons[issueID]; exists {
			item.TriageReason = reasons.Primary
			item.TriageReasons = reasons.All
		} else {
			item.TriageReason = ""
			item.TriageReasons = nil
		}
		item.IsQuickWin = m.ac.quickWinSet[issueID]
		item.IsBlocker = m.ac.blockerSet[issueID]
		item.UnblocksCount = len(m.ac.unblocksMap[issueID])
		items[i] = item
	}

	m.setListItems(items)

	// Restore selection by ID against the (possibly filtered) view.
	if selectedID != "" {
		visible := m.list.VisibleItems()
		for i, it := range visible {
			if issueItem, ok := it.(IssueItem); ok && issueItem.Issue.ID == selectedID {
				m.list.Select(i)
				break
			}
		}
	}
	m.updateViewportContent()
}

// progressOrdinal returns the Progress-sort rank for a status (bt-lm2h).
// Lower = more "in motion" (surface higher); higher = more dormant/done.
// Unknown statuses sort last so additions upstream don't silently reshuffle.
func progressOrdinal(s model.Status) int {
	switch s {
	case model.StatusInProgress:
		return 0
	case model.StatusReview:
		return 1
	case model.StatusOpen:
		return 2
	case model.StatusHooked:
		return 3
	case model.StatusBlocked:
		return 4
	case model.StatusPinned:
		return 5
	case model.StatusDeferred:
		return 6
	case model.StatusClosed:
		return 7
	case model.StatusTombstone:
		return 8
	default:
		return 9
	}
}

// statusFilterCycle is the ordered status-membership set the lens status chip
// cycles through on one key (bt-gpvwe, absorbed by bt-2vshd). It matches the
// approved footer-lens design's enumeration (docs/design/2026-07-16-footer-lens-
// redesign.md): all -> open -> in_progress -> blocked -> closed -> deferred.
// The derived "ready" filter is deliberately NOT in this raw-status cycle; it
// stays reachable on its own key (r). pinned/hooked/review/tombstone are real
// model.Status values but excluded here as rare/internal states not worth a
// top-level cycle stop.
var statusFilterCycle = []string{"all", "open", "in_progress", "blocked", "closed", "deferred"}

// cycleStatusFilter advances the status membership filter to the next entry in
// statusFilterCycle and re-applies. Any non-plain-status filter (ready, a BQL
// query, a recipe) is treated as "off the cycle" and the first press lands on
// "all". It intentionally sets NO status toast: the lens status chip already
// shows the active filter, so a "Filter: X" echo would duplicate it (bt-2vshd).
func (m *Model) cycleStatusFilter() {
	m.filter.activeBQLExpr = nil
	m.setActiveRecipe(nil)
	idx := -1
	for i, s := range statusFilterCycle {
		if s == m.filter.currentFilter {
			idx = i
			break
		}
	}
	m.filter.currentFilter = statusFilterCycle[(idx+1)%len(statusFilterCycle)]
	m.applyFilter()
}

// cycleSortMode cycles through available sort modes (bv-3ita)
func (m *Model) cycleSortMode() {
	m.filter.sortMode = (m.filter.sortMode + 1) % numSortModes
	m.applyFilter() // Re-apply filter with new sort
}

// cycleSortModeReverse cycles through sort modes in the opposite direction (bt-ktcr)
func (m *Model) cycleSortModeReverse() {
	m.filter.sortMode = (m.filter.sortMode - 1 + numSortModes) % numSortModes
	m.applyFilter()
}

// sortFilteredItems sorts the filtered items based on current sortMode (bv-3ita)
func (m *Model) sortFilteredItems(items []list.Item, issues []model.Issue) {
	if len(items) == 0 {
		return
	}

	// Sort indices to keep items and issues in sync
	indices := make([]int, len(items))
	for i := range indices {
		indices[i] = i
	}

	sort.Slice(indices, func(i, j int) bool {
		iItem := items[indices[i]].(IssueItem)
		jItem := items[indices[j]].(IssueItem)

		switch m.filter.sortMode {
		case SortCreatedAsc:
			// Oldest first
			return iItem.Issue.CreatedAt.Before(jItem.Issue.CreatedAt)
		case SortCreatedDesc:
			// Newest first
			return iItem.Issue.CreatedAt.After(jItem.Issue.CreatedAt)
		case SortPriority:
			// Priority ascending (P0 first)
			return iItem.Issue.Priority < jItem.Issue.Priority
		case SortUpdated:
			// Most recently updated first
			return iItem.Issue.UpdatedAt.After(jItem.Issue.UpdatedAt)
		case SortProgress:
			// Status lifecycle: in_progress -> review -> open -> hooked ->
			// blocked -> pinned -> deferred -> closed -> tombstone (bt-lm2h).
			// Ties broken by priority asc, then updated desc.
			iOrd := progressOrdinal(iItem.Issue.Status)
			jOrd := progressOrdinal(jItem.Issue.Status)
			if iOrd != jOrd {
				return iOrd < jOrd
			}
			if iItem.Issue.Priority != jItem.Issue.Priority {
				return iItem.Issue.Priority < jItem.Issue.Priority
			}
			return iItem.Issue.UpdatedAt.After(jItem.Issue.UpdatedAt)
		default:
			// Default: Open first, then priority, then newest
			iClosed := isClosedLikeStatus(iItem.Issue.Status)
			jClosed := isClosedLikeStatus(jItem.Issue.Status)
			if iClosed != jClosed {
				return !iClosed
			}
			if iItem.Issue.Priority != jItem.Issue.Priority {
				return iItem.Issue.Priority < jItem.Issue.Priority
			}
			return iItem.Issue.CreatedAt.After(jItem.Issue.CreatedAt)
		}
	})

	// Reorder items and issues based on sorted indices
	sortedItems := make([]list.Item, len(items))
	sortedIssues := make([]model.Issue, len(issues))
	for newIdx, oldIdx := range indices {
		sortedItems[newIdx] = items[oldIdx]
		sortedIssues[newIdx] = issues[oldIdx]
	}
	copy(items, sortedItems)
	copy(issues, sortedIssues)
}

func matchesRecipeStatus(status model.Status, filter string) bool {
	normalized := strings.ToLower(strings.TrimSpace(filter))
	statusKey := strings.ToLower(string(status))
	switch normalized {
	case string(model.StatusClosed):
		return isClosedLikeStatus(status)
	case string(model.StatusTombstone):
		return status == model.StatusTombstone
	case string(model.StatusOpen):
		return status == model.StatusOpen
	case string(model.StatusInProgress):
		return status == model.StatusInProgress
	case string(model.StatusBlocked):
		return status == model.StatusBlocked
	default:
		return statusKey == normalized
	}
}

// applyRecipe makes r the primary filter and applies it.
func (m *Model) applyRecipe(r *recipe.Recipe) {
	if r == nil {
		return
	}
	if m.filter.activeRecipe != r {
		m.setActiveRecipe(r)
	}
	m.filter.activeBQLExpr = nil
	m.filter.currentFilter = "recipe:" + r.Name
	m.applyFilter()
}

// recalculateSplitPaneSizes updates list and viewport dimensions after pane ratio changes
func (m *Model) recalculateSplitPaneSizes() {
	if !m.isSplitView {
		return
	}

	bodyHeight := m.height - 1
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	// Calculate dimensions accounting for 2 panels with borders(2)+padding(2) = 4 overhead each.
	// bodyWidth reserves space for the shortcuts sidebar when visible (bt-lin9).
	availWidth := m.bodyWidth() - 8
	if availWidth < 10 {
		availWidth = 10
	}

	listInnerWidth := int(float64(availWidth) * m.splitPaneRatio)
	detailInnerWidth := availWidth - listInnerWidth

	listHeight := bodyHeight - 4
	if listHeight < 3 {
		listHeight = 3
	}

	m.list.SetSize(listInnerWidth, listHeight)
	m.viewport = viewport.New(viewport.WithWidth(detailInnerWidth), viewport.WithHeight(bodyHeight-2))
	m.renderer.SetWidthWithTheme(detailInnerWidth, m.theme)
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	selectedItem := m.list.SelectedItem()
	if selectedItem == nil {
		m.viewport.SetContent(m.theme.Text.Metadata.Render("No issues selected"))
		return
	}

	// Safe type assertion
	issueItem, ok := selectedItem.(IssueItem)
	if !ok {
		m.viewport.SetContent(m.theme.Text.Body.Foreground(m.theme.Blocked).Render("Error: invalid item type"))
		return
	}
	item := issueItem.Issue

	// Detail panel is composed as a []renderSection: "md" fragments are
	// concatenated and routed through Glamour; "ansi" fragments are pre-
	// rendered lipgloss strings whose placeholder line is replaced post-
	// Glamour by spliceSections. See detail_sections.go and bt-x5xc4 for
	// why any styled (ANSI SGR) region must bypass Glamour's chroma path.
	var sections []renderSection
	ansiCounter := 0
	addMD := func(s string) {
		if s == "" {
			return
		}
		sections = append(sections, renderSection{kind: "md", content: s})
	}
	addANSI := func(s string) {
		if s == "" {
			return
		}
		ansiCounter++
		sections = append(sections, renderSection{
			kind:        "ansi",
			content:     s,
			placeholder: sectionPlaceholder(ansiCounter),
		})
	}

	// Update notice was previously rendered here as a markdown block above
	// the bead title. As of bt-9u39 it lives in the notifications center
	// (dismissable, doesn't compete with bead content) plus the footer star
	// badge for ambient awareness.

	// Title Block
	addMD(fmt.Sprintf("# %s %s\n\n", GetTypeIcon(string(item.IssueType)), item.Title))

	// Identity strip: ID, status, priority on a single prose line. Type lives
	// in the title icon already, so don't duplicate it here. The wide markdown
	// table this replaces (bt-aw4h) ran out of horizontal room around 5 fields
	// — see bt-2cvx. Property block below scales without truncation.
	addMD(fmt.Sprintf("**%s**  ·  **%s**  ·  %s P%d\n\n",
		item.ID,
		strings.ToUpper(string(item.Status)),
		GetPriorityIcon(item.Priority), item.Priority,
	))

	// Property block (ANSI). Migrated from a fenced code block (chroma path)
	// to lipgloss-styled aligned rows so labels can be muted without going
	// through Glamour's ESC-stripping code-fence rendering (bt-x5xc4 trap
	// generalised by bt-gfxhz).
	addANSI(buildPropertyBlockANSI(item))

	// State dimensions (bt-jprp) are parsed from dimension:value labels and are
	// NOT re-listed here. The property block directly above already prints
	// every label, so a bead labelled area:tooling rendered "Labels
	// area:tooling" and then a State Dimensions section saying "area: tooling"
	// -- a whole heading and row to restate the line above it. The dimensional
	// reading of those labels belongs in how the labels themselves are
	// rendered (bt-eiec / bt-36h7 / bt-6fn2), not in a duplicate section.

	// Slot sections (Capabilities and registered providers) sit right under
	// the properties so they are visible without scrolling past comments.
	for _, s := range m.slotRegistry.Sections(&item, slots.Context{WorkspaceMode: m.workspaceMode}) {
		addMD(renderSlotSection(s))
	}

	// Gate status (bt-c69c) - blocking coordination
	if item.AwaitType != nil {
		var sb strings.Builder
		sb.WriteString("### " + activeGlyphs.Construction + " Gate (Blocking)\n")
		sb.WriteString(fmt.Sprintf("- **Type:** %s\n", *item.AwaitType))
		if item.AwaitID != nil {
			sb.WriteString(fmt.Sprintf("- **Awaiting:** %s\n", *item.AwaitID))
		}
		if item.TimeoutNs != nil && *item.TimeoutNs > 0 {
			sb.WriteString(fmt.Sprintf("- **Timeout:** %s\n", formatNanoseconds(*item.TimeoutNs)))
		}
		sb.WriteString("\n")
		addMD(sb.String())
	} else if hasHumanLabel(item.Labels) {
		// Advisory human flag (label, not gate). One line, not a heading plus a
		// sentence: this restates a label already visible in the property block,
		// and a real gate above gets the section treatment because it actually
		// blocks. Advisory does not earn the same weight as blocking.
		addMD("**" + activeGlyphs.Tag + " Flagged for human review** (advisory, not blocking)\n\n")
	}

	// Molecule/wisp metadata (bt-c69c)
	if item.MolType != nil || (item.Ephemeral != nil && *item.Ephemeral) || (item.IsTemplate != nil && *item.IsTemplate) {
		var sb strings.Builder
		sb.WriteString("### " + activeGlyphs.TestTube + " Molecule\n")
		if item.MolType != nil {
			sb.WriteString(fmt.Sprintf("- **Type:** %s\n", *item.MolType))
		}
		if item.Ephemeral != nil && *item.Ephemeral {
			sb.WriteString("- **Ephemeral:** yes (wisp)\n")
		}
		if item.IsTemplate != nil && *item.IsTemplate {
			sb.WriteString("- **Template:** yes\n")
		}
		sb.WriteString("\n")
		addMD(sb.String())
	}

	// Epic progress: per-child status pills via the shared lipgloss renderer
	// (bt-gfxhz.3) - the same buildEpicProgressANSI that backs the tier-2 focus
	// card. The "### Epic Progress" H3 stays on the Glamour (md) track so it
	// styles like adjacent headings; the styled body goes through the ANSI track
	// (addANSI) because lipgloss SGR cannot survive Glamour's chroma path
	// (bt-x5xc4). buildEpicProgressANSI returns "" for a childless epic, so the
	// heading is suppressed with it (replaces the old total>0 gate + per-status
	// markdown styling: bt-waeh, bt-u05bo).
	if item.IssueType == model.TypeEpic {
		if body := buildEpicProgressANSI(item, m.data.issues, -1, m.viewport.Width()); body != "" {
			addMD("### Epic Progress\n")
			addANSI(body)
		}
	}

	// Overdue/stale notices (bt-5oqf)
	if isOverdue(&item) {
		addMD(fmt.Sprintf("### "+activeGlyphs.Overdue+" Overdue\nDue date **%s** has passed (%s ago).\n\n",
			FormatTimeAbs(*item.DueDate),
			FormatTimeRel(*item.DueDate),
		))
	} else if isStale(&item) {
		addMD(fmt.Sprintf("### "+activeGlyphs.Sleep+" Stale\nNo updates for **%s** (last: %s). Threshold: %d days.\n\n",
			FormatTimeRel(item.UpdatedAt),
			FormatTimeAbs(item.UpdatedAt),
			staleDays(),
		))
	}

	// Derived analytics (centrality, triage, search scores, graph metrics) are
	// BUILT here but EMITTED after the bead's own content, via emitAnalytics
	// below (bt-krx1).
	//
	// They previously rendered above the description, so the only text unique
	// to a bead sat beneath roughly a dozen sections of computed numbers and
	// the reader scrolled past all of it to reach the thing they opened the
	// bead for. Ordering is the fix: what the bead says first, what bt infers
	// about it second.
	emitAnalytics := func() {
		// Centrality (bt-46p6.12 AC3) — surface graph-position signals next to
		// the issue itself, so users don't need to enter the insights view to
		// understand how central a bead is. Gated on Phase 2 readiness because
		// PageRank/betweenness are async and only populated post-warmup.
		if m.data.analysis != nil && m.data.analysis.IsPhase2Ready() {
			if rank, ok := m.data.analysis.PageRankRankValue(item.ID); ok {
				prVal, _ := m.data.analysis.PageRankValue(item.ID)
				var sb strings.Builder
				sb.WriteString("### " + activeGlyphs.BarChart + " Centrality\n")
				sb.WriteString(fmt.Sprintf("- **PageRank:** rank #%d · %.4f\n", rank, prVal))
				if brank, bok := m.data.analysis.BetweennessRankValue(item.ID); bok {
					bval, _ := m.data.analysis.BetweennessValue(item.ID)
					sb.WriteString(fmt.Sprintf("- **Betweenness:** rank #%d · %.4f\n", brank, bval))
				}
				sb.WriteString(fmt.Sprintf("- **Degree:** in %d / out %d\n",
					m.data.analysis.InDegree[item.ID], m.data.analysis.OutDegree[item.ID]))
				sb.WriteString("\n")
				addMD(sb.String())
			}
		}

		// Triage Insights (bv-151)
		if issueItem.TriageScore > 0 || issueItem.TriageReason != "" || issueItem.UnblocksCount > 0 || issueItem.IsQuickWin || issueItem.IsBlocker {
			var sb strings.Builder
			sb.WriteString("### " + activeGlyphs.Target + " Triage Insights\n")

			// Score with visual indicator
			scoreIcon := activeGlyphs.DotInfo
			if issueItem.TriageScore >= 0.7 {
				scoreIcon = activeGlyphs.DotBlocked
			} else if issueItem.TriageScore >= 0.4 {
				scoreIcon = activeGlyphs.DotHigh
			}
			sb.WriteString(fmt.Sprintf("- **Triage Score:** %s %.2f/1.00\n", scoreIcon, issueItem.TriageScore))

			// Special flags
			if issueItem.IsQuickWin {
				sb.WriteString("- **" + activeGlyphs.Bolt + " Quick Win** — Low effort, high impact opportunity\n")
			}
			if issueItem.IsBlocker {
				sb.WriteString("- **" + activeGlyphs.DotBlocked + " Critical Blocker** — Completing this unblocks significant downstream work\n")
			}

			// Unblocks count
			if issueItem.UnblocksCount > 0 {
				sb.WriteString(fmt.Sprintf("- **"+activeGlyphs.Unlock+" Unblocks:** %d downstream items when completed\n", issueItem.UnblocksCount))
			}

			// Reasons. The primary reason is also the first entry of TriageReasons,
			// so printing a "Primary Reason" line AND an "All Reasons" list that
			// repeats it spent two blocks saying one thing. Lead with the primary
			// and list only what it did not already cover.
			if issueItem.TriageReason != "" {
				sb.WriteString(fmt.Sprintf("- **Why:** %s\n", issueItem.TriageReason))
			}
			for _, reason := range issueItem.TriageReasons {
				if reason == issueItem.TriageReason {
					continue
				}
				sb.WriteString(fmt.Sprintf("  - %s\n", reason))
			}

			sb.WriteString("\n")
			addMD(sb.String())
		}

		// Search Scores (hybrid mode). Heading via Glamour for consistent H3
		// styling with neighbouring sections. Body via ANSI so lipgloss bar
		// characters and muted labels survive without Glamour's ESC-stripping
		// code-fence path (bt-x5xc4 class). Same two-track pattern as Graph
		// Analysis. (bt-gfxhz.6)
		if m.semanticSearchEnabled && m.semanticHybridEnabled && issueItem.SearchScoreSet && m.list.FilterState() != list.Unfiltered {
			summary := searchScoreSummary(issueItem.SearchComponents, item)
			heading := "### " + activeGlyphs.Search + " Search Scores"
			if summary != "" {
				heading += "  (" + summary + ")"
			}
			addMD(heading + "\n")
			addANSI(buildSearchScoresANSI(issueItem.SearchComponents, issueItem.SearchScore, issueItem.SearchTextScore, item))
		}

		// Graph Analysis. Heading stays on Glamour so its H3 styling matches
		// neighbouring sections (Centrality, Search Scores). Only the
		// numeric rows go through ANSI — labels in ColorMuted, values default —
		// since that's where the lipgloss styling actually matters (bt-x5xc4).
		var pr, bt, imp, ev, hub, auth float64
		if m.data.analysis != nil {
			pr = m.data.analysis.GetPageRankScore(item.ID)
			bt = m.data.analysis.GetBetweennessScore(item.ID)
			imp = m.data.analysis.GetCriticalPathScore(item.ID)
			ev = m.data.analysis.GetEigenvectorScore(item.ID)
			hub = m.data.analysis.GetHubScore(item.ID)
			auth = m.data.analysis.GetAuthorityScore(item.ID)
		}
		// Rendered only when the graph actually says something. A bead with no
		// edges has all six metrics structurally zero, and printing
		// "PR 0.0000 - BW 0.0000 - EV 0.0000 - Hub 0.0000 - Authority 0.0000"
		// spends four lines to report the absence of information. The
		// dependency-tree section already follows this rule by returning ""
		// when a bead has no edges; this makes the analytics consistent with it.
		if pr != 0 || bt != 0 || imp != 0 || ev != 0 || hub != 0 || auth != 0 {
			addMD("### Graph Analysis\n")
			addANSI(buildGraphAnalysisANSI(pr, bt, imp, ev, hub, auth))
		}
	}

	// Description
	if item.Description != "" {
		addMD("### Description\n" + item.Description + "\n\n")
	}

	// Design Notes
	if item.Design != "" {
		addMD("### Design Notes\n" + item.Design + "\n\n")
	}

	// Acceptance Criteria
	if item.AcceptanceCriteria != "" {
		addMD("### Acceptance Criteria\n" + item.AcceptanceCriteria + "\n\n")
	}

	// Notes
	if item.Notes != "" {
		addMD("### Notes\n" + item.Notes + "\n\n")
	}

	// Resolution (for closed issues with close_reason)
	if item.Status.IsClosed() && item.CloseReason != nil && *item.CloseReason != "" {
		addMD("### Resolution\n" + *item.CloseReason + "\n\n")
	}

	// What bt infers about the bead, after everything the bead itself says.
	emitAnalytics()

	// Dependency Graph (ANSI). Built first, rendered iff the tree has any
	// children — covers both outgoing dep edges and inverse parent_child
	// children (bt-cuyiz). The tree is rendered with lipgloss and CANNOT
	// pass through Glamour (bt-x5xc4); the renderSection / spliceSections
	// primitive in detail_sections.go handles the bypass.
	addANSI(buildDepGraphSection(item.ID, m.data.issueMap))

	// Comments. The comment markdown is built as a single "md" section so
	// the section list stays compact, but each comment's byte offset within
	// the section's content is recorded for the bt-46p6.16 deep-link scroll
	// path. The prefix-render slices that section content at the recorded
	// offset to compute the rendered-line target.
	type commentAnchor struct {
		createdAt        time.Time
		intraSectionByte int
	}
	var commentAnchors []commentAnchor
	var commentsContent string
	commentsSectionIdx := -1
	if len(item.Comments) > 0 {
		var csb strings.Builder
		csb.WriteString(fmt.Sprintf("### Comments (%d)\n", len(item.Comments)))
		for _, comment := range item.Comments {
			commentAnchors = append(commentAnchors, commentAnchor{
				createdAt:        comment.CreatedAt,
				intraSectionByte: csb.Len(),
			})
			csb.WriteString(fmt.Sprintf("> **%s** (%s)\n> \n> %s\n\n",
				comment.Author,
				FormatTimeRel(comment.CreatedAt),
				strings.ReplaceAll(comment.Text, "\n", "\n> ")))
		}
		commentsContent = csb.String()
		commentsSectionIdx = len(sections)
		addMD(commentsContent)
	}

	// History Section (if data is loaded)
	if m.historyView.HasReport() {
		historyMD := m.renderBeadHistoryMD(item.ID)
		if historyMD != "" {
			addMD(historyMD)
		}
	}

	source := buildMarkdownSource(sections)
	rendered, err := m.renderer.Render(source)
	if err != nil {
		m.viewport.SetContent(m.theme.Text.Body.Foreground(m.theme.Blocked).Render(fmt.Sprintf("Error rendering markdown: %v", err)))
		m.pendingCommentScroll = time.Time{}
		return
	}
	rendered = spliceSections(rendered, sections)
	m.viewport.SetContent(rendered)

	// Apply the bt-46p6.16 deep-link scroll if one is queued. Build a prefix
	// sections slice that includes everything before the comments section,
	// plus a truncated comments fragment ending at the target comment's
	// intra-section byte offset. Render through the same pipeline (Glamour
	// + spliceSections) so styling-induced line growth matches the viewport
	// content. Cleared unconditionally — a single user action consumes a
	// single scroll.
	if !m.pendingCommentScroll.IsZero() {
		target := -1
		for i, a := range commentAnchors {
			if a.createdAt.Equal(m.pendingCommentScroll) {
				target = i
				break
			}
		}
		if target >= 0 && commentsSectionIdx >= 0 {
			prefixSections := make([]renderSection, 0, commentsSectionIdx+1)
			prefixSections = append(prefixSections, sections[:commentsSectionIdx]...)
			truncated := commentsContent[:commentAnchors[target].intraSectionByte]
			prefixSections = append(prefixSections, renderSection{kind: "md", content: truncated})
			prefixSource := buildMarkdownSource(prefixSections)
			if prefixRendered, perr := m.renderer.Render(prefixSource); perr == nil {
				prefixRendered = spliceSections(prefixRendered, prefixSections)
				line := strings.Count(strings.TrimRight(prefixRendered, "\n"), "\n")
				m.viewport.SetYOffset(line)
			}
		}
		m.pendingCommentScroll = time.Time{}
	}
}

// buildPropertyBlockANSI renders the bead property block (author, assignee,
// timestamps, labels, session provenance) as lipgloss-styled aligned rows.
// Replaces the previous fenced-code-block path which routed through chroma
// (bt-x5xc4 trap class). Labels use Heading; values use Metadata.
// Returns empty string when no rows are populated.
func buildPropertyBlockANSI(item model.Issue) string {
	type metaRow struct{ label, value string }
	rows := []metaRow{}
	if item.Author != "" {
		rows = append(rows, metaRow{"Author", "@" + item.Author})
	}
	if item.Assignee != "" {
		rows = append(rows, metaRow{"Assignee", "@" + item.Assignee})
	}
	if item.SourceRepo != "" {
		rows = append(rows, metaRow{"Source", model.DisplayRepoName(item.SourceRepo)})
	}
	rows = append(rows, metaRow{"Created", FormatTimeAbs(item.CreatedAt)})
	rows = append(rows, metaRow{"Updated", FormatTimeAbs(item.UpdatedAt)})
	if item.ClosedAt != nil {
		rows = append(rows, metaRow{"Closed", FormatTimeAbs(*item.ClosedAt)})
	}
	if len(item.Labels) > 0 {
		rows = append(rows, metaRow{"Labels", strings.Join(item.Labels, " · ")})
	}
	// Session provenance (bt-2cvx). Raw UUIDs by design — cass-joa1 will
	// introduce a short-id surface; don't gold-plate trimming here.
	if item.CreatedBySession != "" {
		rows = append(rows, metaRow{"Created by", item.CreatedBySession})
	}
	if item.ClaimedBySession != "" {
		rows = append(rows, metaRow{"Claimed by", item.ClaimedBySession})
	}
	if item.ClosedBySession != "" {
		rows = append(rows, metaRow{"Closed by", item.ClosedBySession})
	}
	if len(rows) == 0 {
		return ""
	}
	labelWidth := 0
	for _, r := range rows {
		if n := len(r.label); n > labelWidth {
			labelWidth = n
		}
	}
	labelStyle := ActiveTextStyles.Heading
	var lines []string
	for _, r := range rows {
		paddedLabel := fmt.Sprintf("%-*s", labelWidth, r.label)
		lines = append(lines, labelStyle.Render(paddedLabel)+"  "+ActiveTextStyles.Metadata.Render(r.value))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// buildGraphAnalysisANSI renders the three numeric rows of the graph-
// position panel (Impact Depth, Centrality, Flow Role) with lipgloss.
// Labels use Heading; values use Metadata. The "### Graph
// Analysis" heading is emitted as a separate md section by the caller so
// Glamour styles it consistently with adjacent H3 headings.
func buildGraphAnalysisANSI(pr, bt, imp, ev, hub, auth float64) string {
	label := ActiveTextStyles.Heading
	value := ActiveTextStyles.Metadata
	lines := []string{
		label.Render("Impact Depth:") + value.Render(fmt.Sprintf(" %.0f (downstream chain length)", imp)),
		label.Render("Centrality:") + value.Render(fmt.Sprintf(" PR %.4f • BW %.4f • EV %.4f", pr, bt, ev)),
		label.Render("Flow Role:") + value.Render(fmt.Sprintf(" Hub %.4f • Authority %.4f", hub, auth)),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// buildDepGraphSection returns the lipgloss-rendered dependency tree for
// rootID, or empty string when the bead has no children to draw (no
// outgoing dep edges AND no inverse parent_child children — bt-cuyiz).
// RenderDependencyTree (helpers.go) is the consumer of choice; this is
// the call site that gates on emptiness so the section is skipped when
// there's nothing to show.
func buildDepGraphSection(rootID string, issueMap map[string]*model.Issue) string {
	rootNode := BuildDependencyTree(rootID, issueMap, 3)
	if rootNode == nil || len(rootNode.Children) == 0 {
		return ""
	}
	return RenderDependencyTree(rootNode)
}

// renderBeadHistoryMD generates markdown for a bead's history
func (m *Model) renderBeadHistoryMD(beadID string) string {
	hist := m.historyView.GetHistoryForBead(beadID)
	if hist == nil || len(hist.Commits) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### " + activeGlyphs.Scroll + " History\n\n")

	// Lifecycle milestones from events
	if len(hist.Events) > 0 {
		sb.WriteString("**Lifecycle:**\n")
		for _, event := range hist.Events {
			icon := getEventIcon(event.EventType)
			sb.WriteString(fmt.Sprintf("- %s **%s** %s by %s\n",
				icon,
				event.EventType,
				event.Timestamp.Format("Jan 02 15:04"),
				event.Author,
			))
		}
		sb.WriteString("\n")
	}

	// Correlated commits
	sb.WriteString(fmt.Sprintf("**Related Commits (%d):**\n", len(hist.Commits)))
	for i, commit := range hist.Commits {
		if i >= 5 {
			sb.WriteString(fmt.Sprintf("  ... and %d more commits\n", len(hist.Commits)-5))
			break
		}

		// Confidence indicator
		confIcon := activeGlyphs.DotReady
		if commit.Confidence < 0.5 {
			confIcon = activeGlyphs.DotWarn
		} else if commit.Confidence < 0.8 {
			confIcon = activeGlyphs.DotHigh
		}

		sb.WriteString(fmt.Sprintf("- %s **%.0f%%** `%s` %s\n",
			confIcon,
			commit.Confidence*100,
			commit.ShortSHA,
			truncateString(commit.Message, 40),
		))

		// Show files for high-confidence commits
		if commit.Confidence >= 0.8 && len(commit.Files) > 0 && len(commit.Files) <= 3 {
			for _, f := range commit.Files {
				sb.WriteString(fmt.Sprintf("  - `%s` (+%d, -%d)\n", f.Path, f.Insertions, f.Deletions))
			}
		}
	}

	sb.WriteString("\n*Press h for full history view*\n\n")
	return sb.String()
}

// getEventIcon returns an icon for bead event types
func getEventIcon(eventType correlation.EventType) string {
	switch eventType {
	case correlation.EventCreated:
		return activeGlyphs.DotReady
	case correlation.EventClaimed:
		return activeGlyphs.DotInfo
	case correlation.EventClosed:
		return activeGlyphs.DotClosed
	case correlation.EventReopened:
		return activeGlyphs.DotWarn
	case correlation.EventModified:
		return activeGlyphs.Memo
	default:
		return "•"
	}
}

// shortError extracts the tail of a nested error chain for display in the
// status bar (bv-9x36). Go errors like "connect: cannot reach Dolt server:
// dial tcp ...: connectex: ..." are too verbose for a single-line footer.
func shortError(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i != -1 {
		s = s[i+2:]
	}
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// sessionCell renders a session-id field for the detail pane Sessions block.
// Empty values render as em-dash; otherwise the full UUID is rendered inside
// a code span so it copies cleanly and is visually distinct from prose.
// cass-joa1 will introduce a short-id format (workspace_xxxxxx) — when that
// lands, swap the inner string here without touching call sites.
func sessionCell(uuid string) string {
	if uuid == "" {
		return "—"
	}
	return "`" + uuid + "`"
}

// truncateString truncates a string to maxLen runes with ellipsis.
// Uses rune-based counting to safely handle UTF-8 multi-byte characters.
func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

// searchScoreContrib represents one component's contribution to the hybrid score,
// sorted descending by absolute value for the bar display. (bt-gfxhz.6)
type searchScoreContrib struct {
	key   string
	value float64
}

// searchScoreMinAbs is the suppression threshold for bar display. Components
// whose absolute contribution is below this are collapsed to a single
// "not contributing" line. Matches the former row-badge floor (bt-r3zxj).
const searchScoreMinAbs = 0.05

// buildSearchScoresANSI renders the Search Scores detail-pane section as a
// lipgloss-styled block: contribution bars sorted descending, suppressed-
// components line, and a final Hybrid / Text score row. Called from
// updateViewportContent after the H3 heading is emitted via addMD.
//
// The caller routes this through addANSI so it bypasses Glamour's code-fence
// path (bt-x5xc4 trap) and the ANSI block characters survive intact.
func buildSearchScoresANSI(components map[string]float64, hybridScore, textScore float64, item model.Issue) string {
	muted := ActiveTextStyles.Metadata

	// Sort components by absolute contribution descending; alphabetical key
	// as tiebreaker so the bar order is deterministic when two components
	// have equal absolute value (bt-gfxhz.6 reviewer note). Map iteration
	// in Go is randomised, so without a tiebreak equal-value rows would
	// flip position between renders.
	var contribs []searchScoreContrib
	for k, v := range components {
		contribs = append(contribs, searchScoreContrib{k, v})
	}
	sort.Slice(contribs, func(i, j int) bool {
		ai := contribs[i].value
		if ai < 0 {
			ai = -ai
		}
		aj := contribs[j].value
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return contribs[i].key < contribs[j].key
	})

	// Separate above-threshold from suppressed.
	var active, suppressed []searchScoreContrib
	for _, c := range contribs {
		abs := c.value
		if abs < 0 {
			abs = -abs
		}
		if abs >= searchScoreMinAbs {
			active = append(active, c)
		} else {
			suppressed = append(suppressed, c)
		}
	}

	var lines []string

	// Bar rows for above-threshold components.
	for _, c := range active {
		bar := searchScoreBar(c.value)
		anchor := searchScoreAnchor(c.key, c.value, item)
		sign := "+"
		if c.value < 0 {
			sign = "-"
		}
		absVal := c.value
		if absVal < 0 {
			absVal = -absVal
		}
		label := muted.Render(fmt.Sprintf("  %-9s", c.key))
		scorePart := fmt.Sprintf("%s%.1f", sign, absVal)
		lines = append(lines, label+" "+bar+"  "+anchor+"  "+muted.Render(scorePart))
	}

	// Suppressed components collapsed to one line.
	if len(suppressed) > 0 {
		names := make([]string, len(suppressed))
		for i, c := range suppressed {
			names[i] = c.key
		}
		lines = append(lines, muted.Render("  "+strings.Join(names, ", ")+": not contributing"))
	}

	// Final summary row: hybrid and text scores for power users.
	lines = append(lines, muted.Render(fmt.Sprintf("  Hybrid: %.2f | Text: %.2f", hybridScore, textScore)))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// searchScoreBar returns a 10-cell block-character bar for the given score
// value (expected 0.0-1.0). Full blocks for filled cells, light shade for
// empty. Clamped to [0, 1] before scaling.
func searchScoreBar(value float64) string {
	const barWidth = 10
	if value < 0 {
		value = -value
	}
	if value > 1 {
		value = 1
	}
	filled := int(value * barWidth)
	empty := barWidth - filled
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

// statusToken maps an issue status to a compact human token used by both the
// bar-row anchor and the heading parenthetical summary. Centralising this
// keeps the two surfaces in sync — a "status" component must surface the
// issue's actual state (active / blocked / closed / ...), not a fixed label.
// in_progress is renamed to "active" for readability; everything else passes
// through as the status string.
func statusToken(s model.Status) string {
	if s == model.StatusInProgress {
		return "active"
	}
	return string(s)
}

// searchScoreAnchor maps a component key to a human-readable label derived
// from the issue's actual state, so the bar row is self-annotating.
//
//   - status   -> the issue's status word (in_progress = "active")
//   - priority -> P0/P1/P2/P3/P4
//   - recency  -> relative time already used in the detail pane
//   - pagerank -> tier label based on raw PR score (no rank# available here)
//   - impact   -> downstream chain count (critical path score)
func searchScoreAnchor(key string, value float64, item model.Issue) string {
	switch key {
	case "status":
		return padRight(statusToken(item.Status), 10)
	case "priority":
		return padRight(fmt.Sprintf("P%d", item.Priority), 10)
	case "recency":
		return padRight(FormatTimeRel(item.UpdatedAt), 10)
	case "pagerank":
		// Tier from raw PR score. Cut points are rough quantiles observed
		// across typical beads repos (top 5% ~ 0.10, top 20% ~ 0.04). If
		// the value is zero the component was not contributing anyway.
		var tier string
		switch {
		case value >= 0.10:
			tier = "top 5%"
		case value >= 0.04:
			tier = "top 20%"
		case value >= 0.01:
			tier = "mid"
		case value > 0:
			tier = "low"
		default:
			tier = "-"
		}
		return padRight(tier, 10)
	case "impact":
		// Critical path score is a float representing downstream chain length.
		// Round to int for display.
		chain := int(value + 0.5)
		var label string
		if chain <= 0 {
			label = "0 deps"
		} else if chain == 1 {
			label = "1 dep"
		} else {
			label = fmt.Sprintf("%d deps", chain)
		}
		return padRight(label, 10)
	default:
		return padRight(fmt.Sprintf("%.2f", value), 10)
	}
}

// searchScoreSummary returns a compact parenthetical label for the top 2-3
// above-threshold contributors, joined with " + ". Used in the section
// heading: "Search Scores  (active + P3 + recent)". Returns empty string
// when no components exceed the threshold.
//
// The "status" token reuses statusToken(item.Status) so the summary
// reflects the issue's actual state — a blocked or closed bead whose
// status component contributes strongly surfaces as "(blocked + ...)"
// rather than a fixed "active" label.
func searchScoreSummary(components map[string]float64, item model.Issue) string {
	type kv struct {
		key string
		val float64
	}
	var items []kv
	for k, v := range components {
		abs := v
		if abs < 0 {
			abs = -abs
		}
		if abs >= searchScoreMinAbs {
			items = append(items, kv{k, abs})
		}
	}
	if len(items) == 0 {
		return ""
	}
	// Sort by absolute value descending; alphabetical key for deterministic
	// ordering when contributions are equal (bt-gfxhz.6 reviewer note).
	sort.Slice(items, func(i, j int) bool {
		if items[i].val != items[j].val {
			return items[i].val > items[j].val
		}
		return items[i].key < items[j].key
	})

	// Map keys to compact human tokens for the summary. status delegates to
	// statusToken so it tracks the issue's actual state in both bar-row
	// anchor and heading summary (single source of truth).
	tokenOf := func(key string) string {
		switch key {
		case "status":
			return statusToken(item.Status)
		case "priority":
			return "priority"
		case "recency":
			return "recent"
		case "pagerank":
			return "pagerank"
		case "impact":
			return "impact"
		default:
			return key
		}
	}

	max := 3
	if len(items) < max {
		max = len(items)
	}
	parts := make([]string, max)
	for i := 0; i < max; i++ {
		parts[i] = tokenOf(items[i].key)
	}
	return strings.Join(parts, " + ")
}

// applyBQL makes query the primary filter and applies it. BQL runs after the
// per-issue dimensions inside FilterSpec.Apply because of ORDER BY / EXPAND.
func (m *Model) applyBQL(query *bql.Query, queryStr string) {
	m.filter.activeBQLExpr = query
	m.filter.currentFilter = "bql:" + queryStr
	m.applyFilter()
}
