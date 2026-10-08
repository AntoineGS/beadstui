package ui

// Why hex literals appear in this file (and theme_loader.go / styles.go):
//
// pkg/ui/defaults/theme.yaml is the source-of-truth for color tokens at
// runtime. The Go constants in DefaultTheme() below intentionally MIRROR
// that YAML so the app still renders themed correctly when the embedded
// YAML cannot be loaded — embed failures, future build configurations
// that strip embeds, tests that bypass the loader, and partial-override
// scenarios where the user's overlay omits a key (loader falls back to
// these defaults via getDefaults() / themeColorDefaults).
//
// If you change a default color, change it in BOTH the YAML AND the
// Go-fallback constants. Do not dedupe these by removing the Go side
// without first proving the embedded YAML is loadable in every supported
// build configuration. See bt-pxbc audit (docs/audits/architecture/
// 2026-05-03-theme-system.md) for the full layer hierarchy.
//
// Render code in pkg/ui/ should NOT introduce new hex literals — use
// the Color* package vars (styles.go) or theme fields. Hex literals
// outside the three documented files (this file, theme_loader.go,
// styles.go) are presumed bugs and tracked under the bt-pxbc audit.

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// isDarkBackground defaults to true (dark theme). Updated at runtime via
// tea.BackgroundColorMsg for proper light/dark detection. We intentionally
// avoid calling lipgloss.HasDarkBackground() at init time because it
// queries the terminal and can hang in non-TTY environments (tests, CI, pipes).
var isDarkBackground = true

// TermProfile holds the detected terminal color profile. Computed once at
// package init so every style helper can branch without re-detecting.
var TermProfile colorprofile.Profile

func init() {
	TermProfile = colorprofile.Detect(os.Stdout, os.Environ())
}

// ThemeBg returns the given hex color for TrueColor terminals and
// lipgloss.NoColor{} otherwise, so 16/256-color terminals use the
// terminal's own background instead of a down-converted approximation
// that may clash with palettes like Solarized.
func ThemeBg(hex string) color.Color {
	if TermProfile < colorprofile.TrueColor {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(hex)
}

// ThemeFg returns the given hex color for ANSI256+ terminals and a safe
// ANSI white (color 7) for 16-color or lower terminals.
func ThemeFg(hex string) color.Color {
	if TermProfile < colorprofile.ANSI256 {
		return lipgloss.ANSIColor(7)
	}
	return lipgloss.Color(hex)
}

// resolveColor picks the dark or light hex color based on isDarkBackground
// and returns a resolved color.Color via lipgloss.Color().
func resolveColor(light, dark string) color.Color {
	if isDarkBackground {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

type Theme struct {
	// Colors - resolved color.Color values (not adaptive)
	Bg, BgDark, BgSubtle, BgHighlight    color.Color
	TextColor, TextSecondary, BgContrast color.Color
	Primary                              color.Color
	Secondary                            color.Color
	Subtext                              color.Color

	// Accents (map to Color* globals)
	Info    color.Color
	Success color.Color
	Warning color.Color
	Danger  color.Color

	// Status
	Open       color.Color
	InProgress color.Color
	Blocked    color.Color
	Deferred   color.Color
	Pinned     color.Color
	Hooked     color.Color
	Closed     color.Color
	Tombstone  color.Color
	Review     color.Color

	// Types
	Bug     color.Color
	Feature color.Color
	Task    color.Color
	Epic    color.Color
	Chore   color.Color

	// UI Elements
	Border    color.Color
	Highlight color.Color
	Muted     color.Color

	// Styles
	Text     TextStyles
	Base     lipgloss.Style
	Selected lipgloss.Style
	Column   lipgloss.Style
	Header   lipgloss.Style

	// Pre-computed delegate styles (bv-o4cj optimization)
	// These are created once at startup instead of per-frame
	MutedText         lipgloss.Style // Age (unedited), muted info
	MutedTextItalic   lipgloss.Style // Age (edited since creation)
	InfoText          lipgloss.Style // Comments
	InfoBold          lipgloss.Style // Search scores
	SecondaryText     lipgloss.Style // ID, assignee
	PrimaryBold       lipgloss.Style // Selection indicator
	PriorityUpArrow   lipgloss.Style // Priority hint up
	PriorityDownArrow lipgloss.Style // Priority hint down
	TriageStar        lipgloss.Style // Gold quick-win marker
	TriageUnblocks    lipgloss.Style // Unblocks indicator
	TriageUnblocksAlt lipgloss.Style // Secondary unblocks
}

// DefaultTheme returns the Tomorrow Night theme with colors resolved
// for the current isDarkBackground state.
func DefaultTheme() Theme {
	t := Theme{
		Bg:            resolveColor("#ffffff", "#1d1f21"),
		BgDark:        resolveColor("#f0f0f0", "#191b1d"),
		BgSubtle:      resolveColor("#efefef", "#282a2e"),
		BgHighlight:   resolveColor("#d6d6d6", "#373b41"),
		TextColor:     resolveColor("#4d4d4c", "#c5c8c6"),
		TextSecondary: resolveColor("#333333", "#e8e8e8"),
		BgContrast:    resolveColor("#ffffff", "#1d1f21"),
		// Tomorrow Night palette + matcha-dark-sea teal accent
		Primary:   resolveColor("#3e999f", "#8abeb7"), // Teal
		Secondary: resolveColor("#8e908c", "#969896"), // Comment gray
		Subtext:   resolveColor("#8e908c", "#b4b7b4"), // Lighter muted

		Info:    resolveColor("#4271ae", "#81a2be"), // Blue
		Success: resolveColor("#718c00", "#b5bd68"), // Green
		Warning: resolveColor("#f5871f", "#de935f"), // Orange
		Danger:  resolveColor("#c82829", "#cc6666"), // Red

		Open:       resolveColor("#718c00", "#b5bd68"), // Green
		InProgress: resolveColor("#4271ae", "#81a2be"), // Blue
		Blocked:    resolveColor("#c82829", "#cc6666"), // Red
		Deferred:   resolveColor("#f5871f", "#de935f"), // Orange
		Pinned:     resolveColor("#4271ae", "#7aa6da"), // Blue variant
		Hooked:     resolveColor("#3e999f", "#8abeb7"), // Teal
		Closed:     resolveColor("#8e908c", "#969896"), // Gray
		Tombstone:  resolveColor("#c5c8c6", "#373b41"), // Muted
		Review:     resolveColor("#8959a8", "#b294bb"), // Purple

		Bug:     resolveColor("#c82829", "#cc6666"), // Red
		Feature: resolveColor("#f5871f", "#de935f"), // Orange
		Epic:    resolveColor("#8959a8", "#b294bb"), // Purple
		Task:    resolveColor("#eab700", "#f0c674"), // Yellow
		Chore:   resolveColor("#4271ae", "#81a2be"), // Blue

		Border:    resolveColor("#d6d6d6", "#373b41"),
		Highlight: resolveColor("#d6d6d6", "#373b41"),
		Muted:     resolveColor("#8e908c", "#969896"),
	}

	t.rebuildTextStyles(TextRoleConfigs{})
	return t
}

// TextStyles caches the semantic text roles resolved from a Theme's own palette.
type TextStyles struct {
	Title, Heading, Body, Metadata, Badge, Selected, Callout lipgloss.Style
}

func (t Theme) textPaletteColor(token string) color.Color {
	switch token {
	case "bg":
		return t.Bg
	case "bg_dark":
		return t.BgDark
	case "bg_subtle":
		return t.BgSubtle
	case "bg_highlight":
		return t.BgHighlight
	case "text":
		return t.TextColor
	case "subtext":
		return t.Subtext
	case "muted":
		return t.Muted
	case "primary":
		return t.Primary
	case "secondary":
		return t.Secondary
	case "info":
		return t.Info
	case "success":
		return t.Success
	case "warning":
		return t.Warning
	case "danger":
		return t.Danger
	case "text_secondary":
		return t.TextSecondary
	case "bg_contrast":
		return t.BgContrast
	case "border":
		return t.Border
	case "highlight":
		return t.Highlight
	default:
		return lipgloss.NoColor{}
	}
}

// rebuildTextStyles starts afresh so attributes from a previous config cannot leak.
func (t *Theme) rebuildTextStyles(config TextRoleConfigs) {
	roles := defaultTextRoleConfigs()
	mergeTextRoles(&roles, config)
	// A partial Theme can lack palette fields. Contrast must still use an
	// opaque background and never produce an invisible foreground.
	bgFallback := t.Bg
	if bgFallback == nil {
		bgFallback = resolveColor("#ffffff", "#1d1f21")
	}
	if _, none := bgFallback.(lipgloss.NoColor); none {
		bgFallback = resolveColor("#ffffff", "#1d1f21")
	}
	build := func(c TextRoleConfig) lipgloss.Style {
		bg := t.textPaletteColor(c.Background)
		contrastBg := bg
		if contrastBg == nil {
			contrastBg = bgFallback
			bg = lipgloss.NoColor{}
		}
		if _, none := contrastBg.(lipgloss.NoColor); none {
			contrastBg = bgFallback
		}
		fg := t.textPaletteColor(c.Foreground)
		_, invisible := fg.(lipgloss.NoColor)
		if c.Foreground == "auto" || fg == nil || invisible {
			fg = autoTextForeground(contrastBg)
		}
		return lipgloss.NewStyle().Foreground(fg).Background(bg).
			Bold(c.Bold != nil && *c.Bold).Italic(c.Italic != nil && *c.Italic).
			Underline(c.Underline != nil && *c.Underline)
	}
	t.Text = TextStyles{
		Title: build(roles.Title), Heading: build(roles.Heading), Body: build(roles.Body),
		Metadata: build(roles.Metadata), Badge: build(roles.Badge),
		Selected: build(roles.Selected), Callout: build(roles.Callout),
	}
	t.Base = t.Text.Body

	t.Selected = t.Text.Selected.
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(t.Primary).
		PaddingLeft(1)

	t.Header = t.Text.Title.Padding(0, 1)

	// Pre-computed delegate styles (bv-o4cj optimization).
	//
	// Derived from this Theme's own fields, NOT from the Color* package
	// globals. Reading the globals here made DefaultTheme() return a struct
	// whose color fields were these literals while its styles reflected
	// whatever theme had last been loaded, so its output depended on whether
	// anything had called ApplyThemeToGlobals yet. That is the mutable-global
	// coupling bt-zq6z identified; it surfaced as TestGraphView_GoldenASCII
	// passing alone and failing in-suite once the embedded default named a
	// btop palette (bt-o6xx1).
	t.MutedText = lipgloss.NewStyle().Foreground(t.Muted)
	t.MutedTextItalic = lipgloss.NewStyle().Foreground(t.Muted).Italic(true)
	t.InfoText = lipgloss.NewStyle().Foreground(t.Info)
	t.InfoBold = lipgloss.NewStyle().Foreground(t.Info).Bold(true)
	t.SecondaryText = lipgloss.NewStyle().Foreground(t.Secondary)
	t.PrimaryBold = lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	t.PriorityUpArrow = lipgloss.NewStyle().Foreground(t.Danger).Bold(true)
	t.PriorityDownArrow = lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	// TriageStar has no semantic Color* counterpart yet; closest is yellow
	// (ColorPrioMedium). Kept as ThemeFg literal for now — see bt-pxbc
	// audit follow-up #2 (promote to YAML token or alias).
	t.TriageStar = lipgloss.NewStyle().Foreground(ThemeFg("#f0c674"))
	t.TriageUnblocks = lipgloss.NewStyle().Foreground(t.Success)
	t.TriageUnblocksAlt = lipgloss.NewStyle().Foreground(t.Secondary)

}

func (t Theme) GetStatusColor(s string) color.Color {
	switch s {
	case "open":
		return t.Open
	case "in_progress":
		return t.InProgress
	case "blocked":
		return t.Blocked
	case "deferred":
		return t.Deferred
	case "pinned":
		return t.Pinned
	case "hooked":
		return t.Hooked
	case "closed":
		return t.Closed
	case "tombstone":
		return t.Tombstone
	default:
		return t.Subtext
	}
}

func (t Theme) GetTypeIcon(typ string) (string, color.Color) {
	switch typ {
	case "bug":
		return activeGlyphs.TypeBug, t.Bug
	case "feature":
		return activeGlyphs.TypeFeature, t.Feature
	case "task":
		return activeGlyphs.TypeTask, t.Task
	case "epic":
		return activeGlyphs.TypeEpic, t.Epic
	case "chore":
		return activeGlyphs.TypeChore, t.Chore
	default:
		return activeGlyphs.Bullet, t.Subtext
	}
}
