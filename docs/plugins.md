# Plugins

bt can run plugins: separate executables that add badges to rows, sections to the
detail pane, BQL fields and actions on beads. A plugin speaks JSON-RPC 2.0 over
its stdin and stdout, so it can be written in any language, and a crash in a
plugin cannot take bt down.

Plugins contribute data only. bt renders everything with its own theme. Plugins
run only while the interactive TUI runs; they never run in `bt robot` or
`--as-of` mode.

## Configuration

Plugins are listed in `~/.config/bt/config.yaml`. There are no project-level
plugins.

```yaml
plugins:
  - name: example               # unique; prefixes BQL fields and action IDs
    command: ["example", "plugin"]  # argv, never run through a shell
    enabled: true               # default true
    env: {}                     # added to bt's environment
    options:                    # opaque; sent in initialize
      roots: ["~/gits"]
    keys:                       # overrides by action ID; "none" disables
      dispatch: D
      jump: none
```

`name` must match `^[a-z][a-z0-9_]*$`. An entry with an invalid name or an empty
command is ignored, and a duplicate name disables the later entry. `bt plugins`
reports these errors.

Check the configuration without starting anything:

```console
$ bt plugins
example  enabled  example plugin  (/usr/local/bin/example)  keys: dispatch=D jump=none
$ bt plugins --json
[
  {
    "name": "example",
    "enabled": true,
    "command": ["example", "plugin"],
    "binary": "/usr/local/bin/example",
    "keys": { "dispatch": "D", "jump": "none" }
  }
]
```

`binary` is `""` when the command is not found on `PATH`, and `keys` is left out
when there are no overrides.

Each line shows the plugin name, `enabled` or `disabled`, the command, the binary
it resolves to on `PATH` (or `not found`) and the key overrides. A configuration
error is printed and makes the command exit non-zero; the valid entries are still
listed.

## Lifecycle

1. After the first snapshot is ready, bt starts every enabled plugin with bt's
   working directory.
2. bt sends `initialize`. The plugin must answer within 5 seconds with its
   manifest. Until the manifest is validated, bt sends the plugin nothing else
   and refuses everything the plugin sends (requests get a `not_ready` error).
3. bt sends `beads.sync`, then again whenever the data changes. A plugin must not
   push `state.set` or `ui.toast` until its manifest is accepted: they are
   refused with a `not_ready` error before that. Waiting for the first
   `beads.sync` is recommended, since that is when the plugin learns which beads
   exist.
4. On quit, bt sends the `shutdown` notification, closes the plugin's stdin,
   waits 2 seconds and then kills the process.
5. If the process exits unexpectedly, fails to start, answers `initialize` late
   or sends an invalid manifest, bt drops everything the plugin contributed,
   reports its in-flight actions as "result unknown" and restarts it. The policy
   is the first start plus 3 restarts, after 1s, 5s and 25s. After the third
   restart fails the plugin stays failed until bt restarts, and bt shows a footer
   notice.

All plugin I/O happens in background goroutines. The UI never waits on a plugin.

## Transport

- One JSON object per line (UTF-8, `\n`) on the plugin's stdin (bt to plugin) and
  stdout (plugin to bt). Lines are limited to 4 MiB.
- The plugin's stderr is copied to bt's debug log, prefixed with the plugin name.
- Requests carry an `id`; notifications do not. Either side may send requests.
- A plugin failure is error code `-32000` with `data.type` set to a
  plugin-defined string:
  `{"code": -32000, "message": "no checkout for proj", "data": {"type": "no_repo"}}`.
  The standard codes `-32700` to `-32603` are used for protocol errors.
- An unknown method is ignored when it arrives as a notification, and answered
  with `-32601` when it is a request.
- A line that is not valid JSON-RPC, or a message with invalid params, is logged
  and dropped. After 10 of them bt kills the plugin and treats it as crashed.
- Timeouts: `initialize` 5s, `action.invoke` 60s. `ui.*` requests have no timeout
  of their own, because they wait for the user, but they are cancelled when the
  action they belong to ends.

## Protocol (version 1)

### Bead references

```json
{ "db": "proj", "id": "proj-a1b", "repo": "/home/u/gits/proj" }
```

`db` is the Beads database name. In global mode it is the issue's source
repository; in project mode it is the project's own database; in workspace mode
it is the bead ID prefix before the first `-`. `repo` is the checkout path when
bt can resolve one, otherwise `null`. bt resolves the path once per database per
sync, so every bead of a database carries the same `repo`. Plugins should key all
state by `(db, id)`.

### `initialize` (bt to plugin, request)

```json
{
  "protocolVersion": 1,
  "bt": { "version": "0.4.0" },
  "scope": { "mode": "global", "databases": ["proj", "docs"] },
  "popup": false,
  "options": { "roots": ["~/gits"] }
}
```

`scope.mode` is `project`, `global` or `workspace`. `databases` lists the
databases in view in global mode. `popup` is true when bt was started with
`--popup`. `options` is the config entry's `options`.

The result is the manifest:

```json
{
  "protocolVersion": 1,
  "name": "example",
  "version": "0.3.0",
  "subscribe": {
    "metadata_prefixes": ["example."],
    "statuses": ["open", "in_progress", "review", "blocked"]
  },
  "fields": [{
    "id": "state",
    "label": "Agent",
    "values": [
      { "id": "waiting", "label": "Waiting on you", "badge": "WAIT", "tone": "warn" },
      { "id": "running", "label": "In progress",    "badge": "RUN",  "tone": "accent" },
      { "id": "done",    "label": "Finished",       "badge": "OK",   "tone": "ok" },
      { "id": "unknown", "label": "Unknown",        "badge": "?",    "tone": "muted" },
      { "id": "queued",  "label": "Queued", "lane": false }
    ]
  }],
  "sections": [{ "id": "details", "title": "Example" }],
  "actions": [
    { "id": "dispatch", "label": "Dispatch", "key": "D" },
    { "id": "jump",     "label": "Jump",     "key": "J" }
  ]
}
```

A value's optional `lane` (boolean, default true) says whether it gets a board
swimlane. It is reserved for board swimlanes, which this version does not draw
yet, so it has no visible effect.

If validation fails, the manifest is rejected and the plugin counts as failed
(see [Lifecycle](#lifecycle)):

- `protocolVersion` equals bt's (1).
- `name` equals the config entry's `name`.
- IDs of fields, values, sections and actions match `^[a-z][a-z0-9_]*$` and are
  unique within their list.
- `tone` is one of `accent`, `ok`, `warn`, `error`, `muted`; bt maps it to theme
  colors.
- `badge` is at most 6 cells wide.
- An action's `key` is not empty.

A field is exposed in BQL as `<plugin>.<field>`, for example
`example.state = waiting`. A bead with no value for a field matches no
comparison on it.

`subscribe.statuses` omitted means every status. `metadata_prefixes` omitted
means no metadata is sent.

### `beads.sync` (bt to plugin, notification)

```json
{
  "revision": 12,
  "beads": [{
    "db": "proj", "id": "proj-a1b", "repo": "/home/u/gits/proj",
    "status": "in_progress", "title": "Add retries", "type": "task", "priority": 2,
    "assignee": "alex", "metadata": { "example.session": "s-123" }
  }]
}
```

Always the full set of beads matching `subscribe`. It is sent once after the
manifest is accepted and again whenever a snapshot's data changes. `revision`
increases by one each time. Descriptions and comments are never sent.

### `state.set` and `state.clear` (plugin to bt, notifications)

```json
{
  "replace": false,
  "beads": [{
    "db": "proj", "id": "proj-a1b",
    "fields": { "state": { "value": "waiting", "since": "2026-10-07T14:03:00Z" } },
    "sections": { "details": "**Session** `s-123`\n\nWaiting on a question." },
    "actions": ["jump"]
  }]
}
```

- Each entry replaces that bead's previous entry from this plugin. With
  `replace: true`, everything the plugin pushed before is dropped first.
- `fields` values must be declared in the manifest. An unknown field or value
  drops that field and logs a debug line. `since` is optional; when present the
  badge shows an age ("WAIT 4m").
- `sections` values are markdown, at most 16 KiB each.
- `actions` lists the actions available on this bead. A bead without an entry
  from the plugin has none of the plugin's actions.
- Entries for beads bt does not currently hold are kept and show up if the bead
  appears later.

`state.clear` takes `{ "beads": [{ "db": "proj", "id": "proj-a1b" }] }` and
removes those entries. bt redraws at most every 100ms in response to state
pushes.

All text a plugin sends (badges, sections, toasts, prompts) is sanitized by bt
before it is drawn, so control sequences cannot reach the terminal.

### `action.invoke` (bt to plugin, request)

```json
{ "action": "dispatch", "bead": { "db": "proj", "id": "proj-a1b", "repo": "/home/u/gits/proj" }, "view": "list" }
```

`view` is `list`, `board`, `tree` or `epics`. The result has two optional fields:

```json
{ "toast": { "message": "Started s-123", "tone": "ok" }, "dismiss": false }
```

An error result shows its `message` as an error toast.

While the call runs, a list row shows bt's pending spinner; in the board, tree
and epics views bt shows a notice instead. The pending state clears on the
result, on the next `state.set` entry for that bead, or after 65 seconds,
whichever comes first. A timeout (60s) or a plugin crash is reported as "result
unknown" and the action is never retried or replayed. Only one action per bead
runs at a time; a second request on the same bead is refused with a notice.

`dismiss: true` quits bt only when bt was started with `--popup`; otherwise it
is ignored.

### `ui.confirm` and `ui.select` (plugin to bt, requests)

Allowed only while an `action.invoke` from the same plugin is in flight;
otherwise they fail with `-32000` and `data.type` `no_action`.

- `ui.confirm`: `{ "title", "message", "confirm"?, "cancel"? }` returns `true`,
  `false`, or `null` if the user dismissed it.
- `ui.select`: `{ "title", "options": [{ "value", "label", "description"? }] }`
  returns the chosen `value`, or `null` if dismissed.

Send these as **requests** with an `id`, never as notifications. bt handles
notifications on the same reader that delivers the plugin's other messages, and
a notification that waits for the user would block that reader until the action
ends. Send the request, keep reading, and handle the response when it arrives.

### `ui.toast` (plugin to bt, notification)

`{ "message": "…", "tone": "warn" }`; `tone` is optional. Allowed once the plugin
is active. bt shows at most one toast per plugin per second and drops the rest.

### `shutdown` (bt to plugin, notification)

No params. The plugin should exit promptly.

## Keys and the action menu

`P` opens the plugin action menu in the list, board, tree and epics views. It
lists the actions available on the selected bead, grouped by plugin, with their
keys. Every action is reachable from the menu, even when its key is disabled or
taken.

An action also binds its key directly in those views. The key is the `keys`
override from the config, else the manifest's default; `none` removes the key
and leaves the action in the menu only. A key bt itself binds in a view, or that
an earlier plugin in the config claimed, is not bound.

## `--popup`

`bt --popup` is meant for a terminal popup that should close when the job is
done. It changes one thing: an action result with `dismiss: true` quits bt.
Without the flag, `dismiss` is ignored. Plugins receive the flag as `popup` in
`initialize`.

## Failure behaviour

- A plugin that fails to start, answers `initialize` late, sends an invalid
  manifest or crashes counts as a failure and is restarted as described in
  [Lifecycle](#lifecycle). Once the restarts are used up it is marked failed and
  bt shows one footer notice; details are in the debug log.
- An action in flight when the plugin goes away is reported as "result unknown".
  bt does not know whether it took effect and never repeats it.
- A plugin never blocks rendering or input.
