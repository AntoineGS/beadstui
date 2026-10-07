package ui

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

func TestCompactAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{-5 * time.Minute, "<1m"}, // clock skew: Since in the future
		{30 * time.Second, "<1m"},
		{4 * time.Minute, "4m"},
		{59 * time.Minute, "59m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := compactAge(c.d); got != c.want {
			t.Errorf("compactAge(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestRenderBadgeStripFitsBudget(t *testing.T) {
	now := time.Now()
	badges := []slots.Badge{
		{Text: "WAIT", Tone: slots.ToneWarn, Since: now.Add(-4 * time.Minute)}, // "WAIT 4m" = 7 cells
		{Text: "REV", Tone: slots.ToneOK},                                      // " REV" = 4 more
	}

	strip, w := renderBadgeStrip(badges, 11, now)
	if got := ansi.Strip(strip); got != "WAIT 4m REV" || w != 11 {
		t.Fatalf("budget 11: %q (%d), want %q (11)", got, w, "WAIT 4m REV")
	}

	strip, w = renderBadgeStrip(badges, 10, now)
	if got := ansi.Strip(strip); got != "WAIT 4m" || w != 7 {
		t.Fatalf("budget 10: %q (%d), want %q (7)", got, w, "WAIT 4m")
	}

	strip, w = renderBadgeStrip(badges, 6, now)
	if strip != "" || w != 0 {
		t.Fatalf("budget 6: %q (%d), want empty", strip, w)
	}

	strip, w = renderBadgeStrip(nil, 50, now)
	if strip != "" || w != 0 {
		t.Fatalf("no badges: %q (%d), want empty", strip, w)
	}
}

func TestRenderBadgeStripCapsWidth(t *testing.T) {
	var badges []slots.Badge
	for i := 0; i < 10; i++ {
		badges = append(badges, slots.Badge{Text: "ABCD"})
	}
	_, w := renderBadgeStrip(badges, 200, time.Now())
	// Four badges fit under the cap: 4 + 3×(1 separator + 4).
	if w != 19 || w > maxBadgeStripWidth {
		t.Fatalf("strip width = %d, want 19 (cap %d)", w, maxBadgeStripWidth)
	}
}

func TestRenderBadgeStripMeasuresCells(t *testing.T) {
	// "⏰" is two cells wide; a rune count would think "⏰DUE" fits in 4.
	badges := []slots.Badge{{Text: "⏰DUE"}}
	if strip, w := renderBadgeStrip(badges, 4, time.Now()); strip != "" || w != 0 {
		t.Fatalf("2-cell glyph badge fit in 4 cells: %q (%d)", strip, w)
	}
	if _, w := renderBadgeStrip(badges, 5, time.Now()); w != 5 {
		t.Fatalf("width = %d, want 5", w)
	}
}

func TestBadgeStyleOverride(t *testing.T) {
	custom := lipgloss.NewStyle().Underline(true)
	got := badgeStyle(slots.Badge{Text: "X", Tone: slots.ToneError, Style: &custom})
	if !got.GetUnderline() {
		t.Fatal("Style override not used")
	}
	if fg := badgeStyle(slots.Badge{Tone: slots.ToneError}).GetForeground(); fg != ColorDanger {
		t.Fatalf("ToneError foreground = %v, want ColorDanger", fg)
	}
}

func TestRenderSlotSection(t *testing.T) {
	got := renderSlotSection(slots.Section{Title: "Agent", Markdown: "- one"})
	if want := "### Agent\n\n- one\n\n"; got != want {
		t.Fatalf("renderSlotSection = %q, want %q", got, want)
	}
}
