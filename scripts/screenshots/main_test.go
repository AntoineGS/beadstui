package main

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/seanmartinsmith/beadstui/pkg/ui"
	"golang.org/x/image/font/basicfont"
)

func TestCaptureFrames(t *testing.T) {
	// Wrong action routing, an unsettled viewport, or the default open-only
	// board filter would remove these visible parts of the captured frames.
	t.Chdir(t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("BT_NO_BROWSER", "1")
	t.Setenv("BT_TEST_MODE", "1")
	t.Setenv("BT_THEME", "")
	for _, tc := range []struct{ view, want string }{
		{"list", "Acceptance criteria"},
		{"insights", "Betweenness Centrality"},
		{"board", "CLOSED (3)"},
		{"labels", "Filter by Label"},
	} {
		t.Run(tc.view, func(t *testing.T) {
			frame, err := captureFrame(tc.view)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ansi.Strip(frame), tc.want) {
				t.Fatalf("frame does not show %q", tc.want)
			}
			if _, err := renderFrame(frame, 160, 36, fontSet{normal: basicfont.Face7x13}); err != nil {
				t.Fatalf("captured frame cannot be rasterized: %v", err)
			}
			lines := strings.Split(frame, "\n")
			if len(lines) != 36 {
				t.Fatalf("got %d lines, want 36", len(lines))
			}
			for i, line := range lines {
				if width := ansi.StringWidth(line); width > 160 {
					t.Fatalf("line %d exceeds terminal width: %d", i+1, width)
				}
			}
		})
	}
}

func TestRenderFrameCellBackgrounds(t *testing.T) {
	// Losing ANSI reset handling or counting a wide grapheme as one cell
	// would place these background colors at the wrong pixel coordinates.
	f := fontSet{normal: basicfont.Face7x13}
	img, err := renderFrame("界\x1b[48;2;18;52;86mX\x1b[mY\nZ", 4, 2, f)
	if err != nil {
		t.Fatal(err)
	}
	if bounds := img.Bounds(); bounds.Dx() != 80 || bounds.Dy() != 84 {
		t.Fatalf("unexpected frame dimensions: %v", bounds)
	}
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{40, 20, color.RGBA{18, 52, 86, 255}},
		{50, 20, color.RGBA{29, 31, 33, 255}},
		{20, 42, color.RGBA{29, 31, 33, 255}},
	} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestRenderFrameRejectsInvalidFrames(t *testing.T) {
	// Silently clipping oversized frames or ignoring unsupported SGR would
	// publish misleading screenshots instead of reporting the export problem.
	for _, tc := range []struct {
		name, content string
		cols, rows    int
	}{
		{"columns", "abc", 2, 1},
		{"rows", "a\nb", 1, 1},
		{"empty size", "", 0, 1},
		{"malformed color", "\x1b[38;2;1mX", 1, 1},
		{"unsupported color", "\x1b[38;5;2mX", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := renderFrame(tc.content, tc.cols, tc.rows, fontSet{normal: basicfont.Face7x13}); err == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
}

func TestRenderFrameFaintBackdrop(t *testing.T) {
	// Truecolor modal backdrops can use SGR 2. Rejecting it blocks capture;
	// dropping it would incorrectly show an undimmed background.
	f := fontSet{normal: basicfont.Face7x13}
	normal, err := renderFrame("XX", 2, 1, f)
	if err != nil {
		t.Fatal(err)
	}
	dimmed, err := renderFrame("\x1b[2mX\x1b[22mX", 2, 1, f)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for y := 20; y < 42; y++ {
		for x := 20; x < 40; x++ {
			if normal.RGBAAt(x, y) != (color.RGBA{197, 200, 198, 255}) {
				continue
			}
			want := color.RGBA{98, 100, 99, 255}
			if x >= 30 {
				want = color.RGBA{197, 200, 198, 255}
			}
			if got := dimmed.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no foreground pixels checked")
	}
}

func TestConfigurationIsolation(t *testing.T) {
	// Omitting Windows home/config variables must not expose a user's theme
	// when the exact same capture command is used on Windows.
	for _, name := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "BT_THEME", "BT_NO_BROWSER", "BT_TEST_MODE"} {
		t.Setenv(name, "inherited")
	}
	home := t.TempDir()
	if err := isolateConfiguration(home); err != nil {
		t.Fatal(err)
	}
	if actual, err := os.UserHomeDir(); err != nil || actual != home {
		t.Fatalf("home = %q, %v; want %q", actual, err, home)
	}
	for _, name := range []string{"USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME"} {
		if got := os.Getenv(name); got != home {
			t.Errorf("%s = %q, want isolated %q", name, got, home)
		}
	}
	if got := os.Getenv("BT_THEME"); got != "" {
		t.Errorf("inherited theme remains: %q", got)
	}
}

func TestGlyphIsolationProcess(t *testing.T) {
	// Changing BT_GLYPHS after importing ui cannot reset its init-time glyph
	// selection. Only a real child process can test this isolation boundary.
	if os.Getenv("SCREENSHOT_GLYPH_HELPER") == "1" {
		restarted, err := restartForGlyphs()
		if err != nil {
			t.Fatal(err)
		}
		if restarted {
			return
		}
		for _, r := range ui.Glyphs().TypeBug {
			if r > 127 {
				fmt.Println("default-glyphs-loaded")
				return
			}
		}
		t.Fatal("inherited ASCII glyphs were not isolated before package initialization")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BT_GLYPHS", "ascii")
	t.Setenv("SCREENSHOT_GLYPH_HELPER", "1")
	cmd := exec.Command(executable, "-test.run=^TestGlyphIsolationProcess$", "-test.count=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("glyph isolation process: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "default-glyphs-loaded") {
		t.Fatalf("no default glyphs observed: %s", output)
	}
}
