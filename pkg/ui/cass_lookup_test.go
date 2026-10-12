package ui

import (
	"context"
	"encoding/json"
	"os"
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

func TestCassLookupSnapshotReplacementCancels(t *testing.T) {
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
}

func TestCassLifecycleStopOnOriginalModelCancelsUpdateCopyRequest(t *testing.T) {
	issue := model.Issue{ID: "bt-owned"}
	original := NewModel([]model.Issue{issue}, nil, "", nil, nil)
	started := make(chan struct{})
	finished := make(chan struct{})
	original.cassLookup = func(ctx context.Context, _ model.Issue) (cass.CorrelationResult, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return cass.CorrelationResult{}, ctx.Err()
	}
	runtimeModel := press(t, original, "space", "b", "c")
	if runtimeModel.cassCancel == nil {
		t.Fatal("Cass action did not start through Update")
	}
	<-started

	// The CLI defers Stop on original, not on the value returned from Update.
	original.Stop()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Stop on original model did not cancel the runtime request")
	}
	if original.cassLifecycle.isCurrent(runtimeModel.cassRequestID) {
		t.Fatal("Stop left the runtime request current")
	}
}

func TestCassDatasetReloadsReplaceGenerationAndRejectOldSameIDData(t *testing.T) {
	oldIssue := model.Issue{ID: "bt-stable", Title: "old searchable title", Status: model.StatusOpen, IssueType: model.TypeTask, CreatedAt: time.Now()}
	loaders := []struct {
		name string
		load func(t *testing.T, m Model, issues []model.Issue) Model
	}{
		{
			name: "datasource",
			load: func(_ *testing.T, m Model, issues []model.Issue) Model {
				next, _ := m.Update(DataSourceReloadMsg{Issues: issues})
				return next.(Model)
			},
		},
		{
			name: "file",
			load: func(t *testing.T, m Model, issues []model.Issue) Model {
				m.data.beadsPath = writeCassIssuesFile(t, issues)
				next, _ := m.Update(FileChangedMsg{})
				return next.(Model)
			},
		},
	}
	scenarios := []struct {
		name         string
		updatedTitle string
	}{
		{name: "same ID title changed", updatedTitle: "replacement searchable title"},
		{name: "selected issue unchanged, corpus changed", updatedTitle: oldIssue.Title},
	}
	for _, loader := range loaders {
		for _, scenario := range scenarios {
			t.Run(loader.name+"/"+scenario.name, func(t *testing.T) {
				updatedIssue := oldIssue
				updatedIssue.Title = scenario.updatedTitle
				newCorpus := []model.Issue{updatedIssue, {ID: "bt-new", Title: "new corpus record", Status: model.StatusOpen, IssueType: model.TypeTask, CreatedAt: oldIssue.CreatedAt.Add(-time.Hour)}}
				m := NewModel([]model.Issue{oldIssue}, nil, "", nil, nil)
				m.selectIssueByID(oldIssue.ID)
				oldCache := cass.NewCache()
				oldCache.Set(oldIssue.ID, &cass.CorrelationHint{BeadID: oldIssue.ID, ResultCount: 1})
				oldCorrelator := cass.NewCorrelator(nil, oldCache, "")
				m.cassCorrelator = oldCorrelator
				m.cassCorrelatorSource = m.cassSourceIdentity()
				started := make(chan struct{})
				m.cassLookup = func(ctx context.Context, _ model.Issue) (cass.CorrelationResult, error) {
					close(started)
					<-ctx.Done()
					return cass.CorrelationResult{}, ctx.Err()
				}
				cmd := m.beginCassLookup(oldIssue)
				completion := make(chan tea.Msg, 1)
				go func() { completion <- cmd() }()
				<-started

				m = loader.load(t, m, newCorpus)
				if m.cassCancel != nil || m.cassDatasetID == 0 {
					t.Fatal("successful dataset replacement did not invalidate active Cass request")
				}
				selectedItem := m.list.SelectedItem()
				if selectedItem == nil {
					t.Fatalf("reload lost list selection: issues=%d status=%q", len(m.data.issues), m.statusMsg)
				}
				selected := selectedItem.(IssueItem).Issue
				if selected.ID != oldIssue.ID || selected.Title != scenario.updatedTitle {
					t.Fatalf("selection after reload = %s %q, want same ID and title %q", selected.ID, selected.Title, scenario.updatedTitle)
				}
				var lookedUpTitle string
				m.cassLookup = func(_ context.Context, current model.Issue) (cass.CorrelationResult, error) {
					lookedUpTitle = current.Title
					return cass.CorrelationResult{}, nil
				}
				newLookup := m.beginCassLookup(selected)
				newResult := newLookup().(cassSessionsLoadedMsg)
				if newResult.issueHash != cassIssueIdentity(selected) {
					t.Fatal("new lookup was not bound to the updated issue data")
				}
				updated, _ := m.Update(newResult)
				m = updated.(Model)
				if lookedUpTitle != scenario.updatedTitle {
					t.Fatalf("new lookup title = %q, want %q", lookedUpTitle, scenario.updatedTitle)
				}
				if m.cassCorrelator == oldCorrelator {
					t.Fatal("new dataset reused the old correlator/cache")
				}
				if hint := m.cassCorrelator.GetCached(oldIssue.ID); hint != nil {
					t.Fatal("old successful cache entry leaked into new dataset generation")
				}
				oldResult := <-completion
				updated, _ = m.Update(oldResult)
				if got := updated.(Model); got.activeModal == ModalCassSession {
					t.Fatal("old-generation completion opened after data replacement")
				}
			})
		}
	}
}

func writeCassIssuesFile(t *testing.T, issues []model.Issue) string {
	t.Helper()
	path := t.TempDir() + "/beads.jsonl"
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, issue := range issues {
		if err := encoder.Encode(issue); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
