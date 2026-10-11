package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/analysis"
	"github.com/seanmartinsmith/beadstui/pkg/cass"
)

// rawKeyCase pins one key handler that used to compare msg.String() directly
// (bt-i4k.2). The physical key and the action token must reach the same state.
type rawKeyCase struct {
	name  string
	setup func(t *testing.T) Model
	key   tea.KeyPressMsg
	id    string
	want  func(m Model) bool
}

func rawKeyCases() []rawKeyCase {
	alertsOpen := func(t *testing.T) Model {
		m := seedModel()
		m = openAlerts(m)
		if m.activeModal != ModalAlerts {
			t.Fatalf("setup: modal = %v, want ModalAlerts", m.activeModal)
		}
		return m
	}
	notifsOpen := func(t *testing.T) Model {
		m := alertsOpen(t)
		m = pressRune(m, '1') // the tab key inside the modal
		if m.activeTab != TabNotifications {
			t.Fatalf("setup: tab = %v, want TabNotifications", m.activeTab)
		}
		return m
	}
	return []rawKeyCase{
		{
			name:  "alerts severity cycles to critical",
			setup: alertsOpen,
			key:   keyRune('s'), id: "alerts.severity",
			want: func(m Model) bool { return m.alertFilterSeverity == "critical" },
		},
		{
			name:  "alerts close",
			setup: alertsOpen,
			key:   tea.KeyPressMsg{Code: tea.KeyEscape}, id: "alerts.close",
			want: func(m Model) bool { return m.activeModal == ModalNone },
		},
		{
			name:  "notifications toggle dismissed",
			setup: notifsOpen,
			key:   keyRune('d'), id: "notifications.show_dismissed",
			want: func(m Model) bool { return m.notifShowDismissed },
		},
		{
			name: "label dashboard health",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.mode = ViewLabelDashboard
				m.focused = focusLabelDashboard
				m.labelDashboard.SetData([]analysis.LabelHealth{{Label: "alpha"}})
				return m
			},
			key: keyRune('i'), id: "label_dashboard.health",
			want: func(m Model) bool { return m.activeModal == ModalLabelHealthDetail },
		},
		{
			name: "label dashboard drilldown",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.mode = ViewLabelDashboard
				m.focused = focusLabelDashboard
				m.labelDashboard.SetData([]analysis.LabelHealth{{Label: "alpha"}})
				return m
			},
			key: keyRune('d'), id: "label_dashboard.drilldown",
			want: func(m Model) bool { return m.activeModal == ModalLabelDrilldown },
		},
		{
			name: "label drilldown graph",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.labelDrilldownLabel = "alpha"
				m.openModal(ModalLabelDrilldown)
				return m
			},
			key: keyRune('g'), id: "label_drilldown.graph",
			want: func(m Model) bool { return m.activeModal == ModalLabelGraphAnalysis },
		},
		{
			name: "attention close",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.mode = ViewAttention
				return m
			},
			key: tea.KeyPressMsg{Code: tea.KeyEscape}, id: "attention.close",
			want: func(m Model) bool { return m.mode == ViewList },
		},
		{
			name: "memories search",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.mode = ViewMemories
				m.focused = focusMemories
				return m
			},
			key: keyRune('/'), id: "memories.search",
			want: func(m Model) bool { return m.memories.IsSearchActive() },
		},
		{
			name: "help close",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.openModal(ModalHelp)
				m.focused = focusHelp
				return m
			},
			key: tea.KeyPressMsg{Code: tea.KeyEscape}, id: "help.close",
			want: func(m Model) bool { return m.activeModal == ModalNone },
		},
		{
			name: "help legend toggle",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.openModal(ModalHelp)
				m.focused = focusHelp
				return m
			},
			key: keyRune('l'), id: "help.legend",
			want: func(m Model) bool { return m.activeModal == ModalHelp && m.helpLegend },
		},
		{
			name: "cass close",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.cassModal = NewCassSessionModal("bt-fix", cass.CorrelationResult{BeadID: "bt-fix"}, m.theme)
				m.cassModal.keys = m.keys.Cass
				m.openModal(ModalCassSession)
				return m
			},
			key: tea.KeyPressMsg{Code: tea.KeyEscape}, id: "cass.close",
			want: func(m Model) bool { return m.activeModal == ModalNone },
		},
		{
			name: "agent prompt no",
			setup: func(t *testing.T) Model {
				m := seedModel()
				m.agentPromptModal = NewAgentPromptModal("/test/AGENTS.md", "AGENTS.md", m.theme)
				m.agentPromptModal.keys = m.keys.AgentPrompt
				m.openModal(ModalAgentPrompt)
				return m
			},
			key: keyRune('n'), id: "agent_prompt.no",
			want: func(m Model) bool { return m.activeModal == ModalNone },
		},
	}
}

func TestRawKeyBehaviourPins(t *testing.T) {
	for _, c := range rawKeyCases() {
		t.Run(c.name, func(t *testing.T) {
			m := runKey(t, c.setup(t), c.key)
			if !c.want(m) {
				t.Errorf("physical key %q did not produce the pinned state", c.key.String())
			}
		})
	}
}

func TestAttentionPickLabelKeepsDigit(t *testing.T) {
	m := seedModel()
	m.mode = ViewAttention
	m.attentionCache.Labels = []analysis.LabelAttentionScore{{Label: "one"}, {Label: "two"}, {Label: "three"}}
	m = runKey(t, m, keyRune('3'))
	if m.filter.labelFilter != "three" {
		t.Errorf("labelFilter = %q, want three", m.filter.labelFilter)
	}
}

func TestNewMapsDispatchByToken(t *testing.T) {
	for _, c := range rawKeyCases() {
		t.Run(c.name, func(t *testing.T) {
			m := runKey(t, c.setup(t), tokenMsg(c.id))
			if !c.want(m) {
				t.Errorf("token %s did not produce the pinned state", "@"+c.id)
			}
		})
	}
}
