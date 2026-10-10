package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

func TestPluginKeyConflicts(t *testing.T) {
	statuses := []plugin.Status{
		{Name: "first", State: "active", Actions: []plugin.Action{
			{Plugin: "first", ID: "dispatch", Key: "D"},
			{Plugin: "first", ID: "jump", Key: "j"},
			{Plugin: "first", ID: "off", Key: ""},
		}},
		{Name: "second", State: "failed", Actions: []plugin.Action{
			{Plugin: "second", ID: "deploy", Key: "D"},
			{Plugin: "second", ID: "zap", Key: "Z"},
		}},
	}
	got := PluginKeyConflicts(statuses)
	want := []PluginKeyConflict{
		{Plugin: "first", Action: "jump", Key: "j", Reason: "bt binds it in list, board, tree, epics"},
		{Plugin: "second", Action: "deploy", Key: "D", Reason: "taken by first/dispatch"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conflicts = %+v\nwant %+v", got, want)
	}
}

func TestPluginKeyConflictsPartialView(t *testing.T) {
	// z is bound by the board and epics views only, so the first action
	// still claims z in the list and tree views.
	statuses := []plugin.Status{
		{Name: "a", Actions: []plugin.Action{{Plugin: "a", ID: "one", Key: "z"}}},
		{Name: "b", Actions: []plugin.Action{{Plugin: "b", ID: "two", Key: "z"}, {Plugin: "b", ID: "menu", Key: "P"}}},
	}
	got := PluginKeyConflicts(statuses)
	want := []PluginKeyConflict{
		{Plugin: "a", Action: "one", Key: "z", Reason: "bt binds it in board, epics"},
		{Plugin: "b", Action: "two", Key: "z", Reason: "bt binds it in board, epics"},
		{Plugin: "b", Action: "two", Key: "z", Reason: "taken by a/one"},
		{Plugin: "b", Action: "menu", Key: "P", Reason: "bt opens the plugin action menu with it"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conflicts = %+v\nwant %+v", got, want)
	}
}

func newPluginStatusModel(t *testing.T, fake *fakePluginActions, statuses []plugin.Status) Model {
	t.Helper()
	m := newPluginActionModel(t, fake)
	m.pluginStatuses = func() []plugin.Status { return statuses }
	return m
}

var exampleStatuses = []plugin.Status{
	{Name: "example", State: "active", Version: "0.1.0", Restarts: 2, LastError: "exited: boom",
		Actions: []plugin.Action{{Plugin: "example", ID: "jump", Key: "j"}}},
	{Name: "other", State: "disabled"},
}

func TestPluginStatusOpensWithoutActions(t *testing.T) {
	m := newPluginStatusModel(t, &fakePluginActions{}, exampleStatuses)
	m, _ = sendMsg(m, keyRune('P'))
	if m.activeModal != ModalPluginPrompt || m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptStatus {
		t.Fatalf("P without actions: modal %v prompt %+v", m.activeModal, m.pluginPrompt)
	}
	out := ansi.Strip(m.renderPluginPrompt())
	for _, want := range []string{"Plugin status", "example  active  v0.1.0  restarts 2", "error: exited: boom", "key j (jump): bt binds it in", "other  disabled"} {
		if !strings.Contains(out, want) {
			t.Errorf("status popup missing %q:\n%s", want, out)
		}
	}
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.activeModal != ModalNone || m.focused != focusList || m.pluginPrompt != nil {
		t.Fatalf("esc did not close: modal %v focus %v", m.activeModal, m.focused)
	}
}

func TestPluginStatusFromActionMenu(t *testing.T) {
	fake := &fakePluginActions{actions: []plugin.Action{dispatchAction}}
	m := newPluginStatusModel(t, fake, exampleStatuses)
	m, _ = sendMsg(m, keyRune('P'))
	if m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptMenu {
		t.Fatal("P with actions did not open the menu")
	}
	opts := m.pluginPrompt.options
	if len(opts) != 2 || !opts[1].status {
		t.Fatalf("menu options = %+v, want the action then the status row", opts)
	}
	m, _ = sendMsg(m, keyRune('j'))
	m, _ = sendMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.activeModal != ModalPluginPrompt || m.pluginPrompt == nil || m.pluginPrompt.kind != pluginPromptStatus {
		t.Fatalf("status row: modal %v prompt %+v", m.activeModal, m.pluginPrompt)
	}
	if len(fake.calls) != 0 {
		t.Fatal("status row invoked an action")
	}
	m, _ = sendMsg(m, keyRune('q'))
	if m.activeModal != ModalNone || m.focused != focusList {
		t.Fatalf("q did not close: modal %v focus %v", m.activeModal, m.focused)
	}
}

func TestPluginStatusScrolls(t *testing.T) {
	var statuses []plugin.Status
	for i := 0; i < 60; i++ {
		statuses = append(statuses, plugin.Status{Name: fmt.Sprintf("p%02d", i), State: "active"})
	}
	m := newPluginStatusModel(t, &fakePluginActions{}, statuses)
	m, _ = sendMsg(m, keyRune('P'))
	if out := ansi.Strip(m.renderPluginPrompt()); !strings.Contains(out, "p00") || strings.Contains(out, "p59") {
		t.Fatalf("first page:\n%s", out)
	}
	for i := 0; i < 100; i++ {
		m, _ = sendMsg(m, keyRune('j'))
	}
	out := ansi.Strip(m.renderPluginPrompt())
	if !strings.Contains(out, "p59") || strings.Contains(out, "p00") {
		t.Fatalf("scrolled to the end:\n%s", out)
	}
	_, _, maxScroll := m.pluginStatusLayout(pluginStatusTitle)
	if m.pluginPrompt.cursor != maxScroll {
		t.Fatalf("scroll = %d, want clamped to %d", m.pluginPrompt.cursor, maxScroll)
	}
}
