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
	if shadows != nil {
		t.Errorf("Task 6 Apply shadows = %#v, want nil", shadows)
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
