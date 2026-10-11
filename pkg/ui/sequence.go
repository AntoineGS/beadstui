package ui

import (
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// Defaults for the leader key and the which-key debounce (bt-i4k).
const (
	defaultLeaderKey     = "space"
	defaultWhichKeyDelay = 250 * time.Millisecond
)

// seqState is a pending leader or gg sequence. prefix is "" when idle; node is
// the tree node reached by steps. gen bumps on every change so stale which-key
// ticks are ignored.
type seqState struct {
	prefix    string
	node      *keys.SeqNode
	steps     []string
	gen       int
	showPanel bool
}

func (s seqState) pending() bool { return s.prefix != "" }

// whichKeyMsg is the debounce tick that shows the which-key panel.
type whichKeyMsg struct{ gen int }

// seqMaps returns the maps whose sequence keys are live: Global plus the
// active non-modal view map.
func (m Model) seqMaps() []any {
	maps := []any{m.keys.Global}
	switch m.mode {
	case ViewList:
		if m.list.FilterState() == list.Filtering {
			maps = append(maps, m.keys.ListSearch)
		} else {
			maps = append(maps, m.keys.ListNormal)
		}
	case ViewTree:
		maps = append(maps, m.keys.Tree)
	case ViewBoard:
		if m.board.IsSearchMode() {
			maps = append(maps, m.keys.BoardSearch)
		} else {
			maps = append(maps, m.keys.BoardNormal)
		}
	case ViewGraph:
		maps = append(maps, m.keys.Graph)
	case ViewInsights:
		maps = append(maps, m.keys.Insights)
	case ViewActionable:
		maps = append(maps, m.keys.Actionable)
	case ViewFlowMatrix:
		maps = append(maps, m.keys.FlowMatrix)
	case ViewEpics:
		maps = append(maps, m.keys.Epics)
	case ViewLabelDashboard:
		maps = append(maps, m.keys.LabelDashboard)
	case ViewAttention:
		maps = append(maps, m.keys.Attention)
	case ViewMemories:
		if m.memories.IsSearchActive() {
			maps = append(maps, m.keys.MemoriesSearch)
		} else {
			maps = append(maps, m.keys.MemoriesNormal)
		}
	}
	return maps
}

// inTextInput reports whether typed keys belong to a text field, where the
// leader and gg must stay inactive.
func (m Model) inTextInput() bool {
	switch {
	case m.list.FilterState() == list.Filtering:
		return true
	case m.mode == ViewBoard && m.board.IsSearchMode():
		return true
	case m.mode == ViewHistory && m.historyView.IsSearchActive():
		return true
	case m.mode == ViewMemories && m.memories.IsSearchActive():
		return true
	case m.focused == focusTimeTravelInput:
		return true
	}
	return false
}

// leaderLabel is the leader as shown to the user.
func (m Model) leaderLabel() string {
	if m.leaderKey == "space" {
		return "SPC"
	}
	return m.leaderKey
}

func (m Model) seqTree(prefix string) *keys.SeqNode {
	var groups map[string]string
	if prefix == keys.LeaderPrefix {
		groups = keys.LeaderGroups
	}
	root, _ := keys.BuildSeqTree(prefix, keys.SeqEntries(prefix, m.seqMaps()...), groups)
	return root
}

// pendingSeqText is the footer form of the pending sequence ("SPC v…"), or "".
func (m Model) pendingSeqText() string {
	if !m.seq.pending() {
		return ""
	}
	return keys.FormatSeq(m.seq.prefix, m.seq.steps, m.leaderLabel()) + "…"
}

func (m *Model) resetSeq() {
	m.seq = seqState{gen: m.seq.gen + 1}
}

// whichKeyTick schedules the panel for the current gen, or shows it at once
// with a zero delay.
func (m *Model) whichKeyTick() tea.Cmd {
	if m.whichKeyOff || m.seq.showPanel {
		return nil
	}
	if m.whichKeyDelay <= 0 {
		m.seq.showPanel = true
		return nil
	}
	gen := m.seq.gen
	return tea.Tick(m.whichKeyDelay, func(time.Time) tea.Msg { return whichKeyMsg{gen: gen} })
}

// routeSequence runs the leader / gg router (bt-i4k). handled=true means the
// key was consumed and must not be processed further.
func (m Model) routeSequence(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.activeModal != ModalNone || m.inTextInput() || keys.IsToken(msg.Text) {
		return m, nil, false
	}
	k := msg.String()

	if !m.seq.pending() {
		var prefix string
		switch k {
		case m.leaderKey:
			prefix = keys.LeaderPrefix
		case "g":
			prefix = "g"
		default:
			return m, nil, false
		}
		root := m.seqTree(prefix)
		if len(root.Children) == 0 {
			return m, nil, false
		}
		m.seq = seqState{prefix: prefix, node: root, gen: m.seq.gen + 1}
		m.swallowKey = true
		if prefix == "g" {
			return m, nil, true
		}
		return m, m.whichKeyTick(), true
	}

	switch k {
	case "esc":
		m.resetSeq()
		m.swallowKey = true
		return m, nil, true
	case "backspace":
		if len(m.seq.steps) == 0 {
			m.resetSeq()
		} else {
			m.seq.steps = m.seq.steps[:len(m.seq.steps)-1]
			node := m.seqTree(m.seq.prefix)
			for _, s := range m.seq.steps {
				node = node.Child(s)
			}
			if node == nil {
				m.resetSeq()
			} else {
				m.seq.node = node
				m.seq.gen++
			}
		}
		m.swallowKey = true
		return m, nil, true
	}

	child := m.seq.node.Child(k)
	switch {
	case child != nil && child.IsGroup():
		m.seq.node = child
		m.seq.steps = append(append([]string(nil), m.seq.steps...), k)
		m.seq.gen++
		m.swallowKey = true
		if m.seq.prefix == "g" {
			return m, nil, true
		}
		return m, m.whichKeyTick(), true
	case child != nil:
		m.resetSeq()
		m.swallowKey = true
		return m, actionCmd(child.ID), true
	case m.seq.prefix == keys.LeaderPrefix:
		seq := keys.FormatSeq(m.seq.prefix, append(append([]string(nil), m.seq.steps...), k), m.leaderLabel())
		m.resetSeq()
		m.swallowKey = true
		m.setStatus(seq + " is not bound")
		return m, nil, true
	}
	m.resetSeq()
	return m, nil, false
}
