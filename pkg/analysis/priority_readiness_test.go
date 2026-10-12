package analysis

import (
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

func TestCountTransitiveUnblocksUsesFullReadinessFrontier(t *testing.T) {
	issues := []model.Issue{
		{ID: "gate", Status: model.StatusOpen},
		{ID: "hidden-parent", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "gate", Type: model.DepBlocks}}},
		{ID: "child", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "hidden-parent", Type: model.DepParentChild}}},
		{ID: "downstream", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "child", Type: model.DepBlocks}}},
		{ID: "independent-gate", Status: model.StatusOpen},
		{ID: "independent", Status: model.StatusOpen, Dependencies: []*model.Dependency{
			{DependsOnID: "gate", Type: model.DepBlocks},
			{DependsOnID: "independent-gate", Type: model.DepBlocks},
		}},
		{ID: "already-ready", Status: model.StatusOpen},
		{ID: "closed-parent", Status: model.StatusClosed, Dependencies: []*model.Dependency{{DependsOnID: "gate", Type: model.DepBlocks}}},
		{ID: "preexisting-child", Status: model.StatusOpen, Dependencies: []*model.Dependency{{DependsOnID: "closed-parent", Type: model.DepParentChild}}},
	}
	selected := []model.Issue{issues[0], issues[2], issues[3], issues[4], issues[5], issues[6], issues[8]}
	analyzer := NewAnalyzerWithReadiness(selected, model.NewReadinessIndex(issues), time.Unix(1, 0))

	direct := analyzer.computeUnblocks("gate")
	if len(direct) != 1 || direct[0] != "child" {
		t.Fatalf("direct unblocks = %v, want [child]", direct)
	}
	if got := analyzer.countTransitiveUnblocks("gate"); got != 2 {
		t.Fatalf("transitive unblocks = %d, want child and downstream only", got)
	}
}
