package keys

import "charm.land/bubbles/v2/key"

// AlertsKeys are the bindings for the alerts tab of the shared alerts and
// notifications modal (model_update_input.go). TabSwitch, ShowAlerts and
// ShowNotifications run before per-tab dispatch, so they also apply while the
// notifications tab has focus.
//
// PageLeft/PageRight (h/l) and PageUp/PageDown (arrows, pgup/pgdn) are
// separate fields because each moves the cursor a page in a fixed direction.
type AlertsKeys struct {
	TabSwitch         key.Binding `action:"alerts.tab_switch"`
	ShowAlerts        key.Binding `action:"alerts.show_alerts"`
	ShowNotifications key.Binding `action:"alerts.show_notifications"`
	Up                key.Binding `action:"nav.up"`
	Down              key.Binding `action:"nav.down"`
	PageLeft          key.Binding `action:"alerts.page_left"`
	PageRight         key.Binding `action:"alerts.page_right"`
	PageUp            key.Binding `action:"nav.page_up"`
	PageDown          key.Binding `action:"nav.page_down"`
	JumpTop           key.Binding `action:"nav.top"`
	JumpBottom        key.Binding `action:"nav.bottom"`
	Open              key.Binding `action:"alerts.open"`
	Dismiss           key.Binding `action:"alerts.dismiss"`
	DismissAll        key.Binding `action:"alerts.dismiss_all"`
	Severity          key.Binding `action:"alerts.severity"`
	SeverityReverse   key.Binding `action:"alerts.severity_reverse"`
	Type              key.Binding `action:"alerts.type"`
	TypeReverse       key.Binding `action:"alerts.type_reverse"`
	Project           key.Binding `action:"alerts.project"`
	ProjectReverse    key.Binding `action:"alerts.project_reverse"`
	Sort              key.Binding `action:"alerts.sort"`
	SortReverse       key.Binding `action:"alerts.sort_reverse"`
	Class             key.Binding `action:"alerts.class"`
	ClassReverse      key.Binding `action:"alerts.class_reverse"`
	Reset             key.Binding `action:"alerts.reset"`
	Close             key.Binding `action:"alerts.close"`
}

// NewAlertsKeys returns the default alerts-tab keymap.
func NewAlertsKeys() AlertsKeys {
	return AlertsKeys{
		TabSwitch:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("⇥", "switch tab")),
		ShowAlerts:        key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "alerts tab / close")),
		ShowNotifications: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "notifications tab / close")),
		Up:                key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "move up")),
		Down:              key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "move down")),
		PageLeft:          key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "previous page")),
		PageRight:         key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "next page")),
		PageUp:            key.NewBinding(key.WithKeys("left", "pgup"), key.WithHelp("←/pgup", "previous page")),
		PageDown:          key.NewBinding(key.WithKeys("right", "pgdown"), key.WithHelp("→/pgdn", "next page")),
		JumpTop:           key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home/g", "jump to top")),
		JumpBottom:        key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end/G", "jump to bottom")),
		Open:              key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open issue")),
		Dismiss:           key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "dismiss alert")),
		DismissAll:        key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "dismiss all")),
		Severity:          key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle severity")),
		SeverityReverse:   key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "cycle severity back")),
		Type:              key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "cycle type")),
		TypeReverse:       key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "cycle type back")),
		Project:           key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "cycle project")),
		ProjectReverse:    key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "cycle project back")),
		Sort:              key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "cycle sort")),
		SortReverse:       key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "cycle sort back")),
		Class:             key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "cycle class")),
		ClassReverse:      key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "cycle class back")),
		Reset:             key.NewBinding(key.WithKeys("r", "R"), key.WithHelp("r", "reset filters")),
		Close:             key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "close")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k AlertsKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Dismiss, k.Close}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Move, Act, Filter, Tabs.
func (k AlertsKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageLeft, k.PageRight, k.PageUp, k.PageDown, k.JumpTop, k.JumpBottom},
		{k.Open, k.Dismiss, k.DismissAll, k.Close},
		{k.Severity, k.SeverityReverse, k.Type, k.TypeReverse, k.Project, k.ProjectReverse, k.Sort, k.SortReverse, k.Class, k.ClassReverse, k.Reset},
		{k.TabSwitch, k.ShowAlerts, k.ShowNotifications},
	}
}

// NotificationsKeys are the bindings for the notifications tab of the shared
// alerts and notifications modal. The tab-switch keys live on AlertsKeys.
type NotificationsKeys struct {
	Up            key.Binding `action:"nav.up"`
	Down          key.Binding `action:"nav.down"`
	PageUp        key.Binding `action:"nav.page_up"`
	PageDown      key.Binding `action:"nav.page_down"`
	JumpTop       key.Binding `action:"nav.top"`
	JumpBottom    key.Binding `action:"nav.bottom"`
	Open          key.Binding `action:"notifications.open"`
	Dismiss       key.Binding `action:"notifications.dismiss"`
	DismissAll    key.Binding `action:"notifications.dismiss_all"`
	ShowDismissed key.Binding `action:"notifications.show_dismissed"`
	Kind          key.Binding `action:"notifications.kind"`
	KindReverse   key.Binding `action:"notifications.kind_reverse"`
	Reset         key.Binding `action:"notifications.reset"`
	Close         key.Binding `action:"notifications.close"`
}

// NewNotificationsKeys returns the default notifications-tab keymap.
func NewNotificationsKeys() NotificationsKeys {
	return NotificationsKeys{
		Up:            key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("↑/k", "move up")),
		Down:          key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("↓/j", "move down")),
		PageUp:        key.NewBinding(key.WithKeys("left", "pgup"), key.WithHelp("←/pgup", "previous page")),
		PageDown:      key.NewBinding(key.WithKeys("right", "pgdown"), key.WithHelp("→/pgdn", "next page")),
		JumpTop:       key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("home/g", "jump to top")),
		JumpBottom:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end/G", "jump to bottom")),
		Open:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open issue")),
		Dismiss:       key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "dismiss notification")),
		DismissAll:    key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "dismiss all")),
		ShowDismissed: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "show dismissed")),
		Kind:          key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "cycle kind")),
		KindReverse:   key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "cycle kind back")),
		Reset:         key.NewBinding(key.WithKeys("r", "R"), key.WithHelp("r", "reset kind filter")),
		Close:         key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "close")),
	}
}

// ShortHelp returns the bindings shown in the status-bar L1 hint slot.
func (k NotificationsKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.Dismiss, k.Close}
}

// FullHelp returns column-grouped bindings for the ; sidebar and ? overlay.
// Columns: Move, Act, Filter.
func (k NotificationsKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.JumpTop, k.JumpBottom},
		{k.Open, k.Dismiss, k.DismissAll, k.Close},
		{k.ShowDismissed, k.Kind, k.KindReverse, k.Reset},
	}
}
