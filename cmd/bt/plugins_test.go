package main

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/internal/bdroute"
	"github.com/seanmartinsmith/beadstui/internal/datasource"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/plugin"
	"github.com/seanmartinsmith/beadstui/pkg/workspace"
)

func TestPluginBeadDB(t *testing.T) {
	global := pluginBeadDB("proj", true)
	if got := global(&model.Issue{ID: "x-1", SourceRepo: "other"}); got != "other" {
		t.Fatalf("global mode with SourceRepo = %q, want other", got)
	}
	if got := global(&model.Issue{ID: "x-1"}); got != "proj" {
		t.Fatalf("global mode without SourceRepo = %q, want proj", got)
	}

	project := pluginBeadDB("proj", false)
	if got := project(&model.Issue{ID: "x-1", SourceRepo: "column"}); got != "proj" {
		t.Fatalf("project mode = %q, want proj (SourceRepo is not the database there)", got)
	}

	unknown := pluginBeadDB("", false)
	if got := unknown(&model.Issue{ID: "x-1", SourceRepo: "legacy"}); got != "legacy" {
		t.Fatalf("no project DB = %q, want the SourceRepo fallback", got)
	}
}

func TestPluginScope(t *testing.T) {
	ctx := &appContext{}
	if got := pluginScope(ctx); got.Mode != "project" || got.Databases != nil {
		t.Fatalf("project scope = %+v", got)
	}

	ctx = &appContext{workspaceInfo: &workspace.LoadSummary{}}
	if got := pluginScope(ctx); got.Mode != "workspace" {
		t.Fatalf("workspace scope = %+v", got)
	}

	ctx = &appContext{
		workspaceInfo:  &workspace.LoadSummary{},
		selectedSource: &datasource.DataSource{Type: datasource.SourceTypeDoltGlobal},
		issues: []model.Issue{
			{ID: "b-1", SourceRepo: "proj"},
			{ID: "a-1", SourceRepo: "alpha"},
			{ID: "b-2", SourceRepo: "proj"},
			{ID: "c-1"},
		},
	}
	got := pluginScope(ctx)
	if got.Mode != "global" || !reflect.DeepEqual(got.Databases, []string{"alpha", "proj"}) {
		t.Fatalf("global scope = %+v", got)
	}
}

func TestPluginRepoCache(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	calls := map[string]int{}
	known := map[string]string{"proj": "/src/proj"}
	c := &pluginRepoCache{
		db: func(i *model.Issue) string { return i.SourceRepo },
		resolve: func(i model.Issue) (bdroute.WriteTarget, error) {
			calls[i.SourceRepo]++
			if dir, ok := known[i.SourceRepo]; ok {
				return bdroute.WriteTarget{Dir: dir}, nil
			}
			return bdroute.WriteTarget{}, errors.New("unknown")
		},
		now:     func() time.Time { return now },
		entries: map[string]pluginRepoEntry{},
	}

	if got := c.repo(&model.Issue{ID: "p-1", SourceRepo: "proj"}); got != "/src/proj" {
		t.Fatalf("repo = %q", got)
	}
	c.repo(&model.Issue{ID: "p-2", SourceRepo: "proj"})
	if calls["proj"] != 1 {
		t.Fatalf("hit resolved %d times, want 1", calls["proj"])
	}

	if got := c.repo(&model.Issue{ID: "o-1", SourceRepo: "other"}); got != "" {
		t.Fatalf("miss = %q, want empty", got)
	}
	now = now.Add(29 * time.Second)
	c.repo(&model.Issue{ID: "o-1", SourceRepo: "other"})
	if calls["other"] != 1 {
		t.Fatalf("miss re-resolved within 30s: %d calls", calls["other"])
	}

	known["other"] = "/src/other"
	now = now.Add(2 * time.Second)
	if got := c.repo(&model.Issue{ID: "o-1", SourceRepo: "other"}); got != "/src/other" {
		t.Fatalf("miss not retried after 30s: %q", got)
	}
	now = now.Add(time.Hour)
	c.repo(&model.Issue{ID: "p-3", SourceRepo: "proj"})
	if calls["proj"] != 1 {
		t.Fatalf("hit expired: %d calls", calls["proj"])
	}
}

func TestBuildPluginHostNeedsEnabledPlugin(t *testing.T) {
	ctx := &appContext{routeTable: bdroute.SingleProject(t.TempDir())}
	if h := buildPluginHost(ctx, nil, false); h != nil {
		t.Fatal("host built without plugins")
	}
	off := false
	if h := buildPluginHost(ctx, []plugin.Config{{Name: "example", Command: []string{"example"}, Enabled: &off}}, false); h != nil {
		t.Fatal("host built with only disabled plugins")
	}
	h := buildPluginHost(ctx, []plugin.Config{{Name: "example", Command: []string{"example"}}}, false)
	if h == nil {
		t.Fatal("no host for an enabled plugin")
	}
	h.Stop()
}
