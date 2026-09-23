package ggcheck

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestDatabaseChainMethodSetsMatchDatabaseInterfaces guards the hardcoded
// method sets against drift when methods are added to or removed from the
// gst.Database and gst.DatabaseOption interfaces. It sorts every method by
// what the method returns, not by which of the two interfaces declares it: a
// method handing a Database[M] back keeps the chain open wherever it is
// declared, and the check reads it as a chain method or lets an unterminated
// chain pass.
func TestDatabaseChainMethodSetsMatchDatabaseInterfaces(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "internal", "types", "database.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	chain := map[string]bool{}
	terminal := map[string]bool{}
	for _, interfaceName := range []string{"Database", "DatabaseOption"} {
		for name, keepsChainOpen := range interfaceMethods(t, file, interfaceName) {
			if keepsChainOpen {
				chain[name] = true
			} else {
				terminal[name] = true
			}
		}
	}

	assertMethodSetMatches(t, "chain", chain, databaseChainMethods)
	assertMethodSetMatches(t, "terminal", terminal, databaseTerminalMethods)
}

// assertMethodSetMatches reports every method the interfaces declare that the
// hardcoded set of the named kind lacks, and every entry of that set the
// interfaces no longer declare.
func assertMethodSetMatches(t *testing.T, kind string, declared, hardcoded map[string]bool) {
	t.Helper()

	for name := range declared {
		if !hardcoded[name] {
			t.Errorf("gst.Database method %q is a %s method the gg check method set is missing", name, kind)
		}
	}
	for name := range hardcoded {
		if !declared[name] {
			t.Errorf("gg check %s method set entry %q is no longer a %s method of gst.Database", kind, name, kind)
		}
	}
}

// interfaceMethods returns the methods the named interface declares, each
// mapped to whether it hands a Database[M] back, the result that keeps an
// operation chain open: WithQuery does, Create does not.
func interfaceMethods(t *testing.T, file *ast.File, interfaceName string) map[string]bool {
	t.Helper()

	methods := map[string]bool{}
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
			funcType, ok := method.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			// Fields without names are embedded interfaces, not methods.
			for _, name := range method.Names {
				methods[name.Name] = returnsDatabase(funcType)
			}
		}
		return false
	})
	if len(methods) == 0 {
		t.Fatalf("no methods found for interface %q in types/database.go", interfaceName)
	}
	return methods
}

// returnsDatabase reports whether the method's only result is a Database, in
// its generic form Database[M].
func returnsDatabase(funcType *ast.FuncType) bool {
	if funcType.Results == nil || len(funcType.Results.List) != 1 {
		return false
	}
	result := funcType.Results.List[0].Type
	index, ok := result.(*ast.IndexExpr)
	if !ok {
		return false
	}
	name, ok := index.X.(*ast.Ident)
	return ok && name.Name == "Database"
}
