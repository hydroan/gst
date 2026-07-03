package openapigen

import (
	"go/ast"
	"testing"
)

func TestExtractCommentTextPreservesMarkdownFormatting(t *testing.T) {
	comment := &ast.CommentGroup{List: []*ast.Comment{
		{Text: "// Group is the group record."},
		{Text: "//"},
		{Text: "// Business logic: stores the stable identity."},
		{Text: "//"},
		{Text: "// Field sources:"},
		{Text: "//   - ExternalGroupNo: from group_config.group_id."},
		{Text: "//   - GroupName: from func_config.group_name."},
	}}

	got := extractCommentText(comment)
	want := "Group is the group record.\n\nBusiness logic: stores the stable identity.\n\nField sources:\n  - ExternalGroupNo: from group_config.group_id.\n  - GroupName: from func_config.group_name."
	if got != want {
		t.Fatalf("extractCommentText() = %q, want %q", got, want)
	}
}

func TestExtractCommentTextKeepsFieldCommentText(t *testing.T) {
	comment := &ast.CommentGroup{List: []*ast.Comment{
		{Text: "// GroupName is the group display name."},
	}}

	got := extractCommentText(comment)
	want := "GroupName is the group display name."
	if got != want {
		t.Fatalf("extractCommentText() = %q, want %q", got, want)
	}
}
