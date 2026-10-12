# beadstui (`bt`)

A terminal UI for [beads](https://github.com/gastownhall/beads) - keyboard-driven issue tracking in your terminal.

This is [AntoineGS/beadstui](https://github.com/AntoineGS/beadstui), an independently maintained fork of [Sean Martin Smith's beadstui](https://github.com/seanmartinsmith/beadstui), which grew out of [Jeffrey Emanuel's beads_viewer](https://github.com/Dicklesworthstone/beads_viewer). Development happens here, with no plans to upstream these changes. The executable is still called `bt`.

![bt list and detail view](docs/screenshots/01-list-detail.webp)

> **Alpha.** APIs, keybindings, and on-disk formats may change during development.

Screenshots show the current TUI with sample project data. [Regenerate them](scripts/screenshots/README.md) after UI changes.

## What is bt

[Beads](https://github.com/gastownhall/beads) is a git-native issue tracker backed by [Dolt](https://www.dolthub.com/) (a version-controlled MySQL-compatible database). `bt` is a TUI that sits on top of it - board views, detail panels, dependency graphs, and graph-based triage, all without leaving your terminal.

Think lazygit, but for issue tracking.

## Install

Requires a working [beads](https://github.com/gastownhall/beads) installation with Dolt. Use the commands below to install this fork. For beads/Dolt setup and troubleshooting, see [`docs/install.md`](docs/install.md); its bt installation section still describes the predecessor project.

### From source (requires Go 1.26+)

Clone this repository and install from the checkout:

```bash
git clone https://github.com/AntoineGS/beadstui.git
cd beadstui
go install -mod=readonly ./cmd/bt/
```

Go places `bt` in `GOBIN`, or `$(go env GOPATH)/bin` when `GOBIN` is unset. Add that directory to your `PATH`.

To build without installing, run `go build -mod=readonly -o bt ./cmd/bt/` (`-o bt.exe` on Windows). The `-mod=readonly` flag uses the dependencies declared in `go.mod` because the checked-in `vendor/` directory is currently out of sync.

The Go module still uses `github.com/seanmartinsmith/beadstui`. Installing that module with `@latest` fetches the predecessor project, so use the checkout commands above to get this fork.

### Release builds

Check [this fork's releases](https://github.com/AntoineGS/beadstui/releases) for published binaries. If none are available for your platform, build from source.

## Shared-server mode (for the cross-project view)

For single-project use you need nothing special: `cd` into any beads project and run `bt`. A project in bd's default embedded mode is read directly via `bd export` (into memory, never persisted) - no server and no lock, so a concurrent `bd` command is never blocked. If the project runs on a shared Dolt server bt connects to that; a project configured for server mode with none running gets a per-session server (via `bd dolt start`) that bt stops on exit.

The cross-project global view is what needs shared-server mode. It enumerates projects from the `beads_global` aggregate database on a single shared Dolt server at `~/.beads/shared-server/`. Projects in bd's default embedded mode keep their data in the project's own `.beads/embeddeddolt/` directory, so it never touches the shared server - those projects work fine on their own but won't appear in the global view.

**To make a project visible in the cross-project view, enable shared-server mode.**

For a new project:

```bash
bd init --shared-server
```

For an existing project already initialized in embedded mode:

```bash
bd config set dolt.shared-server true
```

After switching, run `bd dolt start` once to migrate the project onto the shared server. See [bd's docs/DOLT.md - Shared-Server Mode](https://github.com/gastownhall/beads/blob/main/docs/DOLT.md#shared-server-mode) for the full migration walkthrough and background on the two modes.

> **Note:** If you launch the cross-project view and some of your beads projects are missing, this is the most likely cause. The global view shows only projects whose databases are registered on the shared server.

## Quick start

```bash
cd your-project    # any directory with beads initialized
bt                 # launches the TUI
```

How bt reads your project depends on your bd setup:

| Your bd setup | What bt does |
|---|---|
| Default (embedded - bd spawns a Dolt per command) | bt reads the project via `bd export` into memory - no server, no lock, coexists with a concurrent `bd`. |
| You ran `bd dolt start` (one shared server, N project DBs) | bt auto-discovers it via the port file and connects - no new server. |
| You want a cross-project view | `bt --global` queries the `beads_global` aggregate database on the shared server. |

**Embedded mode + concurrent `bd`:** because bt reads embedded projects through `bd export` and holds no server or lock, running `bd` from another shell during a bt session is safe - bt picks up the change on its next refresh. Remaining mode-parity edge cases are tracked in bt-gm6ur.

### Mode parity

Embedded mode (bd's default since v1.0) supports the full core surface - list/detail, board, triage, graph analysis, BQL, search, and every `bt robot` subcommand. A few capabilities differ by backend:

| Capability | Embedded / project (default) | Shared-server / `--global` |
|---|---|---|
| Live refresh | Event-driven - watches the Dolt manifest, re-runs `bd export` on change | SQL polling |
| Startup | Instant (in-process load), no splash | Brief loading indicator during the remote fetch |
| Notifications / alerts | Scoped to the current project | Scoped to the selection (all projects in the global view) |
| History / correlation | Not available yet - explicit banner, never silent (bt-5uaxh) | Available |
| Cross-project view | Single project by nature | Aggregates every project on the shared server |

Two notes:

- **Embedded is single-project by nature.** Its data lives in the project's own `.beads/embeddeddolt/`, so it won't appear in the cross-project `--global` view until you [switch it to shared-server mode](#shared-server-mode-for-the-cross-project-view).
- **Point-in-time (`--as-of`)** is an interactive-TUI feature (`bt --as-of <ref>`); it is not yet supported in robot mode, where it refuses explicitly rather than return current data as historical (tracked in bt-9kiy4).

## Views

| Key | View | What it shows |
|-----|------|---------------|
| `SPC v l` | **List** | Return to the issue list |
| `SPC v b` | **Board** | Kanban columns by status |
| `SPC v g` | **Graph** | Dependency graph |
| `SPC v i` | **Insights** | PageRank, critical path, cycle detection |
| `SPC v h` | **History** | Bead-to-commit correlation timeline |
| `SPC v a` | **Actionable** | Execution plan grouped into parallel tracks |
| `SPC v f` | **Flow matrix** | How labels block each other (cross-label dependency flow) |
| `SPC v t` / `SPC v e` / `SPC v L` / `SPC v A` / `SPC v m` | **Tree / Epics / Labels / Attention / Memories** | Additional project views |

![Board view](docs/screenshots/03-kanban.webp)

## Features

**Board and detail views** - Navigate issues with vim-style keys. Expand any issue to see full markdown-rendered detail (via Glamour). Board view shows kanban columns grouped by status.

**Filter and search** - Pick labels with `SPC f l`, narrow by status with `SPC f o/c/r`, and filter by priority, type, assignee, or anything else with BQL (`:`). Fuzzy-search across the full label taxonomy.

![Label filter](docs/screenshots/04-label-filter.webp)

**Graph-based triage** - PageRank, betweenness centrality, HITS, eigenvector, and k-core metrics computed over the issue dependency graph. Cycle detection, critical path analysis, and articulation point identification. Surfaces what actually matters, not just what's loudest.

![Insights view](docs/screenshots/02-insights.webp)

**BQL (Beads Query Language)** - Composable search and filter from inside the TUI. Press `:` to open the query bar. The parser is adapted from [Perles](https://github.com/zjrosen/perles), MIT-licensed; see [`pkg/bql/LICENSE`](pkg/bql/LICENSE).

```
status = open and priority <= P2
assignee = "sms" and updated_at > -7d
type = bug or label ~ backend
```

Supports `=`, `!=`, `<`, `>`, `~` (substring), `in`, `not in`, `and`/`or`/`not`, parentheses, relative dates (`-7d`, `-3m`, `today`), `order by`, and `expand` for dependency traversal. Full reference: [`docs/bql.md`](docs/bql.md).

**Dolt lifecycle management** - Auto-starts and stops a per-session Dolt server when no shared server is reachable; defers to `bd dolt start` when one is running. Freshness monitoring with configurable stale thresholds. Auto-reconnect on connection loss.

**Theme system** - Ships with Tomorrow Night (dark) and Tomorrow Day (light). Fully customizable via YAML - user-level (`~/.config/bt/theme.yaml`) or project-level (`.bt/theme.yaml`).

**Robot mode** - Machine-readable JSON output via `bt robot <subcmd>` for AI agent integration. Triage recommendations, execution plans, priority analysis, graph metrics - all as structured JSON to stdout. See [docs/robot/README.md](docs/robot/README.md) for the full API.

## Key bindings

### Direct keys

| Key | Action |
|-----|--------|
| `j` / `k`, `up` / `down` | Navigate |
| `gg` / `home`, `G` / `end` | Jump to top / bottom |
| `Enter` | Open detail (focuses the detail pane in split view) |
| `h` / `l`, `left` / `right` | Move between list and details; view-specific horizontal navigation |
| `,` / `.` | Page the details up / down from either pane |
| `/` | Search |
| `:` | BQL query |
| `e` | Edit the selected issue |
| `y` | Copy issue ID (commit SHA in History) |
| `?` / `f1` | Local key reference |
| `esc` | Cancel / go back |
| `q` | Quit |
| `ctrl+c` | Force quit |

### Leader menu (press `Space` in a view)

| Key | Action |
|-----|--------|
| `SPC v l` / `SPC v b` / `SPC v g` | List / Board / Graph |
| `SPC v i` / `SPC v h` / `SPC v a` / `SPC v f` | Insights / History / Actionable / Flow matrix |
| `SPC v t` / `SPC v e` / `SPC v L` / `SPC v A` / `SPC v m` | Tree / Epics / Labels / Attention / Memories |
| `SPC f o` / `SPC f c` / `SPC f r` | Open / closed / ready filters |
| `SPC f s` / `SPC f l` / `SPC f R` / `SPC f x` | Cycle status / labels / recipes / clear filters |
| `SPC s s` / `SPC s S` / `SPC s t` | Cycle sort / reverse sort / triage recipe |
| `SPC b m` / `SPC b Y` / `SPC b o` / `SPC b c` | Claim / copy issue / open in editor / cass session |
| `SPC b x` / `SPC b E` / `SPC b g` | Export markdown / epic card / history bead in graph |
| `SPC p p` / `SPC p h` / `SPC p w` | Projects / home-all / toggle wisps |
| `SPC w h` / `SPC w l` / `SPC w <` / `SPC w >` | Focus or resize list pane |
| `SPC w 1` / `SPC w 2` | Fullscreen issues / details pane |
| `SPC n n` / `SPC n a` / `SPC n h` | Notifications / alerts / priority hints |
| `SPC / m` / `SPC / h` | Search ranking / hybrid preset |
| `SPC x` / `SPC r` / `SPC t` | Plugin actions / refresh / time travel |
| `SPC ,` / `SPC T` / `SPC U` | Settings / tutorial / self-update |

Press `?` for the current view's direct keys. Pause after pressing `Space` to
see leader actions in the which-key panel.

These are the default bindings. The leader key and individual actions can be rebound in `~/.config/bt/config.yaml`; see the [key configuration reference](docs/plugins.md#configuration).

## Configuration

Config is loaded in layers (later overrides earlier):

1. Built-in defaults
2. `~/.config/bt/theme.yaml` - user-level theme
3. `.bt/theme.yaml` - project-level theme

### Environment variables

The most common knobs are the Dolt-connection and freshness vars below. For the full reference — every `BT_*`, the relevant `BEADS_*`, and the bd-side `BD_ACTOR`/`BEADS_ACTOR` — see [`docs/env-vars.md`](docs/env-vars.md).

| Variable | Default | Description |
|----------|---------|-------------|
| `BEADS_DOLT_SERVER_PORT` | - | Dolt port (highest priority) |
| `BT_DOLT_PORT` | - | Dolt port (bt-specific override) |
| `BT_DOLT_POLL_INTERVAL_S` | `5` | Poll interval in seconds |
| `BT_FRESHNESS_STALE_S` | `120` | Seconds before data shows stale |
| `BT_FRESHNESS_WARN_S` | `30` | Seconds before stale warning |

### Plugins

bt can run out-of-process plugins, configured under `plugins:` in `~/.config/bt/config.yaml`, that add row badges, detail sections, BQL fields and bead actions. `SPC x` opens the plugin action menu and `bt plugins` lists what is configured. See [docs/plugins.md](docs/plugins.md) for the protocol and configuration.

## Robot mode

The `bt robot <subcmd>` family emits deterministic JSON to stdout. This is how AI agents interact with bt - no TUI, just structured data.

```bash
bt robot triage          # ranked recommendations, quick wins, blockers
bt robot next            # single top pick with claim command
bt robot plan            # parallel execution tracks
bt robot insights        # full graph metrics
bt robot alerts          # stale issues, blocking cascades
```

Run `bt robot --help` for the full subcommand list (~30+ subcmds including nested groups: `bt robot files`, `bt robot correlation`, `bt robot labels`).

Start with [`docs/agents.md`](docs/agents.md) for agent workflows. Full API reference - output shapes, flags, examples - at [`docs/robot/README.md`](docs/robot/README.md).

## Built with

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework (Elm architecture)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Glamour](https://github.com/charmbracelet/glamour) - Markdown rendering
- [Bubbles](https://github.com/charmbracelet/bubbles) - Reusable TUI components
- [Dolt](https://www.dolthub.com/) - Version-controlled database backend

## Contributing

Open [issues](https://github.com/AntoineGS/beadstui/issues) and [pull requests](https://github.com/AntoineGS/beadstui/pulls) against this repository. Contributions are for this independently maintained project; they do not need to target either predecessor.

The inherited [`CONTRIBUTING.md`](CONTRIBUTING.md) contains development and PR hygiene guidance, but its maintainer-specific policies describe Sean Martin Smith's project.

If you're new here, start with these:

- [`CONTRIBUTING.md`](CONTRIBUTING.md) - inherited development and PR hygiene guidance
- [`AGENTS.md`](AGENTS.md) - project conventions, commit format, issue-tracking workflow
- [`docs/design/testing.md`](docs/design/testing.md) - test patterns, fixtures, coverage thresholds
- [`docs/adr/`](docs/adr/) - architecture decisions ([index](docs/adr/README.md))

```bash
go build -mod=readonly ./...     # build all packages
go test -mod=readonly ./...      # run all tests
go vet -mod=readonly ./...       # static analysis
```

The codebase supports Windows, macOS, and Linux.

## License

MIT License with OpenAI/Anthropic Rider. See [LICENSE](LICENSE).

Copyright (c) 2026 Jeffrey Emanuel
Copyright (c) 2026 Sean Martin Smith

## Acknowledgments

- [Sean Martin Smith](https://github.com/seanmartinsmith) for beadstui, the project this fork continues from
- [Jeffrey Emanuel](https://github.com/Dicklesworthstone) for building [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) - the original TUI architecture and graph algorithms
- [Steve Yegge](https://github.com/steveyegge) for beads
- [Perles](https://github.com/zjrosen/perles) by Zach Rosen, the source for bt's adapted BQL parser (`pkg/bql/`, MIT)
- [Charm](https://charm.sh) for the terminal UI ecosystem
