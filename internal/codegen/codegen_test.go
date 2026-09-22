package codegen_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
)

func TestExtractAPIDocsStructs(t *testing.T) {
	entries := extractTestAPIDocs(t)

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

	sub, hasSub := byKey["example.com/proj/testdata/apidocmodel/sub.ArchiveReq"]
	if !hasSub {
		t.Fatal("sub.ArchiveReq entry missing, nested packages must be extracted")
	}
	if want := "Path is the file path."; sub.Doc.Fields["Path"] != want {
		t.Fatalf("sub.Doc.Fields[Path] = %q, want %q", sub.Doc.Fields["Path"], want)
	}

	if _, hasPlain := byKey["example.com/proj/testdata/apidocmodel.Plain"]; hasPlain {
		t.Fatal("Plain entry present, structs without comments must be skipped")
	}
	if _, hasHidden := byKey["example.com/proj/testdata/apidocmodel.hidden"]; hasHidden {
		t.Fatal("hidden entry present, unexported structs must be skipped")
	}
}

func TestExtractAPIDocsEnums(t *testing.T) {
	entries := extractTestAPIDocs(t)

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
	first := extractTestAPIDocs(t)
	second := extractTestAPIDocs(t)

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

// TestExtractAPIDocsSkipsFilesOutsideCodeGeneration checks the files the
// extraction leaves out: test files, files whose names start with "_", files
// in vendor and testdata directories, and the file names excludes lists.
func TestExtractAPIDocsSkipsFilesOutsideCodeGeneration(t *testing.T) {
	entries, err := codegen.ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", []string{"excluded.go"})
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}

	skipped := map[string]string{
		"example.com/proj/testdata/apidocmodel.InTestFile":       "test files",
		"example.com/proj/testdata/apidocmodel.Ignored":          `files with the "_" prefix`,
		"example.com/proj/testdata/apidocmodel/vendor.Vendored":  "vendor directories",
		"example.com/proj/testdata/apidocmodel/testdata.Fixture": "testdata directories",
		"example.com/proj/testdata/apidocmodel.Excluded":         "the files excludes lists",
	}
	for _, entry := range entries.Structs {
		key := entry.PkgPath + "." + entry.TypeName
		if reason, ok := skipped[key]; ok {
			t.Errorf("%s entry present, %s must be skipped", key, reason)
		}
	}

	// Without excludes the excluded file is extracted like any other.
	found := false
	for _, entry := range extractTestAPIDocs(t).Structs {
		if entry.TypeName == "Excluded" {
			found = true
		}
	}
	if !found {
		t.Fatal("Excluded entry missing, a file excludes does not list must be extracted")
	}
}

// TestFindModels finds the models declared under testdata/findmodel: the
// database model Record of the root package and the model Item of its
// sample package, each with the path of its model file.
func TestFindModels(t *testing.T) {
	models, err := codegen.FindModels("example.com/proj", "testdata/findmodel", nil)
	if err != nil {
		t.Fatalf("FindModels() error = %v", err)
	}

	got := make([]string, 0, len(models))
	for _, m := range models {
		got = append(got, fmt.Sprintf("%s.%s %s migrate=%v", m.ModelPkgName, m.ModelName, m.ModelFilePath, m.Design.Migrate))
	}
	want := []string{
		"findmodel.Record testdata/findmodel/record.go migrate=true",
		"sample.Item testdata/findmodel/sample/item.go migrate=false",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("FindModels() = %q, want %q", got, want)
	}
}

// extractTestAPIDocs extracts the API docs of the model package in
// testdata/apidocmodel, as a project of module example.com/proj would declare
// it, failing the test on error.
func extractTestAPIDocs(t *testing.T) gen.APIDocEntries {
	t.Helper()

	entries, err := codegen.ExtractAPIDocs("example.com/proj", "testdata/apidocmodel", nil)
	if err != nil {
		t.Fatalf("ExtractAPIDocs() error = %v", err)
	}
	return entries
}
