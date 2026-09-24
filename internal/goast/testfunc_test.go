package goast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestIsTestCaseFunc pins the examples in the IsTestCaseFunc documentation.
func TestIsTestCaseFunc(t *testing.T) {
	require.True(t, goast.IsTestCaseFunc(funcDeclOf(t, "func TestCreate(t *testing.T) {}")))
	for _, src := range []string{
		"func TestMain(m *testing.M) {}",
		"func Testcreate(t *testing.T) {}",
		"func TestCreate(t *testing.T, n int) {}",
		"func TestCreate(t *testing.T) error { return nil }",
		"func TestCreate(b *testing.B) {}",
		"func Create(t *testing.T) {}",
	} {
		require.False(t, goast.IsTestCaseFunc(funcDeclOf(t, src)), src)
	}
}

// TestIsTestMainFunc pins the examples in the IsTestMainFunc documentation.
func TestIsTestMainFunc(t *testing.T) {
	require.True(t, goast.IsTestMainFunc(funcDeclOf(t, "func TestMain(m *testing.M) {}")))
	for _, src := range []string{
		"func TestMain(t *testing.T) {}",
		"func TestMain(m *testing.M) int { return 0 }",
		"func TestMain() {}",
	} {
		require.False(t, goast.IsTestMainFunc(funcDeclOf(t, src)), src)
	}
}

// funcDeclOf returns the one function src declares.
func funcDeclOf(t *testing.T, src string) *ast.FuncDecl {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", "package sample\n\n"+src+"\n", 0)
	require.NoError(t, err)
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	return fn
}
