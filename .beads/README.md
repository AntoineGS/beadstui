# Beads - AI-Native Issue Tracking

This fork uses **Beads** for independent issue tracking. Issues are stored in
the `beadstui_antoinegs` Dolt database on the local shared server, with the
`bt-` issue prefix and a project identity separate from upstream.

The Dolt `origin` remote explicitly targets
`git+ssh://git@github.com/AntoineGS/beadstui.git`. Both `sync.remote` (clone
recovery) and `federation.remote` (configuration validation and reconciliation)
declare that same fork-owned URL in `.beads/config.yaml`.
The upstream private Beads repository is not used, and upstream issue history
has not been imported. Git's code remotes are separate from Dolt's remotes;
do not adopt Git `origin`, which may still point upstream.

### Synchronization and recovery

With SSH access to the fork, run:

```bash
# Pull and push the issue database, without adopting a Git remote
bd sync --no-adopt

# Verify the configured target and database health
bd dolt remote list
bd config validate
bd doctor --server
```

Dolt stores its data separately from code branches in the fork repository.
`bd sync` does not push code commits, and code commits alone do not back up
the database. Issue changes are committed locally by Beads; run `bd sync`
after changes to publish them.

After cloning the fork on another machine, install Beads and Dolt, ensure the
configured local shared Dolt server is running, then recover the issues:

```bash
bd bootstrap --dry-run
bd bootstrap --yes
bd sync --no-adopt
```

Bootstrap uses the explicit `sync.remote` rather than the upstream code remote.
Use it for a fresh clone without an existing database; for an already populated
shared-server database, use `bd sync --no-adopt` instead. Review the dry-run plan
before recovery, and do not use `bd init --force`.

If sync refuses to auto-commit pending internal configuration changes, inspect
them with `bd sql "SELECT * FROM dolt_status"`. Once reviewed, explicitly commit
them with `bd dolt commit` before retrying sync; do not discard them or force-push.

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

# Synchronize only with the explicitly configured fork remote
bd sync --no-adopt
```

### Working with Issues

Issues in Beads are:
- **Dolt-native**: Stored in the local Dolt database, with remotely synchronized history
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
- Fork-owned Dolt remote for cross-machine synchronization
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
