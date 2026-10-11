<!-- Related: bt-i4k, bt-i4k.1, bt-i4k.2, bt-i4k.3, bt-i4k.4, bt-i4k.5 -->
# Leader-key keymap redesign (bt-i4k)

Status: approved 2026-10-10. Supersedes in part ADR-004 Decision 1 (Global key
composition and the three-surface help table). ADR-004's data shape (per-view
`key.Binding` maps in `pkg/ui/keys`) stays.

## Problem

bt has about 240 bindings across 30 maps in `pkg/ui/keys/` plus about 146
raw `msg.String()` key comparisons that bypass the registry (inventory in the
appendix). Concretely:

1. Global single-letter view switches (`b g i h a f u l p x`) shadow view-local
   letters. Claim ended up on `m`, FilterAll was lost to `a`, history exits on `h`.
2. The same letter means different things per view: `s` is sort, swimlane, swarm,
   epics status, alert severity, or edit status. `g` is graph, top, `gg`, design,
   or jump-to-graph. `o` is open filter, expand all, browser, alert sort, or reopen.
3. Navigation is not uniform: top is `home`, `g`, `gg`, or `0` depending on the view.
4. Many shifted and number keys: `C O V U S R F E T H W`, `1`, `2`, `3`.

## Decisions (from the brainstorm)

| # | Decision |
|---|---|
| D1 | Neovim-style leader key, configurable, default `space`. |
| D2 | Strict direct-key set (layout A): only the keys in "Direct keys" below work without the leader. List filters and sort move behind the leader. |
| D3 | Any action can be promoted from the leader to a direct key (or moved, or unbound) in config. |
| D4 | `esc` only cancels: it backs out one level and does nothing at the base list. `q` only quits, immediately, whenever no text field has focus (including from list-style modals). The settings menu moves to `SPC ,`. |
| D5 | Plugins keep claiming free direct letters (they feel native) and `SPC x` opens the plugin action menu. Plugin keys show up in `?` and the footer like bt's own keys. |
| D6 | Conflict precedence: user config > bt bindings > plugin keys. Shadowed bindings are reported at startup and shown struck through in `?`. |
| D7 | which-key panel after a debounce (default 250ms, configurable); fast typers never see it. The leader sequence has no timeout. |
| D8 | `?` shows only local non-leader keys (direct + active view/modal + plugin). Leader keys are discovered through which-key only. |
| D9 | Remove the `;` shortcuts sidebar. Footer hints are trimmed to the active view's top direct keys plus `SPC` and `?`. |
| D10 | Architecture: action registry. Every binding has a stable action ID; one router turns key input into actions. |

## Architecture

### Action IDs and tokens (phase 1)

Every `key.Binding` field in `pkg/ui/keys` gets a struct tag:

```go
FilterOpen key.Binding `action:"filter.open"`
```

Every binding's key list includes the binding's **action token** `@<id>` (for
example `@filter.open`) in addition to its physical keys. bubbletea v2's
`KeyPressMsg.String()` returns `Text` when it is non-empty
(`ultraviolet Key.String`), so `tea.KeyPressMsg{Text: "@filter.open"}` matches
`key.Matches(msg, k.FilterOpen)`. The router dispatches completed leader
sequences, and any other action invocation, through `actionCmd(id)`: a `tea.Cmd`
that emits that synthetic message, so it runs the full `Update` (including the
forward to the bubbles list, whose `CursorUp`/`CursorDown` keys also carry the
`@nav.up`/`@nav.down` tokens). Tokens are never written by hand:
`NewAppKeys` calls `stampTokens`, which walks every tagged field by reflection
and appends its token. The roughly 250 existing `key.Matches` call sites keep
working unchanged, and a binding with no physical keys stays `Enabled()`
because its key list is never empty.

ID rules:

- Fields named `Up`, `Down`, `JumpTop`, `JumpBottom`, `PageUp`, `PageDown` use
  shared IDs `nav.up`, `nav.down`, `nav.top`, `nav.bottom`, `nav.page_up`,
  `nav.page_down` in every map. A shared ID names one conceptual action; config
  for it applies to every map that declares it.
- `GlobalKeys` and `ListNormalKeys` use the explicit IDs in the layout tables below.
- Every other field uses `<map-prefix>.<snake_case_field>`, with these prefixes:
  `tree`, `list_search`, `board`, `board_search`, `history`, `history_search`,
  `history_tree`, `insights`, `actionable`, `flow`, `graph`, `epics`, `epic_card`,
  `label_picker`, `label_picker_search`, `recipe_picker`, `bql`, `time_travel`,
  `settings`, `repo_picker`, `repo_picker_search`, `field_select`, `field_picker`,
  `field_input`, `longform`, `plugin_select`, `confirm`. Maps added in phase 2 pick
  a prefix the same way (`alerts`, `notifications`, `label_dashboard`,
  `label_health`, `label_drilldown`, `label_graph`, `attention`, `memories`,
  `memories_search`, `help`, `cass`, `agent_prompt`, `tutorial`).
- IDs are unique within a map. The same ID in two maps is allowed only for the
  shared `nav.*` IDs and for IDs a layout table assigns to several maps on purpose.

`(*AppKeys).Walk(fn func(Ref, *key.Binding))` visits every tagged binding by
reflection (`Ref{Map, Field, ID}`), handing out a pointer into `AppKeys` so
callers can rewrite keys with `SetKeys`. `(*AppKeys).Lookup(id)` returns every
binding with that ID. This is the whole registry; there is no separate index.

### Key notation inside `WithKeys`

| Form | Meaning |
|---|---|
| `j`, `ctrl+d`, `enter` | Direct key, matched by `key.Matches` as today. |
| `@filter.open` | Action token (exactly one per binding, always present). |
| `<leader>fo` | Leader sequence. After `<leader>`, each rune is one key; named keys are written in angle brackets (`<tab>`, `<space>`, `<ctrl+x>`), and the literal `<` and `>` keys are `<lt>` and `<gt>`, as in vim. Never equals a real `msg.String()`, so `key.Matches` ignores it. |
| `gg` | Two-key `g` prefix sequence (the only non-leader sequence). |

### Sequence router (phase 3)

New file `pkg/ui/sequence.go`. Model state:

```go
type seqState struct {
    prefix   string   // "<leader>" or "g"; "" when idle
    keys     []string // keys typed after the prefix
    gen      int      // bumps on every change; stale which-key ticks are ignored
    showPanel bool    // which-key panel visible
}
```

The router runs in `handleKeyPress` right after the modal early-returns and
before the global chord layer (`?`, refresh, ...), so a key typed mid-sequence
always belongs to the sequence. It runs only when `m.activeModal == ModalNone`
and `!m.inTextInput()` (list filtering, board search, history search, memories
search, time-travel input). The leader is therefore inactive in modals and in
text input, so `space` still toggles items in pickers. Keys the router consumes
set `m.swallowKey`, which stops `Update` from forwarding them to the bubbles
list afterwards.

1. Idle, key == leader: start a leader sequence and schedule
   `tea.Tick(whichKeyDelay, whichKeyMsg{gen})`. Delay 0 shows the panel at once.
2. Idle, key == `g`, in a non-modal view: start a `g` sequence (no panel).
3. Pending, `esc`: cancel. `backspace`: pop one key (or cancel when none are left).
4. Pending, other key: walk the active leader tree (built from the Global map
   plus the active view map's enabled bindings).
   - Group: descend. If the panel is visible it stays visible; otherwise reschedule the tick.
   - Leaf: reset, then return `actionCmd(id)`, which emits a key press
     carrying `@id`.
   - Miss: reset and show status `SPC f z is not bound`; the key is consumed.
     A `g` sequence miss resets silently and the second key is then handled
     normally (so `g` then `j` still moves down).
5. `whichKeyMsg{gen}` with a matching gen and a sequence still pending: set
   `showPanel = true`.

Leader groups (prefix -> label) are data in `pkg/ui/keys/leader.go`. An
unlabeled prefix created by user config renders as `+group`.

### which-key panel (phase 3)

New file `pkg/ui/which_key.go`. A full-width panel anchored above the footer
(non-dimming; drawn in the body area like the toast). The title is the pending
sequence and its group label (`SPC f  filter`). Entries are sorted by key, with
groups first, rendered as `<key>  +<label>` for groups and `<key>  <help desc>`
for leaves, in as many columns as fit. Height is capped at a third of the
screen, with a `…` row on overflow. While a sequence is pending, the footer
hint zone shows the sequence (`SPC f…`).

### Help surfaces (phase 4)

- `?` overlay: global direct keys plus the active view or modal map's bindings
  that have at least one direct key, plus plugin keys for the current view.
  Leader-only bindings are not listed. Shadowed bindings are listed struck
  through (phase 5). The footer of `?` reads `SPC  leader menu`.
- The `;` sidebar is removed (`shortcuts_sidebar.go`, its test, `Global.Sidebar`,
  and `Global.SidebarScrollDown/Up`), along with `focus`/layout code that
  reserves sidebar width.
- L1 footer hints: the active map's `ShortHelp()` filtered to bindings with
  direct keys (max 4), then `SPC leader`, then `? help`.

### Config (phase 5)

Read from the existing `~/.config/bt/config.yaml` (the file `pkg/plugin` reads
for `plugins:`), under a new `keys:` section:

```yaml
keys:
  leader: space            # any bubbletea key string: space, ",", "\\", ctrl+space
  which_key_delay: 250ms   # Go duration; 0 = immediately; "off" = never show
  bind:
    filter.open: o                     # promote to a direct key
    view.board: [b, "<leader>vb"]      # direct + leader
    app.refresh: none                  # unbind
```

- A `bind` entry replaces all default keys (direct and leader) of that action in
  every map declaring the ID. The action token is always kept.
- Unknown action IDs, malformed keys, or a bad duration produce errors. bt
  reports them on the status line at startup and ignores the bad entries; it
  never fails to start over keys config.
- Help keys (`Help().Key`) are regenerated from the resolved keys for every
  binding that config changed (arrow keys render as `↑ ↓ ← →`, ctrl as `⌃`).

### Conflict resolution (phase 5)

Two maps **overlap** when they are the same map, or when one is `Global` and the
other is a non-modal view map. Sub-state siblings (`ListNormal` and
`ListSearch`, `HistoryNormal` and `HistorySearch`) do not overlap because their
states are exclusive. Modal maps overlap only themselves.

When config binds key `K` (direct or leader sequence) to action A, every other
binding in a map overlapping one of A's maps loses `K`. A `Shadow{Action,
Key, By}` record is kept for each loss. Two config entries claiming the same key
in overlapping maps is a config error, and the first entry in sorted ID order
wins. A config leader sequence that is a prefix of a default group makes that
group's entries unreachable, and each one is recorded as shadowed.

Plugin keys lose to every bt binding in the plugin's view (today's `btBindsKeyIn`
rule). Plugin actions depend on the selected bead, so plugin shadows are
computed when `?` renders, for the current view and selection.

Reporting: on startup, if config produced any shadows or errors, a status line
reads `keys: 2 shadowed, 1 config error - see ?`. The `?` overlay has a
"Shadowed / errors" group listing config shadows, config errors, and the
current view's shadowed plugin keys, struck through.

## Default layout (phase 3)

### Direct keys

Active in every non-modal view unless noted.

| Key | Action ID | Action |
|---|---|---|
| `j k` `↓ ↑` | `nav.down` `nav.up` | move |
| `h l` `← →` | per view | left/right: list ↔ details pane, board column, tree collapse/expand, insights panel |
| `gg` `home` / `G` `end` | `nav.top` / `nav.bottom` | top / bottom in every cursor view (list, board, tree, epics, flow, label dashboard); replaces the single-`g` variants. A lone `g` never acts. |
| `ctrl+d` `pgdown` / `ctrl+u` `pgup` | `nav.page_down` / `nav.page_up` | page |
| `enter` | per view | open / drill in |
| `tab` | per view | cycle pane focus |
| `esc` | `app.cancel` | cancel / back one level; no-op at base list |
| `q` | `app.quit` | quit immediately when no text input has focus (also from modals) |
| `ctrl+c` | `app.force_quit` | quit, always (even while typing) |
| `/` | `app.search` | search (list filter; board/history/memories search; bounce from details) |
| `n` `N` | per view | next / prev match where search exists |
| `:` | `app.bql` | BQL query |
| `e` | `bead.edit` | edit selected bead (list, details, board, tree, epics) |
| `y` | `bead.copy_id` | copy ID (history: copy SHA) |
| `?` `f1` | `app.help` | local key help |
| `,` `.` | `list.detail_page_up` / `list.detail_page_down` | page the details pane (list view) |
| `space` | (leader) | leader |

`ctrl+r` and `f2`–`f6` are dropped (refresh is `SPC r`).

### Leader groups

| Seq | Action ID | Action | Old key | Scope map |
|---|---|---|---|---|
| `SPC v l` | `view.list` | list (leave the current view) | (esc) | Global |
| `SPC v b` | `view.board` | board (toggle) | `b` | Global |
| `SPC v g` | `view.graph` | graph (toggle) | `g` | Global |
| `SPC v t` | `view.tree` | tree (toggle) | `T` | Global |
| `SPC v e` | `view.epics` | epics (toggle) | `E` | Global |
| `SPC v i` | `view.insights` | insights (toggle) | `i` | Global |
| `SPC v h` | `view.history` | history (toggle) | `h` | Global |
| `SPC v a` | `view.actionable` | actionable (toggle) | `a` | Global |
| `SPC v f` | `view.flow` | flow matrix (toggle) | `f` | Global |
| `SPC v L` | `view.labels` | label dashboard (toggle) | `[` | Global |
| `SPC v A` | `view.attention` | attention (toggle) | `]` | Global |
| `SPC v m` | `view.memories` | memories (toggle) | `u` | Global |
| `SPC f o` | `filter.open` | open / all | `o` | Global (moved from ListNormal) |
| `SPC f c` | `filter.closed` | closed / all | `c` | Global (moved) |
| `SPC f r` | `filter.ready` | ready / all | `r` | Global (moved) |
| `SPC f s` | `filter.cycle_status` | cycle status filter | `#` | Global (moved) |
| `SPC f l` | `filter.labels` | label picker | `l` | Global |
| `SPC f R` | `filter.recipes` | recipe picker | `'` | Global |
| `SPC f x` | `filter.clear` | clear all filters (new; `clearAllFilters`) | (esc) | Global |
| `SPC s s` | `sort.cycle` | cycle sort | `s` | ListNormal |
| `SPC s S` | `sort.cycle_reverse` | cycle sort reverse | `S` | ListNormal |
| `SPC s t` | `sort.triage` | triage recipe | `R` | ListNormal |
| `SPC b m` | `bead.claim` | claim | `m` | ListNormal |
| `SPC b Y` | `bead.copy_issue` | copy full issue | `C` | ListNormal |
| `SPC b o` | `bead.open_editor` | open in editor | `O` | ListNormal |
| `SPC b x` | `bead.export` | export markdown | `x` | Global |
| `SPC b c` | `bead.cass` | cass session | `V` | ListNormal |
| `SPC b E` | `bead.epic_card` | epic card | `F` | ListNormal |
| `SPC b g` | `history.jump_to_graph` | show bead in graph | `g` (history) | HistoryNormal |
| `SPC p p` | `project.picker` | projects (workspace) / wisps | `w` | Global |
| `SPC p h` | `project.home_all` | home ↔ all projects | `W` | Global |
| `SPC p w` | `project.wisps` | toggle wisps | `ctrl+w` | Global |
| `SPC w h` / `SPC w l` | `window.focus_left` / `window.focus_right` | focus pane left/right | (`←`/`→`) | ListNormal |
| `SPC w <` / `SPC w >` (`<leader>w<lt>` / `<leader>w<gt>`) | `window.shrink` / `window.grow` | resize list pane | `<` `>` | ListNormal |
| `SPC w 1` / `SPC w 2` | `window.fullscreen_list` / `window.fullscreen_details` | fullscreen pane | `2` `3` | ListNormal |
| `SPC w d` | `window.debug_dims` | toggle dimension chip | `ctrl+p` (hidden) | Global |
| `SPC n n` | `notify.notifications` | notifications | `1` | Global |
| `SPC n a` | `notify.alerts` | alerts | `!` | Global |
| `SPC n h` | `notify.priority_hints` | priority hints | `p` | Global |
| `SPC / m` | `search.cycle_mode` | cycle search ranker | `ctrl+s` | Global |
| `SPC / h` | `search.hybrid_preset` | hybrid preset | `H` | Global |
| `SPC x` | `plugin.menu` | plugin action menu | `P` | Global |
| `SPC r` | `app.refresh` | refresh | `ctrl+r` `f5` | Global |
| `SPC t` | `app.time_travel` | time travel | `t` | ListNormal |
| `SPC ,` | `app.settings` | settings menu | (esc at base) | Global |
| `SPC T` | `app.tutorial` | tutorial | `` ` `` | Global |
| `SPC U` | `app.self_update` | self-update | `U` | ListNormal |
| `SPC q` | `app.quit` | quit | `q` | Global (leader key on the `app.quit` binding) |

Group labels: `v` view, `f` filter, `s` sort, `b` bead, `p` project, `w`
window, `n` notify, `/` search.

`SPC v l` from the list is a no-op. Every `view.*` toggle keeps today's
semantics (pressing it in its own view returns to the list).

### View-local keys

These stay direct, but only inside their view. They no longer collide with
anything because the global letters moved behind the leader.

| View | Keys (changes in bold) |
|---|---|
| List | universal keys only; `h`/`l` now move between list and details like `←`/`→` (**bubbles list paging moves to `pgup`/`pgdown` only**) |
| Board | `s` swimlane, `z` empty columns, `d` expand card, `H`/`L` first/last column, `1`–`4` column, `$` last in column, `0` first card, `tab`, `ctrl+j`/`ctrl+k` detail scroll; **`gg` handled by the router (board's `waitingForG` removed)** |
| Tree | `o`/`O` expand/collapse all; **`space` no longer toggles (`enter` only)**; **back is `esc` only (`T` removed)** |
| Epics | `s` status cycle, `v` zoom card, `z` collapse all |
| Graph | `s` swarm |
| Insights | `m` heatmap, `x` calc proof, `X` explanations, `ctrl+j`/`ctrl+k` |
| History | `v` mode, `f` file tree (**`F` alias dropped**), `c` confidence, `o` open in browser, `J`/`K` related; **exit is `esc` only (`h` removed)**; jump-to-graph moves to `SPC b g` |
| Flow matrix | **close is `esc` only (`f`, `q` removed)**; **top is `gg`/`home`** |
| Label dashboard | **`i` health detail (was `h`)**, `d` drilldown; close **`esc` only** |
| Attention | `1`–`9` filter by label; close **`esc` only** |
| Memories | `/`, `tab`, `enter`, `backspace` |

### Modals

Modals keep their own letters; the leader is inactive inside them. Changes:

- `q` quits the app from any modal without text focus (alerts, notifications,
  pickers in nav mode, cass, help, settings, settings menu, confirm, agent prompt,
  update, label drilldown/health/graph, epic card, field select, field picker,
  plugin select). The plugin prompt treats `q` like `ctrl+c` (it answers the
  plugin, then quits). Close keys that duplicated a view key (`l` label picker,
  `'` recipes, `w` repo picker, `V` cass, `!`/`1` alerts toggling closed, `g`
  label graph, `d`/`h` label modals) are removed; `esc` closes. `?`/`f1` still
  toggle the help overlay closed. `tab` still switches the alerts/notifications
  tabs.
- The quit-confirm modal is no longer reachable from `q`/`esc`.

## Phases

Each phase is one child bead, lands on `task/bt-i4k`, and passes `go build ./...`,
`go vet ./...`, `go test ./...` before the next starts.

1. **bt-i4k.1 Action registry.** Tags, tokens, `AppKeys.Walk`, `actionCmd`. No behavior change.
2. **bt-i4k.2 Raw-string migration.** New maps for every raw handler in the appendix; no behavior change.
3. **bt-i4k.3 Leader and layout.** Sequence router, which-key panel, default layout tables above, `q`/`esc` semantics, `e` beyond the list, tests rewritten to the new keys.
4. **bt-i4k.4 Help consolidation.** `?` local-only, sidebar removal, footer trim, plugin keys in `?`, tutorial/README/docs sweep, ADR-004 status note.
5. **bt-i4k.5 Config and conflicts.** `keys:` config, rebinding, help-key regeneration, conflict resolution, reporting.

## Testing

- `pkg/ui/keys`: every binding field tagged; IDs unique per map; every binding
  holds exactly one token; leader tree has no duplicate sequences and no leaf
  that is also a group; every leader leaf ID resolves.
- `pkg/ui`: a test helper `pressKeys(m, "space", "v", "b")` drives sequences.
  Layout table tests iterate the tables above and assert each sequence reaches
  the right action (by observable state). Router tests cover cancel, backspace,
  miss, and inactivity in modals and text input. which-key tests cover debounce
  (stale gen ignored, delay 0, panel stays visible on descend).
- Config tests: promote, move, unbind, unknown ID, bad duration, conflicts,
  shadow records, plugin shadowing.
- `readme_keys_test.go` keeps guarding README tables against the new layout.

## Out of scope

Mouse handling, robot mode, CLI flags, and remapping keys inside text inputs
(bubbles textinput/textarea editing keys).

## Appendix: inventory before the redesign (2026-10-10, main @ e824227e)

**Global** (`pkg/ui/keys/global.go`): `?`/F1 help, `;`/F2 sidebar, `^j`/`^k`
sidebar scroll, `` ` `` tutorial, `^c` quit, `q` back/quit, `esc` back/cancel →
settings menu; views `b` board, `g` graph, `i` insights, `h` history, `a`
actionable, `f` flow, `T` tree, `[`/F3 labels, `]`/F4 attention, `E` epics,
`u`/F6 memories; workspace `w` projects/wisps, `W` home/all, `^w` wisps, `H`
hybrid preset; actions `^r`/F5 refresh, `^s` search ranker, `:` BQL, `'`
recipes, `!` alerts, `1` notifications, `/` search, `p` priority hints, `x`
export, `l` label picker. Outside the registry: `P` plugin menu, `^p` debug dims.

**List** (`list.go`): `↑↓jk`, `home`, `G/end`, `^d^u`; filters `o c r #`;
`enter`, `F` epic card, `tab`, `←→` panes, `, .` detail page, `< >` resize,
`2 3` fullscreen; `e` edit, `s S` sort, `R` triage, `t` time travel, `y` copy
ID, `C` copy issue, `O` editor, `V` cass, `U` self-update, `m` claim. The
bubbles list also binds `h l b u f d` paging and `g` top.

**Board**: `hjkl`, `^d^u`, `home/0`, `G/end`, `gg`, `H L`, `1-4`, `$`, `/`, `n N`,
`d` expand, `z` empty columns, `s` swimlane, `y`, `tab`, `^j^k` detail, `enter`.
Search: `esc enter backspace n N`.

**Tree**: `hjkl`, `enter/space` toggle, `g G`, `o O`, `^d^u/pgdn/pgup`, `tab`, `T/esc`.

**Epics**: `hjkl`, `z`, `enter`, `s`, `v`, `esc`. **Epic card**: `jk`, `enter`, `esc`.

**Graph**: `hjkl`, `^d^u`, `enter`, `s`.

**Insights**: `h l tab` panels, `jk`, `^j^k`, `X`, `x`, `m`, `enter`, `esc`.

**History**: `jk`, `J K`, `^d^u/pgdn/pgup`, `tab`, `v`, `/`, `f F`, `y`, `o`,
`enter`, `g`, `c`, `h/esc`. Search: `enter esc`. File tree: `jk`, `enter/l`,
`h`, `tab`, `esc`.

**Actionable**: `jk`, `enter`. **Flow matrix**: `jk`, `home/g`, `end/G`, `tab`,
`enter`, `f/q/esc`; raw also `^d^u`.

**Label dashboard** (raw): `jk`, `home`, `G/end`, `enter`, `h` health detail,
`d` drilldown, `esc/q/[` close. **Label health** (raw): `esc q enter h` close,
`d` drilldown. **Label drilldown** (raw): `enter` apply, `g` graph analysis,
`esc q d` close. **Label graph** (raw): `esc q g`. **Attention** (raw): `1-9`,
`esc q d`.

**Alerts modal** (raw): `tab`, `!`, `1`, `jk`, `h l`, `→ ← pgdn pgup home end g
G`, `enter`, `c C`, `s S` severity, `t T` type, `p P` project, `o O` sort, `a A`
class, `r R` reset, `esc q`. **Notifications** (raw): `jk`, page keys, `enter`,
`c C`, `d` dismissed, `t T` kind, `r R`, `esc q`.

**Memories** (raw): `/`, `jk`, `tab`, `enter`, `backspace`; search `esc enter`.
**Help overlay** (raw): `l`, `jk`, `^d^u`, `home g`, `G end`, `q esc ? f1`,
`space` tutorial. **Tutorial** (raw): `t`, `jk`, `^d^u`, `g/home`, `G/end`,
`1-9`, `h/left`. **Cass** (raw): `jk`, `y`, `V esc enter q`. **Agent prompt**
(raw): `y n d`. **Update modal**: confirm/cancel + raw `q`.

**Modal maps**: confirm `enter y Y / esc n N`; label picker nav `jk`, `← →
pgup pgdn`, `/`, `space`, `enter`, `l`, `esc`; label picker search `↑ ↓ ^p ^n`,
`enter`, `esc`; recipe picker `jk enter ' esc`; settings `jk h l enter esc`; BQL
`↑ ↓ enter esc`; time travel `enter esc`; repo picker nav `jk`, `← → pgup pgdn`,
`/`, `space`, `a`, `enter`, `q w`, `esc`; repo picker search like label picker;
field select `jk enter s p t a d g c n A x o esc`; field picker `jk enter esc`;
field input `enter esc`; longform `^s ^e esc`; plugin select `jk enter esc` (+
raw `q`).
