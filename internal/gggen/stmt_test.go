package gggen_test

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/gggen"
	"github.com/hydroan/gst/internal/gghelper"
)

func TestStmtLogInfo(t *testing.T) {
	fset := token.NewFileSet()
	var buf bytes.Buffer

	tests := []struct {
		name string
		str  string
		want string
	}{
		{
			// The example of the StmtLogInfo doc comment.
			name: "item_create",
			str:  `"item create"`,
			want: `log.Info("item create")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			res := gggen.StmtLogInfo(tt.str)
			if err := format.Node(&buf, fset, res); err != nil {
				t.Error(err)
				return
			}
			got := buf.String()
			if got != tt.want {
				t.Errorf("StmtLogInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEmptyLine prints the body of the example of the EmptyLine doc comment,
// which leaves an empty line before the return statement.
func TestEmptyLine(t *testing.T) {
	fn := &ast.FuncDecl{
		Name: ast.NewIdent("f"),
		Type: &ast.FuncType{},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			gggen.StmtLogInfo(`"item create"`),
			gggen.EmptyLine(),
			gggen.Returns(ast.NewIdent("nil")),
		}},
	}
	got, err := gghelper.FormatNode(fn)
	if err != nil {
		t.Fatalf("FormatNode() error = %v", err)
	}
	want := "func f() {\n\tlog.Info(\"item create\")\n\n\treturn nil\n}"
	if got != want {
		t.Errorf("FormatNode() = %q, want %q", got, want)
	}
}

func TestReturns(t *testing.T) {
	tests := []struct {
		name  string
		exprs []ast.Expr
		want  string
	}{
		{
			name:  "return_error",
			exprs: []ast.Expr{ast.NewIdent("error")},
			want:  `return error`,
		},
		{
			name:  "return_nil",
			exprs: []ast.Expr{ast.NewIdent("nil")},
			want:  `return nil`,
		},
		{
			// The example of the Returns doc comment.
			name:  "return_rsp_and_nil",
			exprs: []ast.Expr{ast.NewIdent("rsp"), ast.NewIdent("nil")},
			want:  `return rsp, nil`,
		},
		{
			name: "return_multiple_values",
			exprs: []ast.Expr{
				&ast.UnaryExpr{
					Op: token.AND,
					X: &ast.CompositeLit{
						Type: &ast.SelectorExpr{
							X:   ast.NewIdent("model"),
							Sel: ast.NewIdent("User"),
						},
					},
				},
				ast.NewIdent("nil"),
			},
			want: "return &model.User{}, nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.Returns(tt.exprs...)
			got, err := gghelper.FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("Returns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStmtLogWithContext(t *testing.T) {
	tests := []struct {
		name         string
		modelVarName string
		want         string
	}{
		{
			name:         "u",
			modelVarName: `u`,
			want:         `log := u.WithContext(ctx, ctx.Phase())`,
		},
		{
			name:         "g",
			modelVarName: `g`,
			want:         `log := g.WithContext(ctx, ctx.Phase())`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.StmtLogWithContext(tt.modelVarName)
			got, err := gghelper.FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("StmtLogWithContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStmtModelRegister(t *testing.T) {
	tests := []struct {
		name       string
		structName string
		want       string
	}{
		// The examples of the StmtModelRegister doc comment.
		{
			name:       "User",
			structName: "User",
			want:       `model.Register[*User]()`,
		},
		{
			name:       "model_of_another_package",
			structName: "sample.Record",
			want:       `model.Register[*sample.Record]()`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gggen.StmtModelRegister(tt.structName)
			var buf bytes.Buffer
			fset := token.NewFileSet()
			if err := format.Node(&buf, fset, got); err != nil {
				t.Error(err)
				return
			}
			if buf.String() != tt.want {
				t.Errorf("StmtModelRegister() = %v, want %v", buf.String(), tt.want)
			}
		})
	}
}

func TestStmtServiceRegister(t *testing.T) {
	tests := []struct {
		name       string
		structName string
		route      string
		phase      consts.Phase
		want       string
	}{
		{
			// The example of the StmtServiceRegister doc comment.
			name:       "user",
			structName: "user.Creator",
			route:      "/api/users",
			phase:      consts.Create,
			want:       `service.Register[*user.Creator](consts.Create, "/api/users")`,
		},
		{
			name:       "box",
			structName: "box.Updater",
			route:      "/api/boxes/:id",
			phase:      consts.Update,
			want:       `service.Register[*box.Updater](consts.Update, "/api/boxes/:id")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.StmtServiceRegister(tt.structName, tt.phase, tt.route)
			got, err := gghelper.FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("StmtServiceRegister() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStmtRouterRegister(t *testing.T) {
	tests := []struct {
		name         string
		modelPkgName string
		modelName    string
		reqName      string
		rspName      string
		gstModelPkg  string
		routerGroup  string
		route        string
		paramName    string
		phase        string
		want         string
	}{
		{
			// An example of the StmtRouterRegister doc comment.
			name:         "model_as_payload_and_result",
			modelPkgName: "model",
			modelName:    "Record",
			reqName:      "*Record",
			rspName:      "*Record",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/records",
			phase:        "Create",
			want:         `router.Register[*model.Record, *model.Record, *model.Record](router.Auth(), "/api/records", &gst.ControllerConfig[*model.Record]{}, consts.Create)`,
		},
		{
			// Bare action type names (the declared form of slice and map
			// action types) are transcribed as value types.
			name:         "bare_names_transcribed",
			modelPkgName: "pkgmodel",
			modelName:    "Record",
			reqName:      "BoxRequest",
			rspName:      "BoxResponse",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/box2",
			phase:        "Update",
			want:         `router.Register[*pkgmodel.Record, pkgmodel.BoxRequest, pkgmodel.BoxResponse](router.Auth(), "/api/box2", &gst.ControllerConfig[*pkgmodel.Record]{}, consts.Update)`,
		},
		{
			name:         "starred_names_in_pub_group",
			modelPkgName: "pkgmodel",
			modelName:    "Record",
			reqName:      "*BoxRequest",
			rspName:      "*BoxResponse",
			gstModelPkg:  "model",
			routerGroup:  "Pub",
			route:        "/api/login",
			phase:        "Update",
			want:         `router.Register[*pkgmodel.Record, *pkgmodel.BoxRequest, *pkgmodel.BoxResponse](router.Pub(), "/api/login", &gst.ControllerConfig[*pkgmodel.Record]{}, consts.Update)`,
		},
		{
			name:         "list_with_empty_payload",
			modelPkgName: "box",
			modelName:    "Record",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*BoxListRsp",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/boxes",
			phase:        "List",
			want:         `router.Register[*box.Record, *model.Empty, *box.BoxListRsp](router.Auth(), "/api/boxes", &gst.ControllerConfig[*box.Record]{}, consts.List)`,
		},
		{
			name:         "create_with_empty_result",
			modelPkgName: "box",
			modelName:    "Record",
			reqName:      "*BoxCreateReq",
			rspName:      dsl.PayloadEmpty,
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/boxes",
			phase:        "Create",
			want:         `router.Register[*box.Record, *box.BoxCreateReq, *model.Empty](router.Auth(), "/api/boxes", &gst.ControllerConfig[*box.Record]{}, consts.Create)`,
		},
		{
			// A project routing a root model package keeps the gstmodel
			// alias so the Empty qualifier cannot clash with the business
			// "model" import. An example of the StmtRouterRegister doc
			// comment.
			name:         "empty_payload_in_root_model_package_keeps_gstmodel_alias",
			modelPkgName: "model",
			modelName:    "Record",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*BoxListRsp",
			gstModelPkg:  "gstmodel",
			routerGroup:  "Auth",
			route:        "/api/boxes",
			phase:        "List",
			want:         `router.Register[*model.Record, *gstmodel.Empty, *model.BoxListRsp](router.Auth(), "/api/boxes", &gst.ControllerConfig[*model.Record]{}, consts.List)`,
		},
		{
			// A route ending in a path parameter names it in the controller
			// config. An example of the StmtRouterRegister doc comment.
			name:         "route_param_named_in_controller_config",
			modelPkgName: "box",
			modelName:    "Record",
			reqName:      "*Record",
			rspName:      "*Record",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/boxes/:id",
			paramName:    "id",
			phase:        "Get",
			want:         `router.Register[*box.Record, *box.Record, *box.Record](router.Auth(), "/api/boxes/:id", &gst.ControllerConfig[*box.Record]{ParamName: "id"}, consts.Get)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.StmtRouterRegister(tt.modelPkgName, tt.modelName, tt.reqName, tt.rspName, tt.gstModelPkg, tt.routerGroup, tt.route, tt.paramName, tt.phase)
			got, err := gghelper.FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("StmtRouterRegister() = %v, want %v", got, tt.want)
			}
		})
	}
}
