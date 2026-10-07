package ui

import (
	"testing"

	"github.com/seanmartinsmith/beadstui/pkg/bql"
	"github.com/seanmartinsmith/beadstui/pkg/model"
)

type agentFields map[string]string

func (a agentFields) Fields() []string { return []string{"agent.state", "status"} }

func (a agentFields) Value(issue *model.Issue, field string) (string, bool) {
	if field != "agent.state" {
		return "closed", true // tries to shadow the built-in status
	}
	v, ok := a[issue.ID]
	return v, ok
}

func TestBQLFieldsIncludeRegistryFields(t *testing.T) {
	m := NewModel([]model.Issue{slotTestIssue()}, nil, "", nil, nil)
	m.slotRegistry.AddFields(agentFields{})

	fields := m.bqlFields()
	if fields["agent.state"] != bql.FieldString {
		t.Fatalf("agent.state = %v, want FieldString", fields["agent.state"])
	}
	if fields["status"] != bql.ValidFields["status"] {
		t.Fatal("registry field shadowed built-in status type")
	}
	if _, ok := bql.ValidFields["agent.state"]; ok {
		t.Fatal("bqlFields mutated bql.ValidFields")
	}
}

func TestApplyBQLFiltersOnRegistryField(t *testing.T) {
	a, b := slotTestIssue(), slotTestIssue()
	a.ID, b.ID = "x-1", "x-2"
	m := NewModel([]model.Issue{a, b}, nil, "", nil, nil)
	m.slotRegistry.AddFields(agentFields{"x-2": "waiting"})

	q, err := bql.Parse("agent.state = waiting and status = open")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := bql.ValidateWithFields(q, m.bqlFields()); err != nil {
		t.Fatalf("ValidateWithFields: %v", err)
	}
	m.applyBQL(q, "agent.state = waiting and status = open")

	items := m.list.Items()
	if len(items) != 1 || items[0].(IssueItem).Issue.ID != "x-2" {
		t.Fatalf("filtered items = %v, want only x-2", items)
	}
}
