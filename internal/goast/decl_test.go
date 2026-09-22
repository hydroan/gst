package goast_test

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestFindStructTypeSpec pins the example in the FindStructTypeSpec
// documentation.
func TestFindStructTypeSpec(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", `package sample

type Creator struct{ Name string }

type Reader interface{ Read() }
`, 0)
	require.NoError(t, err)

	spec := goast.FindStructTypeSpec(file, "Creator")
	require.NotNil(t, spec)
	require.Equal(t, "Creator", spec.Name.Name)
	require.Nil(t, goast.FindStructTypeSpec(file, "Reader"))
	require.Nil(t, goast.FindStructTypeSpec(file, "Missing"))
}

// TestRenameIdent pins the example in the RenameIdent documentation.
func TestRenameIdent(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

func (u *Creator) Create() error { return u.check(req.u) }
`, 0)
	require.NoError(t, err)

	goast.RenameIdent(file.Decls[0], "u", "c")

	require.Equal(t, "func (c *Creator) Create() error { return c.check(req.c) }", printNode(t, fset, file.Decls[0]))
}

// TestRemoveStructInteriorComments pins the example in the
// RemoveStructInteriorComments documentation.
func TestRemoveStructInteriorComments(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

// Creator creates records.
type Creator struct {
	// Name is shown to users.
	Name string // required
}
`, parser.ParseComments)
	require.NoError(t, err)
	spec := goast.FindStructTypeSpec(file, "Creator")
	require.NotNil(t, spec)
	structType, ok := spec.Type.(*ast.StructType)
	require.True(t, ok)

	goast.RemoveStructInteriorComments(file, structType)

	require.Equal(t, `package sample

// Creator creates records.
type Creator struct {
	Name string
}
`, printNode(t, fset, file))
}

// printNode prints node the way gofmt formats it.
func printNode(t *testing.T, fset *token.FileSet, node any) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, format.Node(&buf, fset, node))
	return buf.String()
}
