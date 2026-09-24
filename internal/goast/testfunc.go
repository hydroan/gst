package goast

import (
	"go/ast"
	"strings"
)

// IsTestCaseFunc reports whether fn is a test function the go test runner
// picks up: TestXxx taking exactly one *testing.T parameter and returning
// nothing, where Xxx does not start with a lowercase letter. True for
//
//	func TestCreate(t *testing.T) {}
//
// and false for TestMain, for Testcreate, whose name continues in lowercase,
// and for func TestCreate(t *testing.T, n int).
func IsTestCaseFunc(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if name == "TestMain" || !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) > len("Test") {
		if next := name[len("Test")]; next >= 'a' && next <= 'z' {
			return false
		}
	}
	return hasSingleTestingParam(fn, "T")
}

// IsTestMainFunc reports whether fn is TestMain(m *testing.M): true for
//
//	func TestMain(m *testing.M) {}
//
// and false for func TestMain(t *testing.T).
func IsTestMainFunc(fn *ast.FuncDecl) bool {
	return fn.Name.Name == "TestMain" && hasSingleTestingParam(fn, "M")
}

// hasSingleTestingParam reports whether fn takes exactly one *testing.<sel>
// parameter and returns nothing.
func hasSingleTestingParam(fn *ast.FuncDecl, sel string) bool {
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		return false
	}
	param := fn.Type.Params.List[0]
	if len(param.Names) > 1 {
		return false
	}
	star, ok := param.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != sel {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "testing"
}
