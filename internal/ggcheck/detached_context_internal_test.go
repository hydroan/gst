package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestDatabaseEntryPointsMatchThePackage pins the dot-import list to the
// database package itself: every exported function whose first parameter is
// a context is an entry point the rule has to know, so a function added to
// the package cannot slip past a file that dot-imports it.
func TestDatabaseEntryPointsMatchThePackage(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join(frameworkRepoRoot(t), "database", "*.go"))
	if err != nil {
		t.Fatal(err)
	}

	var want []string
	fset := token.NewFileSet()
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() || len(fn.Type.Params.List) == 0 {
				continue
			}
			if selector, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); ok && selector.Sel.Name == "Context" {
				want = append(want, fn.Name.Name)
			}
		}
	}
	slices.Sort(want)

	got := slices.Clone(databaseEntryPoints)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("databaseEntryPoints must list the package's context-taking functions:\n got %v\nwant %v", got, want)
	}
}

// frameworkRepoRoot returns the absolute path of this repository's root.
func frameworkRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
