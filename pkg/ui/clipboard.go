package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	clipboardHelperDeadline = 2 * time.Second
	clipboardPipeWaitDelay  = 100 * time.Millisecond
	maxOSC52Payload         = 74994
)

type clipboardResultMsg struct {
	requestID    uint64
	label        string
	err          error
	sequence     string
	helperFailed bool
}

type clipboardRequest struct {
	id  uint64
	cmd tea.Cmd
}

type clipboardTerminalOutputMsg struct {
	requestID uint64
	sequence  string
}

type clipboardController struct {
	mu             sync.Mutex
	writeMu        sync.Mutex
	nextID         uint64
	activeID       uint64
	cancel         context.CancelFunc
	runHelper      func(context.Context, string) error
	helperTimeout  time.Duration
	terminalOutput bool
}

func newClipboardController() *clipboardController {
	return &clipboardController{runHelper: runClipboardHelper, helperTimeout: clipboardHelperDeadline}
}

func (c *clipboardController) start(text, label string) clipboardRequest {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.nextID++
	id := c.nextID
	ctx, cancel := context.WithCancel(context.Background())
	c.activeID, c.cancel = id, cancel
	c.mu.Unlock()

	return clipboardRequest{id: id, cmd: func() tea.Msg {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		defer c.finish(id, cancel)

		if !c.isCurrent(id) {
			return clipboardResultMsg{requestID: id, label: label, err: context.Canceled, helperFailed: true}
		}
		timeout := c.helperTimeout
		if timeout <= 0 {
			timeout = clipboardHelperDeadline
		}
		helperCtx, cancelHelper := context.WithTimeout(ctx, timeout)
		err := c.runHelper(helperCtx, text)
		cancelHelper()
		if err == nil {
			return clipboardResultMsg{requestID: id, label: label}
		}
		if ctx.Err() != nil {
			return clipboardResultMsg{requestID: id, label: label, err: context.Canceled, helperFailed: true}
		}
		wrapped := fmt.Errorf("copy to system clipboard: %w", err)
		result := clipboardResultMsg{requestID: id, label: label, err: wrapped, helperFailed: true}
		if c.isCurrent(id) && c.hasTerminalOutput() {
			seq, seqErr := makeOSC52Sequence(text)
			if seqErr != nil {
				result.err = fmt.Errorf("clipboard helper failed (%v); terminal fallback: %w", wrapped, seqErr)
			} else {
				result.sequence = seq
			}
		}
		return result
	}}
}

func (c *clipboardController) finish(id uint64, cancel context.CancelFunc) {
	c.mu.Lock()
	if c.activeID == id {
		c.cancel = nil
	}
	c.mu.Unlock()
	cancel()
}

func (c *clipboardController) isCurrent(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeID == id
}

func (c *clipboardController) hasTerminalOutput() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.terminalOutput
}

func (c *clipboardController) cancelPending() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.activeID++
	c.mu.Unlock()
}

func (m *Model) requestClipboard(text, label string) {
	if m.clipboard == nil {
		m.clipboard = newClipboardController()
	}
	request := m.clipboard.start(text, label)
	m.clipboardCmd = request.cmd
	m.clipboardRequestID = request.id
}

// SetClipboardTerminalOutput enables OSC 52 only when Bubble Tea owns a real
// interactive terminal output; it must not be inferred from environment flags.
func (m *Model) SetClipboardTerminalOutput(enabled bool) {
	if m.clipboard == nil {
		m.clipboard = newClipboardController()
	}
	m.clipboard.mu.Lock()
	m.clipboard.terminalOutput = enabled
	m.clipboard.mu.Unlock()
}

func (m *Model) takeClipboardCmd() tea.Cmd {
	cmd := m.clipboardCmd
	m.clipboardCmd = nil
	return cmd
}

func (m *Model) handleClipboardResult(msg clipboardResultMsg) (Model, tea.Cmd) {
	if m.clipboard == nil || !m.clipboard.isCurrent(msg.requestID) {
		return *m, nil
	}
	if msg.sequence != "" && m.clipboard.hasTerminalOutput() {
		m.setStatus(msg.label + " (sent to terminal; clipboard acceptance is unconfirmed)")
		return *m, func() tea.Msg {
			return clipboardTerminalOutputMsg{requestID: msg.requestID, sequence: msg.sequence}
		}
	}
	if msg.err != nil {
		m.setNotice(fmt.Sprintf("%s Clipboard error: %v", activeGlyphs.Cross, msg.err))
		return *m, nil
	}
	m.setStatus(msg.label)
	return *m, nil
}

// FilterClipboardOutput validates terminal output on Bubble Tea's event loop,
// immediately before RawMsg handling writes it to the owned terminal.
func FilterClipboardOutput(model tea.Model, msg tea.Msg) tea.Msg {
	output, ok := msg.(clipboardTerminalOutputMsg)
	if !ok {
		return msg
	}
	m, ok := model.(Model)
	if !ok || m.clipboard == nil || !m.clipboard.hasTerminalOutput() || !m.clipboard.isCurrent(output.requestID) {
		return nil
	}
	return tea.RawMsg{Msg: output.sequence}
}

func makeOSC52Sequence(text string) (string, error) {
	encodedLen := base64.StdEncoding.EncodedLen(len(text))
	if encodedLen > maxOSC52Payload {
		return "", fmt.Errorf("clipboard text is too large for terminal fallback (%d encoded bytes; limit %d); use a local clipboard helper", encodedLen, maxOSC52Payload)
	}
	return osc52Sequence(text, os.Getenv), nil
}

func osc52Sequence(text string, getenv func(string) string) string {
	payload := base64.StdEncoding.EncodeToString([]byte(text))
	raw := "\x1b]52;c;" + payload + "\x07"
	if getenv("TMUX") != "" {
		return raw
	}
	if getenv("STY") != "" {
		return "\x1bP" + raw + "\x1b\\"
	}
	return raw
}

func runClipboardHelper(ctx context.Context, text string) error {
	name, args, err := clipboardHelperCommand(runtime.GOOS, os.Getenv, exec.LookPath)
	if err != nil {
		return err
	}
	// Leave WaitDelay inside the caller's two-second budget. WaitDelay closes
	// os/exec's stdin-copy pipe if a helper descendant inherits it and never
	// drains it after the direct child exits or is killed.
	if deadline, ok := ctx.Deadline(); ok {
		cmdDeadline := deadline.Add(-clipboardPipeWaitDelay)
		if time.Until(cmdDeadline) < 0 {
			cmdDeadline = time.Now()
		}
		boundedCtx, cancel := context.WithDeadline(ctx, cmdDeadline)
		defer cancel()
		ctx = boundedCtx
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = clipboardPipeWaitDelay
	cmd.Stdin = strings.NewReader(text)
	// Nil stdio makes os/exec connect directly to the null device instead of
	// creating parent-side copy pipes descendants could keep open indefinitely.
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("clipboard helper timed out or was cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

func clipboardHelperCommand(goos string, getenv func(string) string, lookPath func(string) (string, error)) (string, []string, error) {
	switch goos {
	case "darwin":
		return "pbcopy", nil, nil
	case "windows":
		return "clip", nil, nil
	case "linux":
		if getenv("WAYLAND_DISPLAY") != "" {
			if _, err := lookPath("wl-copy"); err == nil {
				return "wl-copy", nil, nil
			}
		}
		if getenv("DISPLAY") != "" {
			if _, err := lookPath("xclip"); err == nil {
				return "xclip", []string{"-selection", "clipboard"}, nil
			}
			if _, err := lookPath("xsel"); err == nil {
				return "xsel", []string{"--clipboard", "--input"}, nil
			}
		}
		return "", nil, errors.New("no clipboard utility found for the active display")
	default:
		return "", nil, fmt.Errorf("unsupported platform: %s", goos)
	}
}
