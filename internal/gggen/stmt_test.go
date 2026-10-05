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
	got, err := gggen.FormatNode(fn)
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
			got, err := gggen.FormatNode(res)
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
			got, err := gggen.FormatNode(res)
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
			structName: "sample.Group",
			want:       `model.Register[*sample.Group]()`,
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
			name:       "group",
			structName: "group.Updater",
			route:      "/api/groups/:id",
			phase:      consts.Update,
			want:       `service.Register[*group.Updater](consts.Update, "/api/groups/:id")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.StmtServiceRegister(tt.structName, tt.phase, tt.route)
			got, err := gggen.FormatNode(res)
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
		verb         string
		want         string
	}{
		{
			// An example of the StmtRouterRegister doc comment.
			name:         "model_as_payload_and_result",
			modelPkgName: "model",
			modelName:    "Group",
			reqName:      "*Group",
			rspName:      "*Group",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/group",
			verb:         "Create",
			want:         `router.Register[*model.Group, *model.Group, *model.Group](router.Auth(), "/api/group", &gst.ControllerConfig[*model.Group]{}, consts.Create)`,
		},
		{
			// Bare action type names (the declared form of slice and map
			// action types) are transcribed as value types.
			name:         "bare_names_transcribed",
			modelPkgName: "pkgmodel",
			modelName:    "Group",
			reqName:      "GroupRequest",
			rspName:      "GroupResponse",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/group2",
			verb:         "Update",
			want:         `router.Register[*pkgmodel.Group, pkgmodel.GroupRequest, pkgmodel.GroupResponse](router.Auth(), "/api/group2", &gst.ControllerConfig[*pkgmodel.Group]{}, consts.Update)`,
		},
		{
			name:         "starred_names_in_pub_group",
			modelPkgName: "pkgmodel",
			modelName:    "Group",
			reqName:      "*GroupRequest",
			rspName:      "*GroupResponse",
			gstModelPkg:  "model",
			routerGroup:  "Pub",
			route:        "/api/login",
			verb:         "Update",
			want:         `router.Register[*pkgmodel.Group, *pkgmodel.GroupRequest, *pkgmodel.GroupResponse](router.Pub(), "/api/login", &gst.ControllerConfig[*pkgmodel.Group]{}, consts.Update)`,
		},
		{
			name:         "list_with_empty_payload",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/groups",
			verb:         "List",
			want:         `router.Register[*group.Group, *model.Empty, *group.GroupListRsp](router.Auth(), "/api/groups", &gst.ControllerConfig[*group.Group]{}, consts.List)`,
		},
		{
			name:         "create_with_empty_result",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      "*GroupCreateReq",
			rspName:      dsl.PayloadEmpty,
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/groups",
			verb:         "Create",
			want:         `router.Register[*group.Group, *group.GroupCreateReq, *model.Empty](router.Auth(), "/api/groups", &gst.ControllerConfig[*group.Group]{}, consts.Create)`,
		},
		{
			// A project routing a root model package keeps the gstmodel
			// alias so the Empty qualifier cannot clash with the business
			// "model" import. An example of the StmtRouterRegister doc
			// comment.
			name:         "empty_payload_in_root_model_package_keeps_gstmodel_alias",
			modelPkgName: "model",
			modelName:    "Group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			gstModelPkg:  "gstmodel",
			routerGroup:  "Auth",
			route:        "/api/groups",
			verb:         "List",
			want:         `router.Register[*model.Group, *gstmodel.Empty, *model.GroupListRsp](router.Auth(), "/api/groups", &gst.ControllerConfig[*model.Group]{}, consts.List)`,
		},
		{
			// A route ending in a path parameter names it in the controller
			// config. An example of the StmtRouterRegister doc comment.
			name:         "route_param_named_in_controller_config",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      "*Group",
			rspName:      "*Group",
			gstModelPkg:  "model",
			routerGroup:  "Auth",
			route:        "/api/groups/:id",
			paramName:    "id",
			verb:         "Get",
			want:         `router.Register[*group.Group, *group.Group, *group.Group](router.Auth(), "/api/groups/:id", &gst.ControllerConfig[*group.Group]{ParamName: "id"}, consts.Get)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := gggen.StmtRouterRegister(tt.modelPkgName, tt.modelName, tt.reqName, tt.rspName, tt.gstModelPkg, tt.routerGroup, tt.route, tt.paramName, tt.verb)
			got, err := gggen.FormatNode(res)
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
