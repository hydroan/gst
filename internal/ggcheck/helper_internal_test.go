package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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

func TestExcludedDir(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, dir := range []string{".git", "vendor", "tools", filepath.Join("service", "sample"), filepath.Join("service", ".cache"), filepath.Join("service", "testdata")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join("tools", "go.mod"), []byte("module example.com/tools\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{name: "the project root", root: ".", path: "."},
		{name: "a project directory", root: ".", path: "service"},
		{name: "a directory below it", root: ".", path: filepath.Join("service", "sample")},
		{name: "a hidden directory", root: ".", path: ".git", want: true},
		{name: "a hidden directory further down", root: ".", path: filepath.Join("service", ".cache"), want: true},
		{name: "vendor", root: ".", path: "vendor", want: true},
		{name: "testdata further down", root: ".", path: filepath.Join("service", "testdata"), want: true},
		{name: "a directory holding its own go.mod", root: ".", path: "tools", want: true},
		{name: "a walk root below the project root", root: "service", path: "service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := excludedDir(tt.root, tt.path); got != tt.want {
				t.Fatalf("excludedDir(%q, %q) = %t, want %t", tt.root, tt.path, got, tt.want)
			}
		})
	}
}
