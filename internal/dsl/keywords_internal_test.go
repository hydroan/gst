package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/testutil/golist"
)

// TestKeywordsMatchThePublicPackage pins that the keywords the parser knows
// (see methodList) are exactly the exported functions of the public dsl
// package, the ones a model file can call: a keyword on one side alone
// would be declarable but never read, or read but never declarable.
func TestKeywordsMatchThePublicPackage(t *testing.T) {
	pkg := golist.Packages(t, ggconst.ImportPathGst+"/dsl")[0]
	fset := token.NewFileSet()
	var exported []string
	for _, name := range pkg.GoFiles(t) {
		file, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() {
				exported = append(exported, fn.Name.Name)
			}
		}
	}
	slices.Sort(exported)
	keywords := slices.Sorted(slices.Values(methodList))
	if !slices.Equal(exported, keywords) {
		t.Fatalf("the public dsl package exports %v, the parser knows %v", exported, keywords)
	}
}
