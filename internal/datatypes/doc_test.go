package datatypes_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEveryTypeIsDeclaredInAFileNamedAfterIt holds the layout the package
// documentation describes: every source file but doc.go and library.go
// declares one exported type and is named after it, date.go for Date,
// library.go declares none, and every test file but this one is named after
// the source file it tests.
func TestEveryTypeIsDeclaredInAFileNamedAfterIt(t *testing.T) {
	names, err := filepath.Glob("*.go")
	require.NoError(t, err)

	sources := make(map[string]bool)
	fset := token.NewFileSet()
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") || name == "doc.go" {
			continue
		}
		sources[name] = true
		file, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		var declared []string
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if typ, isType := spec.(*ast.TypeSpec); isType && typ.Name.IsExported() {
					declared = append(declared, typ.Name.Name)
				}
			}
		}
		if name == "library.go" {
			require.Empty(t, declared, "library.go declares the exported types %v: a type is declared in a file of its own", declared)
			continue
		}
		require.Len(t, declared, 1, "%s declares the exported types %v: a file declares one", name, declared)
		require.Equal(t, strings.ToLower(declared[0])+".go", name, "the type %s is declared in a file named after it", declared[0])
	}
	for _, name := range names {
		if !strings.HasSuffix(name, "_test.go") || name == "doc_test.go" {
			continue
		}
		require.True(t, sources[strings.TrimSuffix(name, "_test.go")+".go"], "%s is named after no source file of the package", name)
	}
}
