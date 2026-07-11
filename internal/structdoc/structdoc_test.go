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

const enumSource = `package demo

// PayoutRuleType is the payout rule of the special settlement.
type PayoutRuleType string

const (
	PayoutRuleTypeFixedOdds   PayoutRuleType = "fixed_odds"   // payout with fixed odds
	PayoutRuleTypeReturnStake PayoutRuleType = "return_stake" // return the stake only
)

// Priority is an integer enum using iota.
type Priority int

const (
	PriorityLow Priority = iota // lowest priority
	PriorityMid
	// PriorityHigh is the highest priority.
	PriorityHigh
)

// NotEnum is a named type without constants.
type NotEnum string

// untypedGroup constants must not leak into the preceding enum.
const (
	UntypedA = "a"
	UntypedB = "b"
)

// Alias types must be skipped.
type StatusAlias = PayoutRuleType
`

func TestParseSourceDocsEnums(t *testing.T) {
	docs, err := ParseSourceDocs("demo.go", []byte(enumSource))
	if err != nil {
		t.Fatalf("ParseSourceDocs() error = %v", err)
	}

	payout, ok := docs.Enums["PayoutRuleType"]
	if !ok {
		t.Fatal("Enums[PayoutRuleType] missing")
	}
	if want := "PayoutRuleType is the payout rule of the special settlement."; payout.Comment != want {
		t.Fatalf("payout.Comment = %q, want %q", payout.Comment, want)
	}
	if len(payout.Values) != 2 {
		t.Fatalf("payout.Values = %#v, want 2 values", payout.Values)
	}
	if payout.Values[0].Value != "fixed_odds" || payout.Values[0].Comment != "payout with fixed odds" {
		t.Fatalf("payout.Values[0] = %#v, want fixed_odds with its comment", payout.Values[0])
	}
	if payout.Values[1].Value != "return_stake" {
		t.Fatalf("payout.Values[1] = %#v, want return_stake", payout.Values[1])
	}

	priority, ok := docs.Enums["Priority"]
	if !ok {
		t.Fatal("Enums[Priority] missing")
	}
	if len(priority.Values) != 3 {
		t.Fatalf("priority.Values = %#v, want 3 values", priority.Values)
	}
	for i, want := range []int{0, 1, 2} {
		if priority.Values[i].Value != want {
			t.Fatalf("priority.Values[%d].Value = %v, want %d", i, priority.Values[i].Value, want)
		}
	}
	if priority.Values[2].Comment != "PriorityHigh is the highest priority." {
		t.Fatalf("priority.Values[2].Comment = %q, want the doc comment", priority.Values[2].Comment)
	}

	notEnum, ok := docs.Enums["NotEnum"]
	if !ok {
		t.Fatal("Enums[NotEnum] missing, a commented named type without constants still carries its comment")
	}
	if len(notEnum.Values) != 0 {
		t.Fatalf("notEnum.Values = %#v, want no values", notEnum.Values)
	}

	if _, ok := docs.Enums["StatusAlias"]; ok {
		t.Fatal("Enums[StatusAlias] present, want alias types skipped")
	}
	for name, enum := range docs.Enums {
		for _, v := range enum.Values {
			if v.Value == "a" || v.Value == "b" {
				t.Fatalf("untyped constant leaked into enum %s: %#v", name, v)
			}
		}
	}
}

func TestParseSourceDocsEnumConstantsInSeparateFile(t *testing.T) {
	docs, err := ParseSourceDocs("consts.go", []byte("package demo\n\nconst StatusOn Status = \"on\" // enabled\n"))
	if err != nil {
		t.Fatalf("ParseSourceDocs() error = %v", err)
	}

	status, ok := docs.Enums["Status"]
	if !ok {
		t.Fatal("Enums[Status] missing, constants of a type declared elsewhere must be collected")
	}
	if len(status.Values) != 1 || status.Values[0].Value != "on" {
		t.Fatalf("status.Values = %#v, want the single on value", status.Values)
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
