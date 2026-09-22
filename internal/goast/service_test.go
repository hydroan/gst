package goast_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/goast"
	"github.com/stretchr/testify/require"
)

// TestIsServiceBase pins the example in the IsServiceBase
// documentation, and the dot import it describes.
func TestIsServiceBase(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "aliased import",
			src: `package sample

import svc "github.com/hydroan/gst/service"

type Creator struct {
	svc.Base[*model.User, *model.UserReq, *model.UserRsp]
}
`,
			want: true,
		},
		{
			name: "dot import",
			src: `package sample

import . "github.com/hydroan/gst/service"

type Creator struct {
	Base[*model.User, *model.UserReq, *model.UserRsp]
}
`,
			want: true,
		},
		{
			name: "named field",
			src: `package sample

import "github.com/hydroan/gst/service"

type Creator struct {
	base service.Base[*model.User, *model.UserReq, *model.UserRsp]
}
`,
		},
		{
			name: "another package named service",
			src: `package sample

import "example.com/app/service"

type Creator struct {
	service.Base[*model.User, *model.UserReq, *model.UserRsp]
}
`,
		},
		{
			name: "two type arguments",
			src: `package sample

import "github.com/hydroan/gst/service"

type Creator struct {
	service.Base[*model.User, *model.UserReq]
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "sample.go", tt.src, 0)
			require.NoError(t, err)

			require.Equal(t, tt.want, goast.IsServiceBase(file, firstStructField(t, file)))
		})
	}
}

// firstStructField returns the first field of the first struct file declares.
func firstStructField(t *testing.T, file *ast.File) *ast.Field {
	t.Helper()

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				return structType.Fields.List[0]
			}
		}
	}
	t.Fatal("no struct field declared")
	return nil
}
