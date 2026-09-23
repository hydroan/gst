package main

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// internalTestSuffix ends the name of every test file that joins the package
// it tests; testpackage's skip-regexp names the same suffix.
const internalTestSuffix = "_internal_test.go"

// checkTestPlacement reports the test files under root named *_internal_test.go
// that could be external tests: those that declare the external test package,
// and those that use nothing unexported of the package they test, which
// checkTestPlacement confirms by type-checking them as external tests. A test
// file whose move would not compile is never reported, and neither is one that
// another internal test file depends on. Test files of a main package are left
// alone, since nothing can import a main package.
func checkTestPlacement(root string, pkgs []*packages.Package) ([]violation, error) {
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
			movable, err := confirmExternal(root, p, externalCandidates(analyzeTestFiles(p)))
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

// isExternalTest reports whether p is an external test package, the one whose
// files declare package <name>_test.
func isExternalTest(p *packages.Package) bool {
	return strings.HasSuffix(p.PkgPath, "_test")
}
