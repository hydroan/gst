package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
)

func TestApplyServiceTypePointerConversion(t *testing.T) {
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
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "UserRsp",
			},
			want: "service.Base[*model.User, model.UserReq, model.UserRsp]",
		},
		{
			name: "convert_non_pointer_to_pointer",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, model.User, model.User]
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			want: "service.Base[*model.User, *model.UserReq, *model.UserRsp]",
		},
		{
			name: "keep_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			want: "service.Base[*model.User, *model.UserReq, *model.UserRsp]",
		},
		{
			name: "keep_non_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, model.User, model.User]
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "UserRsp",
			},
			want: "service.Base[*model.User, model.UserReq, model.UserRsp]",
		},
		{
			name: "mixed_conversion",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, model.User]
}`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "*UserRsp",
			},
			want: "service.Base[*model.User, model.UserReq, *model.UserRsp]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			// Find the type declaration and apply the changes
			var found bool
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if isServiceType(typeSpec) {
								found = true
								changed := applyServiceType(typeSpec, tt.action)
								if !changed {
									t.Errorf("applyServiceType returned false, expected true")
								}
							}
						}
					}
				}
			}
			if !found {
				t.Errorf("No service type found in the code")
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
