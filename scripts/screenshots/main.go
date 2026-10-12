package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	restarted, err := restartForGlyphs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "capture screenshots:", err)
		os.Exit(1)
	}
	if restarted {
		return
	}
	output := flag.String("output", "docs/screenshots", "WebP output directory")
	fontPath := flag.String("font", "/usr/share/fonts/TTF/JetBrainsMonoNerdFontMono-Regular.ttf", "monospace Nerd Font TTF (Regular suffix discovers sibling styles)")
	fallback := flag.String("fallback-font", "/usr/share/fonts/TTF/DejaVuSansMono.ttf", "fallback TTF for Unicode symbols; empty disables fallback")
	flag.Parse()
	if err := run(*output, *fontPath, *fallback); err != nil {
		fmt.Fprintln(os.Stderr, "capture screenshots:", err)
		os.Exit(1)
	}
}

func isolateConfiguration(home string) error {
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home, "APPDATA": home,
		"LOCALAPPDATA": home, "XDG_CONFIG_HOME": home,
		"BT_NO_BROWSER": "1", "BT_TEST_MODE": "1", "BT_THEME": "",
	} {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

func restartForGlyphs() (bool, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("BT_GLYPHS")), "ascii") {
		return false, nil
	}
	// ui resolves glyphs during package initialization. Setting the variable
	// here is too late; start this same executable with a sanitized environment.
	executable, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate capture executable: %w", err)
	}
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "BT_GLYPHS=nerdfont")
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return true, fmt.Errorf("restart with default glyphs: %w", err)
	}
	return true, nil
}

func run(output, fontPath, fallback string) error {
	magick, err := exec.LookPath("magick")
	if err != nil {
		return fmt.Errorf("ImageMagick 7 (magick) is required: %w", err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolve output directory: %w", err)
	}
	work, err := filepath.Abs("_tmp/screenshots")
	if err != nil {
		return fmt.Errorf("resolve scratch directory: %w", err)
	}
	fonts, err := loadFonts(fontPath, fallback)
	if err != nil {
		return err
	}
	defer fonts.close()
	for _, dir := range []string{output, work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	home, err := os.MkdirTemp(work, "home-")
	if err != nil {
		return fmt.Errorf("create isolated configuration directory: %w", err)
	}
	if err := isolateConfiguration(home); err != nil {
		return err
	}
	// NewModel must not discover project themes, plugins, or runtime state.
	if err := os.Chdir(home); err != nil {
		return fmt.Errorf("isolate capture working directory: %w", err)
	}
	for _, view := range []struct{ name, action string }{
		{"01-list-detail", "list"}, {"02-insights", "insights"},
		{"03-kanban", "board"}, {"04-label-filter", "labels"},
	} {
		frame, err := captureFrame(view.action)
		if err != nil {
			return fmt.Errorf("capture %s: %w", view.name, err)
		}
		if err := os.WriteFile(filepath.Join(work, view.name+".ansi"), []byte(frame), 0o644); err != nil {
			return fmt.Errorf("save ANSI frame: %w", err)
		}
		img, err := renderFrame(frame, 160, 36, fonts)
		if err != nil {
			return fmt.Errorf("rasterize %s: %w", view.name, err)
		}
		pngPath := filepath.Join(work, view.name+".png")
		file, err := os.Create(pngPath)
		if err != nil {
			return fmt.Errorf("create PNG: %w", err)
		}
		encodeErr := png.Encode(file, img)
		closeErr := file.Close()
		if encodeErr != nil {
			return fmt.Errorf("encode PNG: %w", encodeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close PNG: %w", closeErr)
		}
		// Only replace a published asset after conversion succeeds. Failed
		// conversion artifacts are retained for inspection, never deleted.
		tmp, err := os.CreateTemp(output, ".capture-*.webp")
		if err != nil {
			return fmt.Errorf("create temporary WebP: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("close temporary WebP: %w", err)
		}
		cmd := exec.Command(magick, pngPath, "-define", "webp:lossless=true", tmp.Name())
		if detail, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("convert %s (temporary file retained at %s): %w: %s", view.name, tmp.Name(), err, detail)
		}
		destination := filepath.Join(output, view.name+".webp")
		if err := os.Rename(tmp.Name(), destination); err != nil {
			return fmt.Errorf("publish %s: %w", view.name, err)
		}
		fmt.Println("Wrote", destination)
	}
	return nil
}
