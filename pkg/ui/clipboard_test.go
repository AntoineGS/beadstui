package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/seanmartinsmith/beadstui/pkg/cass"
)

func TestClipboardSequenceTerminalMultiplexers(t *testing.T) {
	const text = "clipboard"
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	raw := "\x1b]52;c;" + encoded + "\x07"
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "raw", want: raw},
		{name: "tmux", env: map[string]string{"TMUX": "/tmp/tmux"}, want: "\x1bPtmux;\x1b" + strings.TrimSuffix(raw, "\x07") + "\x1b\\"},
		{name: "screen", env: map[string]string{"STY": "123.pts-1"}, want: "\x1bP" + raw + "\x1b\\"},
		{name: "tmux wins over screen", env: map[string]string{"TMUX": "/tmp/tmux", "STY": "123.pts-1"}, want: "\x1bPtmux;\x1b" + strings.TrimSuffix(raw, "\x07") + "\x1b\\"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := osc52Sequence(text, func(k string) string { return tt.env[k] }); got != tt.want {
				t.Fatalf("sequence = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClipboardSequenceRejectsOversizePayload(t *testing.T) {
	if _, err := makeOSC52Sequence(strings.Repeat("x", 56244)); err != nil {
		t.Fatalf("max payload rejected: %v", err)
	}
	if _, err := makeOSC52Sequence(strings.Repeat("x", 56245)); err == nil {
		t.Fatal("oversize payload unexpectedly accepted")
	}
}

func TestClipboardHelperDeadlineAndError(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		entered := make(chan struct{})
		c := newClipboardController()
		c.helperTimeout = 20 * time.Millisecond
		c.runHelper = func(ctx context.Context, _ string) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		}
		result := c.start("data", "Copied").cmd()
		<-entered
		msg := result.(clipboardResultMsg)
		if msg.err == nil || !msg.helperFailed {
			t.Fatalf("timeout result = %#v, want helper failure", msg)
		}
	})
	for _, cause := range []error{errors.New("helper failed")} {
		t.Run("failure", func(t *testing.T) {
			c := newClipboardController()
			c.runHelper = func(context.Context, string) error { return cause }
			msg := c.start("data", "Copied").cmd().(clipboardResultMsg)
			if msg.err == nil || !errors.Is(msg.err, cause) {
				t.Fatalf("error = %v, want wrapped helper failure", msg.err)
			}
		})
	}
}

func TestClipboardHelperTimeoutBoundsUnconsumedInput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses a Linux clipboard-helper executable")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "wl-copy")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := runClipboardHelper(ctx, strings.Repeat("x", 16<<20))
	if err == nil {
		t.Fatal("non-reading helper unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("helper with unconsumed stdin took %s after cancellation", elapsed)
	}
}

func TestClipboardHelperDisplayDiscovery(t *testing.T) {
	lookups := 0
	lookup := func(string) (string, error) { lookups++; return "", errors.New("not installed") }
	if _, _, err := clipboardHelperCommand("linux", func(string) string { return "" }, lookup); err == nil {
		t.Fatal("display-less Linux unexpectedly found a clipboard helper")
	}
	if lookups != 0 {
		t.Fatalf("probed %d helpers without a display", lookups)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"Wayland", map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, "wl-copy"},
		{"X11 fallback", map[string]string{"DISPLAY": ":0"}, "xsel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, args, err := clipboardHelperCommand("linux", func(k string) string { return tc.env[k] }, func(k string) (string, error) {
				if tc.want == "xsel" && k == "xclip" {
					return "", errors.New("not found")
				}
				return "/bin/" + k, nil
			})
			if err != nil || name != tc.want {
				t.Fatalf("helper = %s %v, %v; want %s", name, args, err, tc.want)
			}
		})
	}
}

func TestClipboardHelperSerialAndStaleRequests(t *testing.T) {
	enteredA := make(chan struct{})
	releaseA := make(chan struct{})
	enteredB := make(chan struct{})
	c := newClipboardController()
	c.runHelper = func(ctx context.Context, text string) error {
		if text == "A" {
			close(enteredA)
			select {
			case <-releaseA:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		close(enteredB)
		return nil
	}
	a := c.start("A", "A")
	resultA := make(chan clipboardResultMsg, 1)
	go func() { resultA <- a.cmd().(clipboardResultMsg) }()
	<-enteredA
	b := c.start("B", "B")
	msgB := b.cmd().(clipboardResultMsg)
	if msgB.requestID != b.id || msgB.err != nil {
		t.Fatalf("new result = %#v", msgB)
	}
	close(releaseA)
	msgA := <-resultA
	if msgA.requestID == b.id {
		t.Fatal("obsolete request reused latest identity")
	}
	select {
	case <-enteredB:
	default:
		t.Fatal("new helper did not run")
	}
}

func TestClipboardUpdateRejectsOlderCompletion(t *testing.T) {
	m := layoutModel(t)
	m.clipboard = newClipboardController()
	enteredA := make(chan struct{})
	m.clipboard.runHelper = func(ctx context.Context, text string) error {
		if text == "bt-2" {
			close(enteredA)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	m.selectIssueByID("bt-2")
	updated, cmdA := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	resultA := make(chan clipboardResultMsg, 1)
	go func() { resultA <- cmdA().(clipboardResultMsg) }()
	<-enteredA
	m.selectIssueByID("bt-1")
	updated, cmdB := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	resultB := cmdB().(clipboardResultMsg)
	updated, _ = m.Update(resultB)
	m = updated.(Model)
	want := "Copied bt-1 to clipboard"
	if m.statusMsg != want {
		t.Fatalf("new completion status = %q, want %q", m.statusMsg, want)
	}
	updated, _ = m.Update(<-resultA)
	m = updated.(Model)
	if m.statusMsg != want {
		t.Fatalf("stale completion replaced status with %q", m.statusMsg)
	}
}

func TestClipboardUpdateGatesTerminalFallback(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("terminal=%t", terminal), func(t *testing.T) {
			m := layoutModel(t)
			m.clipboard = newClipboardController()
			m.clipboard.runHelper = func(context.Context, string) error { return errors.New("helper missing") }
			m.SetClipboardTerminalOutput(terminal)
			m.requestClipboard("payload", "Copied payload")
			msg := m.takeClipboardCmd()().(clipboardResultMsg)
			updated, output := m.Update(msg)
			m = updated.(Model)
			if !terminal && output != nil {
				t.Fatal("redirected output received a terminal command")
			}
			if terminal {
				if output == nil {
					t.Fatal("interactive output did not emit OSC 52 command")
				}
				raw, ok := output().(tea.RawMsg)
				if !ok || !strings.Contains(fmt.Sprint(raw.Msg), "\x1b]52;") {
					t.Fatalf("terminal output = %#v, want raw OSC 52", raw)
				}
			} else if msg.sequence != "" {
				t.Fatal("disabled terminal output received a fallback sequence")
			}
			if m.statusMsg == "" {
				t.Fatal("copy failure did not reach model status")
			}
			if !terminal {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				old := os.Stdout
				os.Stdout = writer
				_, _ = m.Update(clipboardResultMsg{requestID: msg.requestID, err: errors.New("retry")})
				_ = writer.Close()
				os.Stdout = old
				captured, err := io.ReadAll(reader)
				_ = reader.Close()
				if err != nil || strings.Contains(string(captured), "\x1b]52;") {
					t.Fatalf("redirected stdout contains clipboard escape sequence: %q (%v)", captured, err)
				}
			}
		})
	}
}

func TestCassCopyUsesParentClipboardLifecycle(t *testing.T) {
	m := layoutModel(t)
	m.activeModal = ModalCassSession
	m.cassModal = NewCassSessionModal("bt-cass", cass.CorrelationResult{Keywords: []string{"one", "two"}}, testTheme())
	m.clipboard = newClipboardController()
	var captured string
	m.clipboard.runHelper = func(_ context.Context, text string) error { captured = text; return nil }
	updated, requestCmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)
	if requestCmd == nil {
		t.Fatal("Cass modal copy did not produce parent request")
	}
	request, ok := requestCmd().(cassClipboardRequestMsg)
	if !ok || request.text != `cass search "one two"` {
		t.Fatalf("Cass request = %#v", request)
	}
	updated, helperCmd := m.Update(request)
	m = updated.(Model)
	if helperCmd == nil {
		t.Fatal("parent did not dispatch shared clipboard helper")
	}
	result := helperCmd().(clipboardResultMsg)
	updated, _ = m.Update(result)
	m = updated.(Model)
	if captured != request.text {
		t.Fatalf("copied %q, want captured Cass command %q", captured, request.text)
	}
	if !strings.Contains(m.statusMsg, "Copied Cass search command") {
		t.Fatalf("Cass copy feedback = %q", m.statusMsg)
	}
}

func TestClipboardStopCancelsAndInvalidatesCompletion(t *testing.T) {
	m := layoutModel(t)
	m.clipboard = newClipboardController()
	entered := make(chan struct{})
	m.clipboard.runHelper = func(ctx context.Context, _ string) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	m.requestClipboard("pending", "Copied pending")
	cmd := m.takeClipboardCmd()
	result := make(chan clipboardResultMsg, 1)
	go func() { result <- cmd().(clipboardResultMsg) }()
	<-entered
	m.Stop()
	updated, _ := m.Update(<-result)
	m = updated.(Model)
	if m.statusMsg == "Copied pending" {
		t.Fatal("completion after shutdown updated clipboard feedback")
	}
}
