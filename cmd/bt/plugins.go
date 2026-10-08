package main

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/seanmartinsmith/beadstui/internal/bdroute"
	"github.com/seanmartinsmith/beadstui/internal/datasource"
	"github.com/seanmartinsmith/beadstui/pkg/debug"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/version"
)

// pluginRepoMissTTL is how long a database without a known checkout is
// remembered before Resolve is tried again.
const pluginRepoMissTTL = 30 * time.Second

// loadPluginHost reads the plugins: list from the user config and builds a
// host for it. Config errors are logged; the valid entries still run.
func loadPluginHost(ctx *appContext, popup bool) *plugin.Host {
	path, err := plugin.DefaultConfigPath()
	if err != nil {
		debug.Log("plugins: %v", err)
		return nil
	}
	configs, err := plugin.LoadConfig(path)
	if err != nil {
		debug.Log("plugins: %v", err)
	}
	return buildPluginHost(ctx, configs, popup)
}

// buildPluginHost returns a host for configs, or nil when none is enabled.
// The host is not started.
func buildPluginHost(ctx *appContext, configs []plugin.Config, popup bool) *plugin.Host {
	enabled := false
	for _, c := range configs {
		if c.IsEnabled() {
			enabled = true
			break
		}
	}
	if !enabled {
		return nil
	}
	scope := pluginScope(ctx)
	db := pluginBeadDB(detectCurrentProjectDB(), scope.Mode)
	repos := &pluginRepoCache{
		db:      db,
		resolve: ctx.routeTable.Resolve,
		now:     time.Now,
		entries: map[string]pluginRepoEntry{},
	}
	return plugin.NewHost(plugin.Options{
		Configs:   configs,
		BTVersion: version.Version,
		Scope:     scope,
		Popup:     popup,
		DB:        db,
		Repo:      repos.repo,
	})
}

func isGlobalSource(ctx *appContext) bool {
	return ctx.selectedSource != nil && ctx.selectedSource.Type == datasource.SourceTypeDoltGlobal
}

// pluginScope reports which beads bt shows: every database of the shared
// server (global), a .bt/workspace.yaml set (workspace), or one project.
func pluginScope(ctx *appContext) plugin.Scope {
	if isGlobalSource(ctx) {
		seen := map[string]bool{}
		var dbs []string
		for _, issue := range ctx.issues {
			if issue.SourceRepo != "" && !seen[issue.SourceRepo] {
				seen[issue.SourceRepo] = true
				dbs = append(dbs, issue.SourceRepo)
			}
		}
		sort.Strings(dbs)
		return plugin.Scope{Mode: "global", Databases: dbs}
	}
	if ctx.workspaceInfo != nil {
		return plugin.Scope{Mode: "workspace"}
	}
	return plugin.Scope{Mode: "project"}
}

// pluginBeadDB returns the database a bead belongs to for scope mode. In
// global mode SourceRepo is the database name. In workspace mode beads carry
// no SourceRepo, so the ID prefix before the first "-" names the repo, the
// same key bdroute.FromWorkspace resolves. Elsewhere SourceRepo is only a bd
// column, so the project's own database wins.
func pluginBeadDB(projectDB, mode string) func(*model.Issue) string {
	return func(i *model.Issue) string {
		switch mode {
		case "global":
			if i.SourceRepo != "" {
				return i.SourceRepo
			}
		case "workspace":
			if prefix, _, ok := strings.Cut(i.ID, "-"); ok && prefix != "" {
				return prefix
			}
		}
		if projectDB != "" {
			return projectDB
		}
		return i.SourceRepo
	}
}

type pluginRepoEntry struct {
	dir string
	at  time.Time
}

// pluginRepoCache resolves a database's checkout path once. Known paths are
// kept for the session; unknown ones are retried after pluginRepoMissTTL.
// It is called from the host's goroutines. repo holds the mutex across
// Resolve, which does file IO in global mode, so it must never be called
// from the UI goroutine.
type pluginRepoCache struct {
	db      func(*model.Issue) string
	resolve func(model.Issue) (bdroute.WriteTarget, error)
	now     func() time.Time

	mu      sync.Mutex
	entries map[string]pluginRepoEntry
}

func (c *pluginRepoCache) repo(issue *model.Issue) string {
	key := c.db(issue)
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok && (e.dir != "" || c.now().Sub(e.at) < pluginRepoMissTTL) {
		return e.dir
	}
	var dir string
	if target, err := c.resolve(*issue); err == nil {
		dir = target.Dir
	} else {
		debug.Log("plugins: no checkout for database %q: %v", key, err)
	}
	c.entries[key] = pluginRepoEntry{dir: dir, at: c.now()}
	return dir
}
