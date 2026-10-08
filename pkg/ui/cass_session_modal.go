package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/cass"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// CassSessionModal displays correlated cass sessions for a bead.
// It shows session previews with agent name, timestamp, match reason, and snippet.
type CassSessionModal struct {
	beadID     string              // The bead this modal is showing sessions for
	sessions   []cass.ScoredResult // Correlated sessions to display
	strategy   cass.CorrelationStrategy
	keywords   []string // Keywords used for correlation (for display)
	selected   int      // Currently selected session (for keyboard nav)
	searchCmd  string   // Command to run for more results
	theme      Theme
	width      int
	height     int
	copied     bool      // Flash feedback for clipboard copy
	copiedAt   time.Time // When copy happened
	maxDisplay int       // Max sessions to show (rest are summarized)
	popupSize  *PopupSize
}

// NewCassSessionModal creates a modal from correlation results.
func NewCassSessionModal(beadID string, result cass.CorrelationResult, theme Theme) CassSessionModal {
	searchCmd := fmt.Sprintf("cass search %q", beadID)
	if len(result.Keywords) > 0 {
		searchCmd = fmt.Sprintf("cass search %q", strings.Join(result.Keywords, " "))
	}

	return CassSessionModal{
		beadID:     beadID,
		sessions:   result.TopSessions,
		strategy:   result.Strategy,
		keywords:   result.Keywords,
		selected:   0,
		searchCmd:  searchCmd,
		theme:      theme,
		width:      70,
		height:     25,
		maxDisplay: 3,
	}
}

// Update handles input for the modal.
func (m CassSessionModal) Update(msg tea.Msg) (CassSessionModal, tea.Cmd) {
	// Calculate the number of sessions actually displayed (capped by maxDisplay)
	displayCount := len(m.sessions)
	if displayCount > m.maxDisplay {
		displayCount = m.maxDisplay
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "j", "down":
			if displayCount > 1 && m.selected < displayCount-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "y":
			// Copy search command to clipboard
			if err := copyToClipboard(m.searchCmd); err == nil {
				m.copied = true
				m.copiedAt = time.Now()
			}
		}
	}
	return m, nil
}

// View renders the modal.
func (m CassSessionModal) View() string {
	showCopied := m.copied && time.Since(m.copiedAt) <= 2*time.Second
	footer := []string{"[j/k] Navigate    [y] Copy search cmd    [V/Esc] Close", "[j/k] [y] copy [V/Esc] close"}
	if showCopied {
		footer = []string{"[j/k] Navigate    " + activeGlyphs.Success + " Copied!    [V/Esc] Close", "[j/k] Copied! [V/Esc] close"}
	}
	opts := PopupOpts{Title: "Related Coding Sessions", Theme: m.theme, Available: m.popupSize, Width: 80, Height: min(28, popupAvailableSize(m.popupSize).Height), MinBodyRows: 7, Footer: footer}
	l := MeasurePopup(nil, opts)
	if l.Compact || l.Height == 0 {
		return RenderPopup(nil, opts)
	}
	dim := lipgloss.NewStyle().Foreground(m.theme.Subtext).Italic(true)
	lines := []string{dim.Render(m.beadID), ""}
	count := min(len(m.sessions), m.maxDisplay)
	if count == 0 {
		lines = append(lines, dim.Render("No correlated sessions found."))
	} else {
		extraRow := 0
		if len(m.sessions) > m.maxDisplay {
			extraRow = 1
		}
		budget := l.BodyHeight - 2 - extraRow
		visible := min(count, max(1, budget/5))
		start := min(max(0, m.selected-visible+1), count-visible)
		end := start + visible
		entries := make([]PopupMenuEntry, count)
		for i, session := range m.sessions[:count] {
			agent := session.Agent
			if agent == "" {
				agent = "Unknown"
			}
			entries[i] = PopupMenuEntry{Label: fmt.Sprintf("%s • %s", agent, formatRelativeTime(session.Timestamp)), Shortcut: fmt.Sprintf("[%d]", i+1), Selected: i == m.selected}
		}
		menu := MeasurePopupMenu(entries, PopupMenuOpts{Shortcuts: true})
		snippetRows := max(1, min(3, budget/visible-4))
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(m.theme.Border).Padding(0, 1).Width(max(1, l.BodyWidth-2))
		for i := start; i < end; i++ {
			lines = append(lines, RenderPopupMenu(entries[i:i+1], menu, m.theme, l.BodyWidth)...)
			lines = append(lines, dim.Render(m.formatMatchReason(m.sessions[i])))
			snippet := strings.Split(m.formatSnippet(m.sessions[i].Snippet), "\n")
			snippet = snippet[:min(len(snippet), snippetRows)]
			for j, row := range snippet {
				snippet[j] = ansi.Truncate(row, max(1, l.BodyWidth-4), "")
			}
			lines = append(lines, box.Render(strings.Join(snippet, "\n")))
		}
		if extraRow > 0 {
			lines = append(lines, dim.Render(fmt.Sprintf("(%d more sessions - run: %s)", len(m.sessions)-m.maxDisplay, m.searchCmd)))
		}
	}
	opts.MinBodyRows = l.BodyHeight
	return RenderPopup(lines, opts)
}

// formatMatchReason creates a human-readable match reason string.
func (m CassSessionModal) formatMatchReason(session cass.ScoredResult) string {
	switch session.Strategy {
	case cass.StrategyIDMention:
		return fmt.Sprintf("Matched via: bead ID mentioned (%s)", m.beadID)
	case cass.StrategyKeywords:
		if len(session.Keywords) > 0 {
			return fmt.Sprintf("Matched via: keywords %q", strings.Join(session.Keywords, ", "))
		}
		return "Matched via: keyword search"
	case cass.StrategyTimestamp:
		return "Matched via: recent activity timeframe"
	case cass.StrategyCombined:
		return "Matched via: multiple signals"
	default:
		return fmt.Sprintf("Matched via: %s", session.Strategy)
	}
}

// formatSnippet cleans and truncates a snippet for display.
func (m CassSessionModal) formatSnippet(snippet string) string {
	if snippet == "" {
		return "(no preview available)"
	}

	// Clean up the snippet
	lines := strings.Split(snippet, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Truncate long lines
		maxLineLen := max(1, m.width-14)
		line = truncateRunesHelper(line, maxLineLen, "...")
		cleaned = append(cleaned, line)
		if len(cleaned) >= 3 {
			break
		}
	}

	if len(cleaned) == 0 {
		return "(no preview available)"
	}
	return strings.Join(cleaned, "\n")
}

// formatRelativeTime formats a timestamp as a relative time string.
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown time"
	}

	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// SetSize sets the modal dimensions based on terminal size.
func (m *CassSessionModal) SetSize(width, height int) {
	m.popupSize = &PopupSize{width, height}
	m.width = min(80, max(0, width))
	m.height = height
}

// HasSessions returns true if there are sessions to display.
func (m CassSessionModal) HasSessions() bool {
	return len(m.sessions) > 0
}

// copyToClipboard copies text to the system clipboard.
// It uses platform-specific commands and fails silently if unavailable.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	if _, err := stdin.Write([]byte(text)); err != nil {
		return err
	}
	stdin.Close()

	return cmd.Wait()
}
