package gggen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/gghelper"
)

func TestApplyServiceMethod4PointerConversion(t *testing.T) {
	// Every case uses the helloworld/model business package.
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
	"github.com/hydroan/gst"
)

func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "UserReq",
				Result:  "UserRsp",
				Phase:   apiaction.Create,
			},
			want: "req model.UserReq",
		},
		{
			name: "convert_non_pointer_to_pointer",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst"
)

func (u *Creator) Create(ctx *gst.ServiceContext, req model.User) (rsp model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   apiaction.Create,
			},
			want: "req *model.UserReq",
		},
		{
			name: "keep_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst"
)

func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   apiaction.Create,
			},
			want: "req *model.UserReq",
		},
		{
			name: "keep_non_pointer_type",
			code: `package service

import (
	"helloworld/model"
	"github.com/hydroan/gst"
)

func (u *Creator) Create(ctx *gst.ServiceContext, req model.User) (rsp model.User, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "UserReq",
				Result:  "UserRsp",
				Phase:   apiaction.Create,
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
						applyServiceMethod4(funcDecl, tt.action, "model")
					}
				}
			}

			got, err := gghelper.FormatFile(file, nil, "helloworld")
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(got, tt.want) {
				t.Errorf("Expected to find %q in generated code, but got:\n%s", tt.want, got)
			}
		})
	}
}

func TestApplyServiceMethod4EmptyPayload(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		action   *dsl.Action
		modelPkg string
		want     string
	}{
		{
			name:     "switch_business_req_to_empty_payload",
			modelPkg: "box",
			code: `package box

import (
	"helloworld/model/box"
	"github.com/hydroan/gst"
)

func (b *Lister) List(ctx *gst.ServiceContext, req *box.BoxListReq) (rsp *box.BoxListRsp, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: dsl.PayloadEmpty,
				Result:  "*BoxListRsp",
				Phase:   apiaction.List,
			},
			want: "req *model.Empty",
		},
		{
			name:     "switch_empty_payload_back_to_model",
			modelPkg: "box",
			code: `package box

import (
	"helloworld/model/box"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
)

func (b *Lister) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *box.BoxListRsp, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "*Box",
				Result:  "*BoxListRsp",
				Phase:   apiaction.List,
			},
			want: "req *box.Box",
		},
		{
			name:     "switch_to_empty_payload_in_root_model_package",
			modelPkg: "model",
			code: `package user

import (
	"helloworld/model"
	"github.com/hydroan/gst"
)

func (u *Getter) Get(ctx *gst.ServiceContext, req *model.UserGetReq) (rsp *model.UserGetRsp, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: dsl.PayloadEmpty,
				Result:  "*UserGetRsp",
				Phase:   apiaction.Get,
			},
			want: "req *gstmodel.Empty",
		},
		{
			name:     "switch_business_rsp_to_empty_result",
			modelPkg: "box",
			code: `package box

import (
	"helloworld/model/box"
	"github.com/hydroan/gst"
)

func (b *Creator) Create(ctx *gst.ServiceContext, req *box.BoxCreateReq) (rsp *box.BoxCreateRsp, err error) {
	return rsp, nil
}`,
			action: &dsl.Action{
				Payload: "*BoxCreateReq",
				Result:  dsl.PayloadEmpty,
				Phase:   apiaction.Create,
			},
			want: "(rsp *model.Empty, err error)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			for _, decl := range file.Decls {
				if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl != nil {
					if isServiceMethod4(funcDecl) {
						applyServiceMethod4(funcDecl, tt.action, tt.modelPkg)
					}
				}
			}

			got, err := gghelper.FormatFile(file, nil, "helloworld")
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(got, tt.want) {
				t.Errorf("Expected to find %q in generated code, but got:\n%s", tt.want, got)
			}
		})
	}
}

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
				Payload: "UserReq",
				Result:  "*UserRsp",
			},
			want: "service.Base[*model.User, model.UserReq, *model.UserRsp]",
		},
		{
			name: "aliased_service_import",
			code: `package service

import (
	"helloworld/model"
	svc "github.com/hydroan/gst/service"
)

type user struct {
	svc.Base[*model.User, *model.User, *model.User]
}`,
			action: &dsl.Action{
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			want: "svc.Base[*model.User, *model.UserReq, *model.UserRsp]",
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
							if isServiceType(file, typeSpec) {
								found = true
								changed := applyServiceType(file, typeSpec, tt.action)
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

			got, err := gghelper.FormatFile(file, nil, "helloworld")
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(got, tt.want) {
				t.Errorf("Expected to find %q in generated code, but got:\n%s", tt.want, got)
			}
		})
	}
}

// TestServiceModelPackageName reads the model package of a service struct
// off the first type argument of its service base, under whatever name the
// file imports the framework's service package by.
func TestServiceModelPackageName(t *testing.T) {
	for name, src := range map[string]string{
		"plain import": `package sample

import "github.com/hydroan/gst/service"

type Creator struct {
	service.Base[*sample.User, *sample.UserReq, *sample.UserRsp]
}`,
		"aliased import": `package sample

import svc "github.com/hydroan/gst/service"

type Creator struct {
	svc.Base[*sample.User, *sample.UserReq, *sample.UserRsp]
}`,
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := serviceModelPackageName(file); got != "sample" {
				t.Errorf("serviceModelPackageName() = %q, want sample", got)
			}
		})
	}
}
