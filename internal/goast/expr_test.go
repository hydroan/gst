package goast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestIsQualified pins the examples in the IsQualified documentation.
func TestIsQualified(t *testing.T) {
	require.True(t, goast.IsQualified(typeExpr(t, "model.User")))
	for _, src := range []string{"*model.User", "User", "a.b.User"} {
		require.False(t, goast.IsQualified(typeExpr(t, src)), src)
	}
}

// TestIsPointerToQualified pins the examples in the IsPointerToQualified
// documentation.
func TestIsPointerToQualified(t *testing.T) {
	require.True(t, goast.IsPointerToQualified(typeExpr(t, "*model.User")))
	for _, src := range []string{"model.User", "**model.User", "*User"} {
		require.False(t, goast.IsPointerToQualified(typeExpr(t, src)), src)
	}
}

// TestTypeQualifier pins the examples in the TypeQualifier documentation.
func TestTypeQualifier(t *testing.T) {
	for _, src := range []string{"sample.User", "*sample.User", "**sample.User"} {
		require.Equal(t, "sample", goast.TypeQualifier(typeExpr(t, src)), src)
	}
	for _, src := range []string{"User", "[]sample.User"} {
		require.Empty(t, goast.TypeQualifier(typeExpr(t, src)), src)
	}
}

// TestIsPointerReceiver pins the examples in the IsPointerReceiver
// documentation.
func TestIsPointerReceiver(t *testing.T) {
	require.True(t, goast.IsPointerReceiver(receiverOf(t, "func (u *Creator) Create() {}")))
	require.False(t, goast.IsPointerReceiver(receiverOf(t, "func (u Creator) Create() {}")))
	require.False(t, goast.IsPointerReceiver(receiverOf(t, "func (u *Creator[T]) Create() {}")))
	require.False(t, goast.IsPointerReceiver(receiverOf(t, "func Create() {}")))
}

// TestIsBuiltinError pins the examples in the IsBuiltinError documentation.
func TestIsBuiltinError(t *testing.T) {
	require.True(t, goast.IsBuiltinError(typeExpr(t, "error")))
	for _, src := range []string{"pkg.Error", "failure"} {
		require.False(t, goast.IsBuiltinError(typeExpr(t, src)), src)
	}
}

// receiverOf returns the receiver list of the one function src declares.
func receiverOf(t *testing.T, src string) *ast.FieldList {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", "package sample\n\n"+src+"\n", 0)
	require.NoError(t, err)
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	return fn.Recv
}
