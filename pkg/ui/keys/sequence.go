package keys

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
)

// LeaderPrefix starts a leader sequence inside WithKeys: "<leader>vb" is the
// leader key, then v, then b (bt-i4k). No terminal produces it, so
// key.Matches never fires on it; the sequence router in pkg/ui reads it.
const LeaderPrefix = "<leader>"

// gPrefix is the only non-leader sequence prefix ("gg").
const gPrefix = "g"

// LeaderGroups labels the leader tree's first-level groups for the which-key
// panel, keyed by the group's steps concatenated ("v", "f", ...).
var LeaderGroups = map[string]string{
	"v": "view",
	"f": "filter",
	"s": "sort",
	"b": "bead",
	"p": "project",
	"w": "window",
	"n": "notify",
	"/": "search",
}

// ParseSeq splits a sequence key string into its prefix and steps. After
// "<leader>" each rune is one step; named keys are written in angle brackets
// ("<tab>", "<ctrl+x>") and the literal < and > are "<lt>" and "<gt>". "gg"
// parses as prefix "g", steps ["g"]. Anything else is not a sequence.
func ParseSeq(k string) (prefix string, steps []string, ok bool) {
	switch {
	case strings.HasPrefix(k, LeaderPrefix):
		steps, ok = parseSteps(strings.TrimPrefix(k, LeaderPrefix))
		if !ok {
			return "", nil, false
		}
		return LeaderPrefix, steps, true
	case k == gPrefix+gPrefix:
		return gPrefix, []string{gPrefix}, true
	}
	return "", nil, false
}

func parseSteps(s string) ([]string, bool) {
	var steps []string
	for s != "" {
		if s[0] == '<' {
			end := strings.IndexByte(s, '>')
			if end < 2 {
				return nil, false
			}
			name := s[1:end]
			switch name {
			case "lt":
				name = "<"
			case "gt":
				name = ">"
			}
			steps = append(steps, name)
			s = s[end+1:]
			continue
		}
		r := []rune(s)[0]
		steps = append(steps, string(r))
		s = s[len(string(r)):]
	}
	return steps, true
}

// FormatSeq renders a sequence for display: "SPC v b" for a leader sequence
// with leaderLabel "SPC", "g g" for gg.
func FormatSeq(prefix string, steps []string, leaderLabel string) string {
	head := prefix
	if prefix == LeaderPrefix {
		head = leaderLabel
	}
	return strings.Join(append([]string{head}, steps...), " ")
}

// SeqEntry is one sequence binding: the steps after its prefix, the action
// it fires, and its help text.
type SeqEntry struct {
	Steps    []string
	ID, Desc string
}

// SeqNode is one node of a sequence tree. Groups carry a "+"-prefixed Label
// and Children; leaves carry ID and Desc.
type SeqNode struct {
	Key      string
	Label    string // group label, "" for leaves
	ID, Desc string // leaf only
	Children []*SeqNode
}

// IsGroup reports whether n has children to descend into.
func (n *SeqNode) IsGroup() bool { return n.ID == "" }

// Child returns the child reached by key k, or nil.
func (n *SeqNode) Child(k string) *SeqNode {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Key == k {
			return c
		}
	}
	return nil
}

// BuildSeqTree assembles entries into a tree rooted at prefix. A duplicate
// sequence or a sequence that is both a leaf and a group prefix is reported
// as an error and the first entry wins; the same action bound twice to one
// sequence is not a conflict. Children are sorted with groups first, then by
// key. groupLabels is keyed by the group's steps concatenated; an unlabeled
// group gets "+group".
func BuildSeqTree(prefix string, entries []SeqEntry, groupLabels map[string]string) (*SeqNode, []error) {
	root := &SeqNode{Key: prefix}
	var errs []error
	for _, e := range entries {
		if len(e.Steps) == 0 {
			errs = append(errs, fmt.Errorf("%s: %s has no keys after the prefix", e.ID, prefix))
			continue
		}
		if err := insertSeq(root, prefix, e, groupLabels); err != nil {
			errs = append(errs, err)
		}
	}
	sortSeqNode(root)
	return root, errs
}

func insertSeq(root *SeqNode, prefix string, e SeqEntry, groupLabels map[string]string) error {
	seq := FormatSeq(prefix, e.Steps, prefix)
	n := root
	for i, step := range e.Steps {
		last := i == len(e.Steps)-1
		c := n.Child(step)
		switch {
		case c == nil && last:
			n.Children = append(n.Children, &SeqNode{Key: step, ID: e.ID, Desc: e.Desc})
			return nil
		case c == nil:
			label := "+group"
			if l, ok := groupLabels[strings.Join(e.Steps[:i+1], "")]; ok {
				label = "+" + l
			}
			c = &SeqNode{Key: step, Label: label}
			n.Children = append(n.Children, c)
		case last && c.IsGroup():
			return fmt.Errorf("%s: %s is a group prefix", e.ID, seq)
		case last && c.ID == e.ID:
			return nil
		case last:
			return fmt.Errorf("%s: %s is already bound to %s", e.ID, seq, c.ID)
		case !c.IsGroup():
			return fmt.Errorf("%s: %s passes through %s, bound to %s",
				e.ID, seq, FormatSeq(prefix, e.Steps[:i+1], prefix), c.ID)
		}
		n = c
	}
	return nil
}

func sortSeqNode(n *SeqNode) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsGroup() != b.IsGroup() {
			return a.IsGroup()
		}
		return a.Key < b.Key
	})
	for _, c := range n.Children {
		sortSeqNode(c)
	}
}

// SeqEntries collects the sequences under prefix from the enabled, tagged
// bindings of each map (a keys map struct value or pointer). Desc is the
// binding's help description.
func SeqEntries(prefix string, maps ...any) []SeqEntry {
	var out []SeqEntry
	for _, m := range maps {
		mv := reflect.Indirect(reflect.ValueOf(m))
		if mv.Kind() != reflect.Struct {
			continue
		}
		for i := 0; i < mv.NumField(); i++ {
			f := mv.Type().Field(i)
			if f.Type != bindingType {
				continue
			}
			id := f.Tag.Get("action")
			if id == "" {
				continue
			}
			b := mv.Field(i).Interface().(key.Binding)
			if !b.Enabled() {
				continue
			}
			for _, k := range b.Keys() {
				if p, steps, ok := ParseSeq(k); ok && p == prefix {
					out = append(out, SeqEntry{Steps: steps, ID: id, Desc: b.Help().Desc})
				}
			}
		}
	}
	return out
}
