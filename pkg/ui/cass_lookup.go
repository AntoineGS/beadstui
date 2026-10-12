package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/cass"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

const cassLookupTimeout = 3 * time.Second

// cassLifecycleOwner is shared by Model value copies so external shutdown can
// cancel the active worker even though the CLI retains the original Model.
type cassLifecycleOwner struct {
	mu        sync.Mutex
	requestID uint64
	cancel    context.CancelFunc
	stopped   bool
}

func newCassLifecycleOwner() *cassLifecycleOwner { return &cassLifecycleOwner{} }

func (o *cassLifecycleOwner) start(cancel context.CancelFunc) uint64 {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		cancel()
		return 0
	}
	previous := o.cancel
	o.requestID++
	id := o.requestID
	o.cancel = cancel
	o.mu.Unlock()
	if previous != nil {
		previous()
	}
	return id
}

func (o *cassLifecycleOwner) isCurrent(id uint64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return !o.stopped && id != 0 && o.requestID == id && o.cancel != nil
}

func (o *cassLifecycleOwner) finish(id uint64) {
	o.mu.Lock()
	if id != o.requestID || o.cancel == nil {
		o.mu.Unlock()
		return
	}
	cancel := o.cancel
	o.cancel = nil
	o.requestID++
	o.mu.Unlock()
	cancel()
}

func (o *cassLifecycleOwner) cancelRequest(id uint64) {
	o.mu.Lock()
	if id != o.requestID || o.cancel == nil {
		o.mu.Unlock()
		return
	}
	cancel := o.cancel
	o.cancel = nil
	o.requestID++
	o.mu.Unlock()
	cancel()
}

func (o *cassLifecycleOwner) stop() {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return
	}
	o.stopped = true
	cancel := o.cancel
	o.cancel = nil
	o.requestID++
	o.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m Model) selectedCassIssueID() string {
	selected := m.list.SelectedItem()
	if item, ok := selected.(IssueItem); ok {
		return item.Issue.ID
	}
	return ""
}

func (m Model) cassSourceIdentity() string {
	if m.data.dataSource != nil {
		source := m.data.dataSource
		return fmt.Sprintf("%s\x00%s\x00%s\x00%s", source.Type, source.Path, source.RepoFilter, m.workDir)
	}
	return m.workDir
}

func cassIssueIdentity(issue model.Issue) string {
	data, _ := json.Marshal(issue)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type cassSessionsLoadedMsg struct {
	requestID uint64
	datasetID uint64
	issueID   string
	issueHash string
	source    string
	result    cass.CorrelationResult
	err       error
}

func (m *Model) beginCassLookup(issue model.Issue) tea.Cmd {
	if m.cassCancel != nil {
		m.cancelCassLookup()
	}
	m.cassRequestID++
	requestID, datasetID, source := m.cassRequestID, m.cassDatasetID, m.cassSourceIdentity()
	ctx, cancel := context.WithTimeout(context.Background(), cassLookupTimeout)
	if m.cassLifecycle == nil {
		m.cassLifecycle = newCassLifecycleOwner()
	}
	requestID = m.cassLifecycle.start(cancel)
	m.cassRequestID = requestID
	datasetID = m.cassDatasetID
	m.cassCancel = cancel
	m.cassPendingIssueID = issue.ID
	m.cassPendingSource = source
	m.cassReturnFocus = m.focused
	m.setStatus("Searching Cass sessions for " + issue.ID + "...")
	issueCopy := cloneCassIssue(issue)
	if m.cassDetector == nil {
		m.cassDetector = cass.NewDetector()
	}
	if m.cassCorrelator == nil || m.cassCorrelatorSource != source {
		m.cassCorrelator = cass.NewCorrelator(cass.NewSearcher(m.cassDetector), cass.NewCache(), m.workDir)
		m.cassCorrelatorSource = source
	}
	lookup := m.cassLookup
	if lookup == nil {
		lookup = m.lookupCass
	}
	return func() tea.Msg {
		result, err := lookup(ctx, issueCopy)
		return cassSessionsLoadedMsg{requestID: requestID, datasetID: datasetID, issueID: issueCopy.ID, issueHash: cassIssueIdentity(issueCopy), source: source, result: result, err: err}
	}
}

func (m *Model) lookupCass(ctx context.Context, issue model.Issue) (cass.CorrelationResult, error) {
	if m.cassDetector == nil {
		return cass.CorrelationResult{}, fmt.Errorf("Cass detector is unavailable")
	}
	status, err := m.cassDetector.CheckContext(ctx)
	if err != nil {
		return cass.CorrelationResult{}, fmt.Errorf("check Cass availability: %w", err)
	}
	if status != cass.StatusHealthy {
		return cass.CorrelationResult{}, fmt.Errorf("Cass is %s", status)
	}
	if m.cassCorrelator == nil {
		return cass.CorrelationResult{}, fmt.Errorf("Cass correlator is unavailable")
	}
	result, err := m.cassCorrelator.CorrelateWithError(ctx, &issue)
	if err != nil {
		return cass.CorrelationResult{}, fmt.Errorf("correlate Cass sessions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return cass.CorrelationResult{}, fmt.Errorf("correlate Cass sessions: %w", err)
	}
	return result, nil
}

func cloneCassIssue(issue model.Issue) model.Issue {
	data, err := json.Marshal(issue)
	if err != nil {
		return issue
	}
	var copy model.Issue
	if err := json.Unmarshal(data, &copy); err != nil {
		return issue
	}
	copy.ContentHash = issue.ContentHash
	return copy
}

func (m *Model) cancelCassLookup() {
	if m.cassLifecycle != nil {
		m.cassLifecycle.cancelRequest(m.cassRequestID)
	}
	if m.cassCancel != nil {
		m.cassCancel()
		m.cassCancel = nil
	}
	m.cassRequestID++
	m.cassPendingIssueID = ""
	m.cassPendingSource = ""
}

// invalidateCassDataset detaches all caches for the replaced corpus. Commands
// already running retain only the old correlator and cannot publish into the new
// generation's cache.
func (m *Model) invalidateCassDataset() {
	m.cassDatasetID++
	m.cancelCassLookup()
	m.cassCorrelator = nil
	m.cassCorrelatorSource = ""
}

func (m Model) handleCassSessionsLoaded(msg cassSessionsLoadedMsg) Model {
	if m.cassCancel == nil || m.cassLifecycle == nil || !m.cassLifecycle.isCurrent(msg.requestID) || msg.requestID != m.cassRequestID || msg.datasetID != m.cassDatasetID || msg.source != m.cassSourceIdentity() || msg.issueID != m.cassPendingIssueID || msg.source != m.cassPendingSource {
		return m
	}
	if m.activeModal != ModalNone {
		m.cancelCassLookup()
		return m
	}
	selected := m.list.SelectedItem()
	if item, ok := selected.(IssueItem); !ok || item.Issue.ID != msg.issueID || cassIssueIdentity(item.Issue) != msg.issueHash {
		m.cancelCassLookup()
		return m
	}
	m.cassCancel()
	m.cassLifecycle.finish(msg.requestID)
	m.cassCancel = nil
	m.cassPendingIssueID = ""
	m.cassPendingSource = ""
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("Cass session lookup failed: %v", msg.err))
		return m
	}
	if len(msg.result.TopSessions) == 0 {
		m.setStatus("No correlated sessions found for " + msg.issueID)
		return m
	}
	m.cassModal = NewCassSessionModal(msg.issueID, msg.result, m.theme)
	m.cassModal.keys = m.keys.Cass
	m.cassModal.SetSize(max(0, m.width), max(0, m.height-1))
	m.openModal(ModalCassSession)
	m.focused = focusCassModal
	return m
}
