<!-- Related: bt-i4k, bt-i4k.1, bt-i4k.2, bt-i4k.3, bt-i4k.4, bt-i4k.5 -->
# Leader-key keymap redesign: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace bt's flat single-letter keymap with a neovim-style layout: a configurable leader (default `space`), a small set of direct keys, leader groups discoverable through a debounced which-key panel, and config-driven rebinding.

**Architecture:** Every `key.Binding` in `pkg/ui/keys` gets a stable action ID (struct tag) plus an `@<id>` token in its key list. A sequence router in `pkg/ui/sequence.go` resolves leader and `gg` sequences to IDs and dispatches them as synthetic `tea.KeyPressMsg{Text: "@id"}` messages, so the existing `key.Matches` call sites keep working. Help, which-key, and config all read the same tagged bindings.

**Tech stack:** Go 1.25, `charm.land/bubbletea/v2`, `charm.land/bubbles/v2/key`, `gopkg.in/yaml.v3`.

**Spec:** `docs/design/2026-10-10-bt-i4k-leader-keymap.md`. Read it before any task; its layout tables are the source of truth for keys and action IDs.

## Execution record (2026-10-12)

- [x] Task 1: action IDs and tokens; independent review approved.
- [x] Task 2: raw key dispatch migration; independent review approved.
- [x] Task 3: sequence router and debounced which-key; independent review approved.
- [x] Task 4: default layout and quit/cancel semantics; independent review approved.
- [x] Task 5: local help, retired sidebar and footer consolidation; independent review approved.
- [x] Task 6: configuration and safe input dispatch; independent review approved.
- [x] Task 7: deterministic collisions and diagnostics; independent review approved.
- [x] Whole-branch review and runtime integration corrections, including literal
  at-sign input, single-owner paging, filtered-result bounds, physical search
  navigation, details navigation, rebound plugin quit and ordinal action IDs.
- [x] Integration preserves main's independent dependency, Beads and empty-data
  CLI changes. Retired sidebar source files and scratch artifacts are retained;
  no files were deleted.

The checklists below record the original plan, not pending work. Modal/text
actions accept direct-key overrides only: sequences are rejected individually
because the leader is intentionally inactive while entering text or operating
modals. Other actions can move between direct keys and leader sequences.

Full uncached tests passed on the feature branch and integrated source. Main's
independent dependency upgrade left its vendor manifest outdated, so integrated
build/vet/tests use `-mod=readonly`, preserving both dependency and vendor files.
The optional race run excludes `TestViewSwitchingPerformance`: its fixed
two-second benchmark threshold failed under instrumentation, without reporting
a data race. No install or push is part of this execution.

## Global constraints

- Work in worktree `.claude/worktrees/bt-i4k`, branch `task/bt-i4k`. One commit per task: `type(tui): ... (bt-i4k.N)`, committed with `git commit --only <paths>` or `git add <paths> && git commit`.
- Every task ends with `go build ./...`, `go vet ./...`, `go test ./...` all passing. Never run `go install`. Never `git push`.
- No file deletion. Task 5 retires the sidebar's runtime state and entry points but preserves `shortcuts_sidebar.go` and `shortcuts_sidebar_test.go` under AGENTS.md. No `_v2` files. No compatibility shims.
- Match surrounding style: arrows-first `WithKeys("up", "k")`, `WithHelp("↑/k", ...)`, comment density like the neighbouring code, bead refs in comments where the code already does that.
- Tests never read the user's `~/.config/bt/config.yaml`.
- Text inputs keep every key: list filtering, board search, history search, memories search, time-travel input, BQL, field input, longform edit, label/repo picker search focus.

## Review focus

1. A key pressed mid-sequence (`SPC` then `?`, `q`, `esc`, `/`) must belong to the sequence, never trigger its direct meaning. Task 3 tests it.
2. `space` must still type a space or toggle in text inputs and pickers. Task 3 tests that the leader is inactive in modals and text input.
3. A synthetic `@id` message must never be typed into a text input or treated as a filter by the bubbles list. Task 1 tests that the bubbles list ignores tokens other than nav; Task 3 tests that a leader leaf dispatched while the list is focused does not start filtering.
4. `q` must quit from list-style modals but not from text inputs (typing `q` in search or BQL). Task 4 tests both.
5. A bad `keys:` config (unknown ID, malformed key, bad duration, two actions on one key) must never stop bt starting. Task 6 tests each.

---

### Task 1: Action IDs, tokens, and `actionCmd` (bt-i4k.1)

**Files:**
- Create: `pkg/ui/keys/action.go`, `pkg/ui/keys/action_test.go`, `pkg/ui/action.go`, `pkg/ui/action_test.go`
- Modify: every `pkg/ui/keys/*.go` map struct (add tags), `pkg/ui/keys/keys.go` (`NewAppKeys` calls `stampTokens`), `pkg/ui/model.go` (bubbles list nav tokens)

**Interfaces produced:**
```go
// package keys
const TokenPrefix = "@"
func Token(id string) string                       // "@" + id
type Ref struct{ Map, Field, ID string }
func (a *AppKeys) Walk(fn func(r Ref, b *key.Binding)) // every tagged key.Binding field of every AppKeys map, in field order
func (a *AppKeys) Lookup(id string) []*key.Binding
func IsToken(k string) bool
// package ui
func actionCmd(id string) tea.Cmd                  // returns func() tea.Msg { return tea.KeyPressMsg{Text: keys.Token(id)} }
```

**ID assignment** (exact):
- `Up Down JumpTop JumpBottom PageUp PageDown` fields in every map: `nav.up nav.down nav.top nav.bottom nav.page_up nav.page_down`.
- `GlobalKeys`: Help `app.help`, Sidebar `app.sidebar`, SidebarScrollDown `app.sidebar_down`, SidebarScrollUp `app.sidebar_up`, Tutorial `app.tutorial`, Quit `app.force_quit`, Back `app.quit`, Cancel `app.cancel`, Board `view.board`, Graph `view.graph`, Insights `view.insights`, History `view.history`, Actionable `view.actionable`, FlowMatrix `view.flow`, Tree `view.tree`, LabelDashboard `view.labels`, Attention `view.attention`, Epics `view.epics`, Memories `view.memories`, ProjectsOrWisps `project.picker`, WorkspaceHomeAll `project.home_all`, WispToggle `project.wisps`, HybridPreset `search.hybrid_preset`, Refresh `app.refresh`, SearchMode `search.cycle_mode`, BQL `app.bql`, Recipes `filter.recipes`, Alerts `notify.alerts`, Notifications `notify.notifications`, SearchBounce `app.search`, PriorityHints `notify.priority_hints`, Export `bead.export`, LabelPicker `filter.labels`.
- `ListNormalKeys`: FilterOpen `filter.open`, FilterClosed `filter.closed`, FilterReady `filter.ready`, CycleStatusFilter `filter.cycle_status`, Enter `list.open`, EpicCard `bead.epic_card`, SplitFocusToggle `list.focus_toggle`, PaneRight `window.focus_right`, PaneLeft `window.focus_left`, DetailPageUp `list.detail_page_up`, DetailPageDown `list.detail_page_down`, SplitShrinkLeft `window.shrink`, SplitShrinkRight `window.grow`, PaneFullscreenIssues `window.fullscreen_list`, PaneFullscreenDetails `window.fullscreen_details`, FieldEdit `bead.edit`, CycleSort `sort.cycle`, CycleSortReverse `sort.cycle_reverse`, RecipeTriage `sort.triage`, TimeTravelInput `app.time_travel`, CopyID `bead.copy_id`, CopyIssue `bead.copy_issue`, OpenInEditor `bead.open_editor`, CassSession `bead.cass`, SelfUpdate `app.self_update`, Claim `bead.claim`.
- Every other field: `<prefix>.<snake_case(Field)>` with the map prefixes listed in the spec (`TreeKeys` → `tree`, `BoardNormalKeys` → `board`, `ListSearchKeys` → `list_search`, ...). Example: `BoardNormalKeys.CycleSwim` → `board.cycle_swim`.

- [ ] **Step 1: Write failing tests** in `pkg/ui/keys/action_test.go`:
  - `TestEveryBindingTagged`: reflect over each `AppKeys` field's struct; every `key.Binding` field has a non-empty `action` tag matching `^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`.
  - `TestActionIDsUniquePerMap`: no ID repeats within one map.
  - `TestEveryBindingHasOneToken`: after `NewAppKeys()`, each binding's `Keys()` contains exactly one `IsToken` entry, equal to `Token(id)`.
  - `TestPhysicalKeysUnchanged`: for each binding, `Keys()` minus the token equals the keys `NewXKeys()` declares. Compare against a fresh `NewXKeys()` call (constructors do not stamp; only `NewAppKeys` does).
  - `TestLookup`: `Lookup("nav.down")` returns one binding per map declaring `Down`; `Lookup("filter.open")` returns exactly one.
- [ ] **Step 2:** `go test ./pkg/ui/keys/ -run 'Tagged|Unique|Token|Physical|Lookup'`: FAIL (undefined).
- [ ] **Step 3: Implement** `action.go`:
```go
package keys

import (
	"reflect"
	"strings"

	"charm.land/bubbles/v2/key"
)

// TokenPrefix marks an action token: a key string no terminal produces, used
// to dispatch an action by ID through key.Matches (bt-i4k).
const TokenPrefix = "@"

// Token returns the action token for id.
func Token(id string) string { return TokenPrefix + id }

// IsToken reports whether k is an action token.
func IsToken(k string) bool { return strings.HasPrefix(k, TokenPrefix) }

// Ref names one tagged binding: the AppKeys field holding its map, the
// binding's field, and its action ID.
type Ref struct{ Map, Field, ID string }

var bindingType = reflect.TypeOf(key.Binding{})

// Walk calls fn for every tagged key.Binding in every map of a, in field order.
func (a *AppKeys) Walk(fn func(r Ref, b *key.Binding)) {
	av := reflect.ValueOf(a).Elem()
	for i := 0; i < av.NumField(); i++ {
		mv := av.Field(i)
		if mv.Kind() != reflect.Struct {
			continue
		}
		mapName := av.Type().Field(i).Name
		for j := 0; j < mv.NumField(); j++ {
			f := mv.Type().Field(j)
			if f.Type != bindingType {
				continue
			}
			id := f.Tag.Get("action")
			if id == "" {
				continue
			}
			fn(Ref{Map: mapName, Field: f.Name, ID: id}, mv.Field(j).Addr().Interface().(*key.Binding))
		}
	}
}

// Lookup returns every binding tagged id.
func (a *AppKeys) Lookup(id string) []*key.Binding {
	var out []*key.Binding
	a.Walk(func(r Ref, b *key.Binding) {
		if r.ID == id {
			out = append(out, b)
		}
	})
	return out
}

// stampTokens appends each binding's action token to its keys.
func stampTokens(a *AppKeys) {
	a.Walk(func(r Ref, b *key.Binding) {
		b.SetKeys(append(b.Keys(), Token(r.ID))...)
	})
}
```
  Note: `SetKeys` on a disabled binding keeps it disabled. Bindings that start disabled (board `NextMatch`/`PrevMatch`) stay disabled.
- [ ] **Step 4:** Add `action:"..."` tags to every binding field in every map file per the ID table. `NewAppKeys` becomes `a := AppKeys{...}; stampTokens(&a); return a`. Embedded structs (if any map embeds a chrome struct) must be walked too: if `Walk` misses one, extend it to recurse into anonymous struct fields.
- [ ] **Step 5:** `pkg/ui/action.go`:
```go
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/ui/keys"
)

// actionCmd dispatches action id as a synthetic key press carrying its token,
// so it runs the full Update path like the physical key would (bt-i4k).
func actionCmd(id string) tea.Cmd {
	return func() tea.Msg { return tea.KeyPressMsg{Text: keys.Token(id)} }
}
```
  In `model.go` list setup (next to `l.KeyMap.PrevPage.SetKeys`), append `keys.Token("nav.up")` to `l.KeyMap.CursorUp` and `keys.Token("nav.down")` to `l.KeyMap.CursorDown`, so the token moves the bubbles-owned list cursor.
- [ ] **Step 6:** `pkg/ui/action_test.go`. Add a helper used by later tasks:
```go
// runKey feeds msg through Update and then drains any action-token key presses
// the resulting commands emit (one level of tea.Batch is flattened).
func runKey(t *testing.T, m Model, msg tea.KeyPressMsg) Model
```
  Implement it by calling `m.Update(msg)` and collecting the cmd. Execute it (expanding `tea.BatchMsg`), feed back only `tea.KeyPressMsg` results whose `Text` `IsToken`, and repeat up to 8 rounds. Ignore every other message. Tests, using the existing test model constructor pattern in `model_test.go`:
  - `@filter.open` toggles `m.filter.currentFilter` to `"open"` in the list.
  - `@nav.down` moves `m.list.Index()` from 0 to 1 with at least 2 issues.
  - `@view.board` switches `m.mode` to `ViewBoard`.
  - `@sort.cycle` does not put the bubbles list into `list.Filtering`.
- [ ] **Step 7:** Build, vet, run all tests. Commit `feat(tui): action IDs and dispatch tokens on every key binding (bt-i4k.1)`.

### Task 2: Raw-string key handlers into the registry (bt-i4k.2)

Move every raw `msg.String()` key comparison that dispatches an action into a tagged map, with no behaviour change. Text value handling stays raw: typed runes into search inputs, `backspace` editing, and digit parsing such as `1`–`9` in attention.

**Files:** create `pkg/ui/keys/alerts.go`, `pkg/ui/keys/labels.go` (label dashboard, health, drilldown, graph, attention), `pkg/ui/keys/misc.go` (memories, help, cass, agent prompt, tutorial). Modify `keys.go` (fields + constructors), `keys_test.go` `allMaps()`, `global.go`, `model_update_input.go`, `memories_keys.go`, `help_keys.go`, `label_dashboard.go`, `cass_session_modal.go`, `agent_prompt_modal.go`, `tutorial.go`, `flow_matrix.go`, `update_modal.go`, `plugin_prompt.go`, `plugin_actions.go`, `model_footer.go` (`modalKeyMap`/`viewSpecificKeyMap` return the new maps).

**New maps** (`AppKeys` field → type, ID prefix; keys exactly as today):

| Field | Prefix | Bindings (today's keys) |
|---|---|---|
| `Alerts` | `alerts` | TabSwitch `tab`, ShowAlerts `!`, ShowNotifications `1`, Up/Down (`k`,`up`/`j`,`down`), PageLeft `h`, PageRight `l`, PageKeys `right left pgdown pgup` (field `Page`), JumpTop `home g`, JumpBottom `end G`, Open `enter`, Dismiss `c`, DismissAll `C`, Severity `s`, SeverityReverse `S`, Type `t`, TypeReverse `T`, Project `p`, ProjectReverse `P`, Sort `o`, SortReverse `O`, Class `a`, ClassReverse `A`, Reset `r R`, Close `esc q` |
| `Notifications` | `notifications` | Up, Down, Page `right left pgdown pgup`, JumpTop `home g`, JumpBottom `end G`, Open `enter`, Dismiss `c`, DismissAll `C`, ShowDismissed `d`, Kind `t`, KindReverse `T`, Reset `r R`, Close `esc q` |
| `LabelDashboard` | `label_dashboard` | Up, Down, JumpTop `home`, JumpBottom `G end`, Apply `enter`, Health `h`, Drilldown `d`, Close `esc q [` |
| `LabelHealth` | `label_health` | Close `esc q enter h`, Drilldown `d` |
| `LabelDrilldown` | `label_drilldown` | Apply `enter`, Graph `g`, Close `esc q d` |
| `LabelGraph` | `label_graph` | Close `esc q g` |
| `Attention` | `attention` | Close `esc q d`, PickLabel `1`..`9` (help `1-9`; the handler still parses the digit) |
| `MemoriesNormal` | `memories` | Search `/`, Up, Down, FocusToggle `tab`, Open `enter`, Back `backspace` |
| `MemoriesSearch` | `memories_search` | Cancel `esc`, Confirm `enter` |
| `Help` | `help` | Up, Down, PageDown `ctrl+d`, PageUp `ctrl+u`, JumpTop `home g`, JumpBottom `G end`, Column `l` (whatever `l` does today: read `help_keys.go`), Close `q esc ? f1`, Tutorial `space` |
| `Cass` | `cass` | Up, Down, Copy `y`, Close `V esc enter q` |
| `AgentPrompt` | `agent_prompt` | Yes `y Y`, No `n N`, Never `d D` |
| `Tutorial` | `tutorial` | keys from `tutorial.go` lines 90–210 (Up, Down, PageDown, PageUp, JumpTop `g home`, JumpBottom `G end`, Section `1`..`9`, Back `h left`, the `t` key). Read the file for each one's meaning and name fields after it |

Global additions: `DebugDims` `ctrl+p` (`window.debug_dims`, help "dimension chip"; it stays out of `ShortHelp`, but `FullHelp` must list it for the completeness test), and `PluginMenu` `P` (`plugin.menu`, help "plugin actions"). `FlowMatrixKeys` gains PageDown/PageUp `ctrl+d`/`ctrl+u` and the internal `j/k` drilldown nav (lines 160–205 of `flow_matrix.go`; reuse `Up`/`Down`).

Rules:
- Sub-models with their own `Update` (`cassModal`, `agentPromptModal`, `tutorialModel`, `labelDashboard`, flow matrix model, `updateModal`) get a `keys keys.XKeys` field. `Model` assigns it from `m.keys.X` wherever it constructs or opens the sub-model, so Task 6's rebinding reaches them. Their `switch msg.String()` becomes `switch { case key.Matches(msg, k.X): ... }`.
- Every raw `msg.String() == "ctrl+c"` becomes `key.Matches(msg, m.keys.Global.Quit)`. `plugin_prompt.go`'s raw `q` becomes a `Close` field `q` on `PluginSelectKeys` (ID `plugin_select.close`). `update_modal.go` gets its own `UpdateKeys` map (`AppKeys.Update`, prefix `update`) with Confirm, Cancel, and Close `q`, instead of borrowing `m.keys.Confirm`.
- `P` handling: today `tryPluginActionKey` opens the menu when `P` is unbound by bt. `Global.PluginMenu` now binds `P`, so dispatch it in the global switch right before `tryPluginActionKey`, gated exactly as before (`m.pluginActions != nil && m.activeModal == ModalNone && m.pluginView() != ""`). Remove `pluginMenuKey`, and replace its use in `pluginKeyBindings` with the existing `btBindsKeyIn` check, which now covers `P`.
- Every new map implements `ShortHelp`/`FullHelp` listing every field, and is added to `allMaps()` in `keys_test.go` (the registry test enforces this).

- [ ] **Step 1:** Write `pkg/ui/raw_keys_test.go` with behaviour pins captured before refactoring, one per new map, against today's code:
  - alerts: `s` cycles `alertFilterSeverity` to `"critical"`, `q` closes.
  - notifications: `d` toggles `notifShowDismissed`.
  - label dashboard: `h` opens `ModalLabelHealthDetail`.
  - drilldown: `g` opens `ModalLabelGraphAnalysis`.
  - attention: `3` sets the label filter to the third label.
  - memories: `/` starts search.
  - help: `q` closes.
  - cass: `V` closes.
  - agent prompt: `n` dismisses.
  - `ctrl+p` toggles `showDebugDims`.
  - `P` opens the plugin menu (use `newPluginActionModel` from the existing tests).
  Run them: PASS on the unchanged code.
- [ ] **Step 2:** Add the token-routing test `TestNewMapsDispatchByToken`: for each pinned case, sending the action's token instead of the physical key produces the same state. Run: FAIL.
- [ ] **Step 3:** Implement the maps and convert the handlers.
- [ ] **Step 4:** `grep -nE 'msg\.String\(\) *==|case "[^"]+"(, "[^"]+")*:' pkg/ui/*.go | grep -v _test` must show only text-value handling (search input, digit parsing, non-key string switches such as sort names). Paste the residual list into the commit body.
- [ ] **Step 5:** Build, vet, test all. Commit `refactor(tui): route raw key handlers through the keymap registry (bt-i4k.2)`.

### Task 3: Sequence router and which-key panel (bt-i4k.3, part 1)

The engine ships inert: no default binding has a leader key until Task 4, and the router only starts a leader sequence when the active leader tree is non-empty. Behaviour is unchanged after this task, apart from `gg` now going through the router, which keeps the same behaviour on board.

**Files:** create `pkg/ui/keys/sequence.go` (notation parsing, leader tree), `pkg/ui/keys/sequence_test.go`, `pkg/ui/sequence.go` (router), `pkg/ui/which_key.go` (panel render), `pkg/ui/sequence_test.go`. Modify `model.go` (state fields, defaults, `whichKeyMsg` case in `Update`, swallow check before forwarding to list), `model_update_input.go` (router call), `model_view.go` (panel overlay), `model_footer.go` (pending sequence in hint zone), `board.go`/`board_keys.go` (drop `waitingForG`; `BoardNormalKeys.GotoTop` removed; `JumpTop` keys become `home`, `0`, `gg`).

**Interfaces produced:**
```go
// package keys
const LeaderPrefix = "<leader>"
func ParseSeq(k string) (prefix string, steps []string, ok bool)
	// "<leader>vb" -> ("<leader>", ["v","b"], true); "<leader>w<lt>" -> (..., ["w","<"], true);
	// "<leader><tab>" -> ["tab"]; "gg" -> ("g", ["g"], true); "j" -> ("", nil, false)
func FormatSeq(prefix string, steps []string, leaderLabel string) string // "SPC v b"
type SeqEntry struct{ Steps []string; ID, Desc string }
type SeqNode struct {
	Key      string
	Label    string // group label, "" for leaves
	ID, Desc string // leaf only
	Children []*SeqNode // sorted: groups first, then by key
}
func BuildSeqTree(prefix string, entries []SeqEntry, groupLabels map[string]string) (*SeqNode, []error)
	// errors: a duplicate sequence, or a sequence that is both a leaf and a group prefix (the first entry wins)
func SeqEntries(prefix string, maps ...any) []SeqEntry // walks enabled tagged bindings of the given map struct values; Desc = Help().Desc
var LeaderGroups = map[string]string{"v": "view", "f": "filter", "s": "sort", "b": "bead", "p": "project", "w": "window", "n": "notify", "/": "search"}
// package ui
type seqState struct{ prefix string; node *keys.SeqNode; steps []string; gen int; showPanel bool }
type whichKeyMsg struct{ gen int }
func (m Model) seqMaps() []any // &m.keys.Global's value plus the active non-modal view map value (same switch as viewSpecificKeyMap, returning struct values)
func (m Model) inTextInput() bool
func (m Model) routeSequence(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) // handled=true means stop processing msg
```
Model fields: `seq seqState`, `leaderKey string` (default `"space"`), `whichKeyDelay time.Duration` (default `250 * time.Millisecond`), `whichKeyOff bool`, `swallowKey bool`.

Router algorithm (spec, "Sequence router"):
1. Only when `m.activeModal == ModalNone && !m.inTextInput()`. Call it in `handleKeyPress` immediately after the last modal early-return (`ModalPluginPrompt`) and before the `?` help check. Tokens (`keys.IsToken(msg.Text)`) bypass the router.
2. Idle and `msg.String() == m.leaderKey`: build the leader tree from `SeqEntries("<leader>", m.seqMaps()...)`. If it is empty, return not handled. Otherwise set pending, `gen++`, `swallowKey = true`, and return `tea.Tick(delay, whichKeyMsg{gen})`. With delay 0, set `showPanel` immediately and return no tick. With `whichKeyOff`, never schedule a tick.
3. Idle and `msg.String() == "g"`: build the `g` tree from `SeqEntries("g", ...)`. If it is non-empty, go pending (no tick) and swallow.
4. Pending `esc`: reset, swallow. Pending `backspace`: pop a step and go back to the parent node (at the root, reset), swallow.
5. Pending other key: look up the child by `msg.String()`.
   - Group: descend and swallow. If the panel is not shown, `gen++` and reschedule the tick.
   - Leaf: reset, swallow, and return `actionCmd(leaf.ID)`.
   - Miss in a leader sequence: reset, swallow, and `setStatus(FormatSeq(...)+" is not bound")`.
   - Miss in a `g` sequence: reset and return not handled, so the key is processed normally.
6. `whichKeyMsg{gen}` in `Update`: if `gen == m.seq.gen` and pending, set `showPanel = true`.
7. In `Update`, skip forwarding the key to `m.list` when `m.swallowKey`, then clear it.

which-key panel: rendered in `View` above the footer, full width, bordered with the theme's border colour like other non-dim overlays (use `OverlayCenter`'s sibling pattern; read `docs/design/tui-modal-compositing.md`). Title: `FormatSeq(...)` plus the group label. Entries: `key  +label` for groups and `key  desc` for leaves, in as many columns as fit with column width `max(entry)+2`. At most `height/3` rows; when entries overflow, show a final `…` cell. Footer: while pending, the hint zone shows `SPC v…`.

- [ ] **Step 1: Failing tests** `pkg/ui/keys/sequence_test.go`: table tests for `ParseSeq` (all examples above, plus `"<leader>"` alone → ok with zero steps → treated as invalid by `BuildSeqTree`), `FormatSeq` (`"<leader>"`,`["w","<"]`,`"SPC"` → `"SPC w <"`), and `BuildSeqTree` (groups sorted first, a duplicate sequence errors, a leaf-vs-group conflict errors, an unlabeled group gets the label `+group`).
- [ ] **Step 2: Failing tests** `pkg/ui/sequence_test.go`. Build a model and give `m.keys.Global.Board` the extra key `"<leader>vb"` via `SetKeys(append(...))`:
  - `space v b` → `ViewBoard`.
  - `space v esc` → still `ViewList`, sequence idle.
  - `space v backspace b`: `b` at the root is not bound (no `<leader>b` leaf), so status shows "is not bound".
  - `space ?` → help NOT opened (miss).
  - In list filtering, `space` is typed into the filter (`FilterInput.Value() == " "`; check the bubbles behaviour).
  - With the label picker open, `space` toggles (picker state changes).
  - which-key: after `space`, the returned cmd is a tick. Feeding `whichKeyMsg{gen}` sets `showPanel`; a stale gen does not. Delay 0 shows it at once.
  - `gg` on board (JumpTop now includes `gg`) moves to the first card.
  - `g` then `j` on board moves down (miss falls through).
  - Without any `<leader>` keys, `space` in the tree still toggles (inert engine).
- [ ] **Step 3:** Implement. Remove board `waitingForG` (`ClearWaitingForG`, `SetWaitingForG`, `IsWaitingForG`) and its uses.
- [ ] **Step 4:** Build, vet, test all. Commit `feat(tui): leader and gg sequence router with debounced which-key panel (bt-i4k.3)`.

### Task 4: New default layout, `q` quits, `esc` cancels (bt-i4k.3, part 2)

**Files:** `pkg/ui/keys/global.go`, `list.go`, `board.go`, `tree.go`, `epics.go`, `history.go`, `flow_matrix.go`, `labels.go`, `alerts.go`, `modals.go`, `misc.go`, `pkg/ui/model_update_input.go`, `list_keys.go`, `board_keys.go`, `tree_keys.go`, `epics_keys.go` (or wherever `handleEpicsKeys` lives), `history_keys.go`, `field_edit.go`, `model.go` (bubbles list paging and `GoToStart` keys), `plugin_actions.go` (`listPagingKeys`), and every test asserting old keys.

Steps:
- [ ] **Step 1:** Write `pkg/ui/layout_test.go` from the spec tables. Use a table `[]struct{ keys []string; setup func(*Model); check func(t, Model) }` covering every row of "Leader groups" and "Direct keys", driven by `runKey` over `keyMsg` presses (a `"space"` entry sends `tea.KeyPressMsg{Code: tea.KeySpace}`). Also add:
  - `q` quits (the cmd yields `tea.QuitMsg`) from: base list, board, alerts modal, label picker in nav mode, help overlay.
  - `q` is typed (no quit) in list filtering and BQL.
  - `esc` in the base list with no filters does nothing (no modal opens), and with filters it clears them.
  - `SPC ,` opens the settings menu.
  - The old direct keys `b i a f E u w W o c r s m x l p !` and `1` do nothing in the base list.
  - `h`/`l` move between list and details.
  - `e` opens field select from board, tree, and epics.
  Run: FAIL.
- [ ] **Step 2:** Rebind per the spec tables. Leader sequences are extra keys in `WithKeys` (`"<leader>vb"`), and help text stays the action description. Moved bindings:
  - `filter.open/closed/ready/cycle_status` move from `ListNormalKeys` to `GlobalKeys` (fields `FilterOpen FilterClosed FilterReady CycleStatusFilter`), so they work from every view as today's board reuse did. Delete the board reuse in `btBindsKeyIn`.
  - New Global fields: `ViewList` (`view.list`, `<leader>vl`), `FilterClear` (`filter.clear`, `<leader>fx`, calls `m.clearAllFilters()`), `Settings` (`app.settings`, `<leader>,`, the body moved from the esc-at-base branch).
  - `Back` (`app.quit`) keys become `q`, `<leader>q`. Its dispatch moves to one check at the very top of `handleKeyPress`, before any modal early-return:
    ```go
    if key.Matches(msg, m.keys.Global.Back) && m.activeModal != ModalPluginPrompt && !m.inTextInput() { return m, tea.Quit }
    ```
    `inTextInput` must include modal text states: BQL, time travel, field input, longform, label/repo picker search focus.
  - Delete the old `q` cascade. The plugin prompt's `q` (`plugin_select.close`) does what its `ctrl+c` path does: answer the plugin, then quit.
  - `esc` cascade: delete the final settings-menu branch. At base with no filters, `esc` is a no-op.
  - Remove `q` from every close list (alerts, notifications, label modals, attention, cass, help, flow, repo picker, label dashboard). Also remove these duplicate close keys: `l` label picker, `'` recipes, `w` repo picker, `V` cass, `!`/`1` closing alerts (they still switch tabs while the modal is open), `g` label graph, `d`/`h` label health and drilldown close, `d` attention, `[` label dashboard, `f` flow, `h` history exit, `T` tree back.
  - Navigation: `nav.top` keys include `gg` and `home` in list, board (plus `0`), tree, flow, label dashboard, and epics (add `JumpTop`/`JumpBottom` to epics if missing, wired to the epics tree's existing top/bottom methods). Remove the lone-`g` meanings: the bubbles list `GoToStart` gets `home` only, and `listPagingKeys` becomes `pgup pgdown` only. Bubbles `PrevPage`/`NextPage` become `pgup`/`pgdown` only.
  - List `PaneLeft`/`PaneRight` gain `h`/`l`.
  - Tree: `Toggle` is `enter` only.
  - History: `ToggleFileTree` is `f` only. `JumpToGraph` becomes `<leader>bg`, and history's map must be included in `seqMaps()` for `ViewHistory`.
  - Label dashboard `Health` becomes `i`.
  - `bead.edit` (`e`): add a `FieldEdit` field with the same ID to the Board, Tree, and Epics maps, and generalize `requestFieldEdit` and `requestClaim` to use `m.selectedIssue()` instead of `m.list.SelectedItem()`.
  - Drop `f2`–`f6` and `ctrl+r`. Keep `f1`.
- [ ] **Step 3:** Update every existing test that presses an old key. Change each one to the new key or sequence; don't delete it unless the behaviour itself was removed (for example the `q` cascade). Run `go test ./pkg/ui/...` until green.
- [ ] **Step 4:** Build, vet, test all. Commit `feat(tui): neovim-style default layout with leader groups; q quits, esc cancels (bt-i4k.3)`.

### Task 5: Help consolidation and docs (bt-i4k.4)

**Files:** `pkg/ui/model_view.go` (`renderHelpOverlay`, `renderHelpMini`), `model_footer.go` (`l1KeyMap`, `sidebarHelpGroups`, hint rendering), delete `shortcuts_sidebar.go` and `shortcuts_sidebar_test.go`, remove `showShortcutsSidebar`/`shortcutsSidebar` uses and sidebar width reservation, `keys/global.go` (remove `Sidebar`, `SidebarScrollDown`, `SidebarScrollUp`), `tutorial_content.go`, `context_help.go`, `README.md` (Views / Key bindings tables), `pkg/ui/keys/readme_keys_test.go`, `docs/adr/004-bubbles-key-adoption.md` (status note), `docs/design/tui-*.md` that list old keys (grep).

- [ ] **Step 1:** Failing tests:
  - `TestHelpOverlayLocalOnly`: the rendered `?` text for the list has no line containing a leader-only action desc ("board", "cycle sort"), and does contain "edit field" and "copy ID".
  - `TestFooterHints`: the base-list footer contains `SPC` and `?` and no more than 4 view hints.
  - `TestHelpListsOnlyDirectKeys`: for every view mode, every binding in the `?` groups has at least one key that is neither a token nor a sequence, and its rendered Help.Key mentions no `SPC`.
- [ ] **Step 2:** Implement:
  - `?` shows Global direct keys, then the active view or modal map, keeping only bindings with at least one direct key (not a token or sequence), with Help.Key rebuilt to show only direct keys. Then plugin keys (`pluginKeyBindings`) as a normal group titled after the view. Its footer reads `SPC  leader menu`.
  - L1 footer: `ShortHelp()` filtered to direct-key bindings (max 4), then `SPC leader`, then `? help`.
  - Delete the sidebar.
- [ ] **Step 3:** Docs:
  - README key tables rewritten to the new layout: a direct-keys table plus a leader table.
  - Keep `readme_keys_test.go` passing. Extend `readmeKeyTick` handling so `SPC v b` cells resolve against `<leader>vb` keys.
  - Tutorial pages mention the new keys.
  - ADR-004 gets a "Superseded in part by docs/design/2026-10-10-bt-i4k-leader-keymap.md" status line.
  - `grep -rnE '\b(press|key) `[bgihafTEuwWlpx;]`' docs README.md` and fix what describes current behaviour (leave dated archives alone).
- [ ] **Step 4:** Build, vet, test all. Commit `feat(tui): local-only ? help, drop ; sidebar, trim footer hints (bt-i4k.4)`.

### Task 6: `keys:` config (bt-i4k.5, part 1)

**Files:** create `pkg/ui/keys/config.go`, `pkg/ui/keys/config_test.go`, `pkg/ui/keys/label.go` (help label rendering). Modify `pkg/ui/model_modes.go` (`SetKeyConfig`), `cmd/bt/root.go` (load next to `loadPluginHost`), `docs/plugins.md` or a new section in `README.md` documenting `keys:`.

**Interfaces produced:**
```go
type BindValue []string // yaml: a string or a list of strings; "none" -> empty
func (b *BindValue) UnmarshalYAML(n *yaml.Node) error
type Config struct {
	Leader        string               `yaml:"leader"`
	WhichKeyDelay string               `yaml:"which_key_delay"`
	Bind          map[string]BindValue `yaml:"bind"`
}
func LoadConfig(path string) (Config, error)   // reads the keys: section; a missing file gives Config{}, nil
type Settings struct{ Leader string; WhichKeyDelay time.Duration; WhichKeyOff bool }
func (a *AppKeys) Apply(cfg Config) (Settings, []Shadow, []error)
type Shadow struct{ ID, Key, By string } // binding ID lost Key to action By
func HelpLabel(keys []string, leaderLabel string) string // "↑/k", "SPC f o", "⌃d"; tokens skipped
func LeaderLabel(leader string) string                   // "space" -> "SPC", else the key itself
// package ui
func (m *Model) SetKeyConfig(cfg keys.Config, loadErr error)
```

Rules (spec "Config"):
- `Apply` validates each key: tokens are rejected, and sequences must parse. Unknown IDs, a bad leader (`""` is allowed and means default), and a duration that is neither a Go duration nor `"off"` are errors; the bad entry is skipped. Apply entries in sorted ID order.
- For each `bind` entry, replace the physical and sequence keys of every binding with that ID, keep the token, and regenerate Help.Key with `HelpLabel`.
- Conflict pass, then `Shadow` records: see Task 7 (Task 6 only returns `nil` shadows; it must still compile).
- `SetKeyConfig` calls `m.keys.Apply`, sets `leaderKey`, `whichKeyDelay`, `whichKeyOff`, and re-assigns the sub-model `keys` fields from Task 2. When there are errors, call `m.setStatus(fmt.Sprintf("keys: %d config error(s) - see ?", n))` and keep the errors on the model (`m.keyConfigErrs`) for `?`.
- `cmd/bt/root.go`: `path, _ := plugin.DefaultConfigPath(); cfg, err := keys.LoadConfig(path); m.SetKeyConfig(cfg, err)`. Skip this when `BT_TEST_MODE` is set.

- [ ] **Step 1: Failing tests:**
  - YAML parsing: scalar, list, `none`, missing `keys:`.
  - Promote: `filter.open: o` makes `o` match `Global.FilterOpen` and `<leader>fo` no longer present.
  - Move: `view.board: "<leader>B"`.
  - Unbind: `app.refresh: none` leaves only the token, and the binding stays `Enabled()`.
  - Unknown ID → error, other entries still applied.
  - `which_key_delay: 0` → 0; `"off"` → `WhichKeyOff`; `"abc"` → error.
  - Leader `","` routes sequences on `,`.
  - `HelpLabel` table.
  - Model test: `SetKeyConfig` with `filter.open: o`, then `runKey("o")` in the list sets the open filter.
- [ ] **Step 2:** Implement. **Step 3:** Build, vet, test all. Commit `feat(tui): keys config for leader, which-key delay and rebinding (bt-i4k.5)`.

### Task 7: Conflict resolution and reporting (bt-i4k.5, part 2)

**Files:** `pkg/ui/keys/config.go` (conflict pass), `pkg/ui/keys/overlap.go` (map kinds), tests, `pkg/ui/model_view.go` (`?` group "Shadowed / errors", strikethrough style), `pkg/ui/plugin_actions.go` (plugin shadow list for the current view).

Rules (spec "Conflict resolution"):
- Map kinds: `Global`; view maps (ListNormal, Tree, BoardNormal, Graph, Insights, HistoryNormal, HistoryFileTree, Actionable, FlowMatrix, Epics, LabelDashboard, Attention, MemoriesNormal); sub-state maps (ListSearch, BoardSearch, HistorySearch, MemoriesSearch); everything else is modal. Overlap: same map, or Global with a view map. Sub-state and modal maps overlap only themselves.
- After applying `bind`, for each configured action A and each key K it now holds: every other binding in a map overlapping one of A's maps that holds K loses K (via `SetKeys`, token kept, Help.Key regenerated), and gets a `Shadow{ID, K, A}`.
- Two configured actions claiming the same K in overlapping maps is an error. The later one in sorted order loses K.
- A configured sequence that is a strict prefix of other bindings' sequences shadows those bindings' sequences (each gets a `Shadow`).
- `?` lists config shadows, config errors, and the current view's plugin shadows. Plugin shadows come from `pluginActionsFor(selectedIssue())` actions whose key `btBindsKeyIn` the view, struck through with `lipgloss.NewStyle().Strikethrough(true)`.
- Startup status: `keys: N shadowed, M config error(s) - see ?`, shown only when N+M > 0.

- [ ] **Step 1: Failing tests:**
  - `filter.open: y` → `ListNormal.CopyID` (`bead.copy_id`) loses `y`, with a shadow recorded.
  - `filter.open: n` does not shadow `BoardSearch.NextMatch` (sub-state).
  - `view.board: "<leader>v"` shadows every `<leader>v…` sequence.
  - Two actions on `o` → one error.
  - A `?` render contains the struck-through shadow line.
  - Plugin action with key `y` in the list appears as shadowed in `?`.
- [ ] **Step 2:** Implement. **Step 3:** Build, vet, test all. Commit `feat(tui): keymap conflict resolution and shadow reporting (bt-i4k.5)`.

### Finish

- [ ] Whole-branch review (reviewer subagent), with fixes committed on the branch.
- [ ] Close beads bt-i4k.1–.5 and bt-i4k with close reasons (file-based, `.beads/tmp/`).
- [ ] Merge `task/bt-i4k` into `main` with `git merge --no-ff` (repo convention), then run build, vet, and test on `main`.
- [ ] Do not push. Do not `go install` while other sessions may be running; report that to the user.
