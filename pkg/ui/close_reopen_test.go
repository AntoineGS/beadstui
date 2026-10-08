package ui

// Tests for closing and reopening beads from the field-edit hub (bt-5wq):
// the hub offers Close (x) on a bead that isn't closed and Reopen (o) on a
// closed one; either opens an optional-reason input that commits through
// commitFieldEdit as `bd close` / `bd reopen`.

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/internal/bdexec"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// lifecycleModel opens the field-edit hub on a bead with the given status.
func lifecycleModel(t *testing.T, status model.Status) Model {
	t.Helper()
	issues := []model.Issue{
		{ID: "zz-life", Title: "Lifecycle bead", Status: status, Priority: 2},
		{ID: "zz-other", Title: "Another bead", Status: model.StatusOpen, Priority: 1},
	}
	m := newSizedModel(t, issues, 120, 32)
	if !m.selectIssueByID("zz-life") {
		t.Fatal("setup: zz-life not in visible list")
	}
	m.requestFieldEdit()
	if m.activeModal != ModalFieldSelect {
		t.Fatalf("setup: activeModal = %v, want ModalFieldSelect", m.activeModal)
	}
	return m
}

func TestFieldSelect_LifecycleEntryFollowsStatus(t *testing.T) {
	for _, tc := range []struct {
		status      model.Status
		want, avoid []string
	}{
		{model.StatusOpen, []string{"Close"}, []string{"Reopen"}},
		{model.StatusBlocked, []string{"Close"}, []string{"Reopen"}},
		{model.StatusClosed, []string{"Reopen"}, []string{"Close"}},
		{model.StatusTombstone, nil, []string{"Close", "Reopen"}},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			m := lifecycleModel(t, tc.status)
			out := ansi.Strip(m.fieldSelect.View())
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("hub for a %s bead lacks %q:\n%s", tc.status, w, out)
				}
			}
			for _, a := range tc.avoid {
				if strings.Contains(out, a) {
					t.Errorf("hub for a %s bead shows %q:\n%s", tc.status, a, out)
				}
			}
		})
	}
}

func TestFieldSelect_InapplicableLifecycleKeyDoesNothing(t *testing.T) {
	for _, tc := range []struct {
		status model.Status
		keys   []rune
	}{
		{model.StatusOpen, []rune{'o'}},
		{model.StatusClosed, []rune{'x'}},
		{model.StatusTombstone, []rune{'x', 'o'}},
	} {
		for _, k := range tc.keys {
			t.Run(string(tc.status)+"/"+string(k), func(t *testing.T) {
				m := lifecycleModel(t, tc.status)
				m, cmd := m.handleFieldSelectKeys(keyRune(k))
				if cmd != nil || m.activeModal != ModalFieldSelect {
					t.Fatalf("%q on a %s bead: activeModal = %v cmd = %v, want the hub unchanged", k, tc.status, m.activeModal, cmd != nil)
				}
			})
		}
	}
}

func TestClose_ReasonCommitsBdClose(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		want         []string
	}{
		{"with reason", "  shipped in v2 ", []string{"close", "zz-life", "--reason", "shipped in v2"}},
		{"without reason", "", []string{"close", "zz-life"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
			m := lifecycleModel(t, model.StatusInProgress)
			m, _ = m.handleFieldSelectKeys(keyRune('x'))
			if m.activeModal != ModalFieldInput || m.focused != focusFieldInput {
				t.Fatalf("x: activeModal = %v focused = %v, want the reason input", m.activeModal, m.focused)
			}
			out := ansi.Strip(m.fieldInput.View())
			if !strings.Contains(out, "Close zz-life") || !strings.Contains(out, "Reason (optional)") {
				t.Fatalf("reason popup lacks its title or label:\n%s", out)
			}
			m.fieldInput.input.SetValue(tc.reason)
			m, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("enter returned nil cmd (expected write dispatch)")
			}
			drainCmd(cmd)
			if !slices.Equal(*got, tc.want) {
				t.Errorf("argv = %v, want %v", *got, tc.want)
			}
			pw, pending := m.pendingWrites["zz-life"]
			if !pending || pw.Kind != writeFieldEdit || pw.Field != "status" || pw.Target != string(model.StatusClosed) {
				t.Errorf("pendingWrite = %+v (pending %v), want status -> closed", pw, pending)
			}
			if m.activeModal != ModalNone {
				t.Errorf("activeModal = %v, want ModalNone after commit", m.activeModal)
			}
		})
	}
}

func TestReopen_ReasonCommitsBdReopen(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		want         []string
	}{
		{"with reason", "regressed", []string{"reopen", "zz-life", "--reason", "regressed"}},
		{"without reason", "   ", []string{"reopen", "zz-life"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
			m := lifecycleModel(t, model.StatusClosed)
			m, _ = m.handleFieldSelectKeys(keyRune('o'))
			if m.activeModal != ModalFieldInput {
				t.Fatalf("o: activeModal = %v, want the reason input", m.activeModal)
			}
			if out := ansi.Strip(m.fieldInput.View()); !strings.Contains(out, "Reopen zz-life") {
				t.Fatalf("reason popup lacks its title:\n%s", out)
			}
			m.fieldInput.input.SetValue(tc.reason)
			m, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
			drainCmd(cmd)
			if !slices.Equal(*got, tc.want) {
				t.Errorf("argv = %v, want %v", *got, tc.want)
			}
			if pw := m.pendingWrites["zz-life"]; pw.Field != "status" || pw.Target != string(model.StatusOpen) {
				t.Errorf("pendingWrite = %+v, want status -> open", pw)
			}
		})
	}
}

func TestClose_EnterOnHubRowOpensReason(t *testing.T) {
	m := lifecycleModel(t, model.StatusOpen)
	for m.fieldSelect.SelectedField() != "close" {
		m.fieldSelect.MoveDown()
		if m.fieldSelect.cursor == 0 {
			t.Fatal("no close row in the hub")
		}
	}
	m, _ = m.handleFieldSelectKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.activeModal != ModalFieldInput || m.fieldInput.field != "close" {
		t.Fatalf("enter on Close: activeModal = %v field = %q, want the close reason input", m.activeModal, m.fieldInput.field)
	}
}

func TestClose_EscReturnsToHub(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := lifecycleModel(t, model.StatusOpen)
	m, _ = m.handleFieldSelectKeys(keyRune('x'))
	m, _ = m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.activeModal != ModalFieldSelect || m.fieldEditTargetID != "zz-life" {
		t.Fatalf("esc: activeModal = %v target = %q, want the hub for zz-life", m.activeModal, m.fieldEditTargetID)
	}
	if len(*got) != 0 {
		t.Fatalf("esc ran bd: %v", *got)
	}
}
