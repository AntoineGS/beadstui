package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/seanmartinsmith/beadstui/pkg/agents"
)

// AgentPromptResult represents the user's choice on the AGENTS.md prompt.
type AgentPromptResult int

const (
	AgentPromptPending AgentPromptResult = iota
	AgentPromptAccept
	AgentPromptDecline
	AgentPromptNeverAsk
)

// AgentPromptModal is a modal dialog for the AGENTS.md prompt.
type AgentPromptModal struct {
	selection int    // 0=yes, 1=no, 2=never
	filePath  string // Which file we're offering to modify
	fileType  string // AGENTS.md or CLAUDE.md
	result    AgentPromptResult
	theme     Theme
	width     int
	height    int
	popupSize *PopupSize
}

// NewAgentPromptModal creates a new AGENTS.md prompt modal.
func NewAgentPromptModal(filePath, fileType string, theme Theme) AgentPromptModal {
	return AgentPromptModal{
		selection: 0, // Default to "Yes"
		filePath:  filePath,
		fileType:  fileType,
		result:    AgentPromptPending,
		theme:     theme,
		width:     60,
		height:    20,
	}
}

// Update handles input for the modal.
func (m AgentPromptModal) Update(msg tea.Msg) (AgentPromptModal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "h", "shift+tab":
			m.selection--
			if m.selection < 0 {
				m.selection = 2
			}
		case "right", "l", "tab":
			m.selection++
			if m.selection > 2 {
				m.selection = 0
			}
		case "enter", "space":
			switch m.selection {
			case 0:
				m.result = AgentPromptAccept
			case 1:
				m.result = AgentPromptDecline
			case 2:
				m.result = AgentPromptNeverAsk
			}
		case "y", "Y":
			m.result = AgentPromptAccept
		case "n", "N":
			m.result = AgentPromptDecline
		case "d", "D":
			m.result = AgentPromptNeverAsk
		case "esc", "q":
			m.result = AgentPromptDecline
		}
	}
	return m, nil
}

// View renders the modal.
func (m AgentPromptModal) View() string {
	opts := PopupOpts{Title: "Enhance AI Agent Integration?", Theme: m.theme, Available: m.popupSize, Width: 70, Height: min(26, popupAvailableSize(m.popupSize).Height), Footer: []string{"← → to select • Enter to confirm • Esc to cancel", "←/→ select Enter confirm Esc cancel", "←/→ Enter Esc"}}
	l := MeasurePopup(nil, opts)
	if l.Compact || l.Height == 0 {
		return RenderPopup(nil, opts)
	}
	intro := strings.Split(ansi.Wrap("We found "+m.fileType+" in this project but it doesn't include beadstui instructions.\nAdding these helps AI coding agents understand how to use your issue tracking workflow.", l.BodyWidth, ""), "\n")
	opts.MinBodyRows = len(intro) + 7 // all actions plus preview heading and a three-row preview
	l = MeasurePopup(nil, opts)
	if l.Compact {
		return RenderPopup(nil, opts)
	}
	previewRows := min(8, l.BodyHeight-len(intro)-4)
	preview := strings.Split(ansi.Wrap(getBlurbPreview(), max(1, l.BodyWidth-4), ""), "\n")
	preview = preview[:min(len(preview), max(1, previewRows-2))]
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(m.theme.Border).Padding(0, 1).Width(max(1, l.BodyWidth-2))
	lines := append(intro, lipgloss.NewStyle().Foreground(m.theme.Subtext).Italic(true).Render("Preview of content to add:"), box.Render(strings.Join(preview, "\n")))
	entries := []PopupMenuEntry{{Label: "Yes, add it", Selected: m.selection == 0}, {Label: "No thanks", Selected: m.selection == 1}, {Label: "Don't ask again", Selected: m.selection == 2}}
	lines = append(lines, RenderPopupMenu(entries, MeasurePopupMenu(entries, PopupMenuOpts{}), m.theme, l.BodyWidth)...)
	opts.MinBodyRows = l.BodyHeight
	return RenderPopup(lines, opts)
}

// Result returns the user's choice, or AgentPromptPending if still deciding.
func (m AgentPromptModal) Result() AgentPromptResult {
	return m.result
}

// FilePath returns the path of the file to modify.
func (m AgentPromptModal) FilePath() string {
	return m.filePath
}

// SetSize sets the modal dimensions.
func (m *AgentPromptModal) SetSize(width, height int) {
	m.popupSize = &PopupSize{width, height}
	m.width = width
	m.height = height
}

// getBlurbPreview returns a truncated preview of the blurb content.
func getBlurbPreview() string {
	// Get first few lines of the blurb content (skip marker)
	lines := strings.Split(agents.AgentBlurb, "\n")

	var preview []string
	lineCount := 0
	for _, line := range lines {
		// Skip marker lines
		if strings.HasPrefix(line, "<!--") {
			continue
		}
		// Skip empty lines at start
		if lineCount == 0 && strings.TrimSpace(line) == "" {
			continue
		}
		// Skip horizontal rules
		if strings.TrimSpace(line) == "---" {
			continue
		}
		preview = append(preview, line)
		lineCount++
		if lineCount >= 6 {
			break
		}
	}

	return strings.Join(preview, "\n") + "\n..."
}
