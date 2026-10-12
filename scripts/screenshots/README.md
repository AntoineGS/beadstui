# Regenerate README screenshots

Run from the repository root:

```bash
go run -mod=readonly ./scripts/screenshots
```

This replaces the four WebP assets in `docs/screenshots/`: list/detail,
insights, board, and the label picker. It captures the current `ui.Model` at
160 columns by 36 rows with synthetic demo issues. No interactive `bt`
process or beads/Dolt connection is needed.

## Requirements

- The Go version required by `go.mod` (currently 1.26+).
- ImageMagick 7, with `magick` on `PATH` and WebP support.
- A monospace Nerd Font TTF. The default is JetBrains Mono Nerd Font Mono
  at `/usr/share/fonts/TTF/JetBrainsMonoNerdFontMono-Regular.ttf`.
- A fallback font for Unicode symbols. The default is
  `/usr/share/fonts/TTF/DejaVuSansMono.ttf`.

On another system, pass your font paths:

```bash
go run -mod=readonly ./scripts/screenshots \
  -font /path/to/JetBrainsMonoNerdFontMono-Regular.ttf \
  -fallback-font /path/to/DejaVuSansMono.ttf
```

If the primary filename ends in `Regular.ttf`, the command also looks for
`Bold.ttf`, `Italic.ttf`, and `BoldItalic.ttf` siblings. Missing sibling
styles use the regular face. Use `-fallback-font ''` to disable fallback
if your primary font covers every displayed symbol.

Preview without replacing the published assets:

```bash
go run -mod=readonly ./scripts/screenshots -output _tmp/screenshots/preview
```

## Capture behavior

The command isolates its working directory and configuration under
`_tmp/screenshots/`, so local themes, keybindings, plugins, and runtime data
do not affect the images. It waits for graph analysis and resize settling,
then switches views through the application's action router. The board
shows all statuses, including closed issues.

Both Unix and Windows home/configuration variables point to the isolated
directory. If `BT_GLYPHS=ascii` is inherited, the command restarts itself
with the default Nerd Font glyph tier before capturing any frames, because
the UI resolves that setting during package initialization.

ANSI frames are rasterized on a fixed terminal-cell grid to preserve
Unicode alignment and footer hints. ImageMagick converts the PNGs to
lossless WebP. Each published asset is replaced only after its conversion
succeeds. Intermediate ANSI frames, PNGs, and isolated configuration
directories remain under `_tmp/screenshots/` for inspection. Issue ages are
relative to capture time; timestamps will change when images are regenerated.

The capture command runs only the analysis waiter from the model's `Init`
batch. If initialization changes and the waiter is renamed or removed, the
command fails explicitly rather than running updater checks or timers.

## Tests

```bash
go test -mod=readonly ./scripts/screenshots
```

Tests check view routing, populated board columns, frame bounds, and ANSI
background/reset handling. They do not require ImageMagick or installed fonts.
