package keys

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func writeKeysConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigReadsKeysBindValues(t *testing.T) {
	path := writeKeysConfig(t, `
plugins:
  - name: example
    command: [example]
    keys: {dispatch: D}
keys:
  leader: ","
  which_key_delay: 0
  bind:
    filter.open: o
    view.board: [b, "<leader>B"]
    app.refresh: none
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Leader != "," || cfg.WhichKeyDelay != "0" {
		t.Errorf("settings = leader %q, delay %q", cfg.Leader, cfg.WhichKeyDelay)
	}
	want := map[string]BindValue{
		"filter.open": {"o"},
		"view.board":  {"b", "<leader>B"},
		"app.refresh": {},
	}
	if !reflect.DeepEqual(cfg.Bind, want) {
		t.Errorf("bind = %#v, want %#v", cfg.Bind, want)
	}
}

func TestLoadConfigMissingFileOrKeysSection(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	cfg, err := LoadConfig(missing)
	if err != nil || cfg.Leader != "" || cfg.WhichKeyDelay != "" || len(cfg.Bind) != 0 {
		t.Fatalf("missing file: cfg=%+v err=%v", cfg, err)
	}

	cfg, err = LoadConfig(writeKeysConfig(t, "plugins: []\n"))
	if err != nil || cfg.Leader != "" || cfg.WhichKeyDelay != "" || len(cfg.Bind) != 0 {
		t.Fatalf("missing keys section: cfg=%+v err=%v", cfg, err)
	}
}

func TestLoadConfigSkipsMalformedBindValuesAndKeepsValidEntries(t *testing.T) {
	cfg, err := LoadConfig(writeKeysConfig(t, `
keys:
  bind:
    filter.open: o
    view.board: [b, 2]
    app.refresh: null
`))
	if err == nil {
		t.Fatal("LoadConfig accepted malformed bind values")
	}
	for _, id := range []string{"view.board", "app.refresh"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("LoadConfig error %q does not identify %s", err, id)
		}
	}
	if got := cfg.Bind["filter.open"]; !reflect.DeepEqual(got, BindValue{"o"}) {
		t.Errorf("valid binding = %v, want [o]", got)
	}
	for _, id := range []string{"view.board", "app.refresh"} {
		if _, ok := cfg.Bind[id]; ok {
			t.Errorf("malformed binding %s was retained: %#v", id, cfg.Bind[id])
		}
	}
}

func TestBindValueRejectsNonStringShapes(t *testing.T) {
	for _, value := range []string{"{key: o}", "[o, 2]", "2", "null"} {
		t.Run(value, func(t *testing.T) {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte("value: "+value+"\n"), &doc); err != nil {
				t.Fatal(err)
			}
			node := doc.Content[0].Content[1]
			var bind BindValue
			err := bind.UnmarshalYAML(node)
			if err == nil {
				t.Fatalf("yaml value %s unexpectedly parsed as %#v", value, bind)
			}
		})
	}
}

func TestApplyPromotesAndMovesBindingsAndKeepsTokens(t *testing.T) {
	a := NewAppKeys()
	settings, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{
		"filter.open": {"o"},
		"view.board":  {"<leader>B"},
		"app.refresh": {},
	}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors: %v", errs)
	}
	wantShadows := []Shadow{
		{ID: "tree.expand_all", Key: "o", By: "filter.open"},
		{ID: "history.open_in_browser", Key: "o", By: "filter.open"},
	}
	if len(shadows) != len(wantShadows) {
		t.Fatalf("Apply shadows = %#v, want %#v", shadows, wantShadows)
	}
	for _, want := range wantShadows {
		if !hasShadow(shadows, want) {
			t.Errorf("Apply shadows = %#v, missing %#v", shadows, want)
		}
	}
	if settings.Leader != "space" || settings.WhichKeyDelay != 250*time.Millisecond || settings.WhichKeyOff {
		t.Errorf("default settings = %+v", settings)
	}

	assertBindingKeys := func(id string, want []string) {
		t.Helper()
		bindings := a.Lookup(id)
		if len(bindings) == 0 {
			t.Fatalf("no binding found for %s", id)
		}
		for _, binding := range bindings {
			if got := binding.Keys(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s keys = %v, want %v", id, got, want)
			}
			if !binding.Enabled() {
				t.Errorf("%s binding should remain enabled", id)
			}
		}
	}
	assertBindingKeys("filter.open", []string{"o", Token("filter.open")})
	assertBindingKeys("view.board", []string{"<leader>B", Token("view.board")})
	assertBindingKeys("app.refresh", []string{Token("app.refresh")})
	if got := a.Global.FilterOpen.Help().Key; got != "o" {
		t.Errorf("filter.open help key = %q, want o", got)
	}
	if got := a.Global.Board.Help().Key; got != "SPC B" {
		t.Errorf("view.board help key = %q, want SPC B", got)
	}
}

func TestApplyValidatesEntriesIndependently(t *testing.T) {
	a := NewAppKeys()
	settings, _, errs := a.Apply(Config{
		Leader:        "not a key",
		WhichKeyDelay: "abc",
		Bind: map[string]BindValue{
			"filter.open": {"o"},
			"view.board":  {"b", "not a key"},
			"unknown.id":  {"x"},
		},
	})
	if len(errs) != 4 {
		t.Fatalf("got %d errors %v, want leader, delay, malformed key and unknown ID", len(errs), errs)
	}
	joined := ""
	for _, err := range errs {
		joined += err.Error() + "\n"
	}
	for _, want := range []string{"leader", "which_key_delay", "view.board", "unknown.id"} {
		if !strings.Contains(joined, want) {
			t.Errorf("errors %q do not mention %q", joined, want)
		}
	}
	if settings.Leader != "space" || settings.WhichKeyDelay != 250*time.Millisecond || settings.WhichKeyOff {
		t.Errorf("invalid settings should fall back to defaults, got %+v", settings)
	}
	if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{"o", Token("filter.open")}) {
		t.Errorf("valid entry was not applied: %v", got)
	}
	if got := a.Global.Board.Keys(); !reflect.DeepEqual(got, []string{"<leader>vb", Token("view.board")}) {
		t.Errorf("malformed entry was partially applied: %v", got)
	}
}

func TestApplyWhichKeyDelayAndLeader(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
		off   bool
		err   bool
	}{
		{name: "zero", value: "0", want: 0},
		{name: "duration", value: "400ms", want: 400 * time.Millisecond},
		{name: "off", value: "off", want: 250 * time.Millisecond, off: true},
		{name: "invalid", value: "abc", want: 250 * time.Millisecond, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAppKeys()
			settings, _, errs := a.Apply(Config{Leader: ",", WhichKeyDelay: tt.value})
			if settings.Leader != "," {
				t.Errorf("leader = %q, want comma", settings.Leader)
			}
			if settings.WhichKeyDelay != tt.want || settings.WhichKeyOff != tt.off {
				t.Errorf("delay settings = %+v, want delay %s off=%v", settings, tt.want, tt.off)
			}
			if (len(errs) != 0) != tt.err {
				t.Errorf("errors = %v, want error=%v", errs, tt.err)
			}
		})
	}
}

func TestApplyRejectsTokensAndUnparsedSequences(t *testing.T) {
	for _, raw := range []string{"@fake", "<leader>", "<leader><not-a-key>", "ctrl+notakey", "not a key"} {
		t.Run(raw, func(t *testing.T) {
			a := NewAppKeys()
			_, _, errs := a.Apply(Config{Bind: map[string]BindValue{"filter.open": {raw}}})
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want one invalid key error", errs)
			}
			if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{"<leader>fo", Token("filter.open")}) {
				t.Errorf("invalid key changed binding: %v", got)
			}
		})
	}
}

func TestHelpLabel(t *testing.T) {
	tests := []struct {
		name        string
		keys        []string
		leaderLabel string
		want        string
	}{
		{name: "arrow aliases", keys: []string{"up", "k"}, want: "↑/k"},
		{name: "leader sequence", keys: []string{"<leader>fo"}, leaderLabel: "SPC", want: "SPC f o"},
		{name: "g sequence", keys: []string{"gg"}, want: "g g"},
		{name: "control", keys: []string{"ctrl+d"}, want: "⌃d"},
		{name: "mixed", keys: []string{"b", "<leader>vb", Token("view.board")}, leaderLabel: "SPC", want: "b / SPC v b"},
		{name: "numeric range", keys: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, want: "1-9"},
		{name: "tokens skipped", keys: []string{Token("filter.open")}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HelpLabel(tt.keys, tt.leaderLabel); got != tt.want {
				t.Errorf("HelpLabel(%v, %q) = %q, want %q", tt.keys, tt.leaderLabel, got, tt.want)
			}
		})
	}
}

func TestLeaderLabel(t *testing.T) {
	if got := LeaderLabel("space"); got != "SPC" {
		t.Errorf("LeaderLabel(space) = %q, want SPC", got)
	}
	if got := LeaderLabel(","); got != "," {
		t.Errorf("LeaderLabel(comma) = %q, want comma", got)
	}
}

func TestApplyAcceptsModifiedPrintableKeys(t *testing.T) {
	for _, raw := range []string{"ctrl+d", "ctrl+s", "alt+x", "<leader><ctrl+x>"} {
		t.Run(raw, func(t *testing.T) {
			a := NewAppKeys()
			_, _, errs := a.Apply(Config{Bind: map[string]BindValue{"filter.open": {raw}}})
			if len(errs) != 0 {
				t.Fatalf("Apply(%q) errors = %v", raw, errs)
			}
			if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{raw, Token("filter.open")}) {
				t.Fatalf("keys = %v, want configured key and token", got)
			}
		})
	}
}

func TestLoadConfigRetainsValidSiblings(t *testing.T) {
	cfg, err := LoadConfig(writeKeysConfig(t, `
keys:
  leader: [space]
  which_key_delay: [250ms]
  bind:
    bad: [x, 2]
    good: o
`))
	if err == nil {
		t.Fatal("LoadConfig accepted malformed settings and binding")
	}
	for _, want := range []string{"leader", "which_key_delay", "keys.bind.bad"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadConfig error %q does not mention %q", err, want)
		}
	}
	if cfg.Leader != "" || cfg.WhichKeyDelay != "" {
		t.Errorf("malformed settings were retained: %+v", cfg)
	}
	if got := cfg.Bind["good"]; !reflect.DeepEqual(got, BindValue{"o"}) {
		t.Errorf("valid sibling bind = %v, want [o]", got)
	}
	if _, ok := cfg.Bind["bad"]; ok {
		t.Errorf("malformed bind was retained: %#v", cfg.Bind["bad"])
	}
}

func TestLoadConfigRejectsMalformedBindContainer(t *testing.T) {
	cfg, err := LoadConfig(writeKeysConfig(t, `
keys:
  leader: ","
  which_key_delay: 300ms
  bind: [o]
`))
	if err == nil || !strings.Contains(err.Error(), "keys.bind") {
		t.Fatalf("LoadConfig error = %v, want malformed bind container", err)
	}
	if cfg.Leader != "," || cfg.WhichKeyDelay != "300ms" {
		t.Errorf("valid settings were discarded with malformed bind: %+v", cfg)
	}
}

func TestApplyRejectsSequenceOverridesForModalTextActions(t *testing.T) {
	a := NewAppKeys()
	defaults := make([][]string, 0, len(a.Lookup("nav.up")))
	for _, binding := range a.Lookup("nav.up") {
		defaults = append(defaults, append([]string(nil), binding.Keys()...))
	}
	listSearchDefault := append([]string(nil), a.ListSearch.Apply.Keys()...)
	_, _, errs := a.Apply(Config{Bind: map[string]BindValue{
		"nav.up":            {"u", "<leader>x"},
		"list_search.apply": {"gg"},
		"filter.open":       {"o"},
	}})
	if len(errs) != 2 {
		t.Fatalf("Apply errors = %v, want direct-only nav.up and list_search.apply errors", errs)
	}
	joinedErrors := errs[0].Error() + " " + errs[1].Error()
	if !strings.Contains(joinedErrors, "nav.up") || !strings.Contains(joinedErrors, "list_search.apply") {
		t.Fatalf("Apply errors = %v, want direct-only nav.up and list_search.apply errors", errs)
	}
	for i, binding := range a.Lookup("nav.up") {
		if got := binding.Keys(); !reflect.DeepEqual(got, defaults[i]) {
			t.Errorf("rejected nav.up override changed binding to %v, want original %v", got, defaults[i])
		}
	}
	if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{"o", Token("filter.open")}) {
		t.Errorf("valid sibling override was not applied: %v", got)
	}
	if got := a.ListSearch.Apply.Keys(); !reflect.DeepEqual(got, listSearchDefault) {
		t.Errorf("rejected gg override changed ListSearch.Apply to %v, want %v", got, listSearchDefault)
	}
}

func TestApplyAllowsDirectOverrideForModalTextAction(t *testing.T) {
	a := NewAppKeys()
	_, _, errs := a.Apply(Config{Bind: map[string]BindValue{"nav.up": {"u"}}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors = %v, want none", errs)
	}
	for _, binding := range a.Lookup("nav.up") {
		if got := binding.Keys(); !reflect.DeepEqual(got, []string{"u", Token("nav.up")}) {
			t.Errorf("direct nav.up override = %v, want [u @nav.up]", got)
		}
	}
}

func TestApplyRejectsSequencesInTextSearchMaps(t *testing.T) {
	for _, id := range []string{
		"list_search.apply",
		"board_search.finish",
		"history_search.confirm",
		"memories_search.confirm",
	} {
		t.Run(id, func(t *testing.T) {
			a := NewAppKeys()
			defaults := append([]string(nil), a.Lookup(id)[0].Keys()...)
			_, _, errs := a.Apply(Config{Bind: map[string]BindValue{id: {"<leader>x"}}})
			if len(errs) != 1 || !strings.Contains(errs[0].Error(), id) {
				t.Fatalf("Apply errors = %v, want direct-only error for %s", errs, id)
			}
			if got := a.Lookup(id)[0].Keys(); !reflect.DeepEqual(got, defaults) {
				t.Errorf("rejected sequence changed %s keys: got %v, want %v", id, got, defaults)
			}
		})
	}
}

func TestApplyShadowsConflictsWithinOneMapAndPreservesOtherViews(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{"list.detail_page_up": {"k"}}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors = %v, want none", errs)
	}
	if !hasShadow(shadows, Shadow{ID: "nav.up", Key: "k", By: "list.detail_page_up"}) {
		t.Errorf("shadows = %#v, want nav.up k shadowed by list.detail_page_up", shadows)
	}
	if containsKey(a.ListNormal.Up.Keys(), "k") {
		t.Errorf("same-map nav.up retained conflicting key: %v", a.ListNormal.Up.Keys())
	}
	if !containsKey(a.Tree.Up.Keys(), "k") {
		t.Errorf("unrelated Tree map lost nav.up's key: %v", a.Tree.Up.Keys())
	}
	if got := a.ListNormal.Up.Keys(); !containsKey(got, Token("nav.up")) {
		t.Errorf("shadowed binding lost its action token: %v", got)
	}
}

func TestApplyShadowsGlobalKeysAcrossViewAndSearchSiblingMaps(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{"app.search": {"enter"}}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors = %v, want none", errs)
	}
	for _, want := range []Shadow{
		{ID: "list.open", Key: "enter", By: "app.search"},
		{ID: "list_search.apply", Key: "enter", By: "app.search"},
	} {
		if !hasShadow(shadows, want) {
			t.Errorf("shadows = %#v, missing %#v", shadows, want)
		}
	}
	if containsKey(a.ListNormal.Enter.Keys(), "enter") {
		t.Errorf("view binding retained Global conflict: %v", a.ListNormal.Enter.Keys())
	}
	if containsKey(a.ListSearch.Apply.Keys(), "enter") {
		t.Errorf("search sibling retained Global conflict: %v", a.ListSearch.Apply.Keys())
	}
	if !containsKey(a.ListNormal.Enter.Keys(), Token("list.open")) || !containsKey(a.ListSearch.Apply.Keys(), Token("list_search.apply")) {
		t.Errorf("conflict resolution removed action tokens: normal=%v search=%v", a.ListNormal.Enter.Keys(), a.ListSearch.Apply.Keys())
	}
}

func TestApplyConflictingConfiguredKeysChooseSortedIDWinner(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{
		"list.detail_page_up":   {"x"},
		"list.detail_page_down": {"x"},
	}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "list.detail_page_up") {
		t.Fatalf("Apply errors = %v, want one error for sorted loser list.detail_page_up", errs)
	}
	want := Shadow{ID: "list.detail_page_up", Key: "x", By: "list.detail_page_down"}
	if !hasShadow(shadows, want) {
		t.Errorf("shadows = %#v, want %#v", shadows, want)
	}
	if got := a.ListNormal.DetailPageDown.Keys(); !reflect.DeepEqual(got, []string{"x", Token("list.detail_page_down")}) {
		t.Errorf("sorted winner keys = %v", got)
	}
	if got := a.ListNormal.DetailPageUp.Keys(); !reflect.DeepEqual(got, []string{Token("list.detail_page_up")}) {
		t.Errorf("sorted loser keys = %v, want token only", got)
	}
}

func TestApplySequencePrefixShadowsDirectPrefixBinding(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{
		"filter.open": {"gg"},
		"view.graph":  {"g"},
	}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "view.graph") {
		t.Fatalf("Apply errors = %v, want one losing direct-prefix error", errs)
	}
	want := Shadow{ID: "view.graph", Key: "g", By: "filter.open"}
	if !hasShadow(shadows, want) {
		t.Errorf("shadows = %#v, want %#v", shadows, want)
	}
	if got := a.Global.Graph.Keys(); !reflect.DeepEqual(got, []string{Token("view.graph")}) {
		t.Errorf("direct-prefix loser keys = %v, want token only", got)
	}
	if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{"gg", Token("filter.open")}) {
		t.Errorf("sequence winner keys = %v", got)
	}
}

func TestApplyDoesNotConflictAcrossMutuallyExclusiveSearchAndNormalMaps(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{"list.open": {"enter"}}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors = %v, want none", errs)
	}
	if shadows != nil {
		t.Errorf("exclusive search/normal maps produced shadows: %#v", shadows)
	}
	if !containsKey(a.ListSearch.Apply.Keys(), "enter") {
		t.Errorf("ListSearch sibling lost enter: %v", a.ListSearch.Apply.Keys())
	}
}

func TestApplySequencePrefixShadowsDefaultGroupEntries(t *testing.T) {
	a := NewAppKeys()
	_, shadows, errs := a.Apply(Config{Bind: map[string]BindValue{"filter.open": {"<leader>f"}}})
	if len(errs) != 0 {
		t.Fatalf("Apply errors = %v, want none", errs)
	}
	for _, entry := range []struct {
		id      string
		key     string
		binding []string
	}{
		{"filter.closed", "<leader>fc", a.Global.FilterClosed.Keys()},
		{"filter.ready", "<leader>fr", a.Global.FilterReady.Keys()},
		{"filter.cycle_status", "<leader>fs", a.Global.CycleStatusFilter.Keys()},
		{"filter.labels", "<leader>fl", a.Global.LabelPicker.Keys()},
		{"filter.recipes", "<leader>fR", a.Global.Recipes.Keys()},
		{"filter.clear", "<leader>fx", a.Global.FilterClear.Keys()},
	} {
		if !reflect.DeepEqual(entry.binding, []string{Token(entry.id)}) {
			t.Errorf("prefix-shadowed %s keys = %v, want token only", entry.id, entry.binding)
		}
		if !hasShadow(shadows, Shadow{ID: entry.id, Key: entry.key, By: "filter.open"}) {
			t.Errorf("shadows = %#v, missing prefix shadow for %s", shadows, entry.id)
		}
	}
	if got := a.Global.FilterOpen.Keys(); !reflect.DeepEqual(got, []string{"<leader>f", Token("filter.open")}) {
		t.Errorf("configured prefix binding = %v", got)
	}
}

func containsKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func hasShadow(shadows []Shadow, want Shadow) bool {
	for _, shadow := range shadows {
		if shadow == want {
			return true
		}
	}
	return false
}
