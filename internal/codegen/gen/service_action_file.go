package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

// IsActionServiceSource reports whether the Go source file at path contains a type that embeds
// service.Base with three type parameters, matching gg-generated per-action service files
// (including those with a custom DSL Filename). It returns false on read/parse errors.
func IsActionServiceSource(path string) bool {
	src, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if ok && isServiceType(ts) {
				return true
			}
		}
	}
	return false
}
