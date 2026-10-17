package ggcheck

import (
	"fmt"
	"go/parser"
	"go/token"

	"github.com/hydroan/gst/internal/gghelper"
)

// TestMainImports keeps the test files declaring TestMain importing the
// optional project packages the way main.go does.
var TestMainImports = Check{
	Name: "TestMain imports",
	Rule: "test files declaring TestMain in an external test package must blank-import the interceptor and pb packages exactly when main.go imports them, which is while they hold Go sources",
	run:  checkTestMainImports,
}

// checkTestMainImports checks that every test file declaring TestMain in an
// external test package (see gghelper.TestMainFiles), which starts the test
// server of its package the way main.go starts the application, imports the
// interceptor and pb packages exactly while the project holds them (see
// gghelper.OptionalImportDrift), as gg gen keeps them: without the import of
// a package the project holds, the test server registers nothing the
// package registers, its interceptors or its gRPC services, and the tests
// pass over them or dial a listener that is not there; with the import of a
// package the project holds no more, the test binary does not build.
func checkTestMainImports(ignore gghelper.ProjectIgnore) []string {
	modulePath, err := gghelper.ModulePath()
	if err != nil {
		return []string{fmt.Sprintf("reading the module path: %v", err)}
	}
	files, err := gghelper.TestMainFiles(ignore)
	if err != nil {
		return []string{fmt.Sprintf("listing the test files declaring TestMain: %v", err)}
	}

	var violations []string
	present := gghelper.PresentOptionalImportDirs()
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			violations = append(violations, fmt.Sprintf("reading %s: %v", path, err))
			continue
		}
		missing, stale := gghelper.OptionalImportDrift(file, modulePath, present)
		for _, importPath := range missing {
			violations = append(violations, fmt.Sprintf("%s declares TestMain without importing %s, which main.go imports: import it the same way, _ %q, as gg gen does", path, importPath, importPath))
		}
		for _, importPath := range stale {
			violations = append(violations, fmt.Sprintf("%s imports %s, a package without Go sources that main.go does not import: drop the import, as gg gen does", path, importPath))
		}
	}
	return violations
}
