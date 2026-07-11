package codegen

import (
	"testing"

	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestExtractStructDocs(t *testing.T) {
	entries, err := ExtractStructDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractStructDocs() error = %v", err)
	}

	byKey := make(map[string]gen.StructDocEntry, len(entries))
	for _, entry := range entries {
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

func TestExtractStructDocsDeterministicOrder(t *testing.T) {
	first, err := ExtractStructDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractStructDocs() error = %v", err)
	}
	second, err := ExtractStructDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractStructDocs() error = %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("entry count differs between runs: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].PkgPath != second[i].PkgPath || first[i].TypeName != second[i].TypeName {
			t.Fatalf("entry order differs at index %d: %s.%s vs %s.%s",
				i, first[i].PkgPath, first[i].TypeName, second[i].PkgPath, second[i].TypeName)
		}
	}
}
