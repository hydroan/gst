package gghelper

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/goast"
)

// PresentOptionalImportDirs lists the directories of
// ggconst.OptionalImportDirs the project in the working directory holds a
// Go source file in, in that order: the packages main.go imports beside the
// standard ones, and the test files declaring TestMain with it (see
// OptionalImportDrift). Test files alone make no package to import, nor
// does a missing directory: for a project holding interceptor/auth.go and
// pb/pb.gen.go it lists interceptor and pb, and for one holding
// interceptor/auth_test.go alone, nothing.
func PresentOptionalImportDirs() []string {
	var dirs []string
	for _, dir := range ggconst.OptionalImportDirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		if slices.ContainsFunc(files, func(path string) bool { return !strings.HasSuffix(path, ggconst.PatternTestFile) }) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// TestMainFiles lists the test files of the project in the working
// directory, among the paths ignore keeps (see ProjectIgnore), that declare
// TestMain at package level in an external test package, in path order: the
// files whose TestMain may start the test server of their package the way
// main.go starts the application, and so import the packages main.go imports
// (see OptionalImportDrift). The TestMain of an internal test package is left
// out: the packages main.go imports may import that package, which its own
// tests cannot import back. For a project holding service/record/main_test.go
// of package record_test and dao/entry/main_test.go of package entry, both
// declaring TestMain, it lists service/record/main_test.go. A test file
// mentioning TestMain that does not parse is an error, gg being unable to tell
// what it declares; one mentioning no TestMain declares none and is not parsed.
func TestMainFiles(ignore ProjectIgnore) ([]string, error) {
	var files []string
	err := ignore.Walk(".", func(path string, info os.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(info.Name(), ggconst.PatternTestFile) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(src, []byte("TestMain")) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.SkipObjectResolution)
		if err != nil {
			return errors.Wrapf(err, "reading the test file %s", path)
		}
		if strings.HasSuffix(file.Name.Name, "_test") && goast.DeclaresTestMain(file) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// OptionalImportDrift tells the imports a TestMain file (see TestMainFiles)
// has for the optional project packages apart from the ones main.go has,
// present being the directories of ggconst.OptionalImportDirs the project
// holds (see PresentOptionalImportDirs) and module its module path: missing
// lists the import paths of the present packages the file does not import,
// stale those of the absent packages it blank-imports, both in the order of
// ggconst.OptionalImportDirs. An absent package the file imports under a
// name is neither: the file's code refers to it, and the compiler reports
// the absence. For a file of the module example.com/app importing
//
//	_ "example.com/app/model"
//	_ "example.com/app/pb"
//
// with interceptor present and pb absent, missing is
// example.com/app/interceptor and stale is example.com/app/pb; with both
// present, missing is example.com/app/interceptor alone.
func OptionalImportDrift(file *ast.File, module string, present []string) (missing, stale []string) {
	for _, dir := range ggconst.OptionalImportDirs {
		importPath := module + "/" + dir
		spec := goast.FindImportSpec(file, importPath)
		switch {
		case slices.Contains(present, dir):
			if spec == nil {
				missing = append(missing, importPath)
			}
		case spec != nil && spec.Name != nil && spec.Name.Name == "_":
			stale = append(stale, importPath)
		}
	}
	return missing, stale
}
