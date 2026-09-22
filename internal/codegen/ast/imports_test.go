package codegenast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	codegenast "github.com/hydroan/gst/internal/codegen/ast"
	"github.com/stretchr/testify/require"
)

// TestImportedNames pins the example in the ImportedNames documentation.
func TestImportedNames(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want codegenast.PackageNames
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
			want: codegenast.PackageNames{Qualifiers: []string{"model", "gstmodel"}, DotImported: true},
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
			want: codegenast.PackageNames{Qualifiers: []string{"m"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "sample.go", tt.src, parser.ImportsOnly)
			require.NoError(t, err)
			require.Equal(t, tt.want, codegenast.ImportedNames(file, "github.com/hydroan/gst/model", "model"))
		})
	}
}

// TestPackageNamesRefers pins the example in the Refers documentation.
func TestPackageNamesRefers(t *testing.T) {
	qualified := codegenast.PackageNames{Qualifiers: []string{"gstmodel"}}
	dotted := codegenast.PackageNames{DotImported: true}

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
