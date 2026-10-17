package gghelper_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestPresentOptionalImportDirsListsThePackagesHoldingGoSources pins the
// examples in the PresentOptionalImportDirs documentation: a directory counts
// once it holds a Go source file, test files alone and a missing directory
// do not, and the result keeps the order of ggconst.OptionalImportDirs.
func TestPresentOptionalImportDirsListsThePackagesHoldingGoSources(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := gghelper.PresentOptionalImportDirs(); len(got) != 0 {
		t.Fatalf("PresentOptionalImportDirs() = %v in an empty project, want none", got)
	}

	writeFile(t, filepath.Join("interceptor", "interceptor_test.go"), "package interceptor_test\n")
	if got := gghelper.PresentOptionalImportDirs(); len(got) != 0 {
		t.Fatalf("PresentOptionalImportDirs() = %v with a test file alone, want none", got)
	}

	writeFile(t, filepath.Join("pb", "pb.gen.go"), "package pb\n")
	if got, want := gghelper.PresentOptionalImportDirs(), []string{"pb"}; !slices.Equal(got, want) {
		t.Fatalf("PresentOptionalImportDirs() = %v, want %v", got, want)
	}

	writeFile(t, filepath.Join("interceptor", "interceptor.go"), "package interceptor\n")
	if got, want := gghelper.PresentOptionalImportDirs(), []string{"interceptor", "pb"}; !slices.Equal(got, want) {
		t.Fatalf("PresentOptionalImportDirs() = %v, want %v", got, want)
	}
}

// TestTestMainFilesListsTheExternalTestFilesDeclaringTestMain pins which
// files TestMainFiles lists: every test file of the project declaring TestMain
// at package level in an external test package, wherever it lies, in path
// order; and not one of an internal test package, one declaring no TestMain,
// nor one the project ignores.
func TestTestMainFilesListsTheExternalTestFilesDeclaringTestMain(t *testing.T) {
	t.Chdir(t.TempDir())
	testMain := "import \"testing\"\n\nfunc TestMain(m *testing.M) {}\n"
	writeFile(t, "go.mod", "module example.com/app\n\ngo 1.27\n")
	writeFile(t, ".gitignore", "service/scratch/\n")
	writeFile(t, filepath.Join("service", "record", "main_test.go"), "package record_test\n\n"+testMain)
	writeFile(t, filepath.Join("service", "record", "create_test.go"), "package record_test\n\nimport \"testing\"\n\nfunc TestCreate(t *testing.T) {}\n")
	writeFile(t, filepath.Join("module", "module_test.go"), "package module_test\n\n"+testMain)
	writeFile(t, filepath.Join("dao", "entry", "main_test.go"), "package entry\n\n"+testMain)
	writeFile(t, filepath.Join("service", "scratch", "main_test.go"), "package scratch_test\n\n"+testMain)
	writeFile(t, filepath.Join("service", "record", "testdata", "main_test.go"), "package record_test\n\n"+testMain)
	writeFile(t, filepath.Join("vendor", "lib", "main_test.go"), "package lib_test\n\n"+testMain)

	got, err := gghelper.TestMainFiles(gghelper.NewProjectIgnore())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join("module", "module_test.go"), filepath.Join("service", "record", "main_test.go")}
	if !slices.Equal(got, want) {
		t.Fatalf("TestMainFiles() = %v, want %v", got, want)
	}
}

// TestTestMainFilesRefusesAnUnparsableTestFile pins that a test file
// mentioning TestMain that does not parse is an error, gg being unable to
// tell what it declares, while one mentioning no TestMain is passed over
// without being parsed.
func TestTestMainFilesRefusesAnUnparsableTestFile(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "go.mod", "module example.com/app\n\ngo 1.27\n")
	writeFile(t, filepath.Join("service", "record", "create_test.go"), "package record_test\n\nfunc TestCreate(\n")

	if got, err := gghelper.TestMainFiles(gghelper.NewProjectIgnore()); err != nil || len(got) != 0 {
		t.Fatalf("TestMainFiles() = %v, %v with an unparsable file mentioning no TestMain, want none and no error", got, err)
	}

	writeFile(t, filepath.Join("service", "record", "main_test.go"), "package record_test\n\nfunc TestMain(\n")
	if _, err := gghelper.TestMainFiles(gghelper.NewProjectIgnore()); err == nil {
		t.Fatal("TestMainFiles() accepted an unparsable file mentioning TestMain")
	}
}

// TestOptionalImportDriftTellsTheMissingFromTheStaleImports pins the
// examples in the OptionalImportDrift documentation: a present package the
// file does not import is missing, an absent one it blank-imports is stale,
// and an absent one it imports under a name is neither, the file's code
// referring to it.
func TestOptionalImportDriftTellsTheMissingFromTheStaleImports(t *testing.T) {
	parse := func(src string) *ast.File {
		t.Helper()
		file, err := parser.ParseFile(token.NewFileSet(), "main_test.go", src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	file := parse(`package record_test

import (
	"testing"

	_ "example.com/app/model"
	_ "example.com/app/pb"

	"github.com/hydroan/gst/testutil"
)
`)
	for _, tc := range []struct {
		present        []string
		missing, stale []string
	}{
		{present: nil, stale: []string{"example.com/app/pb"}},
		{present: []string{"pb"}},
		{present: []string{"interceptor", "pb"}, missing: []string{"example.com/app/interceptor"}},
		{present: []string{"interceptor"}, missing: []string{"example.com/app/interceptor"}, stale: []string{"example.com/app/pb"}},
	} {
		missing, stale := gghelper.OptionalImportDrift(file, "example.com/app", tc.present)
		if !slices.Equal(missing, tc.missing) || !slices.Equal(stale, tc.stale) {
			t.Fatalf("OptionalImportDrift(present %v) = missing %v, stale %v; want missing %v, stale %v", tc.present, missing, stale, tc.missing, tc.stale)
		}
	}

	named := parse("package record_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/app/pb\"\n)\n")
	if missing, stale := gghelper.OptionalImportDrift(named, "example.com/app", nil); len(missing) != 0 || len(stale) != 0 {
		t.Fatalf("OptionalImportDrift() = missing %v, stale %v for a named import of an absent package, want neither", missing, stale)
	}
}
