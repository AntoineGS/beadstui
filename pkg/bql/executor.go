// Adapted from github.com/zjrosen/perles (MIT License). See LICENSE in this directory.
// Interface redesigned for bt: in-memory execution against model.Issue.

package bql

import "github.com/seanmartinsmith/beadstui/pkg/model"

// Executor evaluates a parsed BQL query against issues.
type Executor interface {
	// Execute filters and sorts issues according to the query.
	// ORDER BY is applied if present. EXPAND adds related issues.
	Execute(query *Query, issues []model.Issue, opts ExecuteOpts) []model.Issue

	// Matches evaluates only the filter expression against a single issue.
	// Does not handle ORDER BY or EXPAND (those are set-level operations).
	Matches(query *Query, issue model.Issue, opts ExecuteOpts) bool
}

// FieldResolver answers BQL fields that are not built in, such as fields
// registered by TUI extensions. ok is false when the issue has no value.
type FieldResolver func(issue *model.Issue, field string) (value string, ok bool)

// ExecuteOpts provides context needed by the executor beyond the issue list.
type ExecuteOpts struct {
	// IssueMap enables dependency lookups for blocked field and EXPAND.
	// Uses pointer values to match bt's Model.issueMap type. Pass the full
	// loaded corpus, not a scoped subset: whether an issue is blocked
	// depends on issues outside the result set (bt-5muh4).
	IssueMap map[string]*model.Issue

	// Fields answers fields missing from ValidFields. Built-in names always
	// take precedence. Nil means only built-in fields have values.
	Fields FieldResolver

	// blocked is blockedIDs(IssueMap), computed once per Execute/Matches
	// call when the query references the blocked field.
	blocked map[string]bool
}
