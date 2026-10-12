package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type fontSet struct{ normal, bold, italic, boldItalic, fallback font.Face }

func (f fontSet) close() {
	for _, face := range []font.Face{f.normal, f.bold, f.italic, f.boldItalic, f.fallback} {
		if face != nil {
			_ = face.Close()
		}
	}
}

func loadFonts(path, fallback string) (fontSet, error) {
	load := func(path string) (font.Face, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read font %s: %w", path, err)
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse font %s: %w", path, err)
		}
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 16, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return nil, fmt.Errorf("create font face: %w", err)
		}
		return face, nil
	}
	var f fontSet
	var err error
	f.normal, err = load(path)
	if err != nil {
		return f, err
	}
	for suffix, target := range map[string]*font.Face{"Bold": &f.bold, "Italic": &f.italic, "BoldItalic": &f.boldItalic} {
		if !strings.HasSuffix(path, "Regular.ttf") {
			continue
		}
		sibling := strings.TrimSuffix(path, "Regular.ttf") + suffix + ".ttf"
		if _, err := os.Stat(sibling); os.IsNotExist(err) {
			continue
		}
		*target, err = load(sibling)
		if err != nil {
			f.close()
			return fontSet{}, err
		}
	}
	if fallback != "" {
		f.fallback, err = load(fallback)
		if err != nil {
			f.close()
			return fontSet{}, err
		}
	}
	return f, nil
}

// renderFrame preserves the TUI's grapheme cell positions instead of relying
// on proportional SVG text layout, which can clip Unicode footer hints.
func renderFrame(content string, cols, rows int, fonts fontSet) (*image.RGBA, error) {
	if cols <= 0 || rows <= 0 || cols > 1000 || rows > 1000 || fonts.normal == nil {
		return nil, fmt.Errorf("invalid frame dimensions or missing font")
	}
	defaultFG, defaultBG := color.RGBA{197, 200, 198, 255}, color.RGBA{29, 31, 33, 255}
	frame := image.NewRGBA(image.Rect(0, 0, cols*10+40, rows*22+40))
	draw.Draw(frame, frame.Bounds(), image.NewUniform(defaultBG), image.Point{}, draw.Src)
	fg, bg, style, underline := defaultFG, defaultBG, 0, false
	faint := false
	x, y, state := 0, 0, byte(0)
	parser := ansi.NewParser()
	for len(content) > 0 {
		seq, width, n, nextState := ansi.DecodeSequence(content, state, parser)
		if n == 0 {
			return nil, fmt.Errorf("ANSI parser made no progress")
		}
		content, state = content[n:], nextState
		switch {
		case strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m"):
			codes := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m"), ";")
			for i := 0; i < len(codes); i++ {
				code := 0
				if codes[i] != "" {
					var err error
					code, err = strconv.Atoi(codes[i])
					if err != nil {
						return nil, fmt.Errorf("parse SGR code: %w", err)
					}
				}
				switch code {
				case 0:
					fg, bg, style, underline = defaultFG, defaultBG, 0, false
					faint = false
				case 1:
					style |= 1
				case 2:
					faint = true
				case 3:
					style |= 2
				case 4:
					underline = true
				case 22:
					style &^= 1
					faint = false
				case 23:
					style &^= 2
				case 24:
					underline = false
				case 39:
					fg = defaultFG
				case 49:
					bg = defaultBG
				case 38, 48:
					if i+4 >= len(codes) || codes[i+1] != "2" {
						return nil, fmt.Errorf("expected truecolor SGR, got %q", seq)
					}
					var rgb [3]uint8
					for channel := range rgb {
						value, err := strconv.ParseUint(codes[i+2+channel], 10, 8)
						if err != nil {
							return nil, fmt.Errorf("parse truecolor channel: %w", err)
						}
						rgb[channel] = uint8(value)
					}
					c := color.RGBA{rgb[0], rgb[1], rgb[2], 255}
					if code == 38 {
						fg = c
					} else {
						bg = c
					}
					i += 4
				default:
					return nil, fmt.Errorf("unsupported SGR code %d", code)
				}
			}
		case seq == "\n":
			x = 0
			y++
		case width > 0:
			if x+width > cols || y >= rows {
				return nil, fmt.Errorf("frame exceeds %dx%d terminal at cell %d,%d", cols, rows, x, y)
			}
			cell := image.Rect(20+x*10, 20+y*22, 20+(x+width)*10, 20+(y+1)*22)
			draw.Draw(frame, cell, image.NewUniform(bg), image.Point{}, draw.Src)
			face := fonts.normal
			switch style {
			case 1:
				if fonts.bold != nil {
					face = fonts.bold
				}
			case 2:
				if fonts.italic != nil {
					face = fonts.italic
				}
			case 3:
				if fonts.boldItalic != nil {
					face = fonts.boldItalic
				}
			}
			if fonts.fallback != nil {
				for _, r := range seq {
					if _, ok := face.GlyphAdvance(r); !ok {
						face = fonts.fallback
						break
					}
				}
			}
			origin := fixed.I(cell.Min.X) + (fixed.I(width*10)-font.MeasureString(face, seq))/2
			ink := fg
			if faint {
				ink.R, ink.G, ink.B = ink.R/2, ink.G/2, ink.B/2
			}
			drawer := font.Drawer{Dst: frame, Src: image.NewUniform(ink), Face: face, Dot: fixed.Point26_6{X: origin, Y: fixed.I(cell.Min.Y + 17)}}
			drawer.DrawString(seq)
			if underline {
				draw.Draw(frame, image.Rect(cell.Min.X, cell.Max.Y-3, cell.Max.X, cell.Max.Y-2), image.NewUniform(ink), image.Point{}, draw.Src)
			}
			x += width
		case seq != "":
			return nil, fmt.Errorf("unsupported terminal sequence %q", seq)
		}
	}
	return frame, nil
}
