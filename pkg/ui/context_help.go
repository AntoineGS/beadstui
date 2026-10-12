package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ContextHelpContent contains compact help content for each context.
// This is used when user triggers context-specific help (e.g., double-tap backtick).
// Content should fit on one screen (~20 lines) without scrolling.
var ContextHelpContent = map[Context]string{
	ContextList:           contextHelpList,
	ContextGraph:          contextHelpGraph,
	ContextBoard:          contextHelpBoard,
	ContextInsights:       contextHelpInsights,
	ContextHistory:        contextHelpHistory,
	ContextDetail:         contextHelpDetail,
	ContextSplit:          contextHelpSplit,
	ContextFilter:         contextHelpFilter,
	ContextLabelPicker:    contextHelpLabelPicker,
	ContextRecipePicker:   contextHelpRecipePicker,
	ContextHelp:           contextHelpHelp,
	ContextTimeTravel:     contextHelpTimeTravel,
	ContextLabelDashboard: contextHelpLabelDashboard,
	ContextAttention:      contextHelpAttention,
	ContextAgentPrompt:    contextHelpAgentPrompt,
	ContextCassSession:    contextHelpCassSession,
}

// GetContextHelp returns the help content for a given context.
// Falls back to generic help if the context has no specific content.
func GetContextHelp(ctx Context) string {
	if content, ok := ContextHelpContent[ctx]; ok {
		return content
	}
	return contextHelpGeneric
}

// RenderContextHelp renders the context-specific help modal.
// This is a compact modal (~60 chars wide) that shows quick reference info.
func RenderContextHelp(ctx Context, theme Theme, width, height int) string {
	content := GetContextHelp(ctx)

	// Modal dimensions - compact
	modalWidth := 60
	if modalWidth > width-4 {
		modalWidth = width - 4
	}

	// Title
	titleStyle := theme.Text.Title

	// Content style
	contentStyle := theme.Text.Body

	// Footer hint
	footerStyle := theme.Text.Metadata

	// Build content
	var b strings.Builder
	b.WriteString(titleStyle.Render("Quick Reference"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(theme.Border).Render(strings.Repeat("─", modalWidth-4)))
	b.WriteString("\n\n")
	b.WriteString(contentStyle.Render(content))
	b.WriteString("\n\n")
	b.WriteString(footerStyle.Render("Esc to close · SPC T for tutorial"))

	// Wrap in modal style
	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Secondary).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}

// =============================================================================
// CONTEXT-SPECIFIC HELP CONTENT (bv-4swd)
// =============================================================================

const contextHelpList = `## List View

**Navigation**
  j/k       Move up/down
  Enter     View issue details
  gg/home   Jump to top
  G/end     Jump to bottom
  h/l       Move between list and details

**Search and Filters**
  /         Search issues
  :         BQL query
  SPC f o/c/r  Open/closed/ready filters
  SPC f l      Label picker
  SPC / m      Cycle search ranking

**Switch Views**
  SPC v b/g   Board / graph
  SPC v i/h   Insights / history
  SPC v a/f   Actionable / flow matrix

**Actions**
  e         Edit issue
  y         Copy issue ID
  q         Quit bt`

const contextHelpGraph = `## Graph View

**Navigation**
  j/k       Navigate nodes vertically
  h/l       Navigate siblings
  Enter     View selected issue
  s         Toggle swarm view
  Esc       Return to list

**Understanding the Graph**
• Arrows point TO what's blocked
  (A → B means A blocks B)
• Node size = priority
• Color = status
  Green=closed, Blue=in_progress`

const contextHelpBoard = `## Board View

**Navigation**
  h/l       Move between columns
  j/k       Move within column
  1-4       Jump to column by number
  H/L       Jump to first/last column
  gg/home   Go to top of column
  G/end     Go to bottom of column

**Filtering**
  SPC f o/c/r  Open/closed/ready filters

**Search**
  /         Start search
  n/N       Next/prev match

**Grouping**
  s         Cycle: Status/Priority/Type

**Visual Indicators** (card borders)
  ! Red     Has blockers
  * Yellow  High-impact (blocks others)
  o Green   Ready to work
**Actions**
  Tab       Toggle detail panel
  Ctrl+j/k  Scroll detail panel
  e         Edit issue
  y         Copy issue ID
  Enter     View issue details
  Esc       Return to List view`

const contextHelpInsights = `## Insights Panel

**Navigation**
  h/l       Switch between panels
  j/k       Move within panel
  Ctrl+j/k  Scroll detail section
  Tab       Next panel

**Heatmap** (Priority × Depth grid)
  m         Toggle heatmap view
  Arrows    Navigate cells
  Enter     Drill into cell

**Details**
  X         Toggle explanations
  x         Toggle calculations

**Attention Indicators**
• Stale: Open too long
• Blocked chains: Bottlenecks
• Priority inversions: Low blocking high

  Enter     View selected issue
  Esc       Return to list`

const contextHelpHistory = `## History View

**Navigation**
  j/k       Navigate primary pane
  J/K       Navigate secondary pane
  Tab       Cycle focus (list→detail→files)
  Enter     Jump to selected bead

**View Modes**
  v         Toggle Bead/Git mode
  f         Toggle file tree panel
  /         Search commits/beads
  c         Cycle confidence filter

**Causality Markers**
  @ Direct   Commit mentions bead ID
  ~ Temporal Within time window
  # File     Touches associated files

**Actions**
  y         Copy commit SHA
  o         Open commit in browser
  SPC b g   Show selected bead in graph
  Esc       Return to list`

const contextHelpDetail = `## Detail View

**Navigation**
  j/k       Scroll content
  Esc       Return to list
  Tab       Switch to split view

**Actions (from list view)**
  e         Edit issue
  y         Copy issue ID

**Info Shown**
• Full description (markdown)
• Dependencies
• Labels and metadata`

const contextHelpSplit = `## Split View

**Focus**
  Tab       Switch panes
  h/l       Move focus between panes
  SPC w <   Shrink list pane
  SPC w >   Expand list pane

**Left Pane (List)**
  j/k       Navigate issues

**Right Pane (Detail)**
  j/k       Scroll content

**Exit**
  Esc       Return to list view
  Enter     Open full detail

Tip: Detail updates as you navigate`

const contextHelpFilter = `## Filter Mode

**Status Filters**
  SPC f o   Open only
  SPC f c   Closed only
  SPC f r   Ready (no blockers)
  SPC f x   Clear filters

**Search**
  /         Start fuzzy search
  SPC / m   Cycle search ranking
  SPC / h   Cycle hybrid preset
  n/N       Next/prev match
  Esc       Clear search

**Label Filters**
  SPC f l   Open label picker`

const contextHelpLabelPicker = `## Label Picker

**Navigation**
  j/k       Move selection
  Enter     Apply label
  Space     Toggle multi-select
  Esc       Cancel

**Search**
  /         Filter labels

**Actions**
  q         Quit bt (outside search input)`

const contextHelpRecipePicker = `## Recipe Picker

**Navigation**
  j/k       Move selection
  Enter     Apply recipe
  Esc       Cancel

**Recipes**
Pre-configured filters and sorts:
• Sprint Ready
• Blocked Items
• By Priority
• Recently Updated`

const contextHelpHelp = `## Help Overlay

This panel lists direct keys for the
current view or modal. Leader-only keys
are shown by which-key from a view.

**Navigation**
  j/k       Scroll
  Ctrl+u/d  Scroll a page
  Home/g    Scroll to top
  G/end     Scroll to bottom
  Esc/?/F1  Close this overlay

Space opens the tutorial here. Close
this panel before using the SPC leader
menu in a view.`

const contextHelpTimeTravel = `## Time Travel Mode

**Currently Viewing**: Past state

This is read-only - you're viewing
how the project looked at a specific
point in history.

**Navigation**
  j/k       Navigate issues
  Enter     View issue detail

**Exit**
  Esc       Return to present

Tip: Use History view (SPC v h) to pick
different points in time`

const contextHelpLabelDashboard = `## Label Dashboard

**Overview**
Shows all labels with:
• Issue counts per label
• Health indicators
• Usage trends

**Navigation**
  j/k       Move selection
  Enter     Drill into label
  i         View label health
  d         Open label drilldown
  Esc       Return to list

**Filtering**
  /         Search labels`

const contextHelpAttention = `## Attention View

**Issues Needing Attention**

Sorted by attention score based on:
• Age (older = more attention)
• Priority mismatches
• Blocking factor
• Stale status

**Navigation**
  j/k       Move selection
  Enter     View issue
  1-9       Filter by label

Esc         Return to List view`

const contextHelpAgentPrompt = `## AI Agent Prompt

**Input**
Type your question or request
for the AI agent.

**Actions**
  Enter     Submit prompt
  Esc       Cancel
  Ctrl+C    Quit bt

**Examples**
• "Triage these issues"
• "What should I work on?"
• "Summarize blocked items"`

const contextHelpGeneric = `## Quick Reference

**Direct keys**
  ? / F1    Local key reference
  /         Search
  :         BQL query
  Esc       Cancel / go back
  q         Quit bt

**Navigation**
  j/k       Move up/down
  h/l       Move left/right
  Enter     Select/open

**Leader menu**
  SPC       Open which-key menu
  SPC v b   Switch to Board
  SPC v g   Switch to Graph
  Close, then SPC T for tutorial`

const contextHelpCassSession = `## Cass Session Preview

Shows coding sessions correlated with
the selected bead via cass search.

**Navigation**
  j/k       Move between sessions
  Enter     Expand session details
  Esc       Close modal

**Actions**
  y         Copy cass command
  o         Open session file

**Match Types**
  ID        Direct bead ID match
  File      Modified same files
  Title     Keyword similarity

Sessions ranked by relevance score.
Only shown when cass is installed.`
