package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTextRolesFieldOverrides(t *testing.T) {
	var overlay ThemeFile
	if err := yaml.Unmarshal([]byte(`text:
  title: {foreground: auto, background: primary, bold: false}
  metadata: {italic: false, underline: true}
  nonexistent: {bold: true}
`), &overlay); err != nil {
		t.Fatal(err)
	}
	base := &ThemeFile{Text: defaultTextRoleConfigs()}
	mergeTheme(base, &overlay)
	if base.Text.Title.Bold == nil || *base.Text.Title.Bold {
		t.Fatal("explicit false did not disable title bold")
	}
	if base.Text.Metadata.Underline == nil || !*base.Text.Metadata.Underline {
		t.Fatal("valid underline override was lost")
	}
	if base.Text.Body.Foreground != "text" {
		t.Fatal("unspecified body role did not inherit")
	}
}

func TestTextRolesInvalidFieldsAreIndependent(t *testing.T) {
	for _, invalid := range []string{
		"foreground: typo", "foreground: [text]", "background: auto",
		"bold: absolutely", "italic: [false]", "underline: {bad: true}",
		"foreground: none", "foreground: '#ffffff'", "foreground: null",
		"bold: 'false'", "bold: null", "foreground: ''",
	} {
		t.Run(invalid, func(t *testing.T) {
			var overlay ThemeFile
			input := "text:\n  title:\n    " + invalid + "\n    background: secondary\n"
			if strings.HasPrefix(invalid, "background:") {
				input = "text:\n  title:\n    " + invalid + "\n    italic: true\n"
			}
			if err := yaml.Unmarshal([]byte(input), &overlay); err != nil {
				t.Fatal(err)
			}
			base := &ThemeFile{Text: defaultTextRoleConfigs()}
			mergeTheme(base, &overlay)
			if strings.HasPrefix(invalid, "background:") {
				if base.Text.Title.Background != "primary" || base.Text.Title.Italic == nil || !*base.Text.Title.Italic {
					t.Fatal("bad field erased a valid neighbor or inherited background")
				}
			} else {
				if base.Text.Title.Background != "secondary" || base.Text.Title.Foreground != "auto" || base.Text.Title.Bold == nil || !*base.Text.Title.Bold {
					t.Fatal("bad field erased valid or inherited fields")
				}
			}
		})
	}
}

func TestTextRolesLayerPrecedence(t *testing.T) {
	t.Chdir(t.TempDir())
	user := withThemeConfigHome(t)
	t.Setenv("BT_THEME", "dracula")
	if err := os.MkdirAll(filepath.Dir(user), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".bt", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("text:\n  title: {bold: false}\n  metadata: {italic: true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".bt/theme.yaml", []byte("text:\n  metadata: {italic: false, underline: true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tf := range []*ThemeFile{LoadTheme(), LoadThemeNamed("matcha-dark-sea")} {
		if tf.Text.Title.Bold == nil || *tf.Text.Title.Bold || tf.Text.Metadata.Italic == nil || *tf.Text.Metadata.Italic || tf.Text.Metadata.Underline == nil || !*tf.Text.Metadata.Underline {
			t.Fatal("named/user/project precedence lost false or inherited fields")
		}
		if tf.Text.Body.Foreground != "text" {
			t.Fatal("embedded role was lost")
		}
	}
}

func TestTextRolesParseAllRolesAndFields(t *testing.T) {
	for _, name := range []string{"title", "heading", "body", "metadata", "badge", "selected", "callout"} {
		t.Run(name, func(t *testing.T) {
			var overlay ThemeFile
			input := "text:\n  " + name + ": {foreground: warning, background: none, bold: false, italic: true, underline: true, unknown: [bad]}\n"
			if err := yaml.Unmarshal([]byte(input), &overlay); err != nil {
				t.Fatal(err)
			}
			base := &ThemeFile{Text: defaultTextRoleConfigs()}
			mergeTheme(base, &overlay)
			roles := map[string]TextRoleConfig{
				"title": base.Text.Title, "heading": base.Text.Heading, "body": base.Text.Body,
				"metadata": base.Text.Metadata, "badge": base.Text.Badge,
				"selected": base.Text.Selected, "callout": base.Text.Callout,
			}
			got := roles[name]
			if got.Foreground != "warning" || got.Background != "none" || got.Bold == nil || *got.Bold || got.Italic == nil || !*got.Italic || got.Underline == nil || !*got.Underline {
				t.Fatalf("role fields did not merge: %+v", got)
			}
		})
	}
}

func TestTextRolesNonMappingNodes(t *testing.T) {
	for _, node := range []string{"null", "plain", "[text]", "true"} {
		t.Run(node, func(t *testing.T) {
			var overlay ThemeFile
			if err := yaml.Unmarshal([]byte("text:\n  title: "+node+"\n  body: {underline: true}\n"), &overlay); err != nil {
				t.Fatal(err)
			}
			base := &ThemeFile{Text: defaultTextRoleConfigs()}
			mergeTheme(base, &overlay)
			if base.Text.Title.Foreground != "auto" || base.Text.Title.Background != "primary" || !*base.Text.Title.Bold || !*base.Text.Body.Underline {
				t.Fatal("invalid role erased defaults or a valid neighboring role")
			}
		})
	}
}

func TestTextRolesProgrammaticMergeValidationAndOwnership(t *testing.T) {
	base := &ThemeFile{Text: defaultTextRoleConfigs()}
	bold := false
	overlay := &ThemeFile{Text: TextRoleConfigs{Title: TextRoleConfig{
		Foreground: "typo", Background: "auto", Bold: &bold,
	}}}
	mergeTheme(base, overlay)
	bold = true
	if base.Text.Title.Foreground != "auto" || base.Text.Title.Background != "primary" || *base.Text.Title.Bold {
		t.Fatal("merge accepted invalid references or retained an overlay pointer")
	}
	other := defaultTextRoleConfigs()
	*base.Text.Body.Italic = true
	if *other.Body.Italic || *base.Text.Metadata.Italic {
		t.Fatal("default attributes share mutable pointers")
	}
}

func TestTextRolesSupportedColorReferences(t *testing.T) {
	tokens := []string{"bg", "bg_dark", "bg_subtle", "bg_highlight", "text", "subtext", "muted", "primary", "secondary", "info", "success", "warning", "danger", "text_secondary", "bg_contrast", "border", "highlight"}
	for _, token := range tokens {
		var overlay ThemeFile
		if err := yaml.Unmarshal([]byte("text:\n  body: {foreground: "+token+", background: "+token+"}\n"), &overlay); err != nil {
			t.Fatal(err)
		}
		base := &ThemeFile{Text: defaultTextRoleConfigs()}
		mergeTheme(base, &overlay)
		if base.Text.Body.Foreground != token || base.Text.Body.Background != token {
			t.Errorf("supported token %q was lost", token)
		}
	}
}

func TestTextRolesEmbeddedAndFallbackDefaults(t *testing.T) {
	for _, roles := range []TextRoleConfigs{loadEmbeddedTheme().Text, defaultTextRoleConfigs()} {
		for _, tc := range []struct {
			role   TextRoleConfig
			fg, bg string
			bold   bool
		}{
			{roles.Title, "auto", "primary", true}, {roles.Heading, "primary", "none", true},
			{roles.Body, "text", "none", false}, {roles.Metadata, "subtext", "none", false},
			{roles.Badge, "auto", "secondary", true}, {roles.Selected, "auto", "highlight", true},
			{roles.Callout, "auto", "bg_highlight", true},
		} {
			if tc.role.Foreground != tc.fg || tc.role.Background != tc.bg || tc.role.Bold == nil || *tc.role.Bold != tc.bold || tc.role.Italic == nil || *tc.role.Italic || tc.role.Underline == nil || *tc.role.Underline {
				t.Fatalf("incorrect defaults for %s/%s: %+v", tc.fg, tc.bg, tc.role)
			}
		}
	}
}

func TestLoadTheme_EmbeddedDefaults(t *testing.T) {
	// Isolate from the developer's real ~/.config/bt/theme.yaml. LoadTheme
	// deliberately reads it, so without this the assertion below depends on
	// whatever palette the person running the test has picked -- which is a
	// machine-dependent test, not a failing feature. Latent until bt-4ibsq
	// gave the picker the ability to create that file.
	withThemeConfigHome(t)
	t.Setenv("BT_THEME", "")

	tf := LoadTheme()
	if tf == nil {
		t.Fatal("LoadTheme returned nil")
	}
	// Embedded defaults should have primary teal color.
	//
	// This is matcha-dark-sea's actual hi_fg (bt-o6xx1). The previous value
	// #8abeb7 was labelled "matcha-dark-sea teal" in theme.go but is Tomorrow
	// Night's teal; the hand-copy never took matcha's own accent. Now that the
	// theme is loaded rather than transcribed, the real one applies.
	if tf.Colors.Primary == nil {
		t.Fatal("embedded theme should have primary color")
	}
	if tf.Colors.Primary.Dark != "#2eb398" {
		t.Errorf("expected primary dark #2eb398, got %s", tf.Colors.Primary.Dark)
	}
}

func TestApplyThemeToGlobals(t *testing.T) {
	// Save originals
	origPrimary := ColorPrimary
	defer func() { ColorPrimary = origPrimary }()

	tf := &ThemeFile{
		Colors: ThemeColors{
			Primary: &AdaptiveHex{Dark: "#ff0000", Light: "#00ff00"},
		},
	}

	// In dark mode, the resolved color should be the dark value
	origDark := isDarkBackground
	isDarkBackground = true
	defer func() { isDarkBackground = origDark }()

	ApplyThemeToGlobals(tf)

	// Verify the resolved color is correct for dark mode
	r, g, b, _ := ColorPrimary.RGBA()
	if r>>8 != 0xff || g>>8 != 0x00 || b>>8 != 0x00 {
		t.Errorf("expected dark mode color #ff0000, got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

func TestApplyThemeToGlobals_Nil(t *testing.T) {
	// Should not panic
	ApplyThemeToGlobals(nil)
}

func TestApplyThemeToThemeStruct(t *testing.T) {
	origDark := isDarkBackground
	isDarkBackground = true
	defer func() { isDarkBackground = origDark }()

	theme := DefaultTheme()

	tf := &ThemeFile{
		Colors: ThemeColors{
			Primary: &AdaptiveHex{Dark: "#aabbcc"},
		},
	}
	ApplyThemeToThemeStruct(&theme, tf)

	// In dark mode, should be the overridden dark value
	r, g, b, _ := theme.Primary.RGBA()
	if r>>8 != 0xaa || g>>8 != 0xbb || b>>8 != 0xcc {
		t.Errorf("expected primary #aabbcc in dark mode, got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

func TestAdaptiveHex_ToColor(t *testing.T) {
	// Full override
	hex := AdaptiveHex{Dark: "#111111", Light: "#222222"}
	origDark := isDarkBackground
	defer func() { isDarkBackground = origDark }()

	isDarkBackground = true
	result := hex.toColor("#aaa", "#bbb")
	r, g, b, _ := result.RGBA()
	if r>>8 != 0x11 || g>>8 != 0x11 || b>>8 != 0x11 {
		t.Errorf("dark mode full override failed: got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}

	isDarkBackground = false
	result = hex.toColor("#aaa", "#bbb")
	r, g, b, _ = result.RGBA()
	if r>>8 != 0x22 || g>>8 != 0x22 || b>>8 != 0x22 {
		t.Errorf("light mode full override failed: got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}

	// Partial override (dark only) - light should use fallback
	hex = AdaptiveHex{Dark: "#333333"}
	isDarkBackground = true
	result = hex.toColor("#aaaaaa", "#bbbbbb")
	r, g, b, _ = result.RGBA()
	if r>>8 != 0x33 || g>>8 != 0x33 || b>>8 != 0x33 {
		t.Errorf("partial dark override failed: got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}

	isDarkBackground = false
	result = hex.toColor("#aaaaaa", "#bbbbbb")
	r, g, b, _ = result.RGBA()
	if r>>8 != 0xaa || g>>8 != 0xaa || b>>8 != 0xaa {
		t.Errorf("partial light fallback failed: got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}

	// Empty (no override) - should use fallback
	hex = AdaptiveHex{}
	isDarkBackground = true
	result = hex.toColor("#aaaaaa", "#bbbbbb")
	r, g, b, _ = result.RGBA()
	if r>>8 != 0xbb || g>>8 != 0xbb || b>>8 != 0xbb {
		t.Errorf("empty should use dark fallback: got #%02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

func TestMergeTheme_PartialOverride(t *testing.T) {
	base := &ThemeFile{
		Colors: ThemeColors{
			Primary: &AdaptiveHex{Dark: "#111111", Light: "#222222"},
			Info:    &AdaptiveHex{Dark: "#333333", Light: "#444444"},
		},
	}
	overlay := &ThemeFile{
		Colors: ThemeColors{
			Primary: &AdaptiveHex{Dark: "#aaaaaa"}, // Only override dark
		},
	}
	mergeTheme(base, overlay)

	if base.Colors.Primary.Dark != "#aaaaaa" {
		t.Errorf("expected dark #aaaaaa, got %s", base.Colors.Primary.Dark)
	}
	if base.Colors.Primary.Light != "#222222" {
		t.Errorf("expected light #222222 (unchanged), got %s", base.Colors.Primary.Light)
	}
	// Info should be untouched
	if base.Colors.Info.Dark != "#333333" {
		t.Errorf("expected info dark #333333 (unchanged), got %s", base.Colors.Info.Dark)
	}
}

func TestLoadThemeFile_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(badFile, []byte("{{invalid yaml"), 0644)

	result := loadThemeFile(badFile)
	if result != nil {
		t.Error("malformed YAML should return nil")
	}
}

func TestLoadThemeFile_ValidPartial(t *testing.T) {
	tmpDir := t.TempDir()
	partial := filepath.Join(tmpDir, "theme.yaml")
	os.WriteFile(partial, []byte(`
colors:
  primary: { dark: "#ff79c6" }
`), 0644)

	result := loadThemeFile(partial)
	if result == nil {
		t.Fatal("valid partial YAML should not return nil")
	}
	if result.Colors.Primary == nil {
		t.Fatal("primary should be parsed")
	}
	if result.Colors.Primary.Dark != "#ff79c6" {
		t.Errorf("expected #ff79c6, got %s", result.Colors.Primary.Dark)
	}
	// Light not specified - should be empty
	if result.Colors.Primary.Light != "" {
		t.Errorf("light should be empty, got %s", result.Colors.Primary.Light)
	}
}

func TestLoadThemeFile_Nonexistent(t *testing.T) {
	result := loadThemeFile("/nonexistent/path/theme.yaml")
	if result != nil {
		t.Error("nonexistent file should return nil")
	}
}
