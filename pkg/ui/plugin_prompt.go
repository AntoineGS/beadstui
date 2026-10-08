package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
)

// pluginPromptKind tells a plugin prompt's answer where to go: menu answers
// are handled by bt, confirm and select answers go back to the plugin.
type pluginPromptKind int

const (
	pluginPromptMenu pluginPromptKind = iota
	pluginPromptConfirm
	pluginPromptSelect
)

// pluginPromptOption is one row of a select prompt or the action menu.
type pluginPromptOption struct {
	label, description string
	value              string        // select: the answer sent to the plugin
	action             plugin.Action // menu: the action to invoke
}

// pluginPrompt is the open plugin prompt or action menu (ModalPluginPrompt).
type pluginPrompt struct {
	kind   pluginPromptKind
	plugin string
	title  string

	// confirm
	message, confirmLabel, cancelLabel string

	// select and menu
	options []pluginPromptOption
	cursor  int

	issue *model.Issue    // menu: the bead the actions apply to
	reply func(any)       // confirm and select: delivers the answer
	done  <-chan struct{} // confirm and select: closes when no longer awaited
	prev  focus           // focus to restore on close
}

// pluginPromptStaleMsg reports that the prompt with this done channel is no
// longer awaited.
type pluginPromptStaleMsg struct{ done <-chan struct{} }

// promptEnded reports whether done is closed. A nil channel never closes.
func promptEnded(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
	}
	return false
}

// openPluginMenu opens the action menu for the selected bead.
func (m Model) openPluginMenu() (Model, tea.Cmd) {
	issue := m.selectedIssue()
	if issue == nil {
		m.setNotice("No bead selected")
		return m, nil
	}
	actions := m.pluginActions.Actions(issue)
	if len(actions) == 0 {
		m.setNotice("No plugin actions for " + issue.ID)
		return m, nil
	}
	p := &pluginPrompt{kind: pluginPromptMenu, title: "Plugin actions: " + issue.ID, issue: issue}
	for _, a := range actions {
		desc := a.Plugin
		if a.Key != "" {
			desc = a.Key + "  " + a.Plugin
		}
		p.options = append(p.options, pluginPromptOption{label: a.Label, description: desc, action: a})
	}
	m.showPluginPrompt(p)
	return m, nil
}

// handlePluginPromptMsg shows a confirm or select prompt from a plugin. It
// answers nil at once when another modal is open or the prompt is empty.
func (m Model) handlePluginPromptMsg(msg plugin.PromptMsg) (Model, tea.Cmd) {
	reply := msg.Reply
	if reply == nil {
		reply = func(any) {}
	}
	p := &pluginPrompt{plugin: msg.Plugin, reply: reply, done: msg.Done}
	switch {
	case msg.Confirm != nil:
		p.kind = pluginPromptConfirm
		p.title = msg.Confirm.Title
		p.message = msg.Confirm.Message
		p.confirmLabel = msg.Confirm.Confirm
		p.cancelLabel = msg.Confirm.Cancel
	case msg.Select != nil && len(msg.Select.Options) > 0:
		p.kind = pluginPromptSelect
		p.title = msg.Select.Title
		for _, o := range msg.Select.Options {
			p.options = append(p.options, pluginPromptOption{label: o.Label, description: o.Description, value: o.Value})
		}
	default:
		reply(nil)
		return m, nil
	}
	if m.activeModal != ModalNone {
		reply(nil)
		return m, nil
	}
	m.showPluginPrompt(p)
	if msg.Done == nil {
		return m, nil
	}
	done := msg.Done
	return m, func() tea.Msg {
		<-done
		return pluginPromptStaleMsg{done: done}
	}
}

// handlePluginPromptStale closes the prompt the message is about, without
// answering, if it is still shown.
func (m Model) handlePluginPromptStale(msg pluginPromptStaleMsg) Model {
	if p := m.pluginPrompt; p != nil && p.kind != pluginPromptMenu && p.done == msg.done {
		m.closePluginPrompt()
	}
	return m
}

func (m *Model) showPluginPrompt(p *pluginPrompt) {
	p.prev = m.focused
	m.pluginPrompt = p
	m.openModal(ModalPluginPrompt)
	m.focused = focusPluginPrompt
}

// closePluginPrompt closes the prompt and restores the focus it took.
func (m *Model) closePluginPrompt() {
	if m.pluginPrompt != nil {
		m.focused = m.pluginPrompt.prev
	}
	m.pluginPrompt = nil
	m.closeModal()
}

// answerPluginPrompt closes the prompt and sends answer to the plugin.
func (m *Model) answerPluginPrompt(answer any) {
	reply := m.pluginPrompt.reply
	m.closePluginPrompt()
	reply(answer)
}

// handlePluginPromptKeys handles keys while a plugin prompt or the action
// menu is open.
func (m Model) handlePluginPromptKeys(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p := m.pluginPrompt
	if p == nil {
		m.closeModal()
		return m, nil
	}
	if msg.String() == "ctrl+c" {
		if p.kind == pluginPromptMenu {
			m.closePluginPrompt()
		} else {
			m.answerPluginPrompt(nil)
		}
		return m, tea.Quit
	}

	if p.kind == pluginPromptConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			m.answerPluginPrompt(true)
		case "n", "N", "esc":
			m.answerPluginPrompt(false)
		}
		return m, nil
	}

	k := m.keys.PluginSelect
	switch {
	case key.Matches(msg, k.Up):
		if p.cursor > 0 {
			p.cursor--
		}
	case key.Matches(msg, k.Down):
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	case key.Matches(msg, k.Apply):
		opt := p.options[p.cursor]
		if p.kind == pluginPromptMenu {
			issue := p.issue
			m.closePluginPrompt()
			return m.invokePluginAction(opt.action, issue)
		}
		m.answerPluginPrompt(opt.value)
	case key.Matches(msg, k.Cancel):
		if p.kind == pluginPromptMenu {
			m.closePluginPrompt()
		} else {
			m.answerPluginPrompt(nil)
		}
	}
	return m, nil
}

// renderPluginPrompt returns the plugin prompt panel. View() composites it
// via OverlayCenterDimBackdrop.
func (m Model) renderPluginPrompt() string {
	p := m.pluginPrompt
	if p == nil {
		return ""
	}
	t := m.theme

	maxInner := m.width - 8
	if maxInner < 20 {
		maxInner = 20
	}

	textStyle := lipgloss.NewStyle().Foreground(t.Base.GetForeground())
	cursorStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(t.Muted)
	hintStyle := lipgloss.NewStyle().Foreground(t.Secondary).Italic(true)

	var lines []string
	if p.kind == pluginPromptConfirm {
		for _, l := range wrapPlain(p.message, maxInner) {
			lines = append(lines, textStyle.Render(truncateRunesHelper(l, maxInner, "...")))
		}
		confirm, cancel := p.confirmLabel, p.cancelLabel
		if confirm == "" {
			confirm = "confirm"
		}
		if cancel == "" {
			cancel = "cancel"
		}
		lines = append(lines, "", hintStyle.Render(truncateRunesHelper(
			fmt.Sprintf("y/enter %s  n/esc %s", confirm, cancel), maxInner, "...")))
		return renderFieldModalLines(p.title, lines, t, maxInner)
	}

	for i, o := range p.options {
		cursor := "  "
		labelStyle := textStyle
		if i == p.cursor {
			cursor = cursorStyle.Render("> ")
			labelStyle = cursorStyle
		}
		line := cursor + labelStyle.Render(o.label)
		if o.description != "" {
			line += "  " + descStyle.Render(o.description)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", hintStyle.Render("j/k move  enter select  esc cancel"))
	return renderFieldModalLines(p.title, lines, t, maxInner)
}
