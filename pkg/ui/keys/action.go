package keys

import (
	"reflect"
	"strings"

	"charm.land/bubbles/v2/key"
)

// TokenPrefix marks an action token: a key string no terminal produces, used
// to dispatch an action by ID through key.Matches (bt-i4k).
const TokenPrefix = "@"

// Token returns the action token for id.
func Token(id string) string { return TokenPrefix + id }

// IsToken reports whether k is a syntactically valid action token. Requiring
// an action-ID shape keeps a literal @ key available to text inputs.
func IsToken(k string) bool {
	if !strings.HasPrefix(k, TokenPrefix) {
		return false
	}
	id := strings.TrimPrefix(k, TokenPrefix)
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || part[0] < 'a' || part[0] > 'z' {
			return false
		}
		for i := 1; i < len(part); i++ {
			c := part[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
				return false
			}
		}
	}
	return true
}

// Ref names one tagged binding: the AppKeys field holding its map, the
// binding's field, and its action ID.
type Ref struct{ Map, Field, ID string }

var bindingType = reflect.TypeOf(key.Binding{})

// Walk calls fn for every tagged key.Binding in every map of a, in field order.
func (a *AppKeys) Walk(fn func(r Ref, b *key.Binding)) {
	av := reflect.ValueOf(a).Elem()
	for i := 0; i < av.NumField(); i++ {
		mv := av.Field(i)
		if mv.Kind() != reflect.Struct {
			continue
		}
		mapName := av.Type().Field(i).Name
		for j := 0; j < mv.NumField(); j++ {
			f := mv.Type().Field(j)
			if f.Type != bindingType {
				continue
			}
			id := f.Tag.Get("action")
			if id == "" {
				continue
			}
			fn(Ref{Map: mapName, Field: f.Name, ID: id}, mv.Field(j).Addr().Interface().(*key.Binding))
		}
	}
}

// Lookup returns every binding tagged id.
func (a *AppKeys) Lookup(id string) []*key.Binding {
	var out []*key.Binding
	a.Walk(func(r Ref, b *key.Binding) {
		if r.ID == id {
			out = append(out, b)
		}
	})
	return out
}

// stampTokens appends each binding's action token to its keys.
func stampTokens(a *AppKeys) {
	a.Walk(func(r Ref, b *key.Binding) {
		b.SetKeys(append(b.Keys(), Token(r.ID))...)
	})
}
