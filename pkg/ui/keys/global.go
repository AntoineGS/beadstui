package keys

import "charm.land/bubbles/v2/key"

// GlobalKeys are the bindings that fire identically (or with uniform
// context-aware semantics) regardless of which view or focus is active.
// See ADR-004 Decision 1.
//
// Most of them sit behind the leader (bt-i4k): "<leader>vb" is SPC v b.
// Direct keys are only the universal ones (help, quit, cancel, search, BQL).
// q quits at once whenever no text field has focus, and esc only backs out
// one level: its cascade lives in the dispatcher's case body, not here.
//
// Column layout (FullHelp): Help & Chrome / Views / Workspace / Actions.
type GlobalKeys struct {
	// Help & Chrome
	Help              key.Binding `action:"app.help"`         // ? / f1
	Sidebar           key.Binding `action:"app.sidebar"`      // ;
	SidebarScrollDown key.Binding `action:"app.sidebar_down"` // ctrl+j (gated on showShortcutsSidebar)
	SidebarScrollUp   key.Binding `action:"app.sidebar_up"`   // ctrl+k (gated on showShortcutsSidebar)
	Tutorial          key.Binding `action:"app.tutorial"`     // SPC T
	Settings          key.Binding `action:"app.settings"`     // SPC ,
	Quit              key.Binding `action:"app.force_quit"`   // ctrl+c
	Back              key.Binding `action:"app.quit"`         // q / SPC q
	Cancel            key.Binding `action:"app.cancel"`       // esc (cascade)

	// Views
	ViewList       key.Binding `action:"view.list"`       // SPC v l
	Board          key.Binding `action:"view.board"`      // SPC v b
	Graph          key.Binding `action:"view.graph"`      // SPC v g
	Insights       key.Binding `action:"view.insights"`   // SPC v i
	History        key.Binding `action:"view.history"`    // SPC v h
	Actionable     key.Binding `action:"view.actionable"` // SPC v a
	FlowMatrix     key.Binding `action:"view.flow"`       // SPC v f
	Tree           key.Binding `action:"view.tree"`       // SPC v t
	LabelDashboard key.Binding `action:"view.labels"`     // SPC v L
	Attention      key.Binding `action:"view.attention"`  // SPC v A
	Epics          key.Binding `action:"view.epics"`      // SPC v e
	Memories       key.Binding `action:"view.memories"`   // SPC v m

	// Filter. The status filters toggle back to "all" when pressed again;
	// CycleStatusFilter walks the full status set (bt-gpvwe).
	FilterOpen        key.Binding `action:"filter.open"`         // SPC f o
	FilterClosed      key.Binding `action:"filter.closed"`       // SPC f c
	FilterReady       key.Binding `action:"filter.ready"`        // SPC f r
	CycleStatusFilter key.Binding `action:"filter.cycle_status"` // SPC f s
	LabelPicker       key.Binding `action:"filter.labels"`       // SPC f l
	Recipes           key.Binding `action:"filter.recipes"`      // SPC f R
	FilterClear       key.Binding `action:"filter.clear"`        // SPC f x

	// Workspace
	ProjectsOrWisps  key.Binding `action:"project.picker"`       // SPC p p
	WorkspaceHomeAll key.Binding `action:"project.home_all"`     // SPC p h
	WispToggle       key.Binding `action:"project.wisps"`        // SPC p w
	HybridPreset     key.Binding `action:"search.hybrid_preset"` // SPC / h (gated on focusList + hybrid mode)

	// Actions
	Refresh       key.Binding `action:"app.refresh"`           // SPC r
	SearchMode    key.Binding `action:"search.cycle_mode"`     // SPC / m
	BQL           key.Binding `action:"app.bql"`               // :
	Alerts        key.Binding `action:"notify.alerts"`         // SPC n a
	Notifications key.Binding `action:"notify.notifications"`  // SPC n n
	SearchBounce  key.Binding `action:"app.search"`            // / (gated on split + non-list focus)
	PriorityHints key.Binding `action:"notify.priority_hints"` // SPC n h
	Export        key.Binding `action:"bead.export"`           // SPC b x
	PluginMenu    key.Binding `action:"plugin.menu"`           // SPC x (gated on a plugin view with plugin actions)
	DebugDims     key.Binding `action:"window.debug_dims"`     // SPC w d (out of ShortHelp: a diagnostic aid)
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
			key.WithKeys(";"),
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
			key.WithKeys("<leader>T"),
			key.WithHelp("SPC T", "tutorial"),
		),
		Settings: key.NewBinding(
			key.WithKeys("<leader>,"),
			key.WithHelp("SPC ,", "settings"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("⌃c", "force quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("q", "<leader>q"),
			key.WithHelp("q", "quit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back / cancel"),
		),

		// Views
		ViewList: key.NewBinding(
			key.WithKeys("<leader>vl"),
			key.WithHelp("SPC v l", "list"),
		),
		Board: key.NewBinding(
			key.WithKeys("<leader>vb"),
			key.WithHelp("SPC v b", "board"),
		),
		Graph: key.NewBinding(
			key.WithKeys("<leader>vg"),
			key.WithHelp("SPC v g", "graph"),
		),
		Insights: key.NewBinding(
			key.WithKeys("<leader>vi"),
			key.WithHelp("SPC v i", "insights"),
		),
		History: key.NewBinding(
			key.WithKeys("<leader>vh"),
			key.WithHelp("SPC v h", "history"),
		),
		Actionable: key.NewBinding(
			key.WithKeys("<leader>va"),
			key.WithHelp("SPC v a", "actionable"),
		),
		FlowMatrix: key.NewBinding(
			key.WithKeys("<leader>vf"),
			key.WithHelp("SPC v f", "flow matrix"),
		),
		Tree: key.NewBinding(
			key.WithKeys("<leader>vt"),
			key.WithHelp("SPC v t", "tree"),
		),
		LabelDashboard: key.NewBinding(
			key.WithKeys("<leader>vL"),
			key.WithHelp("SPC v L", "labels"),
		),
		Attention: key.NewBinding(
			key.WithKeys("<leader>vA"),
			key.WithHelp("SPC v A", "attention"),
		),
		Epics: key.NewBinding(
			key.WithKeys("<leader>ve"),
			key.WithHelp("SPC v e", "epics"),
		),
		Memories: key.NewBinding(
			key.WithKeys("<leader>vm"),
			key.WithHelp("SPC v m", "memories"),
		),

		// Filter
		FilterOpen: key.NewBinding(
			key.WithKeys("<leader>fo"),
			key.WithHelp("SPC f o", "open issues / all"),
		),
		FilterClosed: key.NewBinding(
			key.WithKeys("<leader>fc"),
			key.WithHelp("SPC f c", "closed issues"),
		),
		FilterReady: key.NewBinding(
			key.WithKeys("<leader>fr"),
			key.WithHelp("SPC f r", "ready (no blockers)"),
		),
		CycleStatusFilter: key.NewBinding(
			key.WithKeys("<leader>fs"),
			key.WithHelp("SPC f s", "cycle status filter"),
		),
		LabelPicker: key.NewBinding(
			key.WithKeys("<leader>fl"),
			key.WithHelp("SPC f l", "label picker"),
		),
		Recipes: key.NewBinding(
			key.WithKeys("<leader>fR"),
			key.WithHelp("SPC f R", "recipes"),
		),
		FilterClear: key.NewBinding(
			key.WithKeys("<leader>fx"),
			key.WithHelp("SPC f x", "clear all filters"),
		),

		// Workspace
		ProjectsOrWisps: key.NewBinding(
			key.WithKeys("<leader>pp"),
			key.WithHelp("SPC p p", "projects / wisps"),
		),
		WorkspaceHomeAll: key.NewBinding(
			key.WithKeys("<leader>ph"),
			key.WithHelp("SPC p h", "home / all projects"),
		),
		WispToggle: key.NewBinding(
			key.WithKeys("<leader>pw"),
			key.WithHelp("SPC p w", "wisps"),
		),
		HybridPreset: key.NewBinding(
			key.WithKeys("<leader>/h"),
			key.WithHelp("SPC / h", "hybrid preset"),
		),

		// Actions
		Refresh: key.NewBinding(
			key.WithKeys("<leader>r"),
			key.WithHelp("SPC r", "refresh"),
		),
		SearchMode: key.NewBinding(
			key.WithKeys("<leader>/m"),
			key.WithHelp("SPC / m", "cycle search ranker"),
		),
		BQL: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "BQL query"),
		),
		Alerts: key.NewBinding(
			key.WithKeys("<leader>na"),
			key.WithHelp("SPC n a", "alerts"),
		),
		Notifications: key.NewBinding(
			key.WithKeys("<leader>nn"),
			key.WithHelp("SPC n n", "notifications"),
		),
		SearchBounce: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		PriorityHints: key.NewBinding(
			key.WithKeys("<leader>nh"),
			key.WithHelp("SPC n h", "priority hints"),
		),
		Export: key.NewBinding(
			key.WithKeys("<leader>bx"),
			key.WithHelp("SPC b x", "export markdown"),
		),
		PluginMenu: key.NewBinding(
			key.WithKeys("<leader>x"),
			key.WithHelp("SPC x", "plugin actions"),
		),
		DebugDims: key.NewBinding(
			key.WithKeys("<leader>wd"),
			key.WithHelp("SPC w d", "dimension chip"),
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
	return []key.Binding{k.Help, k.Sidebar, k.Refresh, k.Back}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Help & Chrome / Views / Workspace / Actions (the overlay indexes
// them, so the filters sit in Actions).
func (k GlobalKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Help & Chrome
		{k.Help, k.Sidebar, k.SidebarScrollDown, k.SidebarScrollUp, k.Tutorial, k.Settings, k.Quit, k.Back, k.Cancel},
		// Views
		{k.ViewList, k.Board, k.Graph, k.Insights, k.History, k.Actionable, k.FlowMatrix, k.Tree, k.LabelDashboard, k.Attention, k.Epics, k.Memories},
		// Workspace
		{k.ProjectsOrWisps, k.WorkspaceHomeAll, k.WispToggle, k.HybridPreset},
		// Actions
		{k.Refresh, k.SearchMode, k.BQL, k.FilterOpen, k.FilterClosed, k.FilterReady, k.CycleStatusFilter, k.LabelPicker, k.Recipes, k.FilterClear, k.Alerts, k.Notifications, k.SearchBounce, k.PriorityHints, k.Export, k.PluginMenu, k.DebugDims},
	}
}
