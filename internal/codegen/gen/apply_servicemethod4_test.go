package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
)

func TestApplyServiceMethod4PointerConversion(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		action *dsl.Action
		want   string
	}{
		{
			name: "convert_pointer_to_non_pointer",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/types"
)

func (u *Creator) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "UserRsp",
			},
			want: "req model.UserReq",
		},
		{
			name: "convert_non_pointer_to_pointer",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/types"
)

func (u *Creator) Create(ctx *types.ServiceContext, req model.User) (rsp model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			want: "req *model.UserReq",
		},
		{
			name: "keep_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/types"
)

func (u *Creator) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			want: "req *model.UserReq",
		},
		{
			name: "keep_non_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/types"
)

func (u *Creator) Create(ctx *types.ServiceContext, req model.User) (rsp model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "UserRsp",
			},
			want: "req model.UserReq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			// Find the function declaration and apply the changes
			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl != nil {
					if isServiceMethod4(funcDecl) {
						applyServiceMethod4(funcDecl, tt.action)
					}
				}
			}

			got, err := FormatNodeExtra(file)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(got, tt.want) {
				t.Errorf("Expected to find %q in generated code, but got:\n%s", tt.want, got)
			}
		})
	}
}
