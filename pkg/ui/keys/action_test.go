package keys

import (
	"reflect"
	"regexp"
	"testing"

	"charm.land/bubbles/v2/key"
)

var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)+$`)

func TestIsTokenRequiresAValidActionID(t *testing.T) {
	for _, raw := range []string{"@", "@.", "@nav.", "@1nav.down", "@nav.down!", "a@b"} {
		if IsToken(raw) {
			t.Errorf("IsToken(%q) = true, want false", raw)
		}
	}
	if !IsToken("@unregistered.action") {
		t.Fatal("valid-shaped unknown action token was not recognized")
	}
}

func TestEveryBindingTagged(t *testing.T) {
	at := reflect.TypeOf(AppKeys{})
	for i := 0; i < at.NumField(); i++ {
		mt := at.Field(i).Type
		if mt.Kind() != reflect.Struct {
			continue
		}
		for j := 0; j < mt.NumField(); j++ {
			f := mt.Field(j)
			if f.Type != bindingType {
				continue
			}
			id := f.Tag.Get("action")
			if !actionIDPattern.MatchString(id) {
				t.Errorf("%s.%s: action tag %q missing or malformed", at.Field(i).Name, f.Name, id)
			}
		}
	}
}

func TestActionIDsUniquePerMap(t *testing.T) {
	a := NewAppKeys()
	seen := map[string]map[string]string{}
	a.Walk(func(r Ref, _ *key.Binding) {
		if seen[r.Map] == nil {
			seen[r.Map] = map[string]string{}
		}
		if prev, ok := seen[r.Map][r.ID]; ok {
			t.Errorf("%s: ID %q used by both %s and %s", r.Map, r.ID, prev, r.Field)
		}
		seen[r.Map][r.ID] = r.Field
	})
}

func TestEveryBindingHasOneToken(t *testing.T) {
	a := NewAppKeys()
	n := 0
	a.Walk(func(r Ref, b *key.Binding) {
		n++
		var tokens []string
		for _, k := range b.Keys() {
			if IsToken(k) {
				tokens = append(tokens, k)
			}
		}
		if len(tokens) != 1 || tokens[0] != Token(r.ID) {
			t.Errorf("%s.%s: tokens = %v, want [%s]", r.Map, r.Field, tokens, Token(r.ID))
		}
		if !b.Enabled() && len(b.Keys()) == 0 {
			t.Errorf("%s.%s: unexpectedly empty", r.Map, r.Field)
		}
	})
	if n == 0 {
		t.Fatal("Walk visited no bindings")
	}
}

func TestPhysicalKeysUnchanged(t *testing.T) {
	fresh := map[string]any{"Settings": NewSettingsKeys()}
	for name, m := range allMaps() {
		fresh[name] = m
	}
	a := NewAppKeys()
	a.Walk(func(r Ref, b *key.Binding) {
		m, ok := fresh[r.Map]
		if !ok {
			t.Fatalf("%s: no fresh constructor in test registry", r.Map)
		}
		orig := reflect.ValueOf(m).FieldByName(r.Field).Interface().(key.Binding)
		var got []string
		for _, k := range b.Keys() {
			if !IsToken(k) {
				got = append(got, k)
			}
		}
		if !reflect.DeepEqual(got, orig.Keys()) {
			t.Errorf("%s.%s: physical keys %v, constructor declares %v", r.Map, r.Field, got, orig.Keys())
		}
		if b.Enabled() != orig.Enabled() {
			t.Errorf("%s.%s: Enabled changed %v -> %v", r.Map, r.Field, orig.Enabled(), b.Enabled())
		}
	})
}

func TestLookup(t *testing.T) {
	a := NewAppKeys()
	var want int
	a.Walk(func(r Ref, _ *key.Binding) {
		if r.ID == "nav.down" {
			want++
		}
	})
	if got := len(a.Lookup("nav.down")); got != want || want < 2 {
		t.Errorf("Lookup(nav.down) = %d, want %d (>=2)", got, want)
	}
	if got := len(a.Lookup("filter.open")); got != 1 {
		t.Errorf("Lookup(filter.open) = %d, want 1", got)
	}
	if got := len(a.Lookup("no.such")); got != 0 {
		t.Errorf("Lookup(no.such) = %d, want 0", got)
	}
}
