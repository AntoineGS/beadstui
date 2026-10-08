package plugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func validManifest() Manifest {
	return Manifest{
		ProtocolVersion: ProtocolVersion,
		Name:            "example",
		Version:         "0.1.0",
		Fields: []Field{{ID: "state", Label: "Agent", Values: []FieldValue{
			{ID: "waiting", Label: "Waiting", Badge: "WAIT", Tone: "warn"},
			{ID: "queued", Label: "Queued"},
		}}},
		Sections: []SectionDecl{{ID: "agent", Title: "Agent"}},
		Actions:  []ActionDecl{{ID: "dispatch", Label: "Dispatch", Key: "D"}},
	}
}

func TestManifestValidate(t *testing.T) {
	if err := validManifest().Validate("example"); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	cases := map[string]func(m *Manifest){
		"protocol version": func(m *Manifest) { m.ProtocolVersion = 2 },
		"name mismatch":    func(m *Manifest) { m.Name = "other" },
		"bad field id":     func(m *Manifest) { m.Fields[0].ID = "State" },
		"dup value id":     func(m *Manifest) { m.Fields[0].Values[1].ID = "waiting" },
		"bad tone":         func(m *Manifest) { m.Fields[0].Values[0].Tone = "red" },
		"wide badge":       func(m *Manifest) { m.Fields[0].Values[0].Badge = "WAITING" },
		"bad action id":    func(m *Manifest) { m.Actions[0].ID = "do-it" },
		"dup section":      func(m *Manifest) { m.Sections = append(m.Sections, SectionDecl{ID: "agent", Title: "x"}) },
		"empty key":        func(m *Manifest) { m.Actions[0].Key = "" },
	}
	for name, mutate := range cases {
		m := validManifest()
		m.Fields[0].Values = append([]FieldValue(nil), m.Fields[0].Values...)
		mutate(&m)
		if err := m.Validate("example"); err == nil {
			t.Errorf("%s: invalid manifest accepted", name)
		}
	}
}

func TestFieldValueInLane(t *testing.T) {
	no := false
	if !(FieldValue{}).InLane() || (FieldValue{Lane: &no}).InLane() {
		t.Fatal("InLane: default should be true, explicit false false")
	}
}

func TestBeadRepoNullEncoding(t *testing.T) {
	b, _ := json.Marshal(BeadRef{DB: "proj", ID: "proj-1"})
	if !strings.Contains(string(b), `"repo":null`) {
		t.Fatalf("missing repo must encode as null: %s", b)
	}
	repo := "/r"
	b, _ = json.Marshal(BeadRef{DB: "proj", ID: "proj-1", Repo: &repo})
	if !strings.Contains(string(b), `"repo":"/r"`) {
		t.Fatalf("repo not encoded: %s", b)
	}
}
