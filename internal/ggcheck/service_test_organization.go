package ggcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
	"github.com/hydroan/gst/internal/goast"
)

// ServiceTestOrganization pairs every test file under the service directory
// with a source file of its package.
var ServiceTestOrganization = Check{
	Name: "Service test organization",
	Rule: "test files under the service directory must pair with a source file of their package; main_test.go only declares TestMain, fixtures_test.go only holds shared test fixtures, and test cases without a source file of their own belong in the test file of a related source file",
	run:  checkServiceTestOrganization,
}

// checkServiceTestOrganization checks that every test file under the service
// directory maps to a source file of its package: foo_test.go and its internal
// form foo_internal_test.go both pair with foo.go. Two names are reserved
// instead of paired: main_test.go only declares TestMain, and fixtures_test.go
// only holds shared test fixtures, so it declares no test functions. There is
// deliberately no per-file exemption: test cases without a source file of
// their own belong in the test file of a related source file, so pairing
// stays the only shape. Service subtrees owned by copyable framework modules
// are skipped like in checkServiceTestCoverage.
func checkServiceTestOrganization(ignore gghelper.ProjectIgnore) []string {
	var violations []string

	if _, err := os.Stat(ggconst.DirService); os.IsNotExist(err) {
		return violations
	}

	owned, err := copyableModuleOwners()
	if err != nil {
		return append(violations, fmt.Sprintf("listing copyable framework modules: %v", err))
	}

	walkErr := ignore.Walk(ggconst.DirService, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if path != ggconst.DirService && moduleOwnedPath(owned, ggconst.DirService, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		switch filepath.Base(path) {
		case ggconst.FileMainTest:
			violations = append(violations, reservedTestFileViolations(path, true)...)
		case "fixtures_test.go":
			violations = append(violations, reservedTestFileViolations(path, false)...)
		default:
			if pairedSourceFileExists(path) {
				return nil
			}
			violations = append(violations, fmt.Sprintf(
				"Test file '%s' does not match any source file in its package (move its cases into the test file of a related source file)",
				path,
			))
		}
		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Sprintf("walking service directory: %v", walkErr))
	}

	return violations
}

// pairedSourceFileExists reports whether a test file pairs with a source file
// of its package: foo_test.go needs foo.go, and foo_internal_test.go is the
// internal test form of foo.go.
func pairedSourceFileExists(testPath string) bool {
	stem := strings.TrimSuffix(testPath, "_test.go")
	if gghelper.FileExists(stem + ".go") {
		return true
	}
	if internal := strings.TrimSuffix(stem, "_internal"); internal != stem && gghelper.FileExists(internal+".go") {
		return true
	}
	return false
}

// reservedTestFileViolations reports the test functions that do not belong in
// a reserved test file. main_test.go (allowTestMain) only declares TestMain;
// fixtures_test.go holds shared fixtures, so it declares no test functions at
// all. Fixture helpers taking more than the bare *testing.T parameter are not
// test functions and stay allowed.
func reservedTestFileViolations(path string, allowTestMain bool) []string {
	var violations []string

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return violations
	}

	role := "should only hold shared test fixtures"
	if allowTestMain {
		role = "should only declare TestMain"
	}
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil {
			continue
		}
		if goast.IsTestMainFunc(fn) {
			if allowTestMain {
				continue
			}
		} else if !goast.IsTestCaseFunc(fn) {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"Test file '%s' %s (found: %s)", path, role, fn.Name.Name,
		))
	}

	return violations
}
