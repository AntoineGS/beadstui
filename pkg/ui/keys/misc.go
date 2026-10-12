package keys

import "charm.land/bubbles/v2/key"

// MemoriesNormalKeys are the bindings for the Memories view while search is
// inactive. Typed runes during search stay raw text input.
//
// The single-pane return key is BackToList, not Back, so the universal-nav
// consistency test (which pins Help.Key per shared field name) does not
// apply to it.
type MemoriesNormalKeys struct {
	Search      key.Binding `action:"memories.search"`
	Up          key.Binding `action:"nav.up"`
	Down        key.Binding `action:"nav.down"`
	FocusToggle key.Binding `action:"memories.focus_toggle"`
	Open        key.Binding `action:"memories.open"`
	BackToList  key.Binding `action:"memories.back_to_list"`
}

// NewMemoriesNormalKeys returns the default Memories keymap.
func NewMemoriesNormalKeys() MemoriesNormalKeys {
	return MemoriesNormalKeys{
		Search:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Up:          key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "move up")),
		Down:        key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "move down")),
		FocusToggle: key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "toggle pane focus")),
		Open:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "focus detail")),
		BackToList:  key.NewBinding(key.WithKeys("backspace"), key.WithHelp("⌫", "back to list")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k MemoriesNormalKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Search, k.Open}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k MemoriesNormalKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Search, k.Open, k.FocusToggle, k.BackToList},
	}
}

// MemoriesSearchKeys are the bindings for the Memories view while search is
// active. Every other key is typed into the search input.
type MemoriesSearchKeys struct {
	Cancel  key.Binding `action:"memories_search.cancel"`
	Confirm key.Binding `action:"memories_search.confirm"`
}

// NewMemoriesSearchKeys returns the default Memories search keymap.
func NewMemoriesSearchKeys() MemoriesSearchKeys {
	return MemoriesSearchKeys{
		Cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel search")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "apply search")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k MemoriesSearchKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Confirm, k.Cancel}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k MemoriesSearchKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Confirm, k.Cancel}}
}

// HelpKeys are the bindings for the ? help overlay. Any key not listed here
// also closes the overlay (handleHelpKeys' default branch).
type HelpKeys struct {
	Up         key.Binding `action:"nav.up"`
	Down       key.Binding `action:"nav.down"`
	PageDown   key.Binding `action:"nav.page_down"`
	PageUp     key.Binding `action:"nav.page_up"`
	JumpTop    key.Binding `action:"nav.top"`
	JumpBottom key.Binding `action:"nav.bottom"`
	Close      key.Binding `action:"help.close"`
	Tutorial   key.Binding `action:"help.tutorial"`
}

// NewHelpKeys returns the default help overlay keymap.
func NewHelpKeys() HelpKeys {
	return HelpKeys{
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "scroll up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "scroll down")),
		PageDown:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("⌃d", "scroll page down")),
		PageUp:     key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("⌃u", "scroll page up")),
		JumpTop:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home/g", "jump to top")),
		JumpBottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("end/G", "jump to bottom")),
		Close:      key.NewBinding(key.WithKeys("esc", "?", "f1"), key.WithHelp("esc", "close help")),
		Tutorial:   key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "open tutorial")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k HelpKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Tutorial, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k HelpKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageDown, k.PageUp, k.JumpTop, k.JumpBottom},
		{k.Tutorial, k.Close},
	}
}

// CassKeys are the bindings for the related coding sessions modal.
type CassKeys struct {
	Up    key.Binding `action:"nav.up"`
	Down  key.Binding `action:"nav.down"`
	Copy  key.Binding `action:"cass.copy"`
	Close key.Binding `action:"cass.close"`
}

// NewCassKeys returns the default cass session modal keymap.
func NewCassKeys() CassKeys {
	return CassKeys{
		Up:    key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "move up")),
		Down:  key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "move down")),
		Copy:  key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy search command")),
		Close: key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("esc", "close")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k CassKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Copy, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k CassKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down}, {k.Copy, k.Close}}
}

// AgentPromptKeys are the bindings for the AGENTS.md prompt modal. Prev, Next
// and Select drive the menu selection; Yes, No and Never answer directly.
type AgentPromptKeys struct {
	Prev    key.Binding `action:"agent_prompt.prev"`
	Next    key.Binding `action:"agent_prompt.next"`
	Select  key.Binding `action:"agent_prompt.select"`
	Yes     key.Binding `action:"agent_prompt.yes"`
	No      key.Binding `action:"agent_prompt.no"`
	Never   key.Binding `action:"agent_prompt.never"`
	Dismiss key.Binding `action:"agent_prompt.dismiss"`
}

// NewAgentPromptKeys returns the default AGENTS.md prompt keymap.
func NewAgentPromptKeys() AgentPromptKeys {
	return AgentPromptKeys{
		Prev:    key.NewBinding(key.WithKeys("up", "k", "shift+tab"), key.WithHelp("↑/k", "previous option")),
		Next:    key.NewBinding(key.WithKeys("down", "j", "tab"), key.WithHelp("↓/j", "next option")),
		Select:  key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("⏎", "choose highlighted option")),
		Yes:     key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y", "yes, add it")),
		No:      key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "no thanks")),
		Never:   key.NewBinding(key.WithKeys("d", "D"), key.WithHelp("d", "don't ask again")),
		Dismiss: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k AgentPromptKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Yes, k.No, k.Never}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k AgentPromptKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Prev, k.Next, k.Select}, {k.Yes, k.No, k.Never, k.Dismiss}}
}

// TutorialKeys are the bindings for the interactive tutorial overlay. Close,
// ToggleTOC and SwitchFocus apply in any focus; the page and scroll keys apply
// to the content pane; Select and FocusContent apply to the table of contents.
// Up, Down, JumpTop and JumpBottom serve both panes.
type TutorialKeys struct {
	Close        key.Binding `action:"tutorial.close"`
	ToggleTOC    key.Binding `action:"tutorial.toggle_toc"`
	SwitchFocus  key.Binding `action:"tutorial.switch_focus"`
	NextPage     key.Binding `action:"tutorial.next_page"`
	PrevPage     key.Binding `action:"tutorial.prev_page"`
	Up           key.Binding `action:"nav.up"`
	Down         key.Binding `action:"nav.down"`
	PageDown     key.Binding `action:"nav.page_down"`
	PageUp       key.Binding `action:"nav.page_up"`
	JumpTop      key.Binding `action:"nav.top"`
	JumpBottom   key.Binding `action:"nav.bottom"`
	JumpPage1    key.Binding `action:"tutorial.jump_page_1"`
	JumpPage2    key.Binding `action:"tutorial.jump_page_2"`
	JumpPage3    key.Binding `action:"tutorial.jump_page_3"`
	JumpPage4    key.Binding `action:"tutorial.jump_page_4"`
	JumpPage5    key.Binding `action:"tutorial.jump_page_5"`
	JumpPage6    key.Binding `action:"tutorial.jump_page_6"`
	JumpPage7    key.Binding `action:"tutorial.jump_page_7"`
	JumpPage8    key.Binding `action:"tutorial.jump_page_8"`
	JumpPage9    key.Binding `action:"tutorial.jump_page_9"`
	Select       key.Binding `action:"tutorial.select"`
	FocusContent key.Binding `action:"tutorial.focus_content"`
}

// NewTutorialKeys returns the default tutorial keymap.
func NewTutorialKeys() TutorialKeys {
	return TutorialKeys{
		Close:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close tutorial")),
		ToggleTOC:    key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "toggle contents")),
		SwitchFocus:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "switch pane / next page")),
		NextPage:     key.NewBinding(key.WithKeys("right", "l", "n", "space"), key.WithHelp("→/l", "next page")),
		PrevPage:     key.NewBinding(key.WithKeys("left", "h", "p", "shift+tab"), key.WithHelp("←/h", "previous page")),
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "scroll up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "scroll down")),
		PageDown:     key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("⌃d", "half page down")),
		PageUp:       key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("⌃u", "half page up")),
		JumpTop:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("home/g", "jump to top")),
		JumpBottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("end/G", "jump to bottom")),
		JumpPage1:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "jump to page 1")),
		JumpPage2:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "jump to page 2")),
		JumpPage3:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "jump to page 3")),
		JumpPage4:    key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "jump to page 4")),
		JumpPage5:    key.NewBinding(key.WithKeys("5"), key.WithHelp("5", "jump to page 5")),
		JumpPage6:    key.NewBinding(key.WithKeys("6"), key.WithHelp("6", "jump to page 6")),
		JumpPage7:    key.NewBinding(key.WithKeys("7"), key.WithHelp("7", "jump to page 7")),
		JumpPage8:    key.NewBinding(key.WithKeys("8"), key.WithHelp("8", "jump to page 8")),
		JumpPage9:    key.NewBinding(key.WithKeys("9"), key.WithHelp("9", "jump to page 9")),
		Select:       key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("⏎", "open selected page")),
		FocusContent: key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("←/h", "back to content")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k TutorialKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.NextPage, k.PrevPage, k.ToggleTOC, k.Close}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
// Columns: Pages, Scroll, Contents, Exit.
func (k TutorialKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.NextPage, k.PrevPage, k.JumpPage1, k.JumpPage2, k.JumpPage3, k.JumpPage4, k.JumpPage5,
			k.JumpPage6, k.JumpPage7, k.JumpPage8, k.JumpPage9},
		{k.Up, k.Down, k.PageDown, k.PageUp, k.JumpTop, k.JumpBottom},
		{k.ToggleTOC, k.SwitchFocus, k.Select, k.FocusContent},
		{k.Close},
	}
}

// UpdateKeys are the bindings for the self-update modal. Confirm and Cancel
// match the shared confirm keys; either one dismisses the finished modal.
type UpdateKeys struct {
	Confirm key.Binding `action:"update.confirm"`
	Cancel  key.Binding `action:"update.cancel"`
}

// NewUpdateKeys returns the default self-update modal keymap.
func NewUpdateKeys() UpdateKeys {
	return UpdateKeys{
		Confirm: key.NewBinding(key.WithKeys("enter", "y", "Y"), key.WithHelp("⏎/y", "update")),
		Cancel:  key.NewBinding(key.WithKeys("esc", "n", "N"), key.WithHelp("esc/n", "cancel")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k UpdateKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Confirm, k.Cancel}
}

// FullHelp returns column-grouped bindings consumed by the local ? overlay.
func (k UpdateKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Confirm, k.Cancel}}
}
