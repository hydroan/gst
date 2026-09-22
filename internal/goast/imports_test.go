package goast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestImportedNames pins the example in the ImportedNames documentation.
func TestImportedNames(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want goast.PackageNames
	}{
		{
			name: "every import form",
			src: `package sample

import (
	"github.com/hydroan/gst/model"
	gstmodel "github.com/hydroan/gst/model"
	. "github.com/hydroan/gst/model"
	_ "github.com/hydroan/gst/model"
)
`,
			want: goast.PackageNames{Qualifiers: []string{"model", "gstmodel"}, DotImported: true},
		},
		{
			name: "not imported",
			src: `package sample

import "example.com/app/model"
`,
		},
		{
			// A raw string literal is as valid an import path as a quoted one.
			name: "raw string path",
			src:  "package sample\n\nimport m `github.com/hydroan/gst/model`\n",
			want: goast.PackageNames{Qualifiers: []string{"m"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "sample.go", tt.src, parser.ImportsOnly)
			require.NoError(t, err)
			require.Equal(t, tt.want, goast.ImportedNames(file, "github.com/hydroan/gst/model", "model"))
		})
	}
}

// TestPackageNamesRefers pins the example in the Refers documentation.
func TestPackageNamesRefers(t *testing.T) {
	qualified := goast.PackageNames{Qualifiers: []string{"gstmodel"}}
	dotted := goast.PackageNames{DotImported: true}

	require.True(t, qualified.Refers(typeExpr(t, "gstmodel.Base"), "Base"))
	require.False(t, qualified.Refers(typeExpr(t, "Base"), "Base"))
	require.False(t, qualified.Refers(typeExpr(t, "model.Base"), "Base"))
	require.False(t, qualified.Refers(typeExpr(t, "gstmodel.Empty"), "Base", "AutoBase"))
	require.True(t, dotted.Refers(typeExpr(t, "Base"), "Base"))
	require.False(t, dotted.Refers(typeExpr(t, "*Base"), "Base"))
}

// typeExpr parses src as a type expression.
func typeExpr(t *testing.T, src string) ast.Expr {
	t.Helper()

	expr, err := parser.ParseExpr(src)
	require.NoError(t, err)
	return expr
}

// TestFindImportSpec pins the example in the FindImportSpec documentation.
func TestFindImportSpec(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", `package sample

import gstmodel "github.com/hydroan/gst/model"
`, parser.ImportsOnly)
	require.NoError(t, err)

	spec := goast.FindImportSpec(file, "github.com/hydroan/gst/model")
	require.NotNil(t, spec)
	require.Equal(t, "gstmodel", spec.Name.Name)
	require.Nil(t, goast.FindImportSpec(file, "fmt"))
}

// TestInsertImportSpec pins the examples in the InsertImportSpec
// documentation.
func TestInsertImportSpec(t *testing.T) {
	fmtSpec := func() *ast.ImportSpec {
		return &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote("fmt")}}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", `package sample

import (
	"context"
)

var _ context.Context
`, 0)
	require.NoError(t, err)
	goast.InsertImportSpec(file, fmtSpec())
	require.Equal(t, `package sample

import (
	"context"
	"fmt"
)

var _ context.Context
`, printNode(t, fset, file))

	fset = token.NewFileSet()
	file, err = parser.ParseFile(fset, "sample.go", `package sample

var _ = 1
`, 0)
	require.NoError(t, err)
	goast.InsertImportSpec(file, fmtSpec())
	require.Equal(t, `package sample

import "fmt"

var _ = 1
`, printNode(t, fset, file))
}
