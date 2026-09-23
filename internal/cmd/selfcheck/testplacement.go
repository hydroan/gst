package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"golang.org/x/tools/go/packages"
)

// internalTestSuffix ends the name of every test file that joins the package
// it tests; testpackage's skip-regexp names the same suffix.
const internalTestSuffix = "_internal_test.go"

// violation is a test file named as an internal test that does not have to be
// one.
type violation struct {
	// File is the path of the test file, relative to the checked directory.
	File string
	// Message says what is wrong and how to fix it.
	Message string
}

// checkTestPlacement loads the packages under dir, tests included, and reports
// the test files named *_internal_test.go that could be external tests: those
// that declare the external test package, and those that use nothing
// unexported of the package they test, which checkTestPlacement confirms by
// type-checking them as external tests. A test file whose move would not
// compile is never reported, and neither is one that another internal test
// file depends on. Test files of a main package are left alone, since nothing
// can import a main package.
func checkTestPlacement(dir string) ([]violation, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Wrap(err, "testplacement: resolve the checked directory")
	}
	pkgs, err := load(root, nil, "./...")
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return nil, errors.Newf("testplacement: package %s does not type-check: %v", p.ID, p.Errors[0])
		}
	}

	var violations []violation
	for _, p := range pkgs {
		switch {
		case isExternalTest(p):
			for _, path := range p.CompiledGoFiles {
				if strings.HasSuffix(path, internalTestSuffix) {
					file := relative(root, path)
					violations = append(violations, violation{
						File:    file,
						Message: fmt.Sprintf("Test file '%s' is named as an internal test but declares package %s: drop _internal from its name", file, p.Name),
					})
				}
			}
		case isInternalTestVariant(p) && p.Name != "main":
			movable, err := confirmExternal(root, p, candidates(analyze(p)))
			if err != nil {
				return nil, err
			}
			for _, f := range movable {
				file := relative(root, f.path)
				violations = append(violations, violation{
					File:    file,
					Message: fmt.Sprintf("Test file '%s' uses nothing unexported of package %s: declare package %s_test and drop _internal from its name", file, p.Name, p.Name),
				})
			}
		}
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].File < violations[j].File })
	return violations, nil
}

// load loads the packages matching patterns under dir with their tests,
// syntax and type information, reading the overlay in place of the files it
// names.
func load(dir string, overlay map[string][]byte, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Tests:   true,
		Dir:     dir,
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, errors.Wrap(err, "testplacement: load packages")
	}
	return pkgs, nil
}

// isExternalTest reports whether p is an external test package, the one whose
// files declare package <name>_test.
func isExternalTest(p *packages.Package) bool {
	return strings.HasSuffix(p.PkgPath, "_test")
}

// isInternalTestVariant reports whether p is a package compiled together with
// its internal test files, as go test builds it.
func isInternalTestVariant(p *packages.Package) bool {
	return strings.Contains(p.ID, " [") && !strings.HasSuffix(p.ID, ".test") && !isExternalTest(p)
}

// relative returns path relative to root in slash form, or path itself when it
// lies outside root.
func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
