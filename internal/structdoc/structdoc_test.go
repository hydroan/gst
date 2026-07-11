package structdoc

import (
	"go/ast"
	"testing"
)

const testSource = `package demo

// User is a demo model.
// It spans multiple lines.
type User struct {
	// Name is the user name.
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age\"`" + ` // Age is the user age.
	Anonymous string

	/* Address is the user address. */
	Address string
}

// Alias is not a struct and must be skipped.
type Alias = User

// group declares two structs in one type block.
type (
	// Req is the request.
	Req struct {
		// ID is the target id.
		ID string
	}
	Rsp struct{}
)

type undocumented struct {
	Value string
}
`

func TestParseSource(t *testing.T) {
	docs, err := ParseSource("demo.go", []byte(testSource))
	if err != nil {
		t.Fatalf("ParseSource() error = %v", err)
	}

	user, ok := docs["User"]
	if !ok {
		t.Fatal("docs[User] missing")
	}
	if want := "User is a demo model.\nIt spans multiple lines."; user.Comment != want {
		t.Fatalf("user.Comment = %q, want %q", user.Comment, want)
	}
	if want := "Name is the user name."; user.Fields["Name"] != want {
		t.Fatalf("user.Fields[Name] = %q, want %q", user.Fields["Name"], want)
	}
	if want := "Age is the user age."; user.Fields["Age"] != want {
		t.Fatalf("user.Fields[Age] = %q, want %q", user.Fields["Age"], want)
	}
	if want := "Address is the user address."; user.Fields["Address"] != want {
		t.Fatalf("user.Fields[Address] = %q, want %q", user.Fields["Address"], want)
	}
	if _, hasAnonymous := user.Fields["Anonymous"]; hasAnonymous {
		t.Fatal("user.Fields[Anonymous] present, want absent for uncommented field")
	}

	req, ok := docs["Req"]
	if !ok {
		t.Fatal("docs[Req] missing")
	}
	if want := "Req is the request."; req.Comment != want {
		t.Fatalf("req.Comment = %q, want %q", req.Comment, want)
	}
	if want := "ID is the target id."; req.Fields["ID"] != want {
		t.Fatalf("req.Fields[ID] = %q, want %q", req.Fields["ID"], want)
	}

	// Rsp has no own doc; the grouped GenDecl doc must be used as fallback.
	rsp, ok := docs["Rsp"]
	if !ok {
		t.Fatal("docs[Rsp] missing")
	}
	if want := "group declares two structs in one type block."; rsp.Comment != want {
		t.Fatalf("rsp.Comment = %q, want %q", rsp.Comment, want)
	}

	if _, hasAlias := docs["Alias"]; hasAlias {
		t.Fatal("docs[Alias] present, want absent for non-struct type")
	}
	if _, hasUndocumented := docs["undocumented"]; hasUndocumented {
		t.Fatal("docs[undocumented] present, want absent for struct without any comment")
	}
}

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

	got := ExtractCommentText(comment)
	want := "Group is the group record.\n\nBusiness logic: stores the stable identity.\n\nField sources:\n  - ExternalGroupNo: from group_config.group_id.\n  - GroupName: from func_config.group_name."
	if got != want {
		t.Fatalf("ExtractCommentText() = %q, want %q", got, want)
	}
}

func TestExtractCommentTextKeepsFieldCommentText(t *testing.T) {
	comment := &ast.CommentGroup{List: []*ast.Comment{
		{Text: "// GroupName is the group display name."},
	}}

	got := ExtractCommentText(comment)
	want := "GroupName is the group display name."
	if got != want {
		t.Fatalf("ExtractCommentText() = %q, want %q", got, want)
	}
}

func TestParseSourceInvalid(t *testing.T) {
	if _, err := ParseSource("bad.go", []byte("not go source")); err == nil {
		t.Fatal("ParseSource() error = nil, want parse error")
	}
}
