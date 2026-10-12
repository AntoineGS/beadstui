package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

func sidebarTestTheme() Theme {
	return Theme{
		Primary:   lipgloss.Color("#00ff00"),
		Secondary: lipgloss.Color("#888888"),
		Base:      lipgloss.NewStyle(),
	}
}

// TestShortcutsSidebar_RendersBindingGroups verifies the sidebar renders the
// key + desc of every enabled binding fed to it via SetBindings, i.e. it is a
// FullHelp() consumer rather than a hardcoded string table (bt-ift6.10).
func TestShortcutsSidebar_RendersBindingGroups(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 40)
	sidebar.SetBindings([][]key.Binding{
		{
			key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move up")),
			key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move down")),
		},
		{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open detail")),
		},
	})

	view := sidebar.View()
	for _, want := range []string{"↑/k", "move up", "↓/j", "move down", "⏎", "open detail"} {
		if !strings.Contains(view, want) {
			t.Errorf("sidebar view missing %q\n%s", want, view)
		}
	}
}

// TestShortcutsSidebar_SkipsDisabledAndEmptyBindings verifies disabled bindings
// and bindings with no help key are omitted from the rendered sidebar.
func TestShortcutsSidebar_SkipsDisabledAndEmptyBindings(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 40)

	disabled := key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "disabled action"))
	disabled.SetEnabled(false)
	noHelp := key.NewBinding(key.WithKeys("x")) // no WithHelp -> empty Help.Key
	enabled := key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "enabled action"))

	sidebar.SetBindings([][]key.Binding{{disabled, noHelp, enabled}})

	view := sidebar.View()
	if strings.Contains(view, "disabled action") {
		t.Errorf("sidebar should skip disabled bindings\n%s", view)
	}
	if !strings.Contains(view, "enabled action") {
		t.Errorf("sidebar should render enabled bindings\n%s", view)
	}
}

// TestShortcutsSidebar_RendersOnlySuppliedBindings keeps this renderer
// standalone: callers provide groups directly, and Model no longer owns it.
func TestShortcutsSidebar_RendersOnlySuppliedBindings(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 20)
	sidebar.SetBindings([][]key.Binding{{
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "board")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "cycle sort")),
	}})

	view := sidebar.View()
	if !strings.Contains(view, "board") || !strings.Contains(view, "cycle sort") {
		t.Fatalf("standalone renderer omitted supplied bindings:\n%s", view)
	}
}

// TestShortcutsSidebar_CallerCanReplaceGroups confirms the standalone utility
// has no dependency on active Model or modal state.
func TestShortcutsSidebar_CallerCanReplaceGroups(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 20)
	sidebar.SetBindings([][]key.Binding{{
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "board")),
	}})
	sidebar.SetBindings([][]key.Binding{{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply recipe")),
	}})

	view := sidebar.View()
	if !strings.Contains(view, "apply recipe") || strings.Contains(view, "board") {
		t.Fatalf("standalone renderer did not replace the supplied groups:\n%s", view)
	}
}

// TestShortcutsSidebar_EmptyViewFallback verifies the standalone renderer's
// fallback when the active view has no view-specific bindings (bt-dx7k).
func TestShortcutsSidebar_EmptyViewFallback(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 20)
	sidebar.SetBindings(nil) // empty-view (e.g. Attention / LabelDashboard)

	view := sidebar.View()
	if !strings.Contains(view, "?") {
		t.Errorf("empty-view sidebar should direct to ? for global\n%s", view)
	}
	if !strings.Contains(strings.ToLower(view), "no actions") {
		t.Errorf("empty-view sidebar should state there are no view actions\n%s", view)
	}
}

// TestShortcutsSidebar_CrossRefFooter verifies the standalone renderer's
// footer cross-references the ? overlay (bt-dx7k).
func TestShortcutsSidebar_CrossRefFooter(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(34, 20)
	sidebar.SetBindings([][]key.Binding{
		{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open detail"))},
	})

	view := sidebar.View()
	if !strings.Contains(view, "?") {
		t.Errorf("standalone renderer footer should cross-reference ? for global\n%s", view)
	}
}

func TestNewShortcutsSidebar(t *testing.T) {
	theme := DefaultTheme()
	sidebar := NewShortcutsSidebar(theme)

	if sidebar.width != 34 {
		t.Errorf("Expected width 34, got %d", sidebar.width)
	}
}

func TestShortcutsSidebarScrolling(t *testing.T) {
	theme := DefaultTheme()
	sidebar := NewShortcutsSidebar(theme)

	// Initial scroll offset should be 0
	if sidebar.scrollOffset != 0 {
		t.Errorf("Expected initial scroll 0, got %d", sidebar.scrollOffset)
	}

	// Scroll down
	sidebar.ScrollDown()
	if sidebar.scrollOffset != 1 {
		t.Errorf("Expected scroll 1 after ScrollDown, got %d", sidebar.scrollOffset)
	}

	// Scroll up
	sidebar.ScrollUp()
	if sidebar.scrollOffset != 0 {
		t.Errorf("Expected scroll 0 after ScrollUp, got %d", sidebar.scrollOffset)
	}

	// Scroll up at top should stay at 0
	sidebar.ScrollUp()
	if sidebar.scrollOffset != 0 {
		t.Errorf("Expected scroll 0 at top, got %d", sidebar.scrollOffset)
	}

	// Page down
	sidebar.ScrollPageDown()
	if sidebar.scrollOffset != 10 {
		t.Errorf("Expected scroll 10 after PageDown, got %d", sidebar.scrollOffset)
	}

	// Page up
	sidebar.ScrollPageUp()
	if sidebar.scrollOffset != 0 {
		t.Errorf("Expected scroll 0 after PageUp, got %d", sidebar.scrollOffset)
	}

	// Reset
	sidebar.scrollOffset = 5
	sidebar.ResetScroll()
	if sidebar.scrollOffset != 0 {
		t.Errorf("Expected scroll 0 after Reset, got %d", sidebar.scrollOffset)
	}
}

func TestShortcutsSidebarView(t *testing.T) {
	sidebar := NewShortcutsSidebar(sidebarTestTheme())
	sidebar.SetSize(28, 30)
	sidebar.SetBindings([][]key.Binding{
		{key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "open detail"))},
	})

	view := sidebar.View()
	if view == "" {
		t.Error("Expected non-empty view")
	}

	// Should contain the panel title (rendered in the top border).
	if !strings.Contains(view, "Shortcuts") {
		t.Error("Expected view to contain 'Shortcuts'")
	}

	// Should render the fed binding's desc.
	if !strings.Contains(view, "open detail") {
		t.Error("Expected view to render the supplied binding desc")
	}
}

func TestShortcutsSidebarWidth(t *testing.T) {
	theme := DefaultTheme()
	sidebar := NewShortcutsSidebar(theme)

	if sidebar.Width() != 34 {
		t.Errorf("Expected Width() = 34, got %d", sidebar.Width())
	}
}
