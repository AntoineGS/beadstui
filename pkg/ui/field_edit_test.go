package ui

// Tests for the first field edits (bt-oiaj.5): the field-select hub, the
// enum picker (status/priority), and the textinput sub-modal (title/
// assignee), all built on the generic pending/settled write machinery
// (claim.go, bt-oiaj.13). The executor runs through the claimRunner seam
// (shared with claim — see claim_test.go's stubClaimRunner), stubbed here so
// no bd process is spawned.

import (
	"bytes"
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/seanmartinsmith/beadstui/internal/bdexec"
	"github.com/seanmartinsmith/beadstui/internal/bdroute"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestFreshSupportingInputsTextRoles(t *testing.T) {
	theme := DefaultTheme()
	theme.Text.Body = lipgloss.NewStyle().Background(theme.Primary).Underline(true).Bold(false).Italic(false)
	theme.Text.Heading = lipgloss.NewStyle().Background(theme.Secondary).Bold(false).Italic(false)
	theme.Text.Metadata = lipgloss.NewStyle().Underline(true).Bold(false).Italic(false)
	field := NewFieldInputModal("title", "Title", "draft", theme)
	bql := NewBQLQueryModal(theme)
	label := NewLabelPickerModel(nil, nil, theme)
	repo := NewRepoPickerModel(nil, theme)
	for name, input := range map[string]textinput.Model{"field": field.input, "bql": bql.input, "label": label.input, "repo": repo.input} {
		styles := input.Styles()
		for _, state := range []textinput.StyleState{styles.Focused, styles.Blurred} {
			if !reflect.DeepEqual(state.Text, theme.Text.Body) || !reflect.DeepEqual(state.Prompt, theme.Text.Heading) || !reflect.DeepEqual(state.Placeholder, theme.Text.Metadata) {
				t.Errorf("%s constructor did not adopt current text roles", name)
			}
		}
	}
	longform := NewLongformEditModal("description", "Description", "unsaved", theme)
	areaStyles := longform.textarea.Styles()
	for _, state := range []struct{ text, prompt, placeholder lipgloss.Style }{
		{areaStyles.Focused.Text, areaStyles.Focused.Prompt, areaStyles.Focused.Placeholder},
		{areaStyles.Blurred.Text, areaStyles.Blurred.Prompt, areaStyles.Blurred.Placeholder},
	} {
		if !reflect.DeepEqual(state.text, theme.Text.Body) || !reflect.DeepEqual(state.prompt, theme.Text.Heading) || !reflect.DeepEqual(state.placeholder, theme.Text.Metadata) {
			t.Error("textarea constructor did not adopt current roles")
		}
	}
	field.SetSize(80, 24)
	if !strings.Contains(field.View(), theme.Text.Heading.Render("Title:")) {
		t.Error("field label ignored heading role")
	}
	bql.SetSize(100, 40)
	bql.input.SetValue("status:open")
	out := bql.View()
	if !strings.Contains(out, theme.Text.Body.Render("status:open")) {
		t.Error("rendered BQL input ignored body role")
	}
	if !strings.Contains(out, theme.Text.Metadata.Render("↑/↓ history")) {
		t.Error("rendered BQL hint ignored metadata role")
	}
}

func TestSupportingSurfacesTextRoles(t *testing.T) {
	restoreThemeGlobals(t)
	theme := DefaultTheme()
	role := lipgloss.NewStyle().Foreground(theme.TextColor).Background(theme.Secondary).Underline(true).Bold(false).Italic(false)
	theme.Text.Body = role.Foreground(theme.Success)
	theme.Text.Metadata = role.Foreground(theme.Warning)
	theme.Text.Heading = role.Foreground(theme.Info)
	theme.Text.Badge = role.Foreground(theme.Danger)
	theme.Text.Selected, theme.Text.Callout = role, role
	m := settingsTestModel(t)
	m.theme = theme
	m.refreshThemeConsumers()
	ActiveTextStyles = theme.Text
	m.claimTargetID, m.claimTargetTitle = "role-id", "claim draft"
	field := NewFieldInputModal("title", "Title", "draft", theme)
	field.SetSize(100, 40)
	longform := NewLongformEditModal("description", "Description", "draft", theme)
	longform.SetSize(100, 40)
	agent := NewAgentPromptModal("AGENTS.md", "AGENTS.md", theme)
	agent.SetSize(100, 40)
	update := NewUpdateModal("v1", "", theme)
	update.SetSize(100, 40)
	m.cassModal.beadID = "role-id"
	m.cassModal.SetSize(100, 40)
	m.shortcutsSidebar.SetSize(34, 40)
	m.settingsModal.SetSize(100, 40)
	for _, tc := range []struct{ name, out, label string }{
		{"field", field.View(), "Title:"},
		{"longform", longform.View(), "Description:"},
		{"claim", m.renderClaimConfirm(), "role-id"},
		{"update", update.View(), "Current version: "},
		{"agent", agent.View(), "Preview of content to add:"},
		{"cass", m.cassModal.View(), "role-id"},
		{"help", RenderContextHelp(ContextList, theme, 100, 40), "Press ` for full tutorial │ Esc to close"},
		{"sidebar", m.shortcutsSidebar.View(), "? global  -  ; hide"},
		{"settings", strings.Join(m.settingsModal.renderHelp(60), "\n"), "Set the color palette."},
		{"footer scope", renderLens(richLensFixture(160), lensFull), "bt"},
		{"footer hint", renderStaticHints(false), " help"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := theme.Text.Metadata
			switch tc.name {
			case "field", "longform":
				want = theme.Text.Heading
			case "settings":
				want = theme.Text.Body
			case "footer scope":
				want = theme.Text.Badge
			}
			if !strings.Contains(tc.out, want.Render(tc.label)) {
				t.Fatalf("%s did not retain explicit false attributes/background for %q:\n%s", tc.name, tc.label, tc.out)
			}
		})
	}
}

func TestFieldPickerView_AlignedCurrentAndCursor(t *testing.T) {
	for _, width := range []int{48, 80, 120} {
		for _, tc := range []struct {
			picker          FieldPickerModal
			current, cursor string
		}{{NewStatusPickerModal(model.StatusOpen, DefaultTheme()), "Open", "In Progress"}, {NewPriorityPickerModal(2, DefaultTheme()), "P2", "P3"}} {
			p := tc.picker
			p.SetSize(width, 24)
			p.MoveDown()
			out := p.View()
			assertPopupBounds(t, out, width, 24)
			_, a := popupFindRow(t, out, tc.current)
			_, b := popupFindRow(t, out, tc.cursor)
			x, y := ansi.StringWidth(a[:strings.Index(a, tc.current)]), ansi.StringWidth(b[:strings.Index(b, tc.cursor)])
			if x != y || !strings.Contains(a, "*") || popupRowSelected(out, tc.current, p.theme) || !popupRowSelected(out, tc.cursor, p.theme) || strings.Contains(b, ">") {
				t.Fatalf("current/cursor columns or markers wrong:\n%s", ansi.Strip(out))
			}
		}
	}
}

func TestFieldSelectView_ShortTerminalKeepsSelected(t *testing.T) {
	m := NewFieldSelectModal(model.StatusOpen, DefaultTheme())
	m.SetSize(36, 10)
	m.MoveUp()
	out := m.View()
	assertPopupBounds(t, out, 36, 10)
	_, row := popupFindRow(t, out, "Close")
	if !popupRowSelected(out, "x  Close", m.theme) || !strings.Contains(row, "x") {
		t.Fatal("selected accelerator hidden")
	}
	popupFindRow(t, out, "esc")
}

func TestFieldInputPopup_LeftAlignedAndBounded(t *testing.T) {
	m := NewFieldInputModal("title", "Title", "draft", DefaultTheme())
	m.SetError(strings.Repeat("error ", 30))
	for _, size := range []PopupSize{{32, 12}, {80, 24}} {
		m.SetSize(size.Width, size.Height)
		out := m.View()
		assertPopupBounds(t, out, size.Width, size.Height)
		_, label := popupFindRow(t, out, "Title:")
		_, value := popupFindRow(t, out, "draft")
		// textinput's prompt occupies two cells after the shared body origin.
		if x, y := ansi.StringWidth(label[:strings.Index(label, "Title:")]), ansi.StringWidth(value[:strings.Index(value, "draft")]); y != x+2 {
			t.Fatalf("input not aligned under label: %q / %q", label, value)
		}
		if m.Value() != "draft" {
			t.Fatal("sizing changed input")
		}
		assertNoAssumedKeyHints(t, out)
	}
}

func TestFieldEditPopup_BackAfterResizeFitsRetainedHub(t *testing.T) {
	for _, field := range []string{"title", "priority", "description"} {
		t.Run(field, func(t *testing.T) {
			m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
			mustSelectTarget(t, &m)
			m.requestFieldEdit()
			m.fieldSelect.MoveDown()
			m, _ = m.openFieldPickerOrInput(field)
			if field == "description" {
				m.longformEdit.textarea.SetValue("dirty draft")
			}
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 36, Height: 12})
			m = updated.(Model)
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			m = updated.(Model)
			if field == "description" {
				if m.activeModal != ModalLongformEdit || !m.longformEdit.escArmed {
					t.Fatal("first escape lost dirty guard")
				}
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				m = updated.(Model)
			}
			if m.activeModal != ModalFieldSelect || m.fieldSelect.SelectedField() != "priority" || m.fieldEditTargetID != "zz-target" {
				t.Fatal("back changed retained hub selection or target")
			}
			out := m.fieldSelect.View()
			assertPopupBounds(t, out, 36, 11)
			popupFindRow(t, out, "Priority")
		})
	}
}

func TestFieldInputPopup_ShortBudgetShowsInputOrFallback(t *testing.T) {
	for _, err := range []string{"", "invalid title"} {
		for _, height := range []int{3, 4, 5} {
			m := NewFieldInputModal("title", "Title", "typed draft", DefaultTheme())
			m.SetError(err)
			m.SetSize(32, height)
			out := m.View()
			assertPopupBounds(t, out, 32, height)
			// Label + input (+ error) inside the border; no footer rows.
			minimum := 4
			if err != "" {
				minimum = 5
			}
			if height < minimum {
				if !strings.Contains(out, "Terminal too small") {
					t.Fatalf("invisible input without fallback:\n%s", ansi.Strip(out))
				}
			} else {
				popupFindRow(t, out, "typed draft")
				if err != "" {
					popupFindRow(t, out, err)
				}
			}
			if m.Value() != "typed draft" {
				t.Fatal("sizing changed draft")
			}
		}
	}
}

// fieldEditTestIssues seeds a target bead with non-default values on every
// editable field (blocked status, P2, a title, no assignee) so picker
// cursor-placement and commit-argv tests have something to move away from.
// zz-target is BLOCKED (not open), so the list's open-first sort (bt-88qn
// claim precedent — see claim_test.go's closed-bead comment) pushes it below
// zz-other; every test below selects it explicitly rather than relying on
// default cursor position.
func fieldEditTestIssues() []model.Issue {
	return []model.Issue{
		{ID: "zz-target", Title: "Original title", Status: model.StatusBlocked, Priority: 2, Assignee: ""},
		{ID: "zz-other", Title: "Another bead", Status: model.StatusOpen, Priority: 1},
	}
}

// mustSelectTarget selects zz-target and fails the test if it isn't in the
// visible list (setup error, not a test failure).
func mustSelectTarget(t *testing.T, m *Model) {
	t.Helper()
	if !m.selectIssueByID("zz-target") {
		t.Fatal("setup: zz-target not in visible list")
	}
}

// drainCmd recursively executes cmd, unwrapping any tea.BatchMsg it produces,
// so a stubbed executor observes the invocation regardless of whether
// commitFieldEdit batched the write with the spinner-tick cmd. A fresh
// model's first write always arms the spinner (writeSpinnerActive starts
// false), so tea.Batch wraps two cmds into a BatchMsg rather than returning
// the write cmd bare — calling cmd() once would otherwise yield a BatchMsg,
// not the writeResultMsg the write cmd itself produces.
func drainCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainCmd(c)
		}
	}
}

// ---------------------------------------------------------------------------
// Trigger (requestFieldEdit) — mirrors TestRequestClaim_* in claim_test.go.
// ---------------------------------------------------------------------------

func TestRequestFieldEdit_OpensFieldSelectForSelected(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	if m.activeModal != ModalFieldSelect {
		t.Fatalf("activeModal = %v, want ModalFieldSelect", m.activeModal)
	}
	if m.fieldEditTargetID != "zz-target" {
		t.Errorf("fieldEditTargetID = %q, want zz-target", m.fieldEditTargetID)
	}
	if m.focused != focusFieldSelect {
		t.Errorf("focused = %v, want focusFieldSelect", m.focused)
	}
	out := ansi.Strip(m.View().Content)
	if !strings.Contains(out, "Edit field") {
		t.Errorf("field-select modal not rendered; view:\n%s", out)
	}
}

func TestRequestFieldEdit_RefusesWhenAlreadyPending(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.pendingWrites["zz-target"] = pendingWrite{Kind: writeClaim, StartedAt: time.Now()}
	m.requestFieldEdit()
	if m.activeModal != ModalNone {
		t.Errorf("activeModal = %v, want ModalNone (refused, not opened)", m.activeModal)
	}
	if !strings.Contains(m.statusMsg, "write already pending for zz-target") {
		t.Errorf("notice = %q, want it to name the pending write", m.statusMsg)
	}
}

func TestRequestFieldEdit_NoSelectionRefused(t *testing.T) {
	m := newSizedModel(t, []model.Issue{}, 120, 32)
	m.requestFieldEdit()
	if m.activeModal != ModalNone {
		t.Errorf("activeModal = %v, want ModalNone (no selection)", m.activeModal)
	}
	if !strings.Contains(m.statusMsg, "No issue selected") {
		t.Errorf("notice = %q, want it to say no issue selected", m.statusMsg)
	}
}

// ---------------------------------------------------------------------------
// Field-select hub dispatch.
// ---------------------------------------------------------------------------

func TestFieldSelectView_AlignedColumns(t *testing.T) {
	for _, size := range []struct {
		name  string
		width int
	}{
		{"narrow", 48},
		{"medium", 80},
		{"wide", 120},
	} {
		entries := fieldEditEntriesFor(model.StatusOpen)
		for selected := range entries {
			t.Run(size.name+"/"+entries[selected].Field, func(t *testing.T) {
				modal := NewFieldSelectModal(model.StatusOpen, DefaultTheme())
				modal.SetSize(size.width, 32)
				modal.cursor = selected
				out := ansi.Strip(modal.View())
				rows := strings.Split(out, "\n")
				labelColumn := -1
				for _, entry := range entries {
					var row string
					labelIndex := -1
					for _, line := range rows {
						if index := strings.Index(line, entry.Label); index >= 0 {
							row, labelIndex = line, index
							break
						}
					}
					if labelIndex < 3 {
						t.Fatalf("missing field row %q in view:\n%s", entry.Label, out)
					}
					column := ansi.StringWidth(row[:labelIndex])
					if labelColumn < 0 {
						labelColumn = column
					} else if column != labelColumn {
						t.Errorf("%s label at column %d, want %d", entry.Label, column, labelColumn)
					}
					if got := row[labelIndex-3 : labelIndex]; got != entry.Key+"  " {
						t.Errorf("%s shortcut prefix = %q, want %q", entry.Label, got, entry.Key+"  ")
					}
					if strings.Contains(row[:labelIndex], ">") {
						t.Errorf("%s row has a cursor glyph: %q", entry.Label, row)
					}
				}
			})
		}
	}
}

func TestFieldSelectKeys_AcceleratorsOpenCorrectSubmodal(t *testing.T) {
	cases := []struct {
		key       string
		wantModal ModalType
		wantField string // fieldPicker.field or fieldInput.field, per wantModal
	}{
		{"s", ModalFieldPicker, "status"},
		{"p", ModalFieldPicker, "priority"},
		{"t", ModalFieldInput, "title"},
		{"a", ModalFieldInput, "assignee"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
			mustSelectTarget(t, &m)
			m.requestFieldEdit()
			m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: rune(tc.key[0]), Text: tc.key})
			m = m2
			if m.activeModal != tc.wantModal {
				t.Fatalf("key %q -> activeModal = %v, want %v", tc.key, m.activeModal, tc.wantModal)
			}
			switch tc.wantModal {
			case ModalFieldPicker:
				if m.fieldPicker.field != tc.wantField {
					t.Errorf("fieldPicker.field = %q, want %q", m.fieldPicker.field, tc.wantField)
				}
			case ModalFieldInput:
				if m.fieldInput.field != tc.wantField {
					t.Errorf("fieldInput.field = %q, want %q", m.fieldInput.field, tc.wantField)
				}
			}
		})
	}
}

// TestFieldSelectKeys_EnterOpensCursorField verifies Enter opens whichever
// row the cursor is on (not always the first row).
func TestFieldSelectKeys_EnterOpensCursorField(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m.fieldSelect.MoveDown() // status -> priority
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if m.activeModal != ModalFieldPicker || m.fieldPicker.field != "priority" {
		t.Fatalf("enter on cursor=priority -> activeModal=%v field=%q, want ModalFieldPicker/priority",
			m.activeModal, m.fieldPicker.field)
	}
}

func TestFieldSelectKeys_EscCancelsFlow(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = m2
	if m.activeModal != ModalNone {
		t.Errorf("activeModal = %v, want ModalNone after esc", m.activeModal)
	}
	if m.fieldEditTargetID != "" {
		t.Errorf("fieldEditTargetID = %q, want empty after cancel", m.fieldEditTargetID)
	}
}

// TestFieldSelectKeys_TitleAndAssigneePrefillCurrentValue verifies the
// textinput sub-modal opens prefilled with the bead's current value, not
// empty (plan step 4).
func TestFieldSelectKeys_TitleAndAssigneePrefillCurrentValue(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = m2
	if got := m.fieldInput.Value(); got != "Original title" {
		t.Errorf("title input prefill = %q, want %q", got, "Original title")
	}
}

// ---------------------------------------------------------------------------
// Enum picker (status, priority).
// ---------------------------------------------------------------------------

// TestStatusPickerOptions_ExcludesDestructiveStates pins the picker's status
// set: the destructive transitions (closed, tombstone) are excluded — they
// need a reason-bearing form modal, bt-oiaj.2 scope (fork #7, extended per
// the controller decision on the Slice B tombstone flag; no second confirm
// on picker Enter means a stray Enter must not soft-delete) — while all
// seven reversible workflow states are present.
func TestStatusPickerOptions_ExcludesDestructiveStates(t *testing.T) {
	got := make(map[string]bool)
	for _, o := range statusPickerOptions() {
		got[o.Value] = true
	}

	for _, excluded := range []model.Status{model.StatusClosed, model.StatusTombstone} {
		if got[string(excluded)] {
			t.Errorf("statusPickerOptions() must exclude %q (destructive transition, bt-oiaj.2 scope)", excluded)
		}
	}

	want := []model.Status{
		model.StatusOpen, model.StatusInProgress, model.StatusBlocked,
		model.StatusDeferred, model.StatusPinned, model.StatusHooked,
		model.StatusReview,
	}
	for _, s := range want {
		if !got[string(s)] {
			t.Errorf("statusPickerOptions() missing workflow state %q", s)
		}
	}
	if len(got) != len(want) {
		t.Errorf("statusPickerOptions() has %d options, want exactly %d: %v", len(got), len(want), got)
	}
}

// TestFieldSelectKeys_StatusFencedOnTerminalStatus pins the reopen fence
// (fork #7's second clause, "excludes closed (and reopen-from-closed)"): on
// a closed or tombstoned bead, entering the status field — via the `s`
// accelerator OR Enter on the status row — must not open the picker at all.
// Without the guard, newFieldPickerModal's cursor falls back to "open"
// (index 0) and a stray Enter would commit a reopen with no confirm.
// The field-select hub stays open (other fields remain editable), a notice
// points closed beads to the hub's Reopen entry (bt-5wq), the executor is
// never invoked, nothing goes pending.
func TestFieldSelectKeys_StatusFencedOnTerminalStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status model.Status
	}{
		{"closed", model.StatusClosed},
		{"tombstone", model.StatusTombstone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invoked := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
			issues := []model.Issue{
				{ID: "zz-dead", Title: "Terminal bead", Status: tc.status, Priority: 2},
				{ID: "zz-other", Title: "Another bead", Status: model.StatusOpen, Priority: 1},
			}
			m := newSizedModel(t, issues, 120, 32)
			if !m.selectIssueByID("zz-dead") {
				t.Fatal("setup: zz-dead not in visible list")
			}
			m.requestFieldEdit()

			// Accelerator path: `s`.
			m2, cmd := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
			m = m2
			if cmd != nil {
				t.Error("fenced status entry must not dispatch a cmd (accelerator path)")
			}
			if m.activeModal != ModalFieldSelect {
				t.Errorf("activeModal = %v, want ModalFieldSelect (picker must not open, hub stays)", m.activeModal)
			}
			if !strings.Contains(m.statusMsg, string(tc.status)) || strings.Contains(m.statusMsg, "bt-oiaj.2") {
				t.Errorf("notice = %q, want it to name the terminal status", m.statusMsg)
			}
			if wantReopen := tc.status == model.StatusClosed; strings.Contains(m.statusMsg, "o Reopen") != wantReopen {
				t.Errorf("notice = %q, want the o Reopen hint only on closed beads", m.statusMsg)
			}

			// Enter path: cursor starts on the Status row (index 0).
			m2, cmd = m.handleFieldSelectKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = m2
			if cmd != nil {
				t.Error("fenced status entry must not dispatch a cmd (enter path)")
			}
			if m.activeModal != ModalFieldSelect {
				t.Errorf("activeModal = %v after enter, want ModalFieldSelect", m.activeModal)
			}

			if len(*invoked) != 0 {
				t.Errorf("executor must never be invoked through the fence; got argv %v", *invoked)
			}
			if _, pending := m.pendingWrites["zz-dead"]; pending {
				t.Error("fenced status entry must not register a pendingWrite")
			}

			// The fence is status-only: priority on the same bead still opens.
			m2, _ = m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 'p', Text: "p"})
			m = m2
			if m.activeModal != ModalFieldPicker || m.fieldPicker.field != "priority" {
				t.Errorf("priority on a %s bead should still open its picker; activeModal=%v field=%q",
					tc.status, m.activeModal, m.fieldPicker.field)
			}
		})
	}
}

func TestFieldPickerKeys_CursorStartsOnCurrentValue(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = m2
	got := m.fieldPicker.Selected()
	if got.Value != string(model.StatusBlocked) {
		t.Errorf("status picker cursor = %q, want %q (the bead's current status)", got.Value, model.StatusBlocked)
	}
}

func TestFieldPickerKeys_EnterCommitsStatus(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = m2
	m.fieldPicker.MoveDown() // blocked -> deferred
	m2, cmd := m.handleFieldPickerKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd == nil {
		t.Fatal("handleFieldPickerKeys(enter) returned nil cmd (expected write dispatch)")
	}
	drainCmd(cmd)
	want := []string{"update", "zz-target", "--status", "deferred"}
	if !slices.Equal(*got, want) {
		t.Errorf("commit argv = %v, want %v", *got, want)
	}
	pw, pending := m.pendingWrites["zz-target"]
	if !pending {
		t.Fatal("zz-target not marked pending after commit")
	}
	if pw.Kind != writeFieldEdit || pw.Field != "status" || pw.Target != "deferred" {
		t.Errorf("pendingWrite = %+v, want {Kind:writeFieldEdit Field:status Target:deferred}", pw)
	}
	if m.activeModal != ModalNone {
		t.Errorf("activeModal = %v, want ModalNone after commit", m.activeModal)
	}
}

func TestFieldPickerKeys_EnterCommitsPriority(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 'p', Text: "p"})
	m = m2
	m.fieldPicker.MoveDown() // P2 -> P3
	m2, cmd := m.handleFieldPickerKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd == nil {
		t.Fatal("handleFieldPickerKeys(enter) returned nil cmd")
	}
	drainCmd(cmd)
	want := []string{"update", "zz-target", "-p", "3"}
	if !slices.Equal(*got, want) {
		t.Errorf("commit argv = %v, want %v (numeric wire form per fork #8)", *got, want)
	}
	pw := m.pendingWrites["zz-target"]
	if pw.Target != "3" {
		t.Errorf("pendingWrite.Target = %q, want %q (matches fieldValue's strconv.Itoa form)", pw.Target, "3")
	}
}

func TestFieldPickerKeys_EscReturnsToFieldSelect(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = m2
	m2, _ = m.handleFieldPickerKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = m2
	if m.activeModal != ModalFieldSelect {
		t.Errorf("activeModal = %v, want ModalFieldSelect (esc steps back one level)", m.activeModal)
	}
	if m.fieldEditTargetID != "zz-target" {
		t.Errorf("fieldEditTargetID = %q, want zz-target preserved across the step-back", m.fieldEditTargetID)
	}
}

// ---------------------------------------------------------------------------
// Textinput sub-modal (title, assignee).
// ---------------------------------------------------------------------------

func TestFieldInputKeys_EnterCommitsTitle(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = m2
	m.fieldInput.input.SetValue("New title")
	m2, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd == nil {
		t.Fatal("handleFieldInputKeys(enter) returned nil cmd")
	}
	drainCmd(cmd)
	want := []string{"update", "zz-target", "--title", "New title"}
	if !slices.Equal(*got, want) {
		t.Errorf("commit argv = %v, want %v", *got, want)
	}
	pw := m.pendingWrites["zz-target"]
	if pw.Field != "title" || pw.Target != "New title" {
		t.Errorf("pendingWrite = %+v, want Field=title Target=%q", pw, "New title")
	}
}

func TestFieldInputKeys_EnterCommitsAssignee(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = m2
	m.fieldInput.input.SetValue("sms")
	m2, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd == nil {
		t.Fatal("handleFieldInputKeys(enter) returned nil cmd")
	}
	drainCmd(cmd)
	want := []string{"update", "zz-target", "-a", "sms"}
	if !slices.Equal(*got, want) {
		t.Errorf("commit argv = %v, want %v", *got, want)
	}
}

// TestFieldInputKeys_EmptyTitleRefused verifies the client-side refusal
// (plan step 4): an empty title never reaches the executor.
func TestFieldInputKeys_EmptyTitleRefused(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 't', Text: "t"})
	m = m2
	m.fieldInput.input.SetValue("")
	m2, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd != nil {
		t.Error("empty title must not dispatch a write cmd")
	}
	if m.activeModal != ModalFieldInput {
		t.Errorf("activeModal = %v, want ModalFieldInput (modal stays open on refusal)", m.activeModal)
	}
	if m.fieldInput.err == "" {
		t.Error("fieldInput.err should be set for an empty title")
	}
	if len(*got) != 0 {
		t.Errorf("executor must not be invoked for an empty title; got argv %v", *got)
	}
	if _, pending := m.pendingWrites["zz-target"]; pending {
		t.Error("empty title must not mark the row pending")
	}
}

// Assignee has no client-side emptiness guard (clearing an assignee is a
// legitimate edit — unassign); this proves the asymmetry is intentional, not
// a missed case.
func TestFieldInputKeys_EmptyAssigneeAllowed(t *testing.T) {
	got := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = m2
	m.fieldInput.input.SetValue("")
	m2, cmd := m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd == nil {
		t.Fatal("empty assignee should still commit (unassign is legitimate)")
	}
	drainCmd(cmd)
	want := []string{"update", "zz-target", "-a", ""}
	if !slices.Equal(*got, want) {
		t.Errorf("commit argv = %v, want %v", *got, want)
	}
}

func TestFieldInputKeys_EscReturnsToFieldSelect(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = m2
	m2, _ = m.handleFieldInputKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = m2
	if m.activeModal != ModalFieldSelect {
		t.Errorf("activeModal = %v, want ModalFieldSelect (esc steps back one level)", m.activeModal)
	}
}

// ---------------------------------------------------------------------------
// Refusal keypath — bdroute.Resolve refuses before any bd invocation.
// ---------------------------------------------------------------------------

// TestCommitFieldEdit_RefusesWhenUnmappable verifies the write-routing
// consumer contract (write-routing.md "Consumers"): a resolver error is a
// pre-flight refusal with zero bd invocations, mirroring
// TestClaimKeypath_RefusalTeatest's assertions but at the unit level.
func TestCommitFieldEdit_RefusesWhenUnmappable(t *testing.T) {
	invoked := stubClaimRunner(t, bdexec.Result{ExitCode: 0})
	// An empty workspace table maps no prefixes at all - any write in this
	// model is unmappable.
	m := newSizedModelWithRoute(t, fieldEditTestIssues(), 120, 32, bdroute.FromWorkspace(nil))
	mustSelectTarget(t, &m)
	m.requestFieldEdit()
	m2, _ := m.handleFieldSelectKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = m2
	m2, cmd := m.handleFieldPickerKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2
	if cmd != nil {
		t.Fatal("a refused write must not dispatch a cmd")
	}
	if len(*invoked) != 0 {
		t.Errorf("executor must not be invoked on a pre-flight refusal; got argv %v", *invoked)
	}
	if _, pending := m.pendingWrites["zz-target"]; pending {
		t.Error("a refused write must never enter the pending state")
	}
	if m.statusSeverity != SeverityFailure {
		t.Errorf("statusSeverity = %v, want SeverityFailure", m.statusSeverity)
	}
	if !strings.Contains(m.statusMsg, "no workspace mapping") {
		t.Errorf("refusal toast = %q, want it to name the missing workspace mapping", m.statusMsg)
	}
}

// TestOpenFieldPickerOrInput_IssueNotFoundInLoadedData verifies the
// not-found guard mirrors confirmClaim's identical guard: a stale target ID
// (not present in issueMap) refuses cleanly instead of panicking.
func TestOpenFieldPickerOrInput_IssueNotFoundInLoadedData(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	m.fieldEditTargetID = "zz-does-not-exist"
	m2, cmd := m.openFieldPickerOrInput("status")
	m = m2
	if cmd != nil {
		t.Error("not-found guard must not dispatch a cmd")
	}
	if m.activeModal != ModalNone {
		t.Errorf("activeModal = %v, want ModalNone", m.activeModal)
	}
	if !strings.Contains(m.statusMsg, "issue not found in loaded data") {
		t.Errorf("failure toast = %q, want it to mention issue not found", m.statusMsg)
	}
}

// ---------------------------------------------------------------------------
// Full teatest keypaths — real Bubble Tea event loop, stubbed executor.
// ---------------------------------------------------------------------------

func TestFieldEditKeypath_StatusTeatest(t *testing.T) {
	invoked := make(chan []string, 1)
	orig := claimRunner
	claimRunner = func(ctx context.Context, dir string, args ...string) bdexec.Result {
		invoked <- append([]string{}, args...)
		return bdexec.Result{Args: append([]string{"bd"}, args...), ExitCode: 0}
	}
	t.Cleanup(func() { claimRunner = orig })

	// Single-issue fixture: avoids depending on the list's sort order to know
	// which row is selected when the harness opens.
	issues := []model.Issue{
		{ID: "zz-target", Title: "Original title", Status: model.StatusBlocked, Priority: 2},
	}
	m := NewModel(issues, nil, "", nil, bdroute.SingleProject(t.TempDir()))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 32))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("zz-target"))
	}, teatest.WithDuration(8*time.Second))

	// Open the field-select modal.
	tm.Send(tea.KeyPressMsg{Code: 'e', Text: "e"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Edit field"))
	}, teatest.WithDuration(5*time.Second))

	// Open the status picker; cursor starts on "Blocked" (index 2 of
	// open/in_progress/blocked/...).
	tm.Send(tea.KeyPressMsg{Code: 's', Text: "s"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Blocked"))
	}, teatest.WithDuration(5*time.Second))

	// Move up twice: blocked(2) -> in_progress(1) -> open(0). Commit.
	tm.Send(tea.KeyPressMsg{Code: 'k', Text: "k"})
	tm.Send(tea.KeyPressMsg{Code: 'k', Text: "k"})
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	select {
	case args := <-invoked:
		want := []string{"update", "zz-target", "--status", "open"}
		if !slices.Equal(args, want) {
			t.Fatalf("executor argv = %v, want %v", args, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("field-edit executor was not invoked after commit")
	}

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(15*time.Second))
}

// TestFieldEditKeypath_RefusalTeatest mirrors TestClaimKeypath_RefusalTeatest:
// an unmappable route table refuses before any bd invocation, real event loop.
func TestFieldEditKeypath_RefusalTeatest(t *testing.T) {
	invoked := make(chan []string, 1)
	orig := claimRunner
	claimRunner = func(ctx context.Context, dir string, args ...string) bdexec.Result {
		invoked <- append([]string{}, args...)
		return bdexec.Result{Args: append([]string{"bd"}, args...), ExitCode: 0}
	}
	t.Cleanup(func() { claimRunner = orig })

	issues := []model.Issue{
		{ID: "zz-target", Title: "Original title", Status: model.StatusOpen},
	}
	m := NewModel(issues, nil, "", nil, bdroute.FromWorkspace(nil))
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 32))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("zz-target"))
	}, teatest.WithDuration(8*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'e', Text: "e"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Edit field"))
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 't', Text: "t"})
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Edit Title"))
	}, teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	tm.Quit()
	final := tm.FinalModel(t, teatest.WithFinalTimeout(15*time.Second))
	fm, ok := final.(Model)
	if !ok {
		t.Fatalf("final model type = %T, want Model", final)
	}

	if _, pending := fm.pendingWrites["zz-target"]; pending {
		t.Error("a refused field edit must never enter the pending state")
	}
	if fm.statusSeverity != SeverityFailure {
		t.Errorf("statusSeverity = %v, want SeverityFailure", fm.statusSeverity)
	}

	select {
	case args := <-invoked:
		t.Fatalf("field-edit executor was invoked despite a pre-flight refusal; argv=%v", args)
	default:
		// Expected: zero invocations.
	}
}

// ---------------------------------------------------------------------------
// Key-migration regressions (bt-oiaj.5 wave): 'e' moved from EpicCard to
// field-edit; EpicCard moved to 'F'; board ToggleEmpty moved to 'z'; insights
// Explanations moved to 'X'. epic_card_test.go and coverage_extra_test.go
// cover the F/X moves directly; this file covers 'e' itself and the board
// 'z' move (no prior key-dispatch test existed for board ToggleEmpty).
// ---------------------------------------------------------------------------

func TestListKeys_FieldEdit_OpensOnE(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	m = m.handleListKeys(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if m.activeModal != ModalFieldSelect {
		t.Fatalf("e should open ModalFieldSelect, activeModal=%v", m.activeModal)
	}
}

func TestBoardKeys_ToggleEmpty_MigratedToZ(t *testing.T) {
	m := newSizedModel(t, fieldEditTestIssues(), 120, 32)
	m.mode = ViewBoard
	m.focused = focusBoard
	m.refreshBoardAndGraphForCurrentFilter()
	before := m.board.GetEmptyColumnVisibilityMode()
	m = m.handleBoardKeys(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if m.board.GetEmptyColumnVisibilityMode() == before {
		t.Error("z should have toggled empty-column mode")
	}
}
