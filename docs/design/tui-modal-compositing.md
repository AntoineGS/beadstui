# TUI modal compositing

All modal overlays must dim the backdrop via `OverlayCenterDimBackdrop`
(in `pkg/ui/panel.go`). This makes the modal read as a true pop-up
rather than a content-shaped panel embedded in the underlying view, and
is the unified aesthetic across alerts, pickers, and prompts (bt-v8he,
bt-o1hs).

The non-dim `OverlayCenter` is reserved for non-modal overlays only -
debug panels, transient hints, or anything where the user is meant to
keep reading the underlying view through the overlay.

## Adding a new modal

1. Expose the modal's content as a `View()` method on its struct. Use
   `MeasurePopup` to get the body budget, build raw body/footer lines, and
   draw the frame with `RenderPopup` (no centering or overlay logic).
   Selectable rows use `MeasurePopupMenu` over the whole filtered menu,
   then `RenderPopupMenu` over the visible page.
2. In `pkg/ui/model_view.go`, leave the `activeModal` switch case as a
   fall-through comment ("Handled as overlay after background renders
   (below)") so the underlying view renders into `body` first.
3. Add an overlay block at the bottom of `View()` that calls
   `OverlayCenterDimBackdrop(body, m.foo.View(), m.width, m.height-1)`.

This matches the alerts/notifications modal shape and keeps a single
canonical compositor for modal pop-ups.

## Shared layout

`PopupOpts.Width`/`Height` are preferred **outer** dimensions, including
the border. `PopupLayout.BodyWidth`/`BodyHeight` are the available widget
and content dimensions after padding and footer reservation. Pass the real
terminal budget in `Available`: nil means not sized yet (80x24 default),
while an explicit zero width or height renders nothing. Do not pass the
ordinary-view minimum-height floor to a popup.

Use `BodyX`/`BodyY` and `FooterY` for mouse coordinates; do not duplicate
border/padding offsets. Compact fallback panels have no clickable body.
Forward resize messages to the active popup's `SetSize` without recreating
its text buffers or rerunning actions. `Model.View` alone places the popup
using `OverlayCenterDimBackdrop`; rendering and placement remain separate.

Ordinary panels continue using `RenderTitledPanel`. Nonmodal overlays
continue using `OverlayCenter`. See [shared popup rendering](popup-rendering.md)
for the full frame and menu contract.
