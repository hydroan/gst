package dsl

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestKeywordsMatchThePublicPackage pins that the keywords the parser knows
// (see methodList) are exactly the exported functions of the public dsl
// package, the ones a model file can call: a keyword on one side alone
// would be declarable but never read, or read but never declarable.
func TestKeywordsMatchThePublicPackage(t *testing.T) {
	dir := filepath.Join("..", "..", "dsl")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the public dsl package: %v", err)
	}
	fset := token.NewFileSet()
	var exported []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
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
