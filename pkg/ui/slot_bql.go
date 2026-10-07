package ui

import "github.com/seanmartinsmith/beadstui/pkg/bql"

// bqlFields returns bt's built-in BQL fields plus every registry field,
// typed as strings. Built-in names are never overridden.
func (m Model) bqlFields() map[string]bql.FieldType {
	fields := make(map[string]bql.FieldType, len(bql.ValidFields))
	for name, typ := range bql.ValidFields {
		fields[name] = typ
	}
	for _, name := range m.slotRegistry.FieldNames() {
		if _, builtin := fields[name]; !builtin {
			fields[name] = bql.FieldString
		}
	}
	return fields
}

// bqlExecuteOpts returns the options every TUI BQL execution uses.
func (m Model) bqlExecuteOpts() bql.ExecuteOpts {
	return bql.ExecuteOpts{IssueMap: m.data.issueMap, Fields: m.slotRegistry.FieldValue}
}
