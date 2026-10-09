package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

func pluginTestIssues() []model.Issue {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	return []model.Issue{
		{ID: "proj-1", Title: "First", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now},
		{ID: "proj-2", Title: "Second", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now},
	}
}

func TestSyncPluginsOnlyOnHashChange(t *testing.T) {
	m := NewModel(pluginTestIssues(), nil, "", nil, nil)
	m.syncPlugins()
	if m.pluginSyncHash != "" {
		t.Fatalf("syncPlugins without a host set hash %q", m.pluginSyncHash)
	}

	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)
	if m.PluginHost() != h {
		t.Fatal("PluginHost did not return the host set")
	}

	m.syncPlugins()
	first := m.pluginSyncHash
	if first == "" {
		t.Fatal("syncPlugins did not record a hash")
	}
	m.syncPlugins()
	if m.pluginSyncHash != first {
		t.Fatalf("hash changed without a data change: %q -> %q", first, m.pluginSyncHash)
	}

	issues := pluginTestIssues()
	issues[1].UpdatedAt = issues[1].UpdatedAt.Add(time.Hour)
	m.replaceIssues(issues)
	m.syncPlugins()
	if m.pluginSyncHash == first {
		t.Fatal("hash did not change after an issue changed")
	}
}

func TestSnapshotReuseWorkerHashForPluginSync(t *testing.T) {
	m := newSizedModel(t, pluginTestIssues(), 120, 40)
	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)

	snap := NewSnapshotBuilder(pluginTestIssues()).Build()
	snap.DataHash = "worker-hash"
	updated, _ := m.Update(SnapshotReadyMsg{Snapshot: snap, SentAt: time.Now()})
	if got := updated.(Model).pluginSyncHash; got != "worker-hash" {
		t.Fatalf("pluginSyncHash = %q, want the snapshot's DataHash", got)
	}
}

// exampleFields answers example.state from a map the test changes.
type exampleFields map[string]string

func (f exampleFields) Fields() []string { return []string{"example.state"} }

func (f exampleFields) Value(issue *model.Issue, _ string) (string, bool) {
	v, ok := f[issue.ID]
	return v, ok
}

func bqlTestIssues() []model.Issue {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	var out []model.Issue
	for i, id := range []string{"proj-1", "proj-2", "proj-3", "proj-4"} {
		out = append(out, model.Issue{ID: id, Title: id, Status: model.StatusOpen, Priority: i, CreatedAt: now, UpdatedAt: now})
	}
	return out
}

func applyTestBQL(t *testing.T, m *Model, query string) {
	t.Helper()
	q, err := bql.Parse(query)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := bql.ValidateWithFields(q, m.bqlFields()); err != nil {
		t.Fatalf("ValidateWithFields: %v", err)
	}
	m.filter.activeBQLExpr = q
	m.applyBQL(q, query)
}

func listIDs(m Model) []string {
	var ids []string
	for _, it := range m.list.VisibleItems() {
		ids = append(ids, it.(IssueItem).Issue.ID)
	}
	return ids
}

func selectedID(m Model) string {
	if it, ok := m.list.SelectedItem().(IssueItem); ok {
		return it.Issue.ID
	}
	return ""
}

func TestStateChangedRerunsPluginBQL(t *testing.T) {
	m := newSizedModel(t, bqlTestIssues(), 200, 50)
	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)
	m.pluginFields = func() []string { return []string{"example."} }
	fields := exampleFields{"proj-1": "waiting", "proj-2": "waiting"}
	m.slotRegistry.AddFields(fields)

	applyTestBQL(t, &m, "example.state = waiting")
	if got := listIDs(m); !reflect.DeepEqual(got, []string{"proj-1", "proj-2"}) {
		t.Fatalf("initial list = %v", got)
	}
	m.list.Select(1)
	if selectedID(m) != "proj-2" {
		t.Fatalf("selected %q, want proj-2", selectedID(m))
	}

	fields["proj-1"] = "running"
	fields["proj-3"] = "waiting"
	updated, _ := m.Update(plugin.StateChangedMsg{})
	got := updated.(Model)
	if ids := listIDs(got); !reflect.DeepEqual(ids, []string{"proj-2", "proj-3"}) {
		t.Fatalf("list after state change = %v, want [proj-2 proj-3]", ids)
	}
	if selectedID(got) != "proj-2" {
		t.Fatalf("selection moved to %q, want proj-2", selectedID(got))
	}
}

func TestStateChangedLeavesOtherBQLAlone(t *testing.T) {
	m := newSizedModel(t, bqlTestIssues(), 200, 50)
	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)
	m.pluginFields = func() []string { return []string{"example."} }

	applyTestBQL(t, &m, "priority < 2")
	before := listIDs(m)
	// Make the data disagree with the applied filter: a re-run would drop proj-1.
	m.data.issues[0].Priority = 3
	updated, _ := m.Update(plugin.StateChangedMsg{})
	if after := listIDs(updated.(Model)); !reflect.DeepEqual(after, before) {
		t.Fatalf("query without a plugin field was re-run: %v -> %v", before, after)
	}
}

func TestPluginSyncMsgSyncs(t *testing.T) {
	m := NewModel(pluginTestIssues(), nil, "", nil, nil)
	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)

	updated, _ := m.Update(pluginSyncMsg{})
	if updated.(Model).pluginSyncHash == "" {
		t.Fatal("pluginSyncMsg did not sync")
	}
}

func TestStateChangedRefreshesDetail(t *testing.T) {
	m := newSizedModel(t, pluginTestIssues(), 200, 50)
	if !m.isSplitView {
		t.Fatal("test needs the split view so the detail pane is visible")
	}
	h := plugin.NewHost(plugin.Options{})
	defer h.Stop()
	m.SetPluginHost(h)

	text := "BEFOREMARKER"
	m.slotRegistry.AddSections(slots.SectionFunc(func(*model.Issue, slots.Context) []slots.Section {
		return []slots.Section{{Title: "Example", Markdown: text}}
	}))
	m.updateViewportContent()
	if !strings.Contains(m.viewport.View(), "BEFOREMARKER") {
		t.Fatalf("detail pane missing initial section:\n%s", m.viewport.View())
	}

	text = "AFTERMARKER"
	updated, _ := m.Update(plugin.StateChangedMsg{})
	out := updated.(Model).viewport.View()
	if !strings.Contains(out, "AFTERMARKER") {
		t.Fatalf("detail pane not refreshed after state change:\n%s", out)
	}
}

func TestStateChangedSkipsHiddenDetail(t *testing.T) {
	m := newSizedModel(t, pluginTestIssues(), 80, 40)
	if m.detailPaneVisible() {
		t.Fatal("test needs the detail pane hidden")
	}
	text := "BEFOREMARKER"
	m.slotRegistry.AddSections(slots.SectionFunc(func(*model.Issue, slots.Context) []slots.Section {
		return []slots.Section{{Title: "Example", Markdown: text}}
	}))
	m.updateViewportContent()
	if !strings.Contains(m.viewport.View(), "BEFOREMARKER") {
		t.Fatalf("viewport missing initial section:\n%s", m.viewport.View())
	}

	text = "AFTERMARKER"
	updated, _ := m.Update(plugin.StateChangedMsg{})
	if strings.Contains(updated.(Model).viewport.View(), "AFTERMARKER") {
		t.Fatal("hidden detail pane was re-rendered on a state change")
	}
}

func TestPluginToastSeverity(t *testing.T) {
	cases := []struct {
		tone string
		want StatusSeverity
	}{
		{"", SeveritySuccess},
		{"ok", SeveritySuccess},
		{"warn", SeverityNotice},
		{"error", SeverityFailure},
	}
	for _, tc := range cases {
		t.Run(tc.tone, func(t *testing.T) {
			m := NewModel(pluginTestIssues(), nil, "", nil, nil)
			updated, _ := m.Update(plugin.ToastMsg{Plugin: "example", Toast: plugin.Toast{Message: "hello there", Tone: tc.tone}})
			got := updated.(Model)
			if got.statusMsg != "hello there" {
				t.Fatalf("statusMsg = %q, want %q", got.statusMsg, "hello there")
			}
			if got.statusSeverity != tc.want {
				t.Fatalf("statusSeverity = %v, want %v", got.statusSeverity, tc.want)
			}
		})
	}
}

func TestPluginStatusNotices(t *testing.T) {
	m := NewModel(pluginTestIssues(), nil, "", nil, nil)

	updated, _ := m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "active"}})
	if got := updated.(Model).statusMsg; got != "" {
		t.Fatalf("first activation should be silent, got %q", got)
	}

	updated, _ = m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "active", Restarts: 1}})
	if got := updated.(Model).statusMsg; got != "Plugin example restarted" {
		t.Fatalf("restart notice = %q", got)
	}

	updated, _ = m.Update(plugin.StatusMsg{Status: plugin.Status{Name: "example", State: "failed", LastError: "boom"}})
	got := updated.(Model)
	if got.statusMsg != "Plugin example failed: boom (P for details)" || got.statusSeverity != SeverityFailure {
		t.Fatalf("failure notice = %q (%v)", got.statusMsg, got.statusSeverity)
	}
}

func TestPluginPromptRepliesNilUnderModal(t *testing.T) {
	m := NewModel(pluginTestIssues(), nil, "", nil, nil)
	m.openModal(ModalHelp)
	replied := false
	var answer any = "unset"
	m.Update(plugin.PromptMsg{Plugin: "example", Confirm: &plugin.ConfirmParams{}, Reply: func(a any) {
		replied = true
		answer = a
	}})
	if !replied || answer != nil {
		t.Fatalf("prompt reply = %v (replied %v), want nil", answer, replied)
	}
}

func TestQueryMentionsPluginField(t *testing.T) {
	prefixes := []string{"example."}
	if !queryMentionsPluginField("example.state = running", prefixes) {
		t.Fatal("query naming a plugin field not detected")
	}
	if queryMentionsPluginField("status = open", prefixes) {
		t.Fatal("plain query reported as naming a plugin field")
	}
	if queryMentionsPluginField("example.state = running", nil) {
		t.Fatal("no prefixes should never match")
	}
}

func TestPopupModeSetter(t *testing.T) {
	m := NewModel(pluginTestIssues(), nil, "", nil, nil)
	if m.popupMode {
		t.Fatal("popup mode on by default")
	}
	m.SetPopupMode(true)
	if !m.popupMode {
		t.Fatal("SetPopupMode(true) did not enable popup mode")
	}
	m.SetPopupMode(false)
	if m.popupMode {
		t.Fatal("SetPopupMode(false) did not disable popup mode")
	}
}
