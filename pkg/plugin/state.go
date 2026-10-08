package plugin

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/seanmartinsmith/beadstui/pkg/debug"
	"github.com/seanmartinsmith/beadstui/pkg/model"
	"github.com/seanmartinsmith/beadstui/pkg/ui/slots"
)

// maxSection is the largest section markdown kept, in bytes.
const maxSection = 16 << 10

// store holds the state active plugins pushed, keyed by plugin then bead.
// Render-path readers take only the read lock.
type store struct {
	db   func(issue *model.Issue) string
	cfgs []Config

	mu     sync.RWMutex
	active map[string]*Manifest
	beads  map[string]map[BeadKey]BeadState

	dirtyMu sync.Mutex
	dirty   map[BeadKey]struct{}
}

func newStore(cfgs []Config, db func(issue *model.Issue) string) *store {
	if db == nil {
		db = func(issue *model.Issue) string { return issue.SourceRepo }
	}
	return &store{
		db: db, cfgs: cfgs,
		active: map[string]*Manifest{},
		beads:  map[string]map[BeadKey]BeadState{},
		dirty:  map[BeadKey]struct{}{},
	}
}

// activate makes a plugin's contributions visible under its manifest.
func (s *store) activate(name string, m Manifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[name] = &m
	s.beads[name] = map[BeadKey]BeadState{}
}

// deactivate drops everything a plugin pushed and hides it.
func (s *store) deactivate(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markDirty(s.beads[name])
	delete(s.active, name)
	delete(s.beads, name)
}

// set applies a state.set from an active plugin, dropping what the manifest
// does not declare.
func (s *store) set(name string, p StateSetParams) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, beads := s.active[name], s.beads[name]
	if m == nil {
		return
	}
	if p.Replace {
		s.markDirty(beads)
		beads = map[BeadKey]BeadState{}
		s.beads[name] = beads
	}
	for _, b := range p.Beads {
		if b.DB == "" || b.ID == "" {
			continue
		}
		key := BeadKey{DB: b.DB, ID: b.ID}
		beads[key] = clean(name, m, b)
		s.markDirtyKey(key)
	}
}

// clear applies a state.clear from an active plugin.
func (s *store) clear(name string, keys []BeadKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	beads := s.beads[name]
	for _, k := range keys {
		if _, ok := beads[k]; ok {
			delete(beads, k)
			s.markDirtyKey(k)
		}
	}
}

func clean(name string, m *Manifest, b BeadState) BeadState {
	out := BeadState{DB: b.DB, ID: b.ID}
	for id, fs := range b.Fields {
		if fieldValue(m, id, fs.Value) == nil {
			debug.Log("plugin %s: dropping undeclared field %s=%s on %s", name, id, fs.Value, b.ID)
			continue
		}
		if out.Fields == nil {
			out.Fields = map[string]FieldState{}
		}
		out.Fields[id] = fs
	}
	for id, md := range b.Sections {
		if !declaresSection(m, id) {
			debug.Log("plugin %s: dropping undeclared section %s on %s", name, id, b.ID)
			continue
		}
		if out.Sections == nil {
			out.Sections = map[string]string{}
		}
		out.Sections[id] = truncateUTF8(md, maxSection)
	}
	for _, id := range b.Actions {
		if declaresAction(m, id) {
			out.Actions = append(out.Actions, id)
		}
	}
	return out
}

func fieldValue(m *Manifest, field, value string) *FieldValue {
	for _, f := range m.Fields {
		if f.ID != field {
			continue
		}
		for i := range f.Values {
			if f.Values[i].ID == value {
				return &f.Values[i]
			}
		}
	}
	return nil
}

func declaresSection(m *Manifest, id string) bool {
	for _, s := range m.Sections {
		if s.ID == id {
			return true
		}
	}
	return false
}

func declaresAction(m *Manifest, id string) bool {
	for _, a := range m.Actions {
		if a.ID == id {
			return true
		}
	}
	return false
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (s *store) markDirty(beads map[BeadKey]BeadState) {
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	for k := range beads {
		s.dirty[k] = struct{}{}
	}
}

func (s *store) markDirtyKey(k BeadKey) {
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	s.dirty[k] = struct{}{}
}

// takeDirty returns and resets the changed beads, sorted.
func (s *store) takeDirty() []BeadKey {
	s.dirtyMu.Lock()
	defer s.dirtyMu.Unlock()
	if len(s.dirty) == 0 {
		return nil
	}
	keys := make([]BeadKey, 0, len(s.dirty))
	for k := range s.dirty {
		keys = append(keys, k)
	}
	s.dirty = map[BeadKey]struct{}{}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].DB != keys[j].DB {
			return keys[i].DB < keys[j].DB
		}
		return keys[i].ID < keys[j].ID
	})
	return keys
}

// flushLoop sends one StateChangedMsg per interval while state changed,
// until stop closes.
func (s *store) flushLoop(stop <-chan struct{}, interval time.Duration, emit func(tea.Msg)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if keys := s.takeDirty(); keys != nil {
				emit(StateChangedMsg{Beads: keys})
			}
		}
	}
}

// each calls fn for every active plugin in config order with its state for
// issue, under the read lock.
func (s *store) each(issue *model.Issue, fn func(cfg Config, m *Manifest, st BeadState)) {
	if issue == nil {
		return
	}
	key := BeadKey{DB: s.db(issue), ID: issue.ID}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, cfg := range s.cfgs {
		m := s.active[cfg.Name]
		if m == nil {
			continue
		}
		fn(cfg, m, s.beads[cfg.Name][key])
	}
}

func (s *store) badges(issue *model.Issue) []slots.Badge {
	var out []slots.Badge
	s.each(issue, func(_ Config, m *Manifest, st BeadState) {
		for _, f := range m.Fields {
			fs, ok := st.Fields[f.ID]
			if !ok {
				continue
			}
			v := fieldValue(m, f.ID, fs.Value)
			if v == nil || v.Badge == "" {
				continue
			}
			b := slots.Badge{Text: v.Badge, Tone: tone(v.Tone)}
			if fs.Since != nil {
				b.Since = *fs.Since
			}
			out = append(out, b)
		}
	})
	return out
}

func tone(s string) slots.Tone {
	switch s {
	case "accent":
		return slots.ToneAccent
	case "ok":
		return slots.ToneOK
	case "warn":
		return slots.ToneWarn
	case "error":
		return slots.ToneError
	}
	return slots.ToneMuted
}

func (s *store) sections(issue *model.Issue, _ slots.Context) []slots.Section {
	var out []slots.Section
	s.each(issue, func(_ Config, m *Manifest, st BeadState) {
		for _, d := range m.Sections {
			if md := st.Sections[d.ID]; md != "" {
				out = append(out, slots.Section{Title: d.Title, Markdown: md})
			}
		}
	})
	return out
}

// actions lists the actions issue offers, in config then manifest order, with
// keys after config overrides.
func (s *store) actions(issue *model.Issue) []Action {
	var out []Action
	s.each(issue, func(cfg Config, m *Manifest, st BeadState) {
		for _, d := range m.Actions {
			if !contains(st.Actions, d.ID) {
				continue
			}
			key := d.Key
			if k, ok := cfg.Keys[d.ID]; ok {
				key = k
			}
			if key == "none" {
				key = ""
			}
			out = append(out, Action{Plugin: cfg.Name, ID: d.ID, Label: d.Label, Key: key})
		}
	})
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (s *store) activeNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, cfg := range s.cfgs {
		if s.active[cfg.Name] != nil {
			out = append(out, cfg.Name)
		}
	}
	return out
}

// fieldProvider answers "<plugin>.<field>" BQL fields.
type fieldProvider struct{ s *store }

func (p fieldProvider) Fields() []string {
	p.s.mu.RLock()
	defer p.s.mu.RUnlock()
	var out []string
	for _, cfg := range p.s.cfgs {
		if m := p.s.active[cfg.Name]; m != nil {
			for _, f := range m.Fields {
				out = append(out, cfg.Name+"."+f.ID)
			}
		}
	}
	return out
}

func (p fieldProvider) Value(issue *model.Issue, field string) (string, bool) {
	name, id, ok := strings.Cut(field, ".")
	if !ok {
		return "", false
	}
	var value string
	var found bool
	p.s.each(issue, func(cfg Config, _ *Manifest, st BeadState) {
		if cfg.Name != name {
			return
		}
		if fs, ok := st.Fields[id]; ok {
			value, found = fs.Value, true
		}
	})
	return value, found
}
