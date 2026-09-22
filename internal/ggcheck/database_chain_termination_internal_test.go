package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"testing"
)

// TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards the hardcoded method
// sets against drift when methods are added to or removed from the
// gst.Database and gst.DatabaseOption interfaces.
func TestDatabaseChainMethodSetsMatchDatabaseInterfaces(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "..", "internal", "types", "database.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	assertMethodSetMatches(t, "Database", interfaceMethodNames(t, file, "Database"), databaseTerminalMethods)
	assertMethodSetMatches(t, "DatabaseOption", interfaceMethodNames(t, file, "DatabaseOption"), databaseChainMethods)
}

func assertMethodSetMatches(t *testing.T, interfaceName string, declared []string, hardcoded map[string]bool) {
	t.Helper()

	for _, name := range declared {
		if !hardcoded[name] {
			t.Errorf("gst.%s method %q is missing from the gg check method set", interfaceName, name)
		}
	}
	for name := range hardcoded {
		if !slices.Contains(declared, name) {
			t.Errorf("gg check method set entry %q no longer exists on gst.%s", name, interfaceName)
		}
	}
}

func interfaceMethodNames(t *testing.T, file *ast.File, interfaceName string) []string {
	t.Helper()

	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || typeSpec.Name.Name != interfaceName {
			return true
		}
		interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok || interfaceType.Methods == nil {
			return true
		}
		for _, method := range interfaceType.Methods.List {
			// Fields without names are embedded interfaces, not methods.
			for _, name := range method.Names {
				names = append(names, name.Name)
			}
		}
		return false
	})
	if len(names) == 0 {
		t.Fatalf("no methods found for interface %q in types/database.go", interfaceName)
	}
	return names
}
