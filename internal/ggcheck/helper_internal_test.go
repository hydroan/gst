package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/testutil/golist"
)

// TestDatabaseEntryPointsMatchThePackage pins the dot-import list to the
// database package itself: every exported function whose first parameter is
// a context is an entry point the rule has to know, so a function added to
// the package cannot slip past a file that dot-imports it.
func TestDatabaseEntryPointsMatchThePackage(t *testing.T) {
	pkg := golist.Packages(t, gstDatabaseImportPath)[0]
	var want []string
	fset := token.NewFileSet()
	for _, source := range pkg.GoFiles(t) {
		file, parseErr := parser.ParseFile(fset, filepath.Join(pkg.Dir, source), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
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

// TestModuleOwnedPath pins which paths count as owned by a copied framework
// module: only what lies under a subtree named after one of them, and never
// the root itself or a path outside it.
func TestModuleOwnedPath(t *testing.T) {
	owned := map[string]bool{"iam": true}
	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{name: "file under a copied module", root: "model", path: filepath.Join("model", "iam", "user.go"), want: true},
		{name: "the module subtree itself", root: "model", path: filepath.Join("model", "iam"), want: true},
		{name: "file of another subtree", root: "model", path: filepath.Join("model", "record", "record.go")},
		{name: "the root itself", root: "model", path: "model"},
		{name: "path outside the root", root: "model", path: filepath.Join("service", "iam", "login.go")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moduleOwnedPath(owned, tt.root, tt.path); got != tt.want {
				t.Fatalf("moduleOwnedPath(%q, %q) = %t, want %t", tt.root, tt.path, got, tt.want)
			}
		})
	}

	t.Run("no copied module", func(t *testing.T) {
		if moduleOwnedPath(nil, "model", filepath.Join("model", "iam", "user.go")) {
			t.Fatal("moduleOwnedPath() = true without a copied module")
		}
	})
}
