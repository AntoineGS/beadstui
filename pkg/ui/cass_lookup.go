package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/cass"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

const cassLookupTimeout = 3 * time.Second

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
	m.cassCancel = cancel
	m.cassPendingIssueID = issue.ID
	m.cassPendingSource = source
	m.cassReturnFocus = m.focused
	m.setStatus("Searching Cass sessions for " + issue.ID + "...")
	issueCopy := cloneCassIssue(issue)
	if m.cassLookup == nil {
		if m.cassDetector == nil {
			m.cassDetector = cass.NewDetector()
		}
		if m.cassCorrelator == nil || m.cassCorrelatorSource != source {
			m.cassCorrelator = cass.NewCorrelator(cass.NewSearcher(m.cassDetector), cass.NewCache(), m.workDir)
			m.cassCorrelatorSource = source
		}
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
	if m.cassCancel != nil {
		m.cassCancel()
		m.cassCancel = nil
	}
	m.cassRequestID++
	m.cassPendingIssueID = ""
	m.cassPendingSource = ""
}

func (m Model) handleCassSessionsLoaded(msg cassSessionsLoadedMsg) Model {
	if m.cassCancel == nil || msg.requestID != m.cassRequestID || msg.datasetID != m.cassDatasetID || msg.source != m.cassSourceIdentity() || msg.issueID != m.cassPendingIssueID || msg.source != m.cassPendingSource {
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
