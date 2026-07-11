package codegen

import (
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestExtractAPIDocs(t *testing.T) {
	entries, err := ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}

	byKey := make(map[string]gen.StructDocEntry, len(entries.Structs))
	for _, entry := range entries.Structs {
		byKey[entry.PkgPath+"."+entry.TypeName] = entry
	}

	user, ok := byKey["example.com/proj/testdata/apidocmodel.User"]
	if !ok {
		t.Fatal("User entry missing")
	}
	if want := "User is the user record."; user.Doc.Comment != want {
		t.Fatalf("user.Doc.Comment = %q, want %q", user.Doc.Comment, want)
	}
	if want := "Name is the user name."; user.Doc.Fields["Name"] != want {
		t.Fatalf("user.Doc.Fields[Name] = %q, want %q", user.Doc.Fields["Name"], want)
	}
	if want := "Age is the user age."; user.Doc.Fields["Age"] != want {
		t.Fatalf("user.Doc.Fields[Age] = %q, want %q", user.Doc.Fields["Age"], want)
	}

	if _, hasCreateReq := byKey["example.com/proj/testdata/apidocmodel.UserCreateReq"]; !hasCreateReq {
		t.Fatal("UserCreateReq entry missing, custom request types must be extracted")
	}

	sub, hasSub := byKey["example.com/proj/testdata/apidocmodel/sub.EncryptReq"]
	if !hasSub {
		t.Fatal("sub.EncryptReq entry missing, nested packages must be extracted")
	}
	if want := "Path is the file path."; sub.Doc.Fields["Path"] != want {
		t.Fatalf("sub.Doc.Fields[Path] = %q, want %q", sub.Doc.Fields["Path"], want)
	}

	if _, hasPlain := byKey["example.com/proj/testdata/apidocmodel.plain"]; hasPlain {
		t.Fatal("plain entry present, structs without comments must be skipped")
	}
	if _, hasIgnored := byKey["example.com/proj/testdata/apidocmodel.Ignored"]; hasIgnored {
		t.Fatal(`Ignored entry present, files with the "_" prefix must be skipped`)
	}
}

func TestExtractAPIDocsEnums(t *testing.T) {
	entries, err := ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}

	var status *gen.EnumDocEntry
	for i := range entries.Enums {
		if entries.Enums[i].TypeName == "UserStatus" {
			status = &entries.Enums[i]
		}
	}
	if status == nil {
		t.Fatal("UserStatus enum entry missing")
	}
	if status.PkgPath != "example.com/proj/testdata/apidocmodel" {
		t.Fatalf("status.PkgPath = %q, want the model package path", status.PkgPath)
	}
	if want := "UserStatus is the lifecycle status of a user."; status.Doc.Comment != want {
		t.Fatalf("status.Doc.Comment = %q, want %q", status.Doc.Comment, want)
	}
	if len(status.Doc.Values) != 2 || status.Doc.Values[0].Value != "active" || status.Doc.Values[1].Value != "disabled" {
		t.Fatalf("status.Doc.Values = %#v, want active and disabled in order", status.Doc.Values)
	}
	if status.Doc.Values[0].Comment != "the user can log in" {
		t.Fatalf("status.Doc.Values[0].Comment = %q, want the constant comment", status.Doc.Values[0].Comment)
	}
}

func TestExtractAPIDocsDeterministicOrder(t *testing.T) {
	first, err := ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}
	second, err := ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}

	if len(first.Structs) != len(second.Structs) || len(first.Enums) != len(second.Enums) {
		t.Fatalf("entry counts differ between runs: %d/%d vs %d/%d",
			len(first.Structs), len(first.Enums), len(second.Structs), len(second.Enums))
	}
	for i := range first.Structs {
		if first.Structs[i].PkgPath != second.Structs[i].PkgPath || first.Structs[i].TypeName != second.Structs[i].TypeName {
			t.Fatalf("struct entry order differs at index %d", i)
		}
	}
	for i := range first.Enums {
		if first.Enums[i].PkgPath != second.Enums[i].PkgPath || first.Enums[i].TypeName != second.Enums[i].TypeName {
			t.Fatalf("enum entry order differs at index %d", i)
		}
	}
}
