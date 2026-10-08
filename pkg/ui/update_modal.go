package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
	"github.com/seanmartinsmith/beadstui/pkg/updater"
	"github.com/seanmartinsmith/beadstui/pkg/version"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// UpdateState represents the current state of the update process
type UpdateState int

const (
	UpdateStateIdle UpdateState = iota
	UpdateStateConfirm
	UpdateStateDownloading
	UpdateStateVerifying
	UpdateStateInstalling
	UpdateStateSuccess
	UpdateStateError
)

// UpdateProgress represents progress during download
type UpdateProgress struct {
	BytesDownloaded int64
	TotalBytes      int64
	Stage           string
}

// UpdateProgressMsg is sent during the update process
type UpdateProgressMsg struct {
	Progress UpdateProgress
}

// UpdateCompleteMsg is sent when the update completes
type UpdateCompleteMsg struct {
	Success     bool
	Message     string
	NewVersion  string
	BackupPath  string
	RequireRoot bool
}

// UpdateModal displays the update confirmation and progress.
type UpdateModal struct {
	currentVersion string
	newVersion     string
	releaseURL     string
	state          UpdateState
	progress       UpdateProgress
	errorMessage   string
	successMessage string
	backupPath     string
	theme          Theme
	width          int
	height         int
	popupSize      *PopupSize
	startTime      time.Time
	keys           keys.ConfirmKeys
	dismissed      bool
}

// NewUpdateModal creates a new update modal.
func NewUpdateModal(newVersion, releaseURL string, theme Theme) UpdateModal {
	return UpdateModal{
		currentVersion: version.Version,
		newVersion:     newVersion,
		releaseURL:     releaseURL,
		state:          UpdateStateConfirm,
		theme:          theme,
		width:          60,
		height:         20,
		keys:           keys.NewConfirmKeys(),
	}
}

// PerformUpdateCmd returns a command that performs the update in the background.
func PerformUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		release, err := updater.GetLatestRelease()
		if err != nil {
			return UpdateCompleteMsg{
				Success: false,
				Message: fmt.Sprintf("Failed to fetch release info: %v", err),
			}
		}

		result, err := updater.PerformUpdate(release, true) // Skip confirm since TUI already confirmed
		if err != nil {
			msg := fmt.Sprintf("Update failed: %v", err)
			requireRoot := false
			if result != nil {
				if result.RequireRoot {
					requireRoot = true
					msg = "Update requires elevated permissions. Run: sudo bv --update"
				}
			}
			return UpdateCompleteMsg{
				Success:     false,
				Message:     msg,
				RequireRoot: requireRoot,
			}
		}

		return UpdateCompleteMsg{
			Success:    result.Success,
			Message:    result.Message,
			NewVersion: result.NewVersion,
			BackupPath: result.BackupPath,
		}
	}
}

// Update handles input for the modal.
func (m UpdateModal) Update(msg tea.Msg) (UpdateModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.state {
		case UpdateStateConfirm:
			switch {
			case key.Matches(msg, m.keys.Confirm):
				m.state = UpdateStateDownloading
				m.startTime = time.Now()
				return m, PerformUpdateCmd()
			case key.Matches(msg, m.keys.Cancel):
				m.dismissed = true
			}

		case UpdateStateSuccess, UpdateStateError:
			if key.Matches(msg, m.keys.Confirm, m.keys.Cancel) || msg.String() == "q" {
				m.dismissed = true
			}
		}

	case UpdateProgressMsg:
		m.progress = msg.Progress
		switch m.progress.Stage {
		case "downloading":
			m.state = UpdateStateDownloading
		case "verifying":
			m.state = UpdateStateVerifying
		case "installing":
			m.state = UpdateStateInstalling
		}

	case UpdateCompleteMsg:
		if msg.Success {
			m.state = UpdateStateSuccess
			m.successMessage = msg.Message
			m.backupPath = msg.BackupPath
		} else {
			m.state = UpdateStateError
			m.errorMessage = msg.Message
		}
	}

	return m, nil
}

// View renders the modal.
func (m UpdateModal) View() string {
	opts := PopupOpts{Theme: m.theme, Available: m.popupSize, Width: 70, Title: "Updating..."}
	switch m.state {
	case UpdateStateConfirm:
		opts.Title = "Update Available"
		opts.MinBodyRows = 3 // version pair and question
	case UpdateStateSuccess:
		opts.Title = "Update Complete!"
		opts.Accent = ColorStatusOpen
	case UpdateStateError:
		opts.Title = "Update Failed"
		opts.Accent = ColorStatusBlocked
	}

	// Version styles
	currentVersionStyle := m.theme.Text.Body

	newVersionStyle := m.theme.Text.Body

	successStyle := lipgloss.NewStyle().
		Foreground(ColorStatusOpen).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(ColorStatusBlocked).
		Bold(true)

	subtextStyle := m.theme.Text.Metadata

	var b strings.Builder

	switch m.state {
	case UpdateStateConfirm:
		b.WriteString(m.theme.Text.Metadata.Render("Current version: "))
		b.WriteString(currentVersionStyle.Render(m.currentVersion))
		b.WriteString("\n")

		b.WriteString(m.theme.Text.Metadata.Render("New version:     "))
		b.WriteString(newVersionStyle.Render(m.newVersion))
		b.WriteString("\n")

		b.WriteString("Would you like to update now?\n")

	case UpdateStateDownloading:
		b.WriteString(m.renderSpinner())
		b.WriteString(m.theme.Text.Body.Render(" Downloading "))
		b.WriteString(newVersionStyle.Render(m.newVersion))
		b.WriteString(m.theme.Text.Body.Render("...") + "\n\n")
		b.WriteString(m.renderProgressBar())
		b.WriteString("\n\n")
		elapsed := time.Since(m.startTime).Round(time.Second)
		b.WriteString(subtextStyle.Render(fmt.Sprintf("Elapsed: %s", elapsed)))

	case UpdateStateVerifying:
		b.WriteString(m.renderSpinner())
		b.WriteString(" Verifying checksum...\n")

	case UpdateStateInstalling:
		b.WriteString(m.renderSpinner())
		b.WriteString(" Installing new version...\n")

	case UpdateStateSuccess:
		b.WriteString(m.successMessage)
		b.WriteString("\n\n")
		if m.backupPath != "" {
			b.WriteString(subtextStyle.Render(fmt.Sprintf("Backup: %s", m.backupPath)))
			b.WriteString("\n")
			b.WriteString(subtextStyle.Render("Run 'bv --rollback' to restore if needed"))
			b.WriteString("\n\n")
		}
		b.WriteString(successStyle.Render("Restart bv to use the new version."))
		b.WriteString("\n\n")

	case UpdateStateError:
		b.WriteString(errorStyle.Render(m.errorMessage))
		b.WriteString("\n\n")
	}

	return RenderPopup(strings.Split(strings.TrimRight(b.String(), "\n"), "\n"), opts)
}

// renderSpinner returns an animated spinner character
func (m UpdateModal) renderSpinner() string {
	frames := activeGlyphs.Spinner
	idx := int(time.Since(m.startTime).Milliseconds()/100) % len(frames)
	return frames[idx]
}

// renderProgressBar renders a progress bar for downloads
func (m UpdateModal) renderProgressBar() string {
	if m.progress.TotalBytes == 0 {
		// Indeterminate progress
		return "[                    ]"
	}

	percent := float64(m.progress.BytesDownloaded) / float64(m.progress.TotalBytes)
	width := 20
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %.0f%%", bar, percent*100)
}

// SetSize sets the modal dimensions based on terminal size.
func (m *UpdateModal) SetSize(width, height int) {
	m.popupSize = &PopupSize{width, height}
	m.width = min(70, max(0, width))
	m.height = height
}

// IsConfirming returns true if the modal is in confirm state
func (m UpdateModal) IsConfirming() bool {
	return m.state == UpdateStateConfirm
}

// Dismissed reports that the user cancelled the confirm or closed a result.
func (m UpdateModal) Dismissed() bool {
	return m.dismissed
}

// IsComplete returns true if the update is done (success or error)
func (m UpdateModal) IsComplete() bool {
	return m.state == UpdateStateSuccess || m.state == UpdateStateError
}

// IsInProgress returns true if the update is in progress
func (m UpdateModal) IsInProgress() bool {
	return m.state == UpdateStateDownloading ||
		m.state == UpdateStateVerifying ||
		m.state == UpdateStateInstalling
}
