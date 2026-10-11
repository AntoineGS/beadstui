package keys

import "charm.land/bubbles/v2/key"

// GlobalKeys are the bindings that fire identically (or with uniform
// context-aware semantics) regardless of which view or focus is active.
// See ADR-004 Decision 1.
//
// Two cascade exceptions are documented inline: q and Esc are
// 9-/10-branch state cascades in the dispatcher, modeled as a single
// binding here with a single Help.Desc ("back / quit (context-aware)"
// and "back / cancel (context-aware)"). The cascade lives in the
// dispatcher's case body, NOT in the Map.
//
// Column layout (FullHelp): Help & Chrome / Views / Workspace / Actions.
type GlobalKeys struct {
	// Help & Chrome
	Help              key.Binding `action:"app.help"`         // ? / f1
	Sidebar           key.Binding `action:"app.sidebar"`      // ; / f2
	SidebarScrollDown key.Binding `action:"app.sidebar_down"` // ctrl+j (gated on showShortcutsSidebar)
	SidebarScrollUp   key.Binding `action:"app.sidebar_up"`   // ctrl+k (gated on showShortcutsSidebar)
	Tutorial          key.Binding `action:"app.tutorial"`     // `
	Quit              key.Binding `action:"app.force_quit"`   // ctrl+c
	Back              key.Binding `action:"app.quit"`         // q (cascade)
	Cancel            key.Binding `action:"app.cancel"`       // esc (cascade)

	// Views
	Board          key.Binding `action:"view.board"`      // b
	Graph          key.Binding `action:"view.graph"`      // g
	Insights       key.Binding `action:"view.insights"`   // i
	History        key.Binding `action:"view.history"`    // h
	Actionable     key.Binding `action:"view.actionable"` // a
	FlowMatrix     key.Binding `action:"view.flow"`       // f
	Tree           key.Binding `action:"view.tree"`       // T
	LabelDashboard key.Binding `action:"view.labels"`     // [ / f3
	Attention      key.Binding `action:"view.attention"`  // ] / f4
	Epics          key.Binding `action:"view.epics"`      // E
	Memories       key.Binding `action:"view.memories"`   // u / f6 (bt-2ea7t.4 - "m" collides with the
	// focusList claim shortcut and the focusInsights heatmap toggle, both of
	// which the Global switch would shadow; "u" was verified free across
	// every keys/*.go binding and every literal msg.String() comparison in
	// pkg/ui, see bt-2ea7t.4 close notes)

	// Workspace
	ProjectsOrWisps  key.Binding `action:"project.picker"`       // w
	WorkspaceHomeAll key.Binding `action:"project.home_all"`     // W
	WispToggle       key.Binding `action:"project.wisps"`        // ctrl+w (wisp visibility; `w` is claimed by the project picker in workspace/global mode, see bt-8jds)
	HybridPreset     key.Binding `action:"search.hybrid_preset"` // H (gated on focusList + hybrid mode)

	// Actions
	Refresh       key.Binding `action:"app.refresh"`           // ctrl+r / f5
	SearchMode    key.Binding `action:"search.cycle_mode"`     // ctrl+s
	BQL           key.Binding `action:"app.bql"`               // :
	Recipes       key.Binding `action:"filter.recipes"`        // '
	Alerts        key.Binding `action:"notify.alerts"`         // !
	Notifications key.Binding `action:"notify.notifications"`  // 1 (gated on not ViewAttention)
	SearchBounce  key.Binding `action:"app.search"`            // / (gated on split + non-list focus)
	PriorityHints key.Binding `action:"notify.priority_hints"` // p
	Export        key.Binding `action:"bead.export"`           // x
	LabelPicker   key.Binding `action:"filter.labels"`         // l
}

// NewGlobalKeys returns the default global keymap.
func NewGlobalKeys() GlobalKeys {
	return GlobalKeys{
		// Help & Chrome
		Help: key.NewBinding(
			key.WithKeys("?", "f1"),
			key.WithHelp("?", "help"),
		),
		Sidebar: key.NewBinding(
			key.WithKeys(";", "f2"),
			key.WithHelp(";", "shortcuts"),
		),
		SidebarScrollDown: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("⌃j", "sidebar down"),
		),
		SidebarScrollUp: key.NewBinding(
			key.WithKeys("ctrl+k"),
			key.WithHelp("⌃k", "sidebar up"),
		),
		Tutorial: key.NewBinding(
			key.WithKeys("`"),
			key.WithHelp("`", "tutorial"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("⌃c", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "back / quit (context-aware)"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back / cancel (context-aware)"),
		),

		// Views
		Board: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "board"),
		),
		Graph: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "graph"),
		),
		Insights: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "insights"),
		),
		History: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "history"),
		),
		Actionable: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "actionable"),
		),
		FlowMatrix: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "flow matrix"),
		),
		Tree: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "tree"),
		),
		LabelDashboard: key.NewBinding(
			key.WithKeys("[", "f3"),
			key.WithHelp("[", "labels"),
		),
		Attention: key.NewBinding(
			key.WithKeys("]", "f4"),
			key.WithHelp("]", "attention"),
		),
		Epics: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "epics"),
		),
		Memories: key.NewBinding(
			key.WithKeys("u", "f6"),
			key.WithHelp("u", "memories"),
		),

		// Workspace
		ProjectsOrWisps: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "projects / wisps"),
		),
		WorkspaceHomeAll: key.NewBinding(
			key.WithKeys("W"),
			key.WithHelp("W", "home / all projects"),
		),
		WispToggle: key.NewBinding(
			key.WithKeys("ctrl+w"),
			key.WithHelp("⌃w", "wisps"),
		),
		HybridPreset: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "hybrid preset"),
		),

		// Actions
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r", "f5"),
			key.WithHelp("⌃r", "refresh"),
		),
		SearchMode: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("⌃s", "cycle search ranker"),
		),
		BQL: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "BQL query"),
		),
		Recipes: key.NewBinding(
			key.WithKeys("'"),
			key.WithHelp("'", "recipes"),
		),
		Alerts: key.NewBinding(
			key.WithKeys("!"),
			key.WithHelp("!", "alerts"),
		),
		Notifications: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "notifications"),
		),
		SearchBounce: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		PriorityHints: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "priority hints"),
		),
		Export: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "export markdown"),
		),
		LabelPicker: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "label picker"),
		),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot
// when no view-specific Map is active. Most-used / most-orienting first.
//
// Per ADR-004 Decision 1, L1 is "active view's ShortHelp() only" — globals
// are background context. This method exists for the modal-or-empty-view
// fallback path; populated views provide their own ShortHelp.
func (k GlobalKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Sidebar, k.Refresh, k.Quit}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Help & Chrome / Views / Workspace / Actions.
func (k GlobalKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Help & Chrome
		{k.Help, k.Sidebar, k.SidebarScrollDown, k.SidebarScrollUp, k.Tutorial, k.Quit, k.Back, k.Cancel},
		// Views
		{k.Board, k.Graph, k.Insights, k.History, k.Actionable, k.FlowMatrix, k.Tree, k.LabelDashboard, k.Attention, k.Epics, k.Memories},
		// Workspace
		{k.ProjectsOrWisps, k.WorkspaceHomeAll, k.WispToggle, k.HybridPreset},
		// Actions
		{k.Refresh, k.SearchMode, k.BQL, k.Recipes, k.Alerts, k.Notifications, k.SearchBounce, k.PriorityHints, k.Export, k.LabelPicker},
	}
}
