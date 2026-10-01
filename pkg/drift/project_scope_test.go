package drift

import (
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// TestGroupByProject_CrossProjectExcludesAtlas guards bt-vdn2m: cross-project
// grouping partitions by SourceRepo and drops the beads_global (atlas)
// cross-cutting namespace, matching bt robot portfolio.
func TestGroupByProject_CrossProjectExcludesAtlas(t *testing.T) {
	issues := []model.Issue{
		{ID: "bd-1", SourceRepo: "beads"},
		{ID: "mkt-1", SourceRepo: "marketplace"},
		{ID: "global-1", SourceRepo: "beads_global"},
		{ID: "x-1", SourceRepo: ""},
	}
	groups := groupByProject(issues, true, "")
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3 (beads, marketplace, unknown): %+v", len(groups), groups)
	}
	if _, ok := groups["beads_global"]; ok {
		t.Error("beads_global group present, want excluded")
	}
	if _, ok := groups["local"]; ok {
		t.Error("cross-project grouping produced 'local'")
	}
}

func TestGroupByProject_SingleProjectOneGroup(t *testing.T) {
	issues := []model.Issue{{ID: "a-1"}, {ID: "a-2"}}
	groups := groupByProject(issues, false, "")
	if len(groups) != 1 || len(groups["local"]) != 2 {
		t.Errorf("groups = %+v, want one 'local' group of 2", groups)
	}
}
