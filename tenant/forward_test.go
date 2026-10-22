package tenant_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPackageForwardsWhatAProjectUses pins the public surface of the tenant
// package: the struct a model embeds, the default tenant, the two errors a
// tenant-scoped write fails with, and the contexts code acts in. The column's
// type is not among them, so embedding Scope is the only way a model carries
// the tenant column, with the guarantees its tags bring.
func TestPackageForwardsWhatAProjectUses(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	var exported []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					exported = append(exported, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							exported = append(exported, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								exported = append(exported, n.Name)
							}
						}
					}
				}
			}
		}
	}
	slices.Sort(exported)
	require.Equal(t, []string{"Across", "Default", "ErrTenantImmutable", "ErrTenantRequired", "From", "In", "Scope"}, exported)
}
