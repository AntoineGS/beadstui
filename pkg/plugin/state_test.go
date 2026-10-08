package plugin

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

func TestStoreSetValidatesReplacesAndClears(t *testing.T) {
	s := newStore([]Config{{Name: "example"}}, func(*model.Issue) string { return "proj" })
	one, two := &model.Issue{ID: "example-1"}, &model.Issue{ID: "example-2"}
	since := time.Date(2026, 10, 7, 14, 3, 0, 0, time.UTC)

	s.set("example", StateSetParams{Beads: []BeadState{{DB: "proj", ID: "example-1"}}})
	if s.takeDirty() != nil {
		t.Fatal("state.set before activation must be ignored")
	}

	s.activate("example", validManifest())
	s.set("example", StateSetParams{Beads: []BeadState{
		{DB: "proj", ID: "example-1",
			Fields: map[string]FieldState{
				"state":   {Value: "waiting", Since: &since},
				"unknown": {Value: "x"},
			},
			Sections: map[string]string{"agent": strings.Repeat("é", maxSection), "other": "dropped"},
			Actions:  []string{"dispatch", "undeclared"},
		},
		{DB: "proj", ID: "example-2", Fields: map[string]FieldState{"state": {Value: "bogus"}}},
	}})
	if got := s.badges(one); len(got) != 1 || got[0].Text != "WAIT" || got[0].Tone != slots.ToneWarn || !got[0].Since.Equal(since) {
		t.Errorf("badges = %+v", got)
	}
	secs := s.sections(one, slots.Context{})
	if len(secs) != 1 || secs[0].Title != "Agent" || len(secs[0].Markdown) > maxSection || !strings.HasSuffix(secs[0].Markdown, "é") {
		t.Errorf("section not truncated on a rune boundary: %d bytes", len(secs[0].Markdown))
	}
	if got := s.actions(one); len(got) != 1 || got[0].ID != "dispatch" {
		t.Errorf("actions = %+v", got)
	}
	if v, ok := (fieldProvider{s}).Value(two, "example.state"); ok {
		t.Errorf("undeclared value kept: %q", v)
	}
	if got := s.takeDirty(); !reflect.DeepEqual(got, []BeadKey{{DB: "proj", ID: "example-1"}, {DB: "proj", ID: "example-2"}}) {
		t.Errorf("dirty = %v", got)
	}

	s.set("example", StateSetParams{Replace: true, Beads: []BeadState{
		{DB: "proj", ID: "example-2", Fields: map[string]FieldState{"state": {Value: "queued"}}},
	}})
	if got := s.badges(one); len(got) != 0 {
		t.Errorf("replace kept example-1: %+v", got)
	}
	if v, _ := (fieldProvider{s}).Value(two, "example.state"); v != "queued" {
		t.Errorf("example-2 state = %q", v)
	}
	if got := s.takeDirty(); len(got) != 2 {
		t.Errorf("replace must dirty old and new beads, got %v", got)
	}

	s.clear("example", []BeadKey{{DB: "proj", ID: "example-2"}})
	if _, ok := (fieldProvider{s}).Value(two, "example.state"); ok {
		t.Error("state.clear kept example-2")
	}

	s.deactivate("example")
	if got := (fieldProvider{s}).Fields(); len(got) != 0 {
		t.Errorf("fields after deactivate = %v", got)
	}
}
