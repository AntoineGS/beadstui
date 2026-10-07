// Package slots holds the TUI's in-process extension seams: badges on bead
// rows, sections in the detail pane, and extra BQL fields. Built-in features
// and extensions register providers on a Registry; renderers only read from
// it. A provider contributes data, never rendering, so every badge and
// section is drawn with bt's own theme.
package slots

import (
	"sort"
	"sync"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/seanmartinsmith/beadstui/pkg/model"
)

// Tone is a badge's semantic colour; the TUI maps it to theme colours.
type Tone int

const (
	ToneMuted Tone = iota
	ToneAccent
	ToneOK
	ToneWarn
	ToneError
)

// Badge is a short label drawn on a bead's row.
type Badge struct {
	Text string
	Tone Tone
	// Since, when set, is shown after Text as a compact age ("WAIT 4m").
	Since time.Time
	// Style overrides Tone. For built-in providers that already have a
	// house style; extensions use Tone.
	Style *lipgloss.Style
}

// Section is a titled markdown block in the detail pane.
type Section struct {
	Title    string
	Markdown string
}

// Context carries view state a provider may depend on.
type Context struct {
	WorkspaceMode bool
}

// BadgeProvider contributes badges for a bead.
type BadgeProvider interface {
	Badges(issue *model.Issue) []Badge
}

// SectionProvider contributes detail-pane sections for a bead.
type SectionProvider interface {
	Sections(issue *model.Issue, ctx Context) []Section
}

// FieldProvider answers extra BQL fields, compared as strings.
type FieldProvider interface {
	// Fields lists the field names this provider answers, e.g. "agent.state".
	Fields() []string
	// Value returns the field's value for issue; ok is false when the bead
	// has no value.
	Value(issue *model.Issue, field string) (value string, ok bool)
}

// BadgeFunc adapts a function to BadgeProvider.
type BadgeFunc func(issue *model.Issue) []Badge

// Badges calls f.
func (f BadgeFunc) Badges(issue *model.Issue) []Badge { return f(issue) }

// SectionFunc adapts a function to SectionProvider.
type SectionFunc func(issue *model.Issue, ctx Context) []Section

// Sections calls f.
func (f SectionFunc) Sections(issue *model.Issue, ctx Context) []Section { return f(issue, ctx) }

// Registry holds providers in registration order. It is safe for concurrent
// use, and every read method is safe on a nil *Registry, which behaves as
// empty.
type Registry struct {
	mu       sync.RWMutex
	badges   []BadgeProvider
	sections []SectionProvider
	fields   []FieldProvider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// AddBadges registers a badge provider after those already registered.
func (r *Registry) AddBadges(p BadgeProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.badges = append(r.badges, p)
}

// AddSections registers a section provider after those already registered.
func (r *Registry) AddSections(p SectionProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sections = append(r.sections, p)
}

// AddFields registers a field provider. When two providers declare the same
// field, the earlier one answers it.
func (r *Registry) AddFields(p FieldProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fields = append(r.fields, p)
}

// Badges returns every provider's badges for issue, in registration order.
func (r *Registry) Badges(issue *model.Issue) []Badge {
	if r == nil || issue == nil {
		return nil
	}
	r.mu.RLock()
	providers := r.badges
	r.mu.RUnlock()
	var out []Badge
	for _, p := range providers {
		out = append(out, p.Badges(issue)...)
	}
	return out
}

// Sections returns every provider's sections for issue, in registration order.
func (r *Registry) Sections(issue *model.Issue, ctx Context) []Section {
	if r == nil || issue == nil {
		return nil
	}
	r.mu.RLock()
	providers := r.sections
	r.mu.RUnlock()
	var out []Section
	for _, p := range providers {
		out = append(out, p.Sections(issue, ctx)...)
	}
	return out
}

// FieldNames returns the sorted, de-duplicated names of all registered fields.
func (r *Registry) FieldNames() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	providers := r.fields
	r.mu.RUnlock()
	seen := map[string]bool{}
	var names []string
	for _, p := range providers {
		for _, name := range p.Fields() {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// FieldValue asks the first provider that declares field for issue's value.
func (r *Registry) FieldValue(issue *model.Issue, field string) (string, bool) {
	if r == nil || issue == nil {
		return "", false
	}
	r.mu.RLock()
	providers := r.fields
	r.mu.RUnlock()
	for _, p := range providers {
		for _, name := range p.Fields() {
			if name == field {
				return p.Value(issue, field)
			}
		}
	}
	return "", false
}
