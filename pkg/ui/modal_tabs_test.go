package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/seanmartinsmith/beadstui/pkg/drift"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/events"
)

// pressRune drives a single-character key through handleKeyPress.
func pressRune(m Model, r rune) Model {
	got, _ := m.handleKeyPress(tea.KeyPressMsg{Code: r, Text: string(r)})
	return got
}

// pressTab drives the Tab key through handleKeyPress.
func pressTab(m Model) Model {
	got, _ := m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyTab})
	return got
}

func seedModel() Model {
	m := NewModel(nil, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	// Seed one alert so visibleAlerts() is non-empty (prevents the
	// "No active alerts" short-circuit in the `!` handler).
	m.alerts = []drift.Alert{{
		Type:     drift.AlertStale,
		Severity: drift.SeverityWarning,
		Message:  "fixture",
		IssueID:  "bt-fix",
	}}
	return m
}

func pressKey(m Model, code rune) Model {
	got, _ := m.handleKeyPress(tea.KeyPressMsg{Code: code})
	return got
}

// TestPageJumpCursor pins the shared paging math (bt-p4p8.1): jumps land at
// the TOP of the target page and clamp at both ends, including short lists.
func TestPageJumpCursor(t *testing.T) {
	cases := []struct {
		name                          string
		cursor, total, size, delta, w int
	}{
		{"next from first page", 2, 25, 10, 1, 10},
		{"next from mid page lands at top", 14, 25, 10, 1, 20},
		{"next clamps at last page top", 22, 25, 10, 1, 20},
		{"prev from page 2 lands at top of page 1", 14, 25, 10, -1, 0},
		{"prev clamps at first page", 3, 25, 10, -1, 0},
		{"short list next stays put", 1, 4, 10, 1, 0},
		{"short list prev stays put", 3, 4, 10, -1, 0},
		{"exact multiple last page", 19, 20, 10, 1, 10},
		{"empty list", 0, 0, 10, 1, 0},
		{"page size 1", 3, 10, 1, 1, 4},
	}
	for _, c := range cases {
		if got := pageJumpCursor(c.cursor, c.total, c.size, c.delta); got != c.w {
			t.Errorf("%s: pageJumpCursor(%d,%d,%d,%d)=%d want %d", c.name, c.cursor, c.total, c.size, c.delta, got, c.w)
		}
	}
	if got, ok := modalPageKeyCursor("end", 0, 25, 10); !ok || got != 24 {
		t.Errorf("end: got %d,%v want 24,true", got, ok)
	}
	if got, ok := modalPageKeyCursor("G", 3, 0, 10); !ok || got != 0 {
		t.Errorf("G on empty: got %d,%v want 0,true", got, ok)
	}
	if _, ok := modalPageKeyCursor("x", 0, 25, 10); ok {
		t.Errorf("non-paging key must report ok=false")
	}
}

func seedManyNotifications(m *Model, n int) {
	base := time.Now().Add(-time.Duration(n) * time.Minute)
	evs := make([]events.Event, 0, n)
	for i := 0; i < n; i++ {
		evs = append(evs, events.Event{ID: fmt.Sprintf("n%d", i), Kind: events.EventCreated, BeadID: fmt.Sprintf("bt-%d", i), Repo: "bt", Title: "t", At: base.Add(time.Duration(i) * time.Minute)})
	}
	m.events.AppendMany(evs)
}

func TestNotificationsModal_PageKeys(t *testing.T) {
	m := seedModel()
	seedManyNotifications(&m, 60)
	m = pressRune(m, '1')
	if m.activeTab != TabNotifications {
		t.Fatalf("expected notifications tab")
	}
	size := m.notifPageSize()
	if size < 2 {
		t.Fatalf("page size %d", size)
	}
	m = pressKey(m, tea.KeyRight)
	if m.notificationsCursor != size {
		t.Errorf("right: cursor %d want %d", m.notificationsCursor, size)
	}
	m = pressKey(m, tea.KeyPgDown)
	if m.notificationsCursor != 2*size {
		t.Errorf("pgdown: cursor %d want %d", m.notificationsCursor, 2*size)
	}
	m = pressKey(m, tea.KeyLeft)
	if m.notificationsCursor != size {
		t.Errorf("left: cursor %d want %d", m.notificationsCursor, size)
	}
	m = pressKey(m, tea.KeyEnd)
	if m.notificationsCursor != 59 {
		t.Errorf("end: cursor %d want 59", m.notificationsCursor)
	}
	m = pressKey(m, tea.KeyPgDown)
	if m.notificationsCursor != (59/size)*size {
		t.Errorf("pgdown at end must clamp to last page top, got %d", m.notificationsCursor)
	}
	m = pressKey(m, tea.KeyHome)
	if m.notificationsCursor != 0 {
		t.Errorf("home: cursor %d want 0", m.notificationsCursor)
	}
	m = pressKey(m, tea.KeyPgUp)
	if m.notificationsCursor != 0 {
		t.Errorf("pgup at start must stay 0, got %d", m.notificationsCursor)
	}
	m = pressRune(m, 'G')
	if m.notificationsCursor != 59 {
		t.Errorf("G: cursor %d want 59", m.notificationsCursor)
	}
	m = pressRune(m, 'g')
	if m.notificationsCursor != 0 {
		t.Errorf("g: cursor %d want 0", m.notificationsCursor)
	}
}

func TestAlertsModal_PageKeysLandAtTopOfPage(t *testing.T) {
	m := seedModel()
	m.alerts = nil
	for i := 0; i < 60; i++ {
		m.alerts = append(m.alerts, drift.Alert{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "m", IssueID: fmt.Sprintf("bt-%d", i)})
	}
	m = pressRune(m, '!')
	if m.activeTab != TabAlerts {
		t.Fatalf("expected alerts tab")
	}
	size := m.alertsPageSize()
	m = pressKey(m, tea.KeyRight)
	if m.alertsCursor != size {
		t.Errorf("right must land at top of next page: cursor %d want %d", m.alertsCursor, size)
	}
	m = pressKey(m, tea.KeyPgDown)
	if m.alertsCursor != 2*size {
		t.Errorf("pgdown: cursor %d want %d", m.alertsCursor, 2*size)
	}
	m = pressKey(m, tea.KeyLeft)
	if m.alertsCursor != size {
		t.Errorf("left must land at top of previous page: cursor %d want %d", m.alertsCursor, size)
	}
	m = pressKey(m, tea.KeyEnd)
	if m.alertsCursor != 59 {
		t.Errorf("end: cursor %d want 59", m.alertsCursor)
	}
	m = pressKey(m, tea.KeyHome)
	if m.alertsCursor != 0 {
		t.Errorf("home: cursor %d want 0", m.alertsCursor)
	}
}

// TestModalFooters_AdvertisePageKeysWithoutOverflow checks both tabs' footers
// name the page keys and no rendered line outgrows the panel at the user's
// scrunched sizes (bt-p4p8.1).
func TestModalFooters_AdvertisePageKeysWithoutOverflow(t *testing.T) {
	sizes := [][2]int{{60, 16}, {80, 24}, {100, 30}, {120, 40}}
	for _, sz := range sizes {
		for _, tab := range []ModalTab{TabNotifications, TabAlerts} {
			m := seedModel()
			m.width, m.height = sz[0], sz[1]
			seedManyNotifications(&m, 30)
			m.activeTab = tab
			m.openModal(ModalAlerts)
			out := m.renderAlertsPanel()
			if !strings.Contains(out, "page") {
				t.Errorf("%dx%d tab %v: footer must advertise paging:\n%s", sz[0], sz[1], tab, out)
			}
			for _, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > m.alertsPanelWidth() {
					t.Errorf("%dx%d tab %v: line width %d exceeds panel %d: %q", sz[0], sz[1], tab, w, m.alertsPanelWidth(), line)
				}
			}
		}
	}
}

func TestNotificationModal_BangOpensAlertsTab(t *testing.T) {
	m := seedModel()
	m = pressRune(m, '!')
	if m.activeModal != ModalAlerts {
		t.Fatalf("expected ModalAlerts, got %v", m.activeModal)
	}
	if m.activeTab != TabAlerts {
		t.Fatalf("expected TabAlerts, got %v", m.activeTab)
	}
}

func TestNotificationModal_OneOpensNotificationsTab(t *testing.T) {
	m := seedModel()
	m = pressRune(m, '1')
	if m.activeModal != ModalAlerts {
		t.Fatalf("expected ModalAlerts, got %v", m.activeModal)
	}
	if m.activeTab != TabNotifications {
		t.Fatalf("expected TabNotifications, got %v", m.activeTab)
	}
}

func TestNotificationModal_KeySwitchesTab(t *testing.T) {
	m := seedModel()
	m = pressRune(m, '!') // open on alerts
	m = pressRune(m, '1') // switch to notifications
	if m.activeModal != ModalAlerts {
		t.Fatalf("modal should stay open, got %v", m.activeModal)
	}
	if m.activeTab != TabNotifications {
		t.Fatalf("expected TabNotifications after switch, got %v", m.activeTab)
	}
}

func TestNotificationModal_SameKeyCloses(t *testing.T) {
	m := seedModel()
	m = pressRune(m, '!')
	m = pressRune(m, '!')
	if m.activeModal == ModalAlerts {
		t.Fatalf("second ! should close modal")
	}
}

func TestNotificationModal_TabCycles(t *testing.T) {
	m := seedModel()
	m = pressRune(m, '!')
	if m.activeTab != TabAlerts {
		t.Fatalf("setup: should be on alerts tab")
	}
	m = pressTab(m)
	if m.activeTab != TabNotifications {
		t.Fatalf("tab should flip to notifications")
	}
	m = pressTab(m)
	if m.activeTab != TabAlerts {
		t.Fatalf("tab should flip back to alerts")
	}
}

func TestNotificationModal_PerTabCursorPreserved(t *testing.T) {
	m := seedModel()
	m.events.Append(events.Event{
		ID: "e1", Kind: events.EventClosed, BeadID: "bt-1",
		Title: "t1", At: time.Now(),
	})
	m.events.Append(events.Event{
		ID: "e2", Kind: events.EventClosed, BeadID: "bt-2",
		Title: "t2", At: time.Now(),
	})
	m = pressRune(m, '!') // alerts tab
	m.alertsCursor = 1
	m = pressRune(m, '1') // switch to notifications
	m.notificationsCursor = 1
	m = pressTab(m) // back to alerts
	if m.alertsCursor != 1 {
		t.Fatalf("alertsCursor drifted: expected 1, got %d", m.alertsCursor)
	}
	m = pressTab(m) // notifications again
	if m.notificationsCursor != 1 {
		t.Fatalf("notificationsCursor drifted: expected 1, got %d", m.notificationsCursor)
	}
}

func TestNotificationModal_NotificationsFromRingBuffer(t *testing.T) {
	m := seedModel()
	now := time.Now()
	m.events.Append(events.Event{ID: "a", Kind: events.EventCreated, BeadID: "bt-a", Title: "first", At: now.Add(-2 * time.Minute)})
	m.events.Append(events.Event{ID: "b", Kind: events.EventClosed, BeadID: "bt-b", Title: "second", At: now.Add(-1 * time.Minute)})
	m.events.Append(events.Event{ID: "c", Kind: events.EventEdited, BeadID: "bt-c", Title: "third", At: now})

	got := m.visibleNotifications()
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].ID != "c" || got[2].ID != "a" {
		t.Fatalf("expected newest-first [c,b,a], got [%s,%s,%s]", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestNotificationModal_DismissedHidden(t *testing.T) {
	m := seedModel()
	m.events.Append(events.Event{ID: "keep", Kind: events.EventCreated, BeadID: "bt-k", Title: "kept", At: time.Now()})
	m.events.Append(events.Event{ID: "drop", Kind: events.EventCreated, BeadID: "bt-d", Title: "drop", At: time.Now()})
	m.events.Dismiss("drop")

	got := m.visibleNotifications()
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("expected only 'keep' visible, got %+v", got)
	}
}

func TestNotificationModal_AttentionViewOneKeyNoConflict(t *testing.T) {
	m := seedModel()
	m.mode = ViewAttention
	before := m.activeModal
	m = pressRune(m, '1')
	if m.activeModal != before {
		t.Fatalf("1 in ViewAttention should NOT open alerts modal, got %v", m.activeModal)
	}
}

func TestNotificationModal_RespectsActiveRepoFilter(t *testing.T) {
	m := seedModel()
	m.workspaceMode = true
	m.activeRepos = map[string]bool{"bt": true}

	m.events.Append(events.Event{ID: "a", Kind: events.EventCreated, BeadID: "bt-a", Repo: "bt", Title: "bt thing", At: time.Now()})
	m.events.Append(events.Event{ID: "b", Kind: events.EventCreated, BeadID: "other-b", Repo: "other", Title: "other thing", At: time.Now()})

	got := m.visibleNotifications()
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("active-repo filter should hide non-bt events; got %+v", got)
	}

	// Expanding to include both repos shows both events.
	m.activeRepos["other"] = true
	got = m.visibleNotifications()
	if len(got) != 2 {
		t.Fatalf("two-repo filter should show both events; got %d", len(got))
	}

	// nil activeRepos (global) shows all events regardless of repo.
	m.activeRepos = nil
	got = m.visibleNotifications()
	if len(got) != 2 {
		t.Fatalf("nil activeRepos should show all events; got %d", len(got))
	}
}

// TestSelectIssueByID_NarrowFilterDoesNotPanic reproduces the crash from
// the 2026-04-24 dogfood: pressing enter on a notification whose bead was
// at a high unfiltered index, while an active search filter narrowed the
// visible set to one item, drove Paginator.Page past TotalPages-1 and
// panicked on the next render with "slice bounds out of range".
func TestSelectIssueByID_NarrowFilterDoesNotPanic(t *testing.T) {
	m := seedModel()

	// Replace the list with a larger one so Paginator has multiple pages.
	issues := make([]model.Issue, 60)
	items := make([]list.Item, 60)
	for i := range issues {
		id := "proj-" + randID(i)
		if i == 55 {
			id = "proj-target"
		}
		issues[i] = model.Issue{ID: id, Title: "Issue " + id, Status: model.StatusOpen, CreatedAt: time.Now()}
		items[i] = IssueItem{Issue: issues[i]}
	}
	lst := list.New(items, list.NewDefaultDelegate(), 80, 20)
	lst.SetFilteringEnabled(true)
	m.list = lst

	// Put cursor on the late-index match, then apply a narrowing filter.
	m.list.Select(55)
	m.list.SetFilterText("target")
	m.list.SetFilterState(list.FilterApplied)

	if got := len(m.list.VisibleItems()); got != 1 {
		t.Fatalf("precondition: narrow filter should show 1 item, got %d", got)
	}

	// selectIssueByID must not drive the paginator out of bounds.
	if !m.selectIssueByID("proj-target") {
		t.Fatalf("expected to find proj-target in visible items")
	}

	// View() exercises populatedView -> Paginator.GetSliceBounds, where
	// the pre-fix bug crashed with "slice bounds out of range".
	_ = m.list.View()
}

// TestSelectIssueByID_HiddenByFilterResets verifies that when the target
// bead is filtered out, selectIssueByID resets the filter so the jump
// still lands — user intent is "take me there," which outranks the filter.
func TestSelectIssueByID_HiddenByFilterResets(t *testing.T) {
	m := seedModel()
	issues := []model.Issue{
		{ID: "bt-a", Title: "apple", Status: model.StatusOpen, CreatedAt: time.Now()},
		{ID: "bt-b", Title: "banana", Status: model.StatusOpen, CreatedAt: time.Now()},
	}
	items := []list.Item{IssueItem{Issue: issues[0]}, IssueItem{Issue: issues[1]}}
	lst := list.New(items, list.NewDefaultDelegate(), 80, 20)
	lst.SetFilteringEnabled(true)
	m.list = lst

	m.list.SetFilterText("apple")
	m.list.SetFilterState(list.FilterApplied)
	if got := len(m.list.VisibleItems()); got != 1 {
		t.Fatalf("precondition: filter should narrow to apple only, got %d visible", got)
	}

	// Target bt-b is filtered out; selectIssueByID should reset filter and find it.
	if !m.selectIssueByID("bt-b") {
		t.Fatalf("expected filter-reset fallback to find bt-b")
	}
	if m.list.FilterState() != list.Unfiltered {
		t.Fatalf("expected filter reset after fallback, got state=%v", m.list.FilterState())
	}
}

func TestRenderAlertsPanel_BorderReflectsActiveTab(t *testing.T) {
	m := seedModel()
	m.activeTab = TabAlerts
	panel := m.renderAlertsPanel()
	if !strings.Contains(panel, "Alerts!") {
		t.Fatalf("alerts tab border should contain 'Alerts!' title; panel=\n%s", panel)
	}
	if !strings.Contains(panel, "(1)") {
		t.Fatalf("alerts tab border should contain '(1)' count (seeded fixture); panel=\n%s", panel)
	}
	if strings.Contains(panel, "Notifications") {
		t.Fatalf("alerts tab border should NOT contain 'Notifications' title; panel=\n%s", panel)
	}

	m.activeTab = TabNotifications
	m.events.Append(events.Event{ID: "x", Kind: events.EventCreated, BeadID: "bt-x", Title: "t", At: time.Now()})
	panel = m.renderAlertsPanel()
	if !strings.Contains(panel, "Notifications") {
		t.Fatalf("notifications tab border should contain 'Notifications' title; panel=\n%s", panel)
	}
	if !strings.Contains(panel, "(1)") {
		t.Fatalf("notifications tab border should contain '(1)' count; panel=\n%s", panel)
	}
}

// TestUpdateViewportContent_ScrollsToCommentAt verifies bt-46p6.16: when
// pendingCommentScroll is set to a comment's CreatedAt and the selected
// bead has a matching comment, updateViewportContent renders, advances the
// viewport YOffset off zero, and clears the pending field.
func TestUpdateViewportContent_ScrollsToCommentAt(t *testing.T) {
	commentTime := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)

	issues := []model.Issue{{
		ID: "bt-target", Title: "Target bead", Status: model.StatusOpen,
		CreatedAt:   time.Now(),
		Description: strings.Repeat("Long description paragraph. ", 12),
		Comments: []*model.Comment{
			{ID: "c1", Author: "sms", Text: strings.Repeat("first comment body. ", 6), CreatedAt: commentTime.Add(-time.Hour)},
			{ID: "c2", Author: "sms", Text: "deep-link target comment body", CreatedAt: commentTime},
			{ID: "c3", Author: "sms", Text: "later comment", CreatedAt: commentTime.Add(time.Hour)},
		},
	}}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	m.list.Select(0)
	m.viewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))

	m.pendingCommentScroll = commentTime
	m.updateViewportContent()

	if !m.pendingCommentScroll.IsZero() {
		t.Errorf("pendingCommentScroll should be cleared after render; got %v", m.pendingCommentScroll)
	}
	if m.viewport.YOffset() == 0 {
		t.Errorf("viewport.YOffset() should be > 0 after scrolling to comment; still 0")
	}
}

// TestUpdateViewportContent_NoScrollWhenPendingZero confirms the deep-link
// path is opt-in: with pendingCommentScroll unset, updateViewportContent
// leaves the viewport at the top regardless of comment count.
func TestUpdateViewportContent_NoScrollWhenPendingZero(t *testing.T) {
	commentTime := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)
	issues := []model.Issue{{
		ID: "bt-target", Title: "T", Status: model.StatusOpen, CreatedAt: time.Now(),
		Description: strings.Repeat("padding. ", 30),
		Comments: []*model.Comment{
			{ID: "c1", Author: "sms", Text: "x", CreatedAt: commentTime},
		},
	}}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	m.list.Select(0)
	m.viewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))

	m.updateViewportContent()
	if m.viewport.YOffset() != 0 {
		t.Errorf("viewport.YOffset() should stay 0 without pendingCommentScroll; got %d", m.viewport.YOffset())
	}
}

// TestActivateAlert_CentralityChangeOpensInsights covers bt-46p6.12 AC1:
// pressing enter on a centrality_change alert (which has no single-issue
// target) routes to the insights view rather than no-op'ing or jumping
// to a hallucinated bead.
func TestActivateAlert_CentralityChangeOpensInsights(t *testing.T) {
	m := NewModel(nil, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	m.alerts = []drift.Alert{{
		Type:     drift.AlertCentralityChange,
		Severity: drift.SeverityWarning,
		Message:  "3 PageRank changes detected",
		Details:  []string{"bt-x dropped from top", "bt-y entered top"},
		// IssueID intentionally empty: graph-scope alerts don't carry one.
	}}
	m.activeModal = ModalAlerts
	m.activeTab = TabAlerts
	m.alertsCursor = 0

	got, _ := m.activateCurrentModalItem()
	if got.mode != ViewInsights {
		t.Errorf("centrality_change activation should switch to ViewInsights, got %v", got.mode)
	}
	if got.activeModal == ModalAlerts {
		t.Errorf("activation should close the modal")
	}
}

// TestActivateAlert_StaleAlertJumpsToBead guards against the centrality
// routing accidentally swallowing single-issue alerts.
func TestActivateAlert_StaleAlertJumpsToBead(t *testing.T) {
	issues := []model.Issue{{
		ID: "bt-stale", Title: "stale", Status: model.StatusOpen, CreatedAt: time.Now(),
	}}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	m.alerts = []drift.Alert{{
		Type:     drift.AlertStale,
		Severity: drift.SeverityWarning,
		Message:  "stale",
		IssueID:  "bt-stale",
	}}
	m.activeModal = ModalAlerts
	m.activeTab = TabAlerts
	m.alertsCursor = 0

	got, _ := m.activateCurrentModalItem()
	if got.mode == ViewInsights {
		t.Errorf("stale alert should not route to ViewInsights")
	}
	if got.activeModal == ModalAlerts {
		t.Errorf("activation should close the modal")
	}
}

// TestAlertsRender_DependencyLoopShowsCyclePath covers bt-7ye5: the dep loop
// alert's Details (cycle paths) must reach the modal, otherwise the user sees
// only "N new cycle(s) detected" with no way to know which beads are looping.
func TestAlertsRender_DependencyLoopShowsCyclePath(t *testing.T) {
	m := seedModel()
	m.alerts = []drift.Alert{{
		Type:     drift.AlertDependencyLoop,
		Severity: drift.SeverityCritical,
		Message:  "2 new cycle(s) detected",
		Details:  []string{"bt-x → bt-y → bt-x", "bt-z → bt-w → bt-z"},
		// IssueID empty — graph-scope alert
	}}
	m = pressRune(m, '!')

	rendered := m.renderAlertsPanel()
	if !strings.Contains(rendered, "bt-x → bt-y → bt-x") {
		t.Errorf("expected first cycle path in modal, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+1 more") {
		t.Errorf("expected '+1 more' suffix when len(Details) > 1, got:\n%s", rendered)
	}
}

// TestAlertsRender_CentralityChangeShowsFirstChange mirrors the dep-loop case
// for centrality_change alerts — same root cause, same fix.
func TestAlertsRender_CentralityChangeShowsFirstChange(t *testing.T) {
	m := seedModel()
	m.alerts = []drift.Alert{{
		Type:     drift.AlertCentralityChange,
		Severity: drift.SeverityWarning,
		Message:  "3 PageRank changes detected",
		Details:  []string{"bt-x dropped from top", "bt-y entered top", "bt-z: 50.0% change"},
	}}
	m = pressRune(m, '!')

	rendered := m.renderAlertsPanel()
	if !strings.Contains(rendered, "bt-x dropped from top") {
		t.Errorf("expected first detail entry in modal, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+2 more") {
		t.Errorf("expected '+2 more' suffix, got:\n%s", rendered)
	}
}

// TestNotifications_DismissedFilterToggle covers bt-46p6.13's dismissed-events
// filter: pressing `d` on the notifications tab flips visibility of dismissed
// events. v1 hides dismissed unconditionally; v2 lets the user surface them
// for audit without restoring them.
func TestNotifications_DismissedFilterToggle(t *testing.T) {
	m := NewModel(nil, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true

	now := time.Now()
	m.events.Append(events.Event{ID: "evt-1", Kind: events.EventClosed, BeadID: "bt-x", Repo: "bt", Title: "live", At: now})
	m.events.Append(events.Event{ID: "evt-2", Kind: events.EventClosed, BeadID: "bt-y", Repo: "bt", Title: "tomb", At: now})
	m.events.Dismiss("evt-2")

	if got := len(m.visibleNotifications()); got != 1 {
		t.Fatalf("v1 default should hide dismissed; got %d visible", got)
	}

	m.activeModal = ModalAlerts
	m.activeTab = TabNotifications
	m.notificationsCursor = 0

	got, _ := m.handleNotificationsKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !got.notifShowDismissed {
		t.Errorf("expected notifShowDismissed=true after `d`")
	}
	if n := len(got.visibleNotifications()); n != 2 {
		t.Errorf("expected 2 visible after toggle (live + dismissed); got %d", n)
	}
	if got.notificationsCursor != 0 {
		t.Errorf("toggle should reset cursor; got %d", got.notificationsCursor)
	}

	got2, _ := got.handleNotificationsKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if got2.notifShowDismissed {
		t.Errorf("second `d` should toggle off")
	}
	if n := len(got2.visibleNotifications()); n != 1 {
		t.Errorf("expected 1 visible after toggle off; got %d", n)
	}
}

// TestActivateNotification_NonCommentEventDoesNotQueueScroll ensures only
// EventCommented populates pendingCommentScroll. Other event kinds activate
// normally without queueing a comment scroll.
func TestActivateNotification_NonCommentEventDoesNotQueueScroll(t *testing.T) {
	issues := []model.Issue{{
		ID: "bt-x", Title: "X", Status: model.StatusOpen, CreatedAt: time.Now(),
	}}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true

	m.events.Append(events.Event{
		ID: "evt-closed", Kind: events.EventClosed,
		BeadID: "bt-x", Repo: "bt", Title: "X", At: time.Now(),
	})
	m.activeModal = ModalAlerts
	m.activeTab = TabNotifications
	m.notificationsCursor = 0

	got, _ := m.activateCurrentModalItem()
	if !got.pendingCommentScroll.IsZero() {
		t.Errorf("non-comment event must not leave pendingCommentScroll set; got %v", got.pendingCommentScroll)
	}
	if got.activeModal == ModalAlerts {
		t.Errorf("activation should close the modal")
	}
}

// TestNotificationsKindFilter_CycleAndReset drives the t/T/r keys on the
// notifications tab: t cycles forward through kinds present (in display
// order) wrapping to all, T cycles backwards, r resets.
func TestNotificationsKindFilter_CycleAndReset(t *testing.T) {
	m := seedModel()
	m.events = events.NewRingBuffer(10)
	m.events.AppendMany([]events.Event{
		{ID: "e1", Kind: events.EventCreated, BeadID: "bt-1", Repo: "bt", Title: "one", At: time.Now()},
		{ID: "e2", Kind: events.EventClosed, BeadID: "bt-2", Repo: "bt", Title: "two", At: time.Now()},
		{ID: "e3", Kind: events.EventCreated, BeadID: "bt-3", Repo: "bt", Title: "three", At: time.Now()},
	})
	m = pressRune(m, '1')
	if m.activeModal != ModalAlerts || m.activeTab != TabNotifications {
		t.Fatalf("setup: modal/tab not as expected (%v/%v)", m.activeModal, m.activeTab)
	}

	// Kinds present in display order: created, closed.
	m = pressRune(m, 't')
	if m.notifFilterKind != "created" {
		t.Fatalf("first t: expected %q, got %q", "created", m.notifFilterKind)
	}
	if got := len(m.visibleNotifications()); got != 2 {
		t.Fatalf("created filter: expected 2 visible, got %d", got)
	}
	m = pressRune(m, 't')
	if m.notifFilterKind != "closed" {
		t.Fatalf("second t: expected %q, got %q", "closed", m.notifFilterKind)
	}
	m = pressRune(m, 't')
	if m.notifFilterKind != "" {
		t.Fatalf("third t: expected wrap to all, got %q", m.notifFilterKind)
	}

	// Backwards from all lands on the last present kind.
	m = pressRune(m, 'T')
	if m.notifFilterKind != "closed" {
		t.Fatalf("T from all: expected %q, got %q", "closed", m.notifFilterKind)
	}
	m = pressRune(m, 'r')
	if m.notifFilterKind != "" {
		t.Fatalf("r: expected filter reset, got %q", m.notifFilterKind)
	}
}

// TestNotificationsOpen_ResetsKindFilter ensures reopening the notifications
// modal starts unfiltered (mirrors the alerts tab's reset-on-open).
func TestNotificationsOpen_ResetsKindFilter(t *testing.T) {
	m := seedModel()
	m.events.Append(events.Event{ID: "e1", Kind: events.EventCreated, BeadID: "bt-1", Repo: "bt", Title: "one", At: time.Now()})
	m = pressRune(m, '1')
	m = pressRune(m, 't')
	if m.notifFilterKind == "" {
		t.Fatalf("setup: expected an active kind filter")
	}
	m = pressRune(m, 'q') // close
	m = pressRune(m, '1') // reopen
	if m.notifFilterKind != "" {
		t.Fatalf("reopen must reset kind filter, got %q", m.notifFilterKind)
	}
}

// syncModel builds a model with real issues (so the list has a selection)
// for the open-hovers-selected-bead tests.
func syncModel(t *testing.T) Model {
	t.Helper()
	issues := []model.Issue{
		{ID: "bt-1", Title: "One", Status: model.StatusOpen, CreatedAt: time.Now()},
		{ID: "bt-2", Title: "Two", Status: model.StatusOpen, CreatedAt: time.Now()},
	}
	m := NewModel(issues, nil, "", nil, nil)
	m.width = 120
	m.height = 40
	m.mode = ViewList
	m.ready = true
	return m
}

// TestAlertsOpen_CursorHoversSelectedBead: opening the alerts modal lands the
// cursor on the alert referencing the issue selected in the list/detail pane.
func TestAlertsOpen_CursorHoversSelectedBead(t *testing.T) {
	m := syncModel(t)
	m.alerts = []drift.Alert{
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "a", IssueID: "bt-1"},
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "b", IssueID: "bt-2"},
	}
	if !m.selectIssueByID("bt-2") {
		t.Fatalf("setup: could not select bt-2")
	}
	m = pressRune(m, '!')
	if m.activeModal != ModalAlerts {
		t.Fatalf("setup: modal did not open")
	}
	if m.alertsCursor != 1 {
		t.Fatalf("expected alertsCursor=1 (hovering bt-2), got %d", m.alertsCursor)
	}
}

// TestNotificationsOpen_CursorHoversSelectedBead: same contract for the
// notifications tab, with newest-first ordering.
func TestNotificationsOpen_CursorHoversSelectedBead(t *testing.T) {
	m := syncModel(t)
	m.events.AppendMany([]events.Event{
		{ID: "e1", Kind: events.EventCreated, BeadID: "bt-1", Repo: "bt", Title: "one", At: time.Now()},
		{ID: "e2", Kind: events.EventEdited, BeadID: "bt-2", Repo: "bt", Title: "two", At: time.Now()},
		{ID: "e3", Kind: events.EventCreated, BeadID: "bt-9", Repo: "bt", Title: "nine", At: time.Now()},
	})
	if !m.selectIssueByID("bt-2") {
		t.Fatalf("setup: could not select bt-2")
	}
	m = pressRune(m, '1')
	if m.activeModal != ModalAlerts || m.activeTab != TabNotifications {
		t.Fatalf("setup: modal/tab not as expected (%v/%v)", m.activeModal, m.activeTab)
	}
	// Newest-first: [e3(bt-9), e2(bt-2), e1(bt-1)] → bt-2 at index 1.
	if m.notificationsCursor != 1 {
		t.Fatalf("expected notificationsCursor=1 (hovering bt-2), got %d", m.notificationsCursor)
	}
}

// TestModalTabSwitch_SyncsCursorToSelection: switching tabs inside the modal
// re-seeks the newly shown tab's cursor to the selected bead.
func TestModalTabSwitch_SyncsCursorToSelection(t *testing.T) {
	m := syncModel(t)
	m.alerts = []drift.Alert{
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "a", IssueID: "bt-1"},
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "b", IssueID: "bt-2"},
	}
	m.events.Append(events.Event{ID: "e1", Kind: events.EventCreated, BeadID: "bt-1", Repo: "bt", Title: "one", At: time.Now()})
	if !m.selectIssueByID("bt-2") {
		t.Fatalf("setup: could not select bt-2")
	}
	m = pressRune(m, '1') // open on notifications
	m = pressRune(m, '!') // switch to alerts tab
	if m.activeTab != TabAlerts {
		t.Fatalf("expected TabAlerts after !, got %v", m.activeTab)
	}
	if m.alertsCursor != 1 {
		t.Fatalf("expected alertsCursor=1 after tab switch, got %d", m.alertsCursor)
	}
}

// TestAlertsOpen_NoMatchLeavesCursorAtTop: selected bead without a matching
// alert keeps the reset cursor (no stale hover).
func TestAlertsOpen_NoMatchLeavesCursorAtTop(t *testing.T) {
	m := syncModel(t)
	m.alerts = []drift.Alert{
		{Type: drift.AlertStale, Severity: drift.SeverityWarning, Message: "b", IssueID: "bt-2"},
	}
	if !m.selectIssueByID("bt-1") {
		t.Fatalf("setup: could not select bt-1")
	}
	m = pressRune(m, '!')
	if m.alertsCursor != 0 {
		t.Fatalf("expected alertsCursor=0 with no matching alert, got %d", m.alertsCursor)
	}
}
