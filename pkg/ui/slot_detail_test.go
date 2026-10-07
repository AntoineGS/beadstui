package ui

import (
	"strings"
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

func TestDetailPaneShowsSlotSectionsAfterProperties(t *testing.T) {
	issue := slotTestIssue()
	issue.Description = "DESCRIPTIONMARKER"
	m := NewModel([]model.Issue{issue}, nil, "", nil, nil)
	m.slotRegistry.AddSections(slots.SectionFunc(func(*model.Issue, slots.Context) []slots.Section {
		return []slots.Section{{Title: "Agent", Markdown: "SESSIONMARKER"}}
	}))

	m.updateViewportContent()
	out := m.viewport.View()

	agent := strings.Index(out, "SESSIONMARKER")
	desc := strings.Index(out, "DESCRIPTIONMARKER")
	if agent < 0 {
		t.Fatalf("detail pane missing slot section:\n%s", out)
	}
	if desc >= 0 && agent > desc {
		t.Fatalf("slot section should precede the description:\n%s", out)
	}
}
