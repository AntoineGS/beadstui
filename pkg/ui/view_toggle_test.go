package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// TestViewSwitchKeysToggleBack (bt-yzfp2): the flow-matrix (SPC v f),
// attention (SPC v A), and label-dashboard (SPC v L) view switches must
// toggle back to the list on a second press, matching Board/Graph/etc. Before
// the fix they stranded the user (only Esc exited).
func TestViewSwitchKeysToggleBack(t *testing.T) {
	cases := []struct {
		name string
		seq  []string
		view ViewMode
	}{
		{"flow-matrix", []string{"space", "v", "f"}, ViewFlowMatrix},
		{"attention", []string{"space", "v", "A"}, ViewAttention},
		{"label-dashboard", []string{"space", "v", "L"}, ViewLabelDashboard},
	}

	issues := []model.Issue{
		{ID: "bv-1", Title: "One", Status: model.StatusOpen, Priority: 0},
		{ID: "bv-2", Title: "Two", Status: model.StatusInProgress, Priority: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(issues, nil, "", nil, nil)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
			m = updated.(Model)

			m = press(t, m, tc.seq...)
			if m.mode != tc.view {
				t.Fatalf("first %v: expected %v, got %v", tc.seq, tc.view, m.mode)
			}

			m = press(t, m, tc.seq...)
			if m.mode != ViewList {
				t.Errorf("second %v: expected ViewList (toggle back), got %v", tc.seq, m.mode)
			}
		})
	}
}
