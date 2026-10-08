package ui

import (
	"fmt"
	"strings"

	"github.com/seanmartinsmith/beadstui/pkg/recipe"

	"charm.land/lipgloss/v2"
)

// RecipePickerModel represents the recipe picker overlay
type RecipePickerModel struct {
	recipes       []recipe.Recipe
	selectedIndex int
	width         int
	height        int
	theme         Theme
	popupSize     *PopupSize
}

// NewRecipePickerModel creates a new recipe picker
func NewRecipePickerModel(recipes []recipe.Recipe, theme Theme) RecipePickerModel {
	return RecipePickerModel{
		recipes:       recipes,
		selectedIndex: 0,
		theme:         theme,
	}
}

// SetSize updates the picker dimensions
func (m *RecipePickerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.popupSize = &PopupSize{width, height}
}

// MoveUp moves selection up
func (m *RecipePickerModel) MoveUp() {
	if m.selectedIndex > 0 {
		m.selectedIndex--
	}
}

// MoveDown moves selection down
func (m *RecipePickerModel) MoveDown() {
	if m.selectedIndex < len(m.recipes)-1 {
		m.selectedIndex++
	}
}

// SelectedRecipe returns the currently selected recipe
func (m *RecipePickerModel) SelectedRecipe() *recipe.Recipe {
	if len(m.recipes) == 0 || m.selectedIndex >= len(m.recipes) {
		return nil
	}
	return &m.recipes[m.selectedIndex]
}

// SelectedIndex returns the current selection index
func (m *RecipePickerModel) SelectedIndex() int {
	return m.selectedIndex
}

// View renders the recipe picker panel. Composited into the view via
// OverlayCenterDimBackdrop in model_view.go (bt-vklk Phase 1 + bt-rhfo) so
// callers do not center it again. Returns the rendered titled panel; the
// dimmed-backdrop centering is the compositor's job.
//
// Panel height is capped at ~70% of the body height (matches the alerts modal
// pop-up sizing). When the recipe count exceeds the visible window, the
// viewport scrolls to keep the selected recipe in view and "↑ N more" /
// "↓ N more" indicators surface the truncated edges. Without the cap, an
// 11-recipe list grew to 38 rows and ate the entire body height, leaving no
// surrounding bg for OverlayCenterDimBackdrop to dim — the user perceived
// this as "the dim layer is invisible" (bt-rhfo dogfood, 2026-05-07).
func (m *RecipePickerModel) View() string {
	entries := make([]PopupMenuEntry, len(m.recipes))
	shapeRows := 2
	for i, r := range m.recipes {
		entries[i] = PopupMenuEntry{Label: r.Name, Detail: r.Description, Selected: i == m.selectedIndex}
		shapeRows += PopupMenuEntryRows(entries[i])
	}
	menu := MeasurePopupMenu(entries, PopupMenuOpts{})
	minimum := 3
	if len(entries) > 0 {
		minimum = 2 + PopupMenuEntryRows(entries[m.selectedIndex])
	} else {
		shapeRows++
	}
	opts := PopupOpts{Title: "Select Recipe", Theme: m.theme, Available: m.popupSize, Width: 50, Height: max(8, popupAvailableSize(m.popupSize).Height*7/10), MinBodyRows: minimum, Footer: []string{"j/k: navigate  enter: apply  esc: cancel", "j/k enter esc"}}
	shape := make([]string, shapeRows)
	shape[0] = strings.Repeat(" ", menu.Width)
	l := MeasurePopup(shape, opts)
	if l.Compact || l.Height == 0 {
		return RenderPopup(shape, opts)
	}
	start, end := popupMenuWindowRange(entries, m.selectedIndex, l.BodyHeight-2)
	lines := []string{""}
	hint := lipgloss.NewStyle().Foreground(m.theme.Subtext).Italic(true)
	if start > 0 {
		lines[0] = hint.Render(fmt.Sprintf("↑ %d more", start))
	}
	if len(entries) == 0 {
		lines = append(lines, "No recipes available")
	} else {
		lines = append(lines, RenderPopupMenu(entries[start:end], menu, m.theme, l.BodyWidth)...)
	}
	for len(lines) < l.BodyHeight-1 {
		lines = append(lines, "")
	}
	bottom := ""
	if end < len(entries) {
		bottom = hint.Render(fmt.Sprintf("↓ %d more", len(entries)-end))
	}
	lines = append(lines, bottom)
	opts.Width, opts.Height, opts.MinBodyRows = l.Width, l.Height, l.BodyHeight
	return RenderPopup(lines, opts)
}

// RecipeCount returns the number of recipes
func (m *RecipePickerModel) RecipeCount() int {
	return len(m.recipes)
}

// FormatRecipeInfo returns a formatted string for the active recipe display
func FormatRecipeInfo(r *recipe.Recipe) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("Recipe: %s", r.Name)
}
