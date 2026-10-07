package slots

import (
	"reflect"
	"sync"
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

type stubFields struct {
	names  []string
	values map[string]string
}

func (s stubFields) Fields() []string { return s.names }

func (s stubFields) Value(issue *model.Issue, field string) (string, bool) {
	v, ok := s.values[issue.ID+"/"+field]
	return v, ok
}

func TestNilRegistryIsEmpty(t *testing.T) {
	var r *Registry
	issue := &model.Issue{ID: "x-1"}
	if got := r.Badges(issue); got != nil {
		t.Fatalf("Badges on nil registry = %v, want nil", got)
	}
	if got := r.Sections(issue, Context{}); got != nil {
		t.Fatalf("Sections on nil registry = %v, want nil", got)
	}
	if got := r.FieldNames(); got != nil {
		t.Fatalf("FieldNames on nil registry = %v, want nil", got)
	}
	if _, ok := r.FieldValue(issue, "a.b"); ok {
		t.Fatal("FieldValue on nil registry reported a value")
	}
}

func TestBadgesKeepProviderOrder(t *testing.T) {
	r := NewRegistry()
	r.AddBadges(BadgeFunc(func(*model.Issue) []Badge { return []Badge{{Text: "A"}} }))
	r.AddBadges(BadgeFunc(func(*model.Issue) []Badge { return nil }))
	r.AddBadges(BadgeFunc(func(*model.Issue) []Badge { return []Badge{{Text: "B"}, {Text: "C"}} }))

	var got []string
	for _, b := range r.Badges(&model.Issue{ID: "x-1"}) {
		got = append(got, b.Text)
	}
	if want := []string{"A", "B", "C"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("badge order = %v, want %v", got, want)
	}
	if got := r.Badges(nil); got != nil {
		t.Fatalf("Badges(nil issue) = %v, want nil", got)
	}
}

func TestSectionsReceiveContext(t *testing.T) {
	r := NewRegistry()
	r.AddSections(SectionFunc(func(_ *model.Issue, ctx Context) []Section {
		if !ctx.WorkspaceMode {
			return nil
		}
		return []Section{{Title: "W", Markdown: "w"}}
	}))
	issue := &model.Issue{ID: "x-1"}
	if got := r.Sections(issue, Context{}); len(got) != 0 {
		t.Fatalf("sections outside workspace mode = %v, want none", got)
	}
	if got := r.Sections(issue, Context{WorkspaceMode: true}); len(got) != 1 || got[0].Title != "W" {
		t.Fatalf("sections in workspace mode = %v, want one titled W", got)
	}
}

func TestFieldsFirstProviderWins(t *testing.T) {
	r := NewRegistry()
	r.AddFields(stubFields{names: []string{"p.state", "p.owner"}, values: map[string]string{"x-1/p.state": "waiting"}})
	r.AddFields(stubFields{names: []string{"p.state", "q.lane"}, values: map[string]string{"x-1/p.state": "shadowed", "x-1/q.lane": "two"}})

	if got, want := r.FieldNames(), []string{"p.owner", "p.state", "q.lane"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FieldNames = %v, want %v", got, want)
	}
	issue := &model.Issue{ID: "x-1"}
	if v, ok := r.FieldValue(issue, "p.state"); !ok || v != "waiting" {
		t.Fatalf("p.state = %q, %v; want waiting, true", v, ok)
	}
	if v, ok := r.FieldValue(issue, "q.lane"); !ok || v != "two" {
		t.Fatalf("q.lane = %q, %v; want two, true", v, ok)
	}
	if _, ok := r.FieldValue(issue, "p.owner"); ok {
		t.Fatal("p.owner has no value for x-1 but FieldValue reported one")
	}
	if _, ok := r.FieldValue(issue, "nope"); ok {
		t.Fatal("unregistered field reported a value")
	}
}

func TestRegistryConcurrentUse(t *testing.T) {
	r := NewRegistry()
	issue := &model.Issue{ID: "x-1"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.AddBadges(BadgeFunc(func(*model.Issue) []Badge { return []Badge{{Text: "A"}} }))
			r.AddFields(stubFields{names: []string{"p.state"}})
			r.AddSections(SectionFunc(func(*model.Issue, Context) []Section { return nil }))
		}()
		go func() {
			defer wg.Done()
			_ = r.Badges(issue)
			_ = r.FieldNames()
			_, _ = r.FieldValue(issue, "p.state")
			_ = r.Sections(issue, Context{})
		}()
	}
	wg.Wait()
	if got := len(r.Badges(issue)); got != 8 {
		t.Fatalf("badges after concurrent adds = %d, want 8", got)
	}
}
