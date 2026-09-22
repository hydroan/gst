package ts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsesBitwiseOperatorFindsCombinedConstants(t *testing.T) {
	tests := map[string]bool{
		"1 << iota":      true,
		"read | write":   true,
		"all &^ write":   true,
		"mask & read":    true,
		"iota + 1":       false,
		`"active"`:       false,
		"parse(base, 1)": false,
		"first * 10":     false,
	}
	for source, want := range tests {
		expr, err := parser.ParseExpr(source)
		require.NoError(t, err, source)
		require.Equal(t, want, usesBitwiseOperator(expr), source)
	}
}

func TestCommentTextPrefersTheDocComment(t *testing.T) {
	doc := &ast.CommentGroup{List: []*ast.Comment{{Text: "// Title is the display title."}}}
	trailing := &ast.CommentGroup{List: []*ast.Comment{{Text: "// the trailing one"}}}

	require.Equal(t, "Title is the display title.", commentText(doc, trailing))
	require.Equal(t, "the trailing one", commentText(nil, trailing))
	require.Equal(t, "the trailing one", commentText(&ast.CommentGroup{}, trailing))
	require.Empty(t, commentText(nil, nil))
}

func TestComparePositionsOrdersByFileThenOffset(t *testing.T) {
	fset := token.NewFileSet()
	first := fset.AddFile("a.go", -1, 100)
	second := fset.AddFile("b.go", -1, 100)

	require.Negative(t, comparePositions(fset, first.Pos(10), first.Pos(20)))
	require.Positive(t, comparePositions(fset, first.Pos(20), first.Pos(10)))
	require.Negative(t, comparePositions(fset, first.Pos(90), second.Pos(0)))
	require.Zero(t, comparePositions(fset, first.Pos(5), first.Pos(5)))
}
