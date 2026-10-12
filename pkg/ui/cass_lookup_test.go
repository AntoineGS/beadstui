package ui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/cass"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestCassLookupDoesNotBlockUpdateAndRejectsStaleCompletion(t *testing.T) {
	started := make(chan struct{})
	continueLookup := make(chan struct{})
	issue := model.Issue{ID: "bt-cass", Title: "cass async lookup"}
	other := model.Issue{ID: "bt-next", Title: "next issue"}
	m := NewModel([]model.Issue{issue, other}, nil, "", nil, nil)
	m.selectIssueByID(issue.ID)
	m.cassLookup = func(ctx context.Context, _ model.Issue) (cass.CorrelationResult, error) {
		close(started)
		select {
		case <-continueLookup:
			return cass.CorrelationResult{BeadID: issue.ID}, nil
		case <-ctx.Done():
			return cass.CorrelationResult{}, ctx.Err()
		}
	}
	cmd := m.beginCassLookup(issue)
	if cmd == nil {
		t.Fatal("beginCassLookup returned no command")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	<-started

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if updated == nil {
		t.Fatal("resize was not handled while Cass lookup was pending")
	}
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	if m.selectedCassIssueID() == issue.ID || m.cassCancel != nil {
		t.Fatal("navigation did not continue and cancel the old Cass request")
	}
	completion := <-result
	updated, _ = m.Update(completion)
	got := updated.(Model)
	if got.activeModal == ModalCassSession || got.focused == focusCassModal {
		t.Fatal("stale Cass lookup opened its modal")
	}
}

func TestCassLookupCancellationSuppressesLateCompletion(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	m := NewModel(nil, nil, "", nil, nil)
	m.cassLookup = func(ctx context.Context, _ model.Issue) (cass.CorrelationResult, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return cass.CorrelationResult{}, ctx.Err()
	}
	cmd := m.beginCassLookup(model.Issue{ID: "bt-cass"})
	if cmd == nil {
		t.Fatal("beginCassLookup returned no command")
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	<-started
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("lookup did not observe cancellation")
	}
	updated, _ = m.Update(<-result)
	if got := updated.(Model); got.activeModal == ModalCassSession {
		t.Fatal("cancelled Cass lookup opened a modal")
	}
}

func TestCassLookupNewerRequestWins(t *testing.T) {
	issue := model.Issue{ID: "bt-cass"}
	m := NewModel([]model.Issue{issue}, nil, "", nil, nil)
	var calls int
	m.cassLookup = func(context.Context, model.Issue) (cass.CorrelationResult, error) {
		calls++
		return cass.CorrelationResult{BeadID: issue.ID}, nil
	}
	old := m.beginCassLookup(issue)
	newer := m.beginCassLookup(issue)
	oldCompletion := old()
	updated, _ := m.Update(oldCompletion)
	got := updated.(Model)
	if got.activeModal == ModalCassSession || got.cassCancel == nil {
		t.Fatal("older completion replaced or cancelled the newer request")
	}
	newCompletion := newer()
	updated, _ = got.Update(newCompletion)
	got = updated.(Model)
	if calls != 2 || got.activeModal == ModalCassSession {
		t.Fatalf("empty newer result handling: calls=%d modal=%v", calls, got.activeModal)
	}
}

func TestCassLookupRestoresOriginFocusAndConflictingModalCancels(t *testing.T) {
	issue := model.Issue{ID: "bt-cass"}
	m := NewModel([]model.Issue{issue}, nil, "", nil, nil)
	m.focused = focusDetail
	m.cassLookup = func(context.Context, model.Issue) (cass.CorrelationResult, error) {
		return cass.CorrelationResult{}, nil
	}
	cmd := m.beginCassLookup(issue)
	completion := cmd().(cassSessionsLoadedMsg)
	completion.result.TopSessions = []cass.ScoredResult{{SearchResult: cass.SearchResult{Agent: "agent", Snippet: "session"}}}
	opened := m.handleCassSessionsLoaded(completion)
	if opened.activeModal != ModalCassSession || opened.focused != focusCassModal {
		t.Fatal("successful correlation did not open the Cass modal")
	}
	closed, _ := opened.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := closed.(Model); got.activeModal != ModalNone || got.focused != focusDetail {
		t.Fatalf("Cass close restored focus=%v modal=%v, want detail and no modal", got.focused, got.activeModal)
	}

	pending := m.beginCassLookup(issue)
	if pending == nil || m.cassCancel == nil {
		t.Fatal("expected a pending request")
	}
	m.openModal(ModalHelp)
	if m.cassCancel != nil {
		t.Fatal("opening a conflicting modal did not cancel Cass lookup")
	}
}

func TestCassLookupDatasetReloadAndShutdownCancel(t *testing.T) {
	issue := model.Issue{ID: "bt-cass"}
	m := NewModel([]model.Issue{issue}, nil, "", nil, nil)
	m.cassLookup = func(ctx context.Context, _ model.Issue) (cass.CorrelationResult, error) {
		<-ctx.Done()
		return cass.CorrelationResult{}, ctx.Err()
	}
	_ = m.beginCassLookup(issue)
	updated, _ := m.Update(SnapshotReadyMsg{Snapshot: mkSnapshot([]model.Issue{issue})})
	if got := updated.(Model); got.cassCancel != nil || got.cassDatasetID != 1 {
		t.Fatal("dataset replacement did not cancel/invalidate the pending request")
	}
	m = updated.(Model)
	_ = m.beginCassLookup(issue)
	updated, _ = m.Update(tea.QuitMsg{})
	if got := updated.(Model); got.cassCancel != nil {
		t.Fatal("shutdown did not cancel the pending Cass request")
	}
}
