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

// TestRenameIdent pins the example in the RenameIdent documentation: the
// receiver's uses are renamed, the field that shares its name is not.
func TestRenameIdent(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

func (u *Creator) Create() error { return u.check(req.u) }
`, 0)
	require.NoError(t, err)
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)

	require.NoError(t, goast.RenameIdent(fn, fn.Recv.List[0].Names[0], "c"))

	require.Equal(t, "func (c *Creator) Create() error { return c.check(req.u) }", printNode(t, fset, fn))
}

// TestRenameIdentRenamesTheObjectAlone pins the two sides of renaming by
// object: a local shadowing the receiver keeps its name and its uses, and a
// new name the body already uses for something else refuses the rename,
// which would make one of the two refer to the other.
func TestRenameIdentRenamesTheObjectAlone(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

func (u *Creator) Create() error {
	if u := u.other(); u != nil {
		return u
	}
	return u.check()
}

func (s *Creator) Note(names []string) {
	for _, a := range names {
		s.note(a)
	}
}
`, 0)
	require.NoError(t, err)
	shadowing, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)
	clashing, ok := file.Decls[1].(*ast.FuncDecl)
	require.True(t, ok)

	require.NoError(t, goast.RenameIdent(shadowing, shadowing.Recv.List[0].Names[0], "c"))
	require.Equal(t, "func (c *Creator) Create() error {\n\tif u := c.other(); u != nil {\n\t\treturn u\n\t}\n\treturn c.check()\n}", printNode(t, fset, shadowing))

	err = goast.RenameIdent(clashing, clashing.Recv.List[0].Names[0], "a")
	require.ErrorContains(t, err, "a")
	require.Equal(t, "s", clashing.Recv.List[0].Names[0].Name, "a refused rename changes nothing")
}

// TestRenameIdentTellsFieldKeysFromMapKeys pins the keys of composite
// literals: the field key of a struct literal keeps its name even when it
// spells the receiver's, the key of a map literal that is the receiver is
// renamed, a literal of an elided type is taken for a struct, and a key that
// is no single identifier is an expression whatever the literal's type.
func TestRenameIdentTellsFieldKeysFromMapKeys(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

func (s *Archive) Create() error {
	opts := signOptions{s: true}
	seen := map[*Archive]int{s: 1}
	nested := []signOptions{{s: false}}
	tagged := labels{s.name: 1}
	return s.sign(opts, seen, nested, tagged)
}
`, 0)
	require.NoError(t, err)
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)

	require.NoError(t, goast.RenameIdent(fn, fn.Recv.List[0].Names[0], "a"))

	require.Equal(t, "func (a *Archive) Create() error {\n\topts := signOptions{s: true}\n\tseen := map[*Archive]int{a: 1}\n\tnested := []signOptions{{s: false}}\n\ttagged := labels{a.name: 1}\n\treturn a.sign(opts, seen, nested, tagged)\n}", printNode(t, fset, fn))
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
