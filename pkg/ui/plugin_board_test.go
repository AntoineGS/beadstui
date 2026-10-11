package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

func exampleLaneFields() []plugin.LaneField {
	return []plugin.LaneField{{Name: "example.state", Label: "State", Values: []plugin.FieldValue{
		{ID: "waiting", Label: "Waiting", Badge: "WAIT", Tone: "warn"},
		{ID: "running", Label: "Running"},
	}}}
}

// boardLaneIDs returns the IDs in each lane of the board's active mode.
func boardLaneIDs(b BoardModel) [][]string {
	out := make([][]string, len(b.columns))
	for i, col := range b.columns {
		for _, issue := range col {
			out[i] = append(out[i], issue.ID)
		}
	}
	return out
}

func boardModeKeys(b BoardModel) []SwimLaneMode {
	var keys []SwimLaneMode
	for _, m := range b.modeList() {
		keys = append(keys, m.Key)
	}
	return keys
}

func TestPluginBoardModeFollowsPlugin(t *testing.T) {
	m := newSizedModel(t, bqlTestIssues(), 200, 50)
	// proj-4's value is not a lane value (lane=false), so it stays off the board.
	fields := exampleFields{"proj-1": "waiting", "proj-2": "running", "proj-4": "done"}
	m.slotRegistry.AddFields(fields)
	var active []plugin.LaneField
	m.pluginLaneFields = func() []plugin.LaneField { return active }

	updated, _ := m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "starting"}})
	m = updated.(Model)
	if got := boardModeKeys(m.board); len(got) != 3 {
		t.Fatalf("modes before the plugin is active = %v", got)
	}

	active = exampleLaneFields()
	updated, _ = m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "active"}})
	m = updated.(Model)
	want := []SwimLaneMode{SwimByStatus, SwimByPriority, SwimByType, "example.state"}
	if got := boardModeKeys(m.board); !reflect.DeepEqual(got, want) {
		t.Fatalf("modes = %v, want %v", got, want)
	}

	for i := 0; i < 3; i++ {
		m.board.CycleSwimLaneMode()
	}
	if m.board.GetSwimLaneMode() != "example.state" || m.board.GetSwimLaneModeName() != "example.state" {
		t.Fatalf("mode = %q (%s), want example.state", m.board.GetSwimLaneMode(), m.board.GetSwimLaneModeName())
	}
	titles, icons := m.board.getColumnHeaders()
	if want := []string{"WAITING", "RUNNING", "NO STATE"}; !reflect.DeepEqual(titles, want) {
		t.Fatalf("lanes = %v, want %v", titles, want)
	}
	if want := []string{"WAIT", "", ""}; !reflect.DeepEqual(icons, want) {
		t.Fatalf("lane icons = %v, want %v", icons, want)
	}
	if got, want := boardLaneIDs(m.board), [][]string{{"proj-1"}, {"proj-2"}, {"proj-3"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lanes = %v, want %v", got, want)
	}
	view := m.board.View(200, 40)
	for _, s := range []string{"WAIT WAITING (1)", "RUNNING (1)", "NO STATE (1)"} {
		if !strings.Contains(view, s) {
			t.Fatalf("board view missing %q:\n%s", s, view)
		}
	}

	// A state.set moving proj-1 to running and proj-3 to waiting.
	if !m.board.SelectIssueByID("proj-1") {
		t.Fatal("proj-1 not on the board")
	}
	fields["proj-1"] = "running"
	fields["proj-3"] = "waiting"
	updated, _ = m.Update(plugin.StateChangedMsg{})
	m = updated.(Model)
	if got, want := boardLaneIDs(m.board), [][]string{{"proj-3"}, {"proj-1", "proj-2"}, nil}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lanes after state change = %v, want %v", got, want)
	}
	if sel := m.board.SelectedIssue(); sel == nil || sel.ID != "proj-1" {
		t.Fatalf("selection did not follow proj-1: %+v", sel)
	}

	// The plugin stops: its mode goes and the board falls back to status.
	active = nil
	updated, _ = m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "starting"}})
	m = updated.(Model)
	if got := boardModeKeys(m.board); len(got) != 3 {
		t.Fatalf("modes after the plugin stopped = %v", got)
	}
	if m.board.GetSwimLaneMode() != SwimByStatus || len(m.board.columns) != 4 {
		t.Fatalf("mode = %q with %d lanes, want status", m.board.GetSwimLaneMode(), len(m.board.columns))
	}
	if m.board.TotalCount() != 4 {
		t.Fatalf("status board shows %d cards, want 4", m.board.TotalCount())
	}
}

func TestPluginBoardModeRefreshKeepsSearch(t *testing.T) {
	b := NewBoardModel(bqlTestIssues(), DefaultTheme())
	fields := exampleFields{"proj-1": "waiting"}
	b.SetExtraModes(pluginBoardModes(exampleLaneFields(), fields.Value))
	b.SetSwimLaneMode("example.state")

	b.StartSearch()
	for _, r := range "proj-1" {
		b.AppendSearchChar(r)
	}
	if b.SearchMatchCount() != 1 || !b.IsMatchHighlighted(0, 0) {
		t.Fatal("search did not find proj-1 in the waiting lane")
	}

	fields["proj-1"] = "running"
	b.RefreshLanes()
	if !b.IsSearchMode() || b.SearchQuery() != "proj-1" || !b.IsMatchHighlighted(1, 0) {
		t.Fatalf("search not kept across a lane refresh: mode %v query %q matches %+v", b.IsSearchMode(), b.SearchQuery(), b.searchMatches)
	}
}

func TestBuiltinModesIgnoreRefresh(t *testing.T) {
	b := NewBoardModel(bqlTestIssues(), DefaultTheme())
	b.StartSearch()
	b.AppendSearchChar('p')
	b.SetExtraModes(pluginBoardModes(exampleLaneFields(), exampleFields{}.Value))
	b.RefreshLanes()
	if !b.IsSearchMode() || b.GetSwimLaneMode() != SwimByStatus {
		t.Fatal("adding plugin modes disturbed the status board")
	}
}
