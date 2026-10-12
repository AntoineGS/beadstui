package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui"
)

// captureFrame drives the real model without a terminal or a datasource.
func captureFrame(view string) (string, error) {
	actions := map[string]string{"list": "", "insights": "view.insights", "board": "view.board", "labels": "filter.labels"}
	action, ok := actions[view]
	if !ok {
		return "", fmt.Errorf("unknown view %q", view)
	}
	// A redirected stdout must not downgrade screenshot styling.
	ui.TermProfile = colorprofile.TrueColor
	m := ui.NewModel(sampleIssues(), nil, "", nil, nil)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		return "", fmt.Errorf("expected batched initialization commands")
	}
	// Run only the graph-analysis waiter. Executing the entire Init batch
	// would also start updater checks, agent-file checks, and recurring timers.
	var ready tea.Msg
	for _, cmd := range batch {
		if cmd == nil {
			continue
		}
		fn := runtime.FuncForPC(reflect.ValueOf(cmd).Pointer())
		if fn != nil && strings.Contains(fn.Name(), "WaitForPhase2Cmd") {
			ready = cmd()
			break
		}
	}
	if _, ok := ready.(ui.Phase2ReadyMsg); !ok {
		return "", fmt.Errorf("could not find phase-two analysis waiter in model initialization")
	}
	update := func(msg tea.Msg) tea.Cmd {
		next, cmd := m.Update(msg)
		m = next.(ui.Model)
		return cmd
	}
	update(ready)
	var settle func(tea.Cmd)
	settle = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				settle(child)
			}
			return
		}
		// Only resize commands are drained here, not Init or view commands.
		update(msg)
	}
	settle(update(tea.WindowSizeMsg{Width: 160, Height: 36}))
	if view == "board" {
		update(tea.KeyPressMsg{Text: "@filter.open"})
	}
	if action != "" {
		update(tea.KeyPressMsg{Text: "@" + action})
	}
	return m.View().Content, nil
}

// Demo issues deliberately contain no real project or user information.
func sampleIssues() []model.Issue {
	now := time.Now().Truncate(time.Hour)
	issues := []model.Issue{
		{ID: "demo-a1", Title: "Fix stale results after changing search filters", Status: model.StatusInProgress, Priority: 0, IssueType: model.TypeBug, Assignee: "alex", Labels: []string{"area:tui", "ux"}, Description: "## Problem\n\nChanging a label filter while search is active should immediately refresh the visible results.\n\n## Approach\n\nKeep the search query and label filter together when rebuilding the issue list. Preserve the selected issue when it still matches.\n\n## Acceptance criteria\n\n- Results update after every filter change\n- Clearing a filter restores matching issues\n- Keyboard focus stays in the active pane", Notes: "Verify both list and board views at narrow and wide terminal sizes."},
		{ID: "demo-b2", Title: "Keyboard navigation and discoverable actions", Status: model.StatusInProgress, Priority: 1, IssueType: model.TypeEpic, Assignee: "sam", Labels: []string{"area:tui", "ux"}, Description: "Make everyday issue tracking comfortable from the keyboard. Group view switches, filters, and project actions behind a configurable leader key."},
		{ID: "demo-c3", Title: "Add configurable leader-key bindings", Status: model.StatusClosed, Priority: 1, IssueType: model.TypeFeature, Labels: []string{"area:tui"}},
		{ID: "demo-d4", Title: "Show available actions in the which-key panel", Status: model.StatusOpen, Priority: 1, IssueType: model.TypeFeature, Labels: []string{"area:tui", "accessibility"}},
		{ID: "demo-e5", Title: "Preserve cursor when refreshing project data", Status: model.StatusBlocked, Priority: 1, IssueType: model.TypeBug, Labels: []string{"area:data", "ux"}},
		{ID: "demo-f6", Title: "Dependency-aware triage and execution planning", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeEpic, Labels: []string{"area:analysis"}},
		{ID: "demo-g7", Title: "Rank issues by the work they unblock", Status: model.StatusInProgress, Priority: 2, IssueType: model.TypeFeature, Assignee: "alex", Labels: []string{"area:analysis"}},
		{ID: "demo-h8", Title: "Explain critical paths in the insights view", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeFeature, Labels: []string{"area:analysis", "area:docs"}},
		{ID: "demo-i9", Title: "Document robot-mode JSON examples", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, Assignee: "sam", Labels: []string{"area:docs"}},
		{ID: "demo-j1", Title: "Reconnect cleanly after a Dolt server restart", Status: model.StatusBlocked, Priority: 2, IssueType: model.TypeBug, Labels: []string{"area:data"}},
		{ID: "demo-k2", Title: "Add regression coverage for modal resizing", Status: model.StatusOpen, Priority: 2, IssueType: model.TypeTask, Labels: []string{"area:tests", "accessibility"}},
		{ID: "demo-l3", Title: "Keep BQL filters consistent across views", Status: model.StatusInProgress, Priority: 2, IssueType: model.TypeFeature, Labels: []string{"area:bql", "ux"}},
		{ID: "demo-m4", Title: "Improve light-theme contrast for status badges", Status: model.StatusOpen, Priority: 3, IssueType: model.TypeTask, Labels: []string{"area:tui", "accessibility"}},
		{ID: "demo-n5", Title: "Refresh README installation instructions", Status: model.StatusClosed, Priority: 3, IssueType: model.TypeTask, Labels: []string{"area:docs"}},
		{ID: "demo-o6", Title: "Export compact issue summaries for agents", Status: model.StatusClosed, Priority: 2, IssueType: model.TypeFeature, Labels: []string{"area:export"}},
		{ID: "demo-p7", Title: "Reduce refresh work for unchanged snapshots", Status: model.StatusOpen, Priority: 3, IssueType: model.TypeChore, Labels: []string{"area:data", "performance"}},
	}
	parents := map[string]string{"demo-c3": "demo-b2", "demo-d4": "demo-b2", "demo-g7": "demo-f6", "demo-h8": "demo-f6"}
	blockers := map[string]string{"demo-e5": "demo-a1", "demo-j1": "demo-e5", "demo-h8": "demo-g7", "demo-i9": "demo-h8", "demo-k2": "demo-d4"}
	for i := range issues {
		issue := &issues[i]
		issue.CreatedAt = now.Add(-time.Duration(i+2) * 24 * time.Hour)
		issue.UpdatedAt = now.Add(-time.Duration(i+1) * time.Hour)
		if issue.Status == model.StatusClosed {
			closed := issue.UpdatedAt
			issue.ClosedAt = &closed
		}
		if parent := parents[issue.ID]; parent != "" {
			issue.Dependencies = append(issue.Dependencies, &model.Dependency{IssueID: issue.ID, DependsOnID: parent, Type: model.DepParentChild})
		}
		if blocker := blockers[issue.ID]; blocker != "" {
			issue.Dependencies = append(issue.Dependencies, &model.Dependency{IssueID: issue.ID, DependsOnID: blocker, Type: model.DepBlocks})
		}
	}
	return issues
}
