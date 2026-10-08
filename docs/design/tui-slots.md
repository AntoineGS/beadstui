# TUI slots

`pkg/ui/slots` holds the TUI's in-process extension seams. A provider
contributes data; bt renders it with its own theme.

| Slot | Interface | Drawn by |
|---|---|---|
| Row badges | `BadgeProvider` | list delegate, board card line 1, tree node, epics child row, all through `renderBadgeStrip` |
| Detail sections | `SectionProvider` | `updateViewportContent`, right after the property block |
| BQL fields | `FieldProvider` | the BQL modal (`bqlFields`) and every BQL execution (`bqlExecuteOpts`) |

## Adding a built-in provider

Register it in `registerBuiltinSlots` (`pkg/ui/builtin_slots.go`). Built-ins
are registered before anything else, so their badges and sections come first.

```go
reg.AddBadges(slots.BadgeFunc(func(issue *model.Issue) []slots.Badge {
	if !issue.Status.IsOpen() {
		return nil
	}
	return []slots.Badge{{Text: "NEW", Tone: slots.ToneAccent}}
}))
```

## Layout rules

- Badges never take cells the title needs below `minTitleWidthWithBadges`
  (20), and all badges on a row share at most `maxBadgeStripWidth` (20) cells.
  Badges that do not fit are dropped from the end, so register the most
  important provider first. This holds for list, tree and epics rows. On
  board cards the title is on the second line, so badges use only what is left
  of the first line between the ID and the age.
- The list shows badges only above 80 columns, as it did for the overdue/stale
  badge they replaced. Epic rows and lane headers in the epics view show none.
- List badges live after the title in right-side columns. Column widths, including
  the compact ID, are shared across the current page; empty cells are padded and
  entirely empty optional columns disappear. Paging and filtering remeasure them.
  At tight widths, lower-value metadata is hidden to protect title space. The
  quick-win bolt retains its reserved right-edge cell.
- `Badge.Since` adds a compact age ("WAIT 4m"). `Badge.Style` is for built-ins
  with a house style; other providers use `Tone`.

## BQL fields

Provider fields are compared as case-insensitive strings (`=`, `!=`, `~`,
`!~`, `IN`, `NOT IN`, `ORDER BY`). A bead without a value matches no
comparison on that field. A provider cannot shadow a built-in field.

## Concurrency

`Registry` is safe for concurrent use and every read method is safe on a nil
registry. Providers are called from the render path: they must be fast and
must not block.
