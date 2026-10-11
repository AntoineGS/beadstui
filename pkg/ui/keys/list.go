package keys

import "charm.land/bubbles/v2/key"

// ListNormalKeys are the bindings available when the main list is focused
// and not in filter mode. handleListKeys dispatches against these via
// key.Matches; help surfaces consume them via ShortHelp / FullHelp.
//
// Universal nav (Up / Down) is declared here per ADR-004 Decision 1
// [v1 — revisit]. Help-only — the bubbles list owns dispatch for arrow /
// vim nav — but declared so the help surface is truthful.
//
// Tab and the pane-resize keys sit on this Map (not on GlobalKeys) per
// ADR-004 Decision 1's no-match-and-fall-through rule. Each view that wants
// split-view detail-toggle / pane-resize declares its own bindings. Their
// leader sequences (SPC s, SPC b, SPC w, ...) therefore work only in the list
// view (bt-i4k).
//
// Convention (per ADR-004 Decision 1, set by ListKeys in bt-ift6.2):
// arrows-primary, vim-keys-as-alternate. WithKeys lists arrow first, vim
// second; help text shows arrow first ("↑/k", not "k/↑").
type ListNormalKeys struct {
	// Move
	Up         key.Binding `action:"nav.up"`
	Down       key.Binding `action:"nav.down"`
	JumpTop    key.Binding `action:"nav.top"`
	JumpBottom key.Binding `action:"nav.bottom"`
	PageDown   key.Binding `action:"nav.page_down"`
	PageUp     key.Binding `action:"nav.page_up"`

	// The status filters (SPC f ...) live on GlobalKeys so they work from
	// every view (bt-i4k).

	// Detail & pane
	Enter            key.Binding `action:"list.open"`
	EpicCard         key.Binding `action:"bead.epic_card"`
	SplitFocusToggle key.Binding `action:"list.focus_toggle"`
	// PaneRight / PaneLeft move between the list and the details, like
	// lazygit's panels (bt-tg0). The bubbles list no longer pages on them.
	PaneRight key.Binding `action:"window.focus_right"`
	PaneLeft  key.Binding `action:"window.focus_left"`
	// DetailPageUp / DetailPageDown page the details from either pane, like
	// lazygit's , and . keys.
	DetailPageUp     key.Binding `action:"list.detail_page_up"`
	DetailPageDown   key.Binding `action:"list.detail_page_down"`
	SplitShrinkLeft  key.Binding `action:"window.shrink"`
	SplitShrinkRight key.Binding `action:"window.grow"`

	// Pane fullscreen (bt-530vn): btop-style per-pane maximize toggle,
	// reachable at any terminal width - distinct from the width-driven
	// single-pane auto-collapse (bt-9a3wv). Pressing the same key again (or
	// esc) restores the prior layout; the dispatcher cascade lives in
	// model_update_input.go, not here.
	PaneFullscreenIssues  key.Binding `action:"window.fullscreen_list"`
	PaneFullscreenDetails key.Binding `action:"window.fullscreen_details"`

	// Field edit (bt-oiaj.5). 'e' opens the field-select modal; it moved off
	// EpicCard (bt-oiaj.13 wave key migration - see
	// docs/plans/2026-07-07-bt-edits-wave-oiaj13-5-6.md) so 'e' is free for
	// the ratified edit binding (tkhq #1: "e reserved globally for edit").
	FieldEdit key.Binding `action:"bead.edit"`

	// Sort / triage
	CycleSort        key.Binding `action:"sort.cycle"`
	CycleSortReverse key.Binding `action:"sort.cycle_reverse"`
	RecipeTriage     key.Binding `action:"sort.triage"`

	// Time travel
	TimeTravelInput key.Binding `action:"app.time_travel"`

	// Actions
	CopyID       key.Binding `action:"bead.copy_id"`
	CopyIssue    key.Binding `action:"bead.copy_issue"`
	OpenInEditor key.Binding `action:"bead.open_editor"`
	CassSession  key.Binding `action:"bead.cass"`
	SelfUpdate   key.Binding `action:"app.self_update"`

	// Write verbs (bt-oiaj.10). Claim is the first bt write to ship: it shells
	// out `bd update <id> --claim` behind a confirm (SPC b m, "claim it for
	// me").
	Claim key.Binding `action:"bead.claim"`
}

// NewListNormalKeys returns the default list (non-filter-mode) keymap.
func NewListNormalKeys() ListNormalKeys {
	return ListNormalKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		JumpTop: key.NewBinding(
			key.WithKeys("home", "gg"),
			key.WithHelp("gg/home", "jump to top"),
		),
		JumpBottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G/end", "jump to bottom"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("⌃d", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("⌃u", "page up"),
		),

		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "open detail"),
		),
		EpicCard: key.NewBinding(
			key.WithKeys("<leader>bE"),
			key.WithHelp("SPC b E", "epic card"),
		),
		SplitFocusToggle: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("⇥", "toggle split focus"),
		),
		PaneRight: key.NewBinding(
			key.WithKeys("right", "l", "<leader>wl"),
			key.WithHelp("→/l", "go to details"),
		),
		PaneLeft: key.NewBinding(
			key.WithKeys("left", "h", "<leader>wh"),
			key.WithHelp("←/h", "back to list"),
		),
		DetailPageUp: key.NewBinding(
			key.WithKeys(","),
			key.WithHelp(",", "details page up"),
		),
		DetailPageDown: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "details page down"),
		),
		FieldEdit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit field"),
		),
		SplitShrinkLeft: key.NewBinding(
			key.WithKeys("<leader>w<lt>"),
			key.WithHelp("SPC w <", "shrink list pane"),
		),
		SplitShrinkRight: key.NewBinding(
			key.WithKeys("<leader>w<gt>"),
			key.WithHelp("SPC w >", "expand list pane"),
		),
		PaneFullscreenIssues: key.NewBinding(
			key.WithKeys("<leader>w1"),
			key.WithHelp("SPC w 1", "fullscreen issues pane"),
		),
		PaneFullscreenDetails: key.NewBinding(
			key.WithKeys("<leader>w2"),
			key.WithHelp("SPC w 2", "fullscreen details pane"),
		),

		CycleSort: key.NewBinding(
			key.WithKeys("<leader>ss"),
			key.WithHelp("SPC s s", "cycle sort"),
		),
		CycleSortReverse: key.NewBinding(
			key.WithKeys("<leader>sS"),
			key.WithHelp("SPC s S", "cycle sort reverse"),
		),
		RecipeTriage: key.NewBinding(
			key.WithKeys("<leader>st"),
			key.WithHelp("SPC s t", "triage recipe"),
		),

		TimeTravelInput: key.NewBinding(
			key.WithKeys("<leader>t"),
			key.WithHelp("SPC t", "time travel"),
		),

		CopyID: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy ID"),
		),
		CopyIssue: key.NewBinding(
			key.WithKeys("<leader>bY"),
			key.WithHelp("SPC b Y", "copy issue"),
		),
		OpenInEditor: key.NewBinding(
			key.WithKeys("<leader>bo"),
			key.WithHelp("SPC b o", "open in editor"),
		),
		CassSession: key.NewBinding(
			key.WithKeys("<leader>bc"),
			key.WithHelp("SPC b c", "cass session"),
		),
		SelfUpdate: key.NewBinding(
			key.WithKeys("<leader>U"),
			key.WithHelp("SPC U", "self-update"),
		),

		Claim: key.NewBinding(
			key.WithKeys("<leader>bm"),
			key.WithHelp("SPC b m", "claim (assign to me)"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
// Most-used / most-orienting first; arrow-nav is intrinsic so it's not
// repeated here in favor of action-driving bindings.
func (k ListNormalKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.FieldEdit, k.CopyID, k.CycleSort}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Move / Detail & Pane / Sort / Time / Actions.
func (k ListNormalKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Move
		{k.Up, k.Down, k.JumpTop, k.JumpBottom, k.PageDown, k.PageUp},
		// Detail & pane
		{k.Enter, k.PaneRight, k.PaneLeft, k.DetailPageUp, k.DetailPageDown, k.EpicCard, k.SplitFocusToggle, k.SplitShrinkLeft, k.SplitShrinkRight, k.PaneFullscreenIssues, k.PaneFullscreenDetails},
		// Sort / triage
		{k.CycleSort, k.CycleSortReverse, k.RecipeTriage},
		// Time travel
		{k.TimeTravelInput},
		// Actions
		{k.CopyID, k.CopyIssue, k.OpenInEditor, k.CassSession, k.SelfUpdate, k.Claim, k.FieldEdit},
	}
}

// ListSearchKeys is the help-only Map for the list's filter-typing sub-state
// (m.list.FilterState() == list.Filtering). The bubbles list owns dispatch
// for filter typing, result navigation, apply, and cancel; this Map exists
// so the help surface is truthful in that state — Decision 7's principle
// applied at the within-handler level.
//
// Globals that pre-empt the filter-state guard (Ctrl+C quit) are
// intentionally not redeclared here. They
// surface via GlobalKeys.FullHelp() concat in help surfaces; redeclaring
// them would create a drift surface.
//
// Up/Down field names match ListNormalKeys / TreeKeys for the universal-nav
// consistency test (same Help.Key strings). Apply / Cancel use field names
// outside the universal list so they don't trigger comparison.
type ListSearchKeys struct {
	Up     key.Binding `action:"nav.up"`
	Down   key.Binding `action:"nav.down"`
	Apply  key.Binding `action:"list_search.apply"`
	Cancel key.Binding `action:"list_search.cancel"`
}

// NewListSearchKeys returns the help-only filter-mode keymap.
func NewListSearchKeys() ListSearchKeys {
	return ListSearchKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "prev result"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "next result"),
		),
		Apply: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("⏎", "apply filter"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel filter"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot
// during filter mode. Apply / Cancel first because they're the resolution
// actions; result-nav is intrinsic.
func (k ListSearchKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Apply, k.Cancel, k.Up, k.Down}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay
// during filter mode. Columns: Move / Resolve.
func (k ListSearchKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Apply, k.Cancel},
	}
}
