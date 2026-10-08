# Beads - AI-Native Issue Tracking

This fork uses **Beads** for independent issue tracking. Issues are stored in
the `beadstui_antoinegs` Dolt database on the local shared server, with the
`bt-` issue prefix and a project identity separate from upstream.

`dolt.local-only: true` disables remote synchronization and prevents adopting
the upstream code repository as a Dolt remote. The upstream private Beads
repository is not used, and upstream issue history has not been imported.

The database is local runtime state, not part of git. Cloning this fork does
not restore its issues. Use a Dolt backup or explicitly configure a fork-owned
Dolt remote before relying on cross-machine persistence. `bd sync` is a safe
no-op while local-only mode is enabled.

Beads 1.3.1's `bd config validate` reports `federation.remote: required for
Dolt sync` even with local-only mode enabled. That validator does not check
`dolt.local-only`; use `bd doctor --server` to check database health and
`bd sync --json` to confirm the `disabled` sync status instead.

## What is Beads?

Beads is issue tracking that lives in your repo, making it perfect for AI coding agents and developers who want their issues close to their code. No web UI required - everything works through the CLI and integrates seamlessly with git.

**Learn more:** [github.com/gastownhall/beads](https://github.com/gastownhall/beads)

## Quick Start

### Essential Commands

```bash
# Create new issues
bd create "Add user authentication"

# View all issues
bd list

# View issue details
bd show <issue-id>

# Update issue status
bd update <issue-id> --status in_progress
bd close <issue-id> --reason="Summary: ..."

# Safe no-op in this fork's local-only mode
bd sync
```

### Working with Issues

Issues in Beads are:
- **Dolt-native**: Stored in the local Dolt database, with versioned history
- **AI-friendly**: CLI-first design works perfectly with AI coding agents
- **Branch-aware**: Issues can follow your branch workflow
- **Independent**: Fork issues do not modify upstream issue state

## Why Beads?

✨ **AI-Native Design**
- Built specifically for AI-assisted development workflows
- CLI-first interface works seamlessly with AI coding agents
- No context switching to web UIs

🚀 **Developer Focused**
- Issues live in your repo, right next to your code
- Works without access to the upstream Beads repository
- Fast, lightweight, and stays out of your way

🔧 **Git Integration**
- Dolt commit history for issue changes
- Optional Dolt remotes for cross-machine synchronization
- Code commits alone do not back up the issue database

## Get Started with Beads

Try Beads in your own projects:

```bash
# Install Beads
curl -sSL https://raw.githubusercontent.com/gastownhall/beads/main/scripts/install.sh | bash

# Initialize in your repo
bd init

# Create your first issue
bd create "Try out Beads"
```

## Learn More

- **Documentation**: [github.com/gastownhall/beads/docs](https://github.com/gastownhall/beads/tree/main/docs)
- **Quick Start Guide**: Run `bd quickstart`
- **Examples**: [github.com/gastownhall/beads/examples](https://github.com/gastownhall/beads/tree/main/examples)

---

*Beads: Issue tracking that moves at the speed of thought* ⚡
