package keys

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestParseSeq(t *testing.T) {
	tests := []struct {
		in     string
		prefix string
		steps  []string
		ok     bool
	}{
		{"<leader>vb", LeaderPrefix, []string{"v", "b"}, true},
		{"<leader>w<lt>", LeaderPrefix, []string{"w", "<"}, true},
		{"<leader>w<gt>", LeaderPrefix, []string{"w", ">"}, true},
		{"<leader><tab>", LeaderPrefix, []string{"tab"}, true},
		{"<leader>x<ctrl+x>", LeaderPrefix, []string{"x", "ctrl+x"}, true},
		{"<leader>", LeaderPrefix, nil, true},
		{"gg", "g", []string{"g"}, true},
		{"j", "", nil, false},
		{"g", "", nil, false},
		{"ctrl+g", "", nil, false},
		{"@nav.top", "", nil, false},
		{"<leader>w<tab", "", nil, false},
	}
	for _, tt := range tests {
		prefix, steps, ok := ParseSeq(tt.in)
		if prefix != tt.prefix || !reflect.DeepEqual(steps, tt.steps) || ok != tt.ok {
			t.Errorf("ParseSeq(%q) = (%q, %#v, %v), want (%q, %#v, %v)",
				tt.in, prefix, steps, ok, tt.prefix, tt.steps, tt.ok)
		}
	}
}

func TestFormatSeq(t *testing.T) {
	tests := []struct {
		prefix string
		steps  []string
		label  string
		want   string
	}{
		{LeaderPrefix, []string{"w", "<"}, "SPC", "SPC w <"},
		{LeaderPrefix, []string{"v", "b"}, "SPC", "SPC v b"},
		{LeaderPrefix, nil, "SPC", "SPC"},
		{LeaderPrefix, []string{"v"}, ",", ", v"},
		{"g", []string{"g"}, "SPC", "g g"},
	}
	for _, tt := range tests {
		if got := FormatSeq(tt.prefix, tt.steps, tt.label); got != tt.want {
			t.Errorf("FormatSeq(%q, %v, %q) = %q, want %q", tt.prefix, tt.steps, tt.label, got, tt.want)
		}
	}
}

func TestBuildSeqTree_GroupsFirstAndLabels(t *testing.T) {
	root, errs := BuildSeqTree(LeaderPrefix, []SeqEntry{
		{Steps: []string{"a"}, ID: "x.a", Desc: "leaf a"},
		{Steps: []string{"v", "b"}, ID: "view.board", Desc: "board"},
		{Steps: []string{"z", "q"}, ID: "x.zq", Desc: "zq"},
		{Steps: []string{"v", "a"}, ID: "view.actionable", Desc: "actionable"},
	}, map[string]string{"v": "view"})
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if root.Key != LeaderPrefix {
		t.Errorf("root.Key = %q, want %q", root.Key, LeaderPrefix)
	}
	var got []string
	for _, c := range root.Children {
		got = append(got, c.Key)
	}
	if want := []string{"v", "z", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("root children = %v, want %v (groups first, then by key)", got, want)
	}
	v, z, a := root.Children[0], root.Children[1], root.Children[2]
	if v.Label != "+view" {
		t.Errorf("v label = %q, want +view", v.Label)
	}
	if z.Label != "+group" {
		t.Errorf("unlabeled group label = %q, want +group", z.Label)
	}
	if a.Label != "" || a.ID != "x.a" || a.Desc != "leaf a" {
		t.Errorf("leaf a = %+v", a)
	}
	if len(v.Children) != 2 || v.Children[0].Key != "a" || v.Children[1].ID != "view.board" {
		t.Errorf("v children = %+v", v.Children)
	}
}

func TestBuildSeqTree_Errors(t *testing.T) {
	tests := []struct {
		name    string
		entries []SeqEntry
		wantIDs map[string]string // path -> ID expected to survive
	}{
		{
			name: "duplicate sequence",
			entries: []SeqEntry{
				{Steps: []string{"v", "b"}, ID: "first"},
				{Steps: []string{"v", "b"}, ID: "second"},
			},
			wantIDs: map[string]string{"vb": "first"},
		},
		{
			name: "leaf then group",
			entries: []SeqEntry{
				{Steps: []string{"v"}, ID: "leaf"},
				{Steps: []string{"v", "b"}, ID: "deeper"},
			},
			wantIDs: map[string]string{"v": "leaf"},
		},
		{
			name: "group then leaf",
			entries: []SeqEntry{
				{Steps: []string{"v", "b"}, ID: "deeper"},
				{Steps: []string{"v"}, ID: "leaf"},
			},
			wantIDs: map[string]string{"vb": "deeper"},
		},
		{
			name:    "empty sequence",
			entries: []SeqEntry{{Steps: nil, ID: "bare"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, errs := BuildSeqTree(LeaderPrefix, tt.entries, nil)
			if len(errs) != 1 {
				t.Fatalf("errs = %v, want exactly one", errs)
			}
			for path, id := range tt.wantIDs {
				n := root
				for _, r := range path {
					n = n.Child(string(r))
					if n == nil {
						t.Fatalf("path %q missing", path)
					}
				}
				if n.ID != id {
					t.Errorf("path %q ID = %q, want %q", path, n.ID, id)
				}
			}
		})
	}
}

func TestBuildSeqTree_SameActionTwiceIsNotAConflict(t *testing.T) {
	root, errs := BuildSeqTree("g", []SeqEntry{
		{Steps: []string{"g"}, ID: "nav.top"},
		{Steps: []string{"g"}, ID: "nav.top"},
	}, nil)
	if len(errs) != 0 || len(root.Children) != 1 {
		t.Errorf("errs = %v, children = %d; want no error and one leaf", errs, len(root.Children))
	}
}

func TestSeqEntries(t *testing.T) {
	type testMap struct {
		Board    key.Binding `action:"view.board"`
		Disabled key.Binding `action:"x.disabled"`
		Top      key.Binding `action:"nav.top"`
		Untagged key.Binding
	}
	disabled := key.NewBinding(key.WithKeys("<leader>xd"), key.WithHelp("", "disabled"))
	disabled.SetEnabled(false)
	m := testMap{
		Board:    key.NewBinding(key.WithKeys("b", "<leader>vb", "@view.board"), key.WithHelp("b", "board")),
		Disabled: disabled,
		Top:      key.NewBinding(key.WithKeys("home", "gg"), key.WithHelp("home", "top")),
		Untagged: key.NewBinding(key.WithKeys("<leader>u")),
	}
	got := SeqEntries(LeaderPrefix, m)
	want := []SeqEntry{{Steps: []string{"v", "b"}, ID: "view.board", Desc: "board"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeqEntries(leader) = %+v, want %+v", got, want)
	}
	got = SeqEntries("g", m, &m)
	want = []SeqEntry{
		{Steps: []string{"g"}, ID: "nav.top", Desc: "top"},
		{Steps: []string{"g"}, ID: "nav.top", Desc: "top"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeqEntries(g) = %+v, want %+v", got, want)
	}
}

func TestDefaultKeymapHasNoLeaderYet(t *testing.T) {
	var maps []any
	for _, m := range allMaps() {
		maps = append(maps, m)
	}
	if got := SeqEntries(LeaderPrefix, maps...); len(got) != 0 {
		t.Errorf("default keymap leader entries = %+v, want none until bt-i4k.3 layout", got)
	}
}
