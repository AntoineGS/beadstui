package keys

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

var (
	readmeKeyTick = regexp.MustCompile("`([^`]+)`")
	wordSplit     = regexp.MustCompile(`[^a-z0-9]+`)
)

// readmeAliases maps README spellings to the key strings used in WithKeys.
var readmeAliases = map[string]string{"Enter": "enter"}

// TestReadmeKeyTables guards the README "Views" and "Key bindings" tables
// against drift from the keymaps (bt-aj3l.6). Rules:
//  1. Every backtick-quoted key in either table must appear in some
//     key.WithKeys(...) in any keymap (after readmeAliases).
//  2. In the Key bindings table, a row with exactly one key must share at
//     least one word (>2 chars, case-insensitive) between its description
//     and the WithHelp text of a Global or ListNormal binding for that key.
func TestReadmeKeyTables(t *testing.T) {
	data, err := os.ReadFile("../../../README.md")
	if err != nil {
		t.Skipf("README.md not found: %v", err)
	}
	all := map[string][]key.Binding{} // key string -> bindings, all maps
	ctx := map[string][]key.Binding{} // Global + ListNormal only
	for name, km := range allMaps() {
		for _, group := range km.FullHelp() {
			for _, b := range group {
				for _, k := range b.Keys() {
					all[k] = append(all[k], b)
					if name == "Global" || name == "ListNormal" {
						ctx[k] = append(ctx[k], b)
					}
				}
			}
		}
	}

	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimPrefix(line, "## ")
		}
		if section != "Views" && section != "Key bindings" {
			continue
		}
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if !strings.HasPrefix(line, "|") || len(cells) < 2 || strings.Contains(cells[0], "---") || strings.TrimSpace(cells[0]) == "Key" {
			continue
		}
		var keys []string
		for _, km := range readmeKeyTick.FindAllStringSubmatch(cells[0], -1) {
			k := km[1]
			if a, ok := readmeAliases[k]; ok {
				k = a
			}
			keys = append(keys, k)
		}
		for _, k := range keys {
			if len(all[k]) == 0 {
				t.Errorf("README %q table: key %q is not bound in any keymap (row: %s)", section, k, strings.TrimSpace(line))
			}
		}
		desc := cells[1]
		if section != "Key bindings" || len(keys) != 1 || len(all[keys[0]]) == 0 {
			continue
		}
		ok := false
		for _, b := range ctx[keys[0]] {
			if sharesWord(desc, b.Help().Desc) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("README key %q says %q but Global/ListNormal help says %v", keys[0], strings.TrimSpace(desc), helpDescs(ctx[keys[0]]))
		}
	}
}

func sharesWord(a, b string) bool {
	seen := map[string]bool{}
	for _, w := range wordSplit.Split(strings.ToLower(a), -1) {
		if len(w) > 2 {
			seen[w] = true
		}
	}
	for _, w := range wordSplit.Split(strings.ToLower(b), -1) {
		if seen[w] {
			return true
		}
	}
	return false
}

func helpDescs(bs []key.Binding) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Help().Desc)
	}
	return out
}
