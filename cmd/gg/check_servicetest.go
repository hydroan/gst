package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/hydroan/gst/internal/ggconfig"
	"github.com/hydroan/gst/internal/ggmodule"
)

// CheckServiceTestCoverage checks that every service file generated for a DSL
// Service() action has a matching test file next to it: create.go pairs with
// create_test.go, or with create_internal_test.go for internal tests. The
// expected file list comes from the same DSL scan and route-ignore pipeline
// that drives gg gen, so route-ignored actions and service files gg gen has
// not generated yet are not reported. Service subtrees owned by copyable
// framework modules are skipped: copied module code is tested inside the
// framework repository and stays unmodified in projects.
func CheckServiceTestCoverage(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		return violations
	}

	owned, err := copyableModuleServiceOwners()
	if err != nil {
		return append(violations, fmt.Sprintf("listing copyable framework modules: %v", err))
	}
	cfg, err := ggconfig.Load(".")
	if err != nil {
		return append(violations, fmt.Sprintf("loading gst.yaml: %v", err))
	}
	allModels, err := codegen.FindModels(currentProjectModulePath(), modelDir, serviceDir, excludes)
	if err != nil {
		return append(violations, fmt.Sprintf("scanning model designs: %v", err))
	}

	// Route-ignored actions are disabled here for the same reason gg gen
	// disables them: their service files stay on disk without a registered
	// route, so no test can exercise them.
	buildHierarchicalEndpoints(allModels)
	propagateParentParams(allModels)
	applyRouteIgnores(allModels, cfg.Gen.Routes.Ignore)

	seen := make(map[string]bool)
	for _, m := range allModels {
		if isIgnoredProjectPath(ignore, m.ModelFilePath, false) {
			continue
		}
		m.Design.Range(func(_ string, act *dsl.Action) {
			if !act.Enabled || !act.Service {
				return
			}
			target := gen.ServiceTarget(m, act, modelDir, serviceDir)
			if seen[target.FilePath] {
				return
			}
			seen[target.FilePath] = true

			if moduleOwnedServicePath(owned, target.FilePath) || isIgnoredProjectPath(ignore, target.FilePath, false) {
				return
			}
			// A service file that does not exist yet is gg gen's business:
			// requiring its test here would block the gen run that scaffolds it.
			if !fileExists(target.FilePath) {
				return
			}
			if serviceTestFileExists(target.FilePath) {
				return
			}

			stem := strings.TrimSuffix(filepath.Base(target.FilePath), ".go")
			violations = append(violations, fmt.Sprintf(
				"Service file '%s' has no matching test file (want %s_test.go or %s_internal_test.go)",
				target.FilePath, stem, stem,
			))
		})
	}

	return violations
}

// serviceTestFileExists reports whether a service file has a matching test
// file in its directory: <stem>_test.go or its internal form <stem>_internal_test.go.
func serviceTestFileExists(servicePath string) bool {
	stem := strings.TrimSuffix(servicePath, ".go")
	return fileExists(stem+"_test.go") || fileExists(stem+"_internal_test.go")
}

// CheckServiceTestOrganization checks that every test file under the service
// directory maps to a source file of its package: foo_test.go and its internal
// form foo_internal_test.go both pair with foo.go. Two names are reserved
// instead of paired: main_test.go only declares TestMain, and fixtures_test.go
// only holds shared test fixtures, so it declares no test functions. There is
// deliberately no per-file exemption: test cases without a source file of
// their own belong in the test file of a related source file, so pairing
// stays the only shape. Service subtrees owned by copyable framework modules
// are skipped like in CheckServiceTestCoverage.
func CheckServiceTestOrganization(ignore gitignore.Matcher) []string {
	var violations []string

	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return violations
	}

	owned, err := copyableModuleServiceOwners()
	if err != nil {
		return append(violations, fmt.Sprintf("listing copyable framework modules: %v", err))
	}

	walkErr := walkProjectDir(serviceDir, ignore, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if path != serviceDir && moduleOwnedServicePath(owned, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		switch filepath.Base(path) {
		case "main_test.go":
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
	if fileExists(stem + ".go") {
		return true
	}
	if internal := strings.TrimSuffix(stem, "_internal"); internal != stem && fileExists(internal+".go") {
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
		if isTestMainFunc(fn) {
			if allowTestMain {
				continue
			}
		} else if !isTestCaseFunc(fn) {
			continue
		}
		violations = append(violations, fmt.Sprintf(
			"Test file '%s' %s (found: %s)", path, role, fn.Name.Name,
		))
	}

	return violations
}

// isTestCaseFunc reports whether fn is a test function the go test runner
// picks up: TestXxx taking exactly one *testing.T parameter and returning
// nothing, where Xxx does not start with a lowercase letter.
func isTestCaseFunc(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if name == "TestMain" || !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) > len("Test") {
		if next := name[len("Test")]; next >= 'a' && next <= 'z' {
			return false
		}
	}
	return hasSingleTestingParam(fn, "T")
}

// isTestMainFunc reports whether fn is TestMain(m *testing.M).
func isTestMainFunc(fn *ast.FuncDecl) bool {
	return fn.Name.Name == "TestMain" && hasSingleTestingParam(fn, "M")
}

// hasSingleTestingParam reports whether fn takes exactly one *testing.<sel>
// parameter and returns nothing.
func hasSingleTestingParam(fn *ast.FuncDecl, sel string) bool {
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		return false
	}
	param := fn.Type.Params.List[0]
	if len(param.Names) > 1 {
		return false
	}
	star, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != sel {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "testing"
}

// copyableModuleServiceOwners returns the first path segments under the
// service directory owned by copyable framework modules: gg module copy
// writes module service code to service/<module>/... subtrees.
func copyableModuleServiceOwners() (map[string]bool, error) {
	names, err := ggmodule.CopyableModuleNames()
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(names))
	for _, name := range names {
		owned[name] = true
	}
	return owned, nil
}

// moduleOwnedServicePath reports whether a path below the service directory
// falls under a service subtree owned by a copyable framework module.
func moduleOwnedServicePath(owned map[string]bool, path string) bool {
	if len(owned) == 0 {
		return false
	}
	rel, err := filepath.Rel(serviceDir, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	first, _, _ := strings.Cut(rel, string(filepath.Separator))
	return owned[first]
}
