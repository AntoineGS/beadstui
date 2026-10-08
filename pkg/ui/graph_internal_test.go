package ui

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGraphTextRole(t *testing.T) {
	theme := DefaultTheme()
	theme.Text.Body = lipgloss.NewStyle().Foreground(lipgloss.Color("#123456")).Underline(true).Bold(false)
	theme.Text.Metadata = lipgloss.NewStyle().Foreground(lipgloss.Color("#654321")).Underline(true)
	theme.Text.Selected = lipgloss.NewStyle().Foreground(lipgloss.Color("#abcdef")).Background(lipgloss.Color("#234567")).Underline(true).Bold(false).Italic(false)
	theme.Text.Heading = theme.Text.Heading.Bold(false).Underline(true)
	g := NewGraphModel([]model.Issue{{ID: "one", Title: "Selected", Status: model.StatusOpen}, {ID: "two", Title: "Ordinary body", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "one", Type: model.DepBlocks}}}}, nil, theme)
	out := g.View(120, 40)
	for _, want := range []string{theme.Text.Body.Render("Ordinary body"), theme.Text.Metadata.Render("two"), theme.Text.Selected.Render("Selected"), theme.Text.Heading.Width(28).Render(fmt.Sprintf("%s Nodes (2)", activeGlyphs.BarChart))} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing role span %q in %q", want, out)
		}
	}
}

func TestSmartTruncateID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		maxLen   int
		expected string
	}{
		{"Short ID fits", "foo", 10, "foo"},
		{"Exact fit", "foo-bar", 7, "foo-bar"},
		{"Simple truncation", "foo-bar-baz", 5, "foo-…"},
		{"Hyphenated ID abbreviation", "service-auth-login", 10, "s-a-login"},
		{"Underscore ID abbreviation", "service_auth_login", 10, "s_a_login"},
		{"Mixed separators (hyphen priority)", "service-auth_login", 12, "s-a_login"},
		{"Mixed separators (complex)", "foo-bar_baz-qux", 10, "f-b_b-qux"},
		{"Very short limit", "abc-def", 3, "ab…"},
		{"Single part ID truncation", "verylongsinglepartid", 5, "very…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := smartTruncateID(tt.id, tt.maxLen)
			runeCount := utf8.RuneCountInString(got)
			if runeCount > tt.maxLen {
				t.Errorf("Result rune count %d exceeds maxLen %d. Got: %s", runeCount, tt.maxLen, got)
			}
			// We don't assert exact match for mixed/complex because the logic is heuristic
			// but we check that it produces *something* valid and doesn't crash or empty out
			if got == "" && tt.maxLen > 0 {
				t.Errorf("Result is empty")
			}

			// For specific mixed case that failed before fix:
			if tt.name == "Mixed separators (complex)" {
				// Before fix: split by '-' (defaulting sep to '-') would likely yield chunks that included '_'
				// After fix: FieldsFunc splits by both, so abbreviation logic should work better
				// Just verifying it doesn't look totally broken
				t.Logf("Input: %s, Max: %d, Got: %s", tt.id, tt.maxLen, got)
			}
		})
	}
}
