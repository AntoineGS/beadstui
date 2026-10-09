package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdateModal_AllStatePopupsBounded(t *testing.T) {
	for _, tc := range []struct {
		state UpdateState
		label string
	}{{UpdateStateConfirm, "Update Available"}, {UpdateStateDownloading, "Downloading"}, {UpdateStateVerifying, "Verifying"}, {UpdateStateInstalling, "Installing"}, {UpdateStateSuccess, "Update Complete!"}, {UpdateStateError, "Update Failed"}} {
		t.Run(tc.label, func(t *testing.T) {
			m := NewUpdateModal("v1.0.0", "", DefaultTheme())
			m.SetSize(42, 18)
			m.state = tc.state
			m.errorMessage = "failed"
			m.successMessage = "updated"
			m.startTime = time.Now()
			out := m.View()
			assertPopupBounds(t, out, 42, 18)
			popupFindRow(t, out, tc.label)
			// Every state closes or confirms with the shared keys, so none
			// spells them out, and the confirm state has no buttons.
			plain := ansi.Strip(out)
			for _, hint := range []string{"Enter", "Esc", "Cancel", ">"} {
				if strings.Contains(plain, hint) {
					t.Errorf("%s popup shows %q:\n%s", tc.label, hint, plain)
				}
			}
		})
	}
}

func TestUpdatePopup_ShortConfirmationKeepsQuestion(t *testing.T) {
	for _, height := range []int{11, 7, 5, 4} {
		m := NewUpdateModal("v1.0.0", "", DefaultTheme())
		m.SetSize(42, height)
		out := m.View()
		assertPopupBounds(t, out, 42, height)
		if strings.Contains(out, "Terminal too small") {
			continue
		}
		popupFindRow(t, out, "Current version")
		popupFindRow(t, out, "New version")
		popupFindRow(t, out, "update now?")
	}
}

// ============================================================================
// NewUpdateModal tests
// ============================================================================

func TestNewUpdateModal_InitialState(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "https://github.com/example/release", theme)

	if m.state != UpdateStateConfirm {
		t.Errorf("expected initial state UpdateStateConfirm, got %v", m.state)
	}
	if m.newVersion != "v1.0.0" {
		t.Errorf("expected newVersion v1.0.0, got %s", m.newVersion)
	}
	if m.Dismissed() {
		t.Error("expected a new modal not to be dismissed")
	}
	if !m.IsConfirming() {
		t.Error("expected IsConfirming() to return true")
	}
	if m.IsComplete() {
		t.Error("expected IsComplete() to return false")
	}
	if m.IsInProgress() {
		t.Error("expected IsInProgress() to return false")
	}
}

// ============================================================================
// State helper tests
// ============================================================================

func TestUpdateModal_IsConfirming(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	if !m.IsConfirming() {
		t.Error("expected IsConfirming() true in confirm state")
	}

	m.state = UpdateStateDownloading
	if m.IsConfirming() {
		t.Error("expected IsConfirming() false in downloading state")
	}
}

func TestUpdateModal_EscDismissesConfirm(t *testing.T) {
	m := NewUpdateModal("v1.0.0", "", DefaultTheme())
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !updated.Dismissed() || updated.state != UpdateStateConfirm || cmd != nil {
		t.Errorf("esc: dismissed=%v state=%v cmd=%v, want dismissed confirm with no cmd", updated.Dismissed(), updated.state, cmd != nil)
	}
}

func TestUpdateModal_IsComplete(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	if m.IsComplete() {
		t.Error("expected IsComplete() false in confirm state")
	}

	m.state = UpdateStateSuccess
	if !m.IsComplete() {
		t.Error("expected IsComplete() true in success state")
	}

	m.state = UpdateStateError
	if !m.IsComplete() {
		t.Error("expected IsComplete() true in error state")
	}
}

func TestUpdateModal_IsInProgress(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	if m.IsInProgress() {
		t.Error("expected IsInProgress() false in confirm state")
	}

	for _, state := range []UpdateState{UpdateStateDownloading, UpdateStateVerifying, UpdateStateInstalling} {
		m.state = state
		if !m.IsInProgress() {
			t.Errorf("expected IsInProgress() true for state %v", state)
		}
	}
}

// ============================================================================
// Update (key handling) tests
// ============================================================================

// There are no buttons to move between; arrows and tab do nothing.
func TestUpdateModal_Update_ArrowsAndTabIgnored(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: tea.KeyRight}, {Code: tea.KeyTab}, {Code: 'h', Text: "h"}, {Code: 'l', Text: "l"}} {
		m := NewUpdateModal("v1.0.0", "", DefaultTheme())
		updated, cmd := m.Update(msg)
		if updated.state != UpdateStateConfirm || updated.Dismissed() || cmd != nil {
			t.Errorf("%q changed the confirm dialog", msg.String())
		}
	}
}

func TestUpdateModal_Update_QuickConfirmY(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	msg := tea.KeyPressMsg{Code: 'y', Text: "y"}
	updated, cmd := m.Update(msg)

	if updated.state != UpdateStateDownloading {
		t.Errorf("expected state Downloading after Y, got %v", updated.state)
	}
	if cmd == nil {
		t.Error("expected command to be returned for update")
	}
	if updated.startTime.IsZero() {
		t.Error("expected startTime to be set")
	}
}

func TestUpdateModal_Update_QuickConfirmUpperY(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	msg := tea.KeyPressMsg{Code: 'Y', Text: "Y"}
	updated, cmd := m.Update(msg)

	if updated.state != UpdateStateDownloading {
		t.Errorf("expected state Downloading after Y, got %v", updated.state)
	}
	if cmd == nil {
		t.Error("expected command to be returned")
	}
}

func TestUpdateModal_Update_QuickCancelN(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	updated, cmd := m.Update(msg)

	// State should remain confirm, cmd should be nil (parent closes it)
	if updated.state != UpdateStateConfirm || !updated.Dismissed() {
		t.Errorf("expected dismissed Confirm, got %v dismissed=%v", updated.state, updated.Dismissed())
	}
	if cmd != nil {
		t.Error("expected nil command for cancel")
	}
}

func TestUpdateModal_Update_EnterConfirmsUpdate(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, cmd := m.Update(msg)

	if updated.state != UpdateStateDownloading {
		t.Errorf("expected state Downloading, got %v", updated.state)
	}
	if cmd == nil {
		t.Error("expected command to be returned")
	}
}

func TestUpdateModal_Update_IgnoresKeysWhenInProgress(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.state = UpdateStateDownloading

	// Try to cancel - should be ignored
	msg := tea.KeyPressMsg{Code: 'n', Text: "n"}
	updated, _ := m.Update(msg)

	if updated.state != UpdateStateDownloading || updated.Dismissed() {
		t.Errorf("expected undismissed Downloading, got %v dismissed=%v", updated.state, updated.Dismissed())
	}
}

func TestUpdateModal_Update_DismissOnComplete(t *testing.T) {
	theme := DefaultTheme()

	for _, state := range []UpdateState{UpdateStateSuccess, UpdateStateError} {
		t.Run(state.String(), func(t *testing.T) {
			m := NewUpdateModal("v1.0.0", "", theme)
			m.state = state

			msg := tea.KeyPressMsg{Code: tea.KeyEnter}
			updated, cmd := m.Update(msg)

			// State remains; the parent closes a dismissed modal
			if updated.state != state || !updated.Dismissed() {
				t.Errorf("expected dismissed %v, got %v dismissed=%v", state, updated.state, updated.Dismissed())
			}
			if cmd != nil {
				t.Error("expected nil command")
			}
		})
	}
}

// ============================================================================
// Message handling tests
// ============================================================================

func TestUpdateModal_Update_ProgressMessage(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.state = UpdateStateDownloading

	msg := UpdateProgressMsg{
		Progress: UpdateProgress{
			BytesDownloaded: 500,
			TotalBytes:      1000,
			Stage:           "downloading",
		},
	}

	updated, _ := m.Update(msg)

	if updated.progress.BytesDownloaded != 500 {
		t.Errorf("expected BytesDownloaded 500, got %d", updated.progress.BytesDownloaded)
	}
	if updated.progress.TotalBytes != 1000 {
		t.Errorf("expected TotalBytes 1000, got %d", updated.progress.TotalBytes)
	}
}

func TestUpdateModal_Update_ProgressStageTransitions(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		stage    string
		expected UpdateState
	}{
		{"downloading", UpdateStateDownloading},
		{"verifying", UpdateStateVerifying},
		{"installing", UpdateStateInstalling},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			m := NewUpdateModal("v1.0.0", "", theme)
			m.state = UpdateStateDownloading

			msg := UpdateProgressMsg{
				Progress: UpdateProgress{Stage: tt.stage},
			}

			updated, _ := m.Update(msg)
			if updated.state != tt.expected {
				t.Errorf("expected state %v, got %v", tt.expected, updated.state)
			}
		})
	}
}

func TestUpdateModal_Update_CompleteMessageSuccess(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.state = UpdateStateDownloading

	msg := UpdateCompleteMsg{
		Success:    true,
		Message:    "Updated successfully",
		NewVersion: "v1.0.0",
		BackupPath: "/tmp/bv.backup",
	}

	updated, _ := m.Update(msg)

	if updated.state != UpdateStateSuccess {
		t.Errorf("expected state Success, got %v", updated.state)
	}
	if updated.successMessage != "Updated successfully" {
		t.Errorf("expected successMessage, got %s", updated.successMessage)
	}
	if updated.backupPath != "/tmp/bv.backup" {
		t.Errorf("expected backupPath, got %s", updated.backupPath)
	}
}

func TestUpdateModal_Update_CompleteMessageError(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.state = UpdateStateDownloading

	msg := UpdateCompleteMsg{
		Success: false,
		Message: "Download failed",
	}

	updated, _ := m.Update(msg)

	if updated.state != UpdateStateError {
		t.Errorf("expected state Error, got %v", updated.state)
	}
	if updated.errorMessage != "Download failed" {
		t.Errorf("expected errorMessage, got %s", updated.errorMessage)
	}
}

// ============================================================================
// View rendering tests
// ============================================================================

func TestUpdateModal_View_ConfirmState(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v2.0.0", "", theme)
	m.SetSize(80, 24)

	view := m.View()

	if !strings.Contains(view, "Update Available") {
		t.Error("expected 'Update Available' header in view")
	}
	if !strings.Contains(view, "v2.0.0") {
		t.Error("expected new version in view")
	}
	if !strings.Contains(view, "update now?") {
		t.Error("expected the confirm question in view")
	}
}

func TestUpdateModal_View_DownloadingState(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v2.0.0", "", theme)
	m.state = UpdateStateDownloading
	m.startTime = time.Now()
	m.SetSize(80, 24)

	view := m.View()

	if !strings.Contains(view, "Updating") {
		t.Error("expected 'Updating' in view")
	}
	if !strings.Contains(view, "Downloading") {
		t.Error("expected 'Downloading' in view")
	}
}

func TestUpdateModal_View_SuccessState(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v2.0.0", "", theme)
	m.state = UpdateStateSuccess
	m.successMessage = "Update complete"
	m.backupPath = "/tmp/backup"
	m.SetSize(80, 24)

	view := m.View()

	if !strings.Contains(view, "Update Complete") {
		t.Error("expected 'Update Complete' in view")
	}
	if !strings.Contains(view, "Update complete") {
		t.Error("expected success message in view")
	}
	if !strings.Contains(view, "/tmp/backup") {
		t.Error("expected backup path in view")
	}
	if !strings.Contains(view, "rollback") {
		t.Error("expected rollback hint in view")
	}
}

func TestUpdateModal_View_ErrorState(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v2.0.0", "", theme)
	m.state = UpdateStateError
	m.errorMessage = "Network error"
	m.SetSize(80, 24)

	view := m.View()

	if !strings.Contains(view, "Update Failed") {
		t.Error("expected 'Update Failed' in view")
	}
	if !strings.Contains(view, "Network error") {
		t.Error("expected error message in view")
	}
}

// ============================================================================
// SetSize tests
// ============================================================================

func TestUpdateModal_SetSize(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)

	m.SetSize(100, 40)
	if m.width != 70 { // Capped at 70
		t.Errorf("expected width 70 (max), got %d", m.width)
	}

	m.SetSize(40, 20)
	if m.width != 40 {
		t.Errorf("expected actual narrow budget 40, got %d", m.width)
	}

	m.SetSize(65, 30)
	if m.width != 65 {
		t.Errorf("expected actual budget 65, got %d", m.width)
	}
}

// ============================================================================
// Spinner and progress bar tests
// ============================================================================

func TestUpdateModal_RenderSpinner(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.startTime = time.Now()

	spinner := m.renderSpinner()

	// Should return a non-empty spinner character
	if spinner == "" {
		t.Error("expected non-empty spinner")
	}
}

func TestUpdateModal_RenderProgressBar_Indeterminate(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.progress.TotalBytes = 0

	bar := m.renderProgressBar()

	if !strings.Contains(bar, "[") || !strings.Contains(bar, "]") {
		t.Errorf("expected brackets in progress bar, got %s", bar)
	}
}

func TestUpdateModal_RenderProgressBar_WithProgress(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.progress.BytesDownloaded = 500
	m.progress.TotalBytes = 1000

	bar := m.renderProgressBar()

	if !strings.Contains(bar, "50%") {
		t.Errorf("expected 50%% in progress bar, got %s", bar)
	}
	if !strings.Contains(bar, "█") {
		t.Errorf("expected filled blocks in progress bar, got %s", bar)
	}
}

func TestUpdateModal_RenderProgressBar_Complete(t *testing.T) {
	theme := DefaultTheme()
	m := NewUpdateModal("v1.0.0", "", theme)
	m.progress.BytesDownloaded = 1000
	m.progress.TotalBytes = 1000

	bar := m.renderProgressBar()

	if !strings.Contains(bar, "100%") {
		t.Errorf("expected 100%% in progress bar, got %s", bar)
	}
}

// ============================================================================
// UpdateState String helper (for test output)
// ============================================================================

func (s UpdateState) String() string {
	switch s {
	case UpdateStateIdle:
		return "Idle"
	case UpdateStateConfirm:
		return "Confirm"
	case UpdateStateDownloading:
		return "Downloading"
	case UpdateStateVerifying:
		return "Verifying"
	case UpdateStateInstalling:
		return "Installing"
	case UpdateStateSuccess:
		return "Success"
	case UpdateStateError:
		return "Error"
	default:
		return "Unknown"
	}
}
