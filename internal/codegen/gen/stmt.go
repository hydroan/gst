package gen

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/hydroan/gst/consts"
)

// StmtLogInfo builds the statement that logs str, a quoted Go string
// literal: for str `"item create"` it builds
//
//	log.Info("item create")
func StmtLogInfo(str string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			// log.Info
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("log"),
				Sel: ast.NewIdent("Info"),
			},
			// str
			Args: []ast.Expr{
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: str,
				},
			},
		},
	}
}

// EmptyLine builds an empty statement, which the generated service methods
// place before their return statement. It prints as nothing, so no blank line
// shows up there: a body built from StmtLogInfo(`"item create"`), EmptyLine()
// and Returns(ast.NewIdent("nil")) prints as
//
//	log.Info("item create")
//	return nil
func EmptyLine() *ast.EmptyStmt {
	return &ast.EmptyStmt{}
}

// Returns builds the return statement of exprs: for ast.NewIdent("rsp") and
// ast.NewIdent("nil") it builds
//
//	return rsp, nil
func Returns(exprs ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{
		Results: exprs,
	}
}

// StmtLogWithContext builds the statement that opens a generated service
// method, taking the logger of the phase from the receiver named
// modelVarName: for u it builds
//
//	log := u.WithContext(ctx, ctx.Phase())
func StmtLogWithContext(modelVarName string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent("log"),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(modelVarName),
					Sel: ast.NewIdent("WithContext"),
				},
				Args: []ast.Expr{
					ast.NewIdent("ctx"),
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("ctx"),
							Sel: ast.NewIdent("Phase"),
						},
					},
				},
			},
		},
	}
}

// StmtModelRegister builds the registration of a model in model.gen.go.
// modelName is the model type as the file refers to it, qualified when the
// model is declared outside the root model package: for User and
// sample.Group it builds
//
//	model.Register[*User]()
//	model.Register[*sample.Group]()
func StmtModelRegister(modelName string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("model"),
					Sel: ast.NewIdent("Register"),
				},
				Index: &ast.StarExpr{
					X: ast.NewIdent(modelName),
				},
			},
		},
	}
}

// StmtServiceRegister builds the registration of a service in
// service.gen.go. serviceImport is the service type as the file refers to
// it: for user.Creator, consts.PHASE_CREATE and the route users it builds
//
//	service.Register[*user.Creator](consts.PHASE_CREATE, "users")
//
// The route argument must be the same raw route string the matching
// StmtRouterRegister statement carries, because the service registry keys
// services by route and phase.
func StmtServiceRegister(serviceImport string, phase consts.Phase, route string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("service"),
					Sel: ast.NewIdent("Register"),
				},
				Index: &ast.StarExpr{
					X: ast.NewIdent(serviceImport),
				},
			},
			Args: []ast.Expr{
				&ast.SelectorExpr{
					X:   ast.NewIdent("consts"),
					Sel: ast.NewIdent(phase.Name()),
				},
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", route),
				},
			},
		},
	}
}

// StmtRouterRegister builds the registration of an action in router.gen.go:
// the model type, then the action's request and result types, reqName and
// rspName qualified by modelPkgName, the qualifier the file refers to the
// model package by. For example, it builds
//
//	router.Register[*model.Group, *model.Group, *model.Group](router.Auth(), "group", &gst.ControllerConfig[*model.Group]{}, consts.Create)
//	router.Register[*model.Group, *gstmodel.Empty, *model.GroupListRsp](router.Auth(), "groups", &gst.ControllerConfig[*model.Group]{}, consts.List)
//	router.Register[*group.Group, *group.Group, *group.Group](router.Auth(), "groups/:id", &gst.ControllerConfig[*group.Group]{ParamName: "id"}, consts.Get)
//
// A dsl.PayloadEmpty side becomes *model.Empty under gstModelPkg, the
// qualifier the router file uses for model.Empty, resolved once per file by
// RouterGstModelUse. routerGroup names the router group accessor ("Auth" or
// "Pub"), route is the raw route string, shared verbatim with the matching
// StmtServiceRegister statement, paramName is the route parameter the
// controller reads the resource id from, "" for none, and verb names the
// consts value of the action, such as Create.
func StmtRouterRegister(modelPkgName, modelName, reqName, rspName, gstModelPkg string, routerGroup string, route string, paramName string, verb string) *ast.ExprStmt {
	// The dsl.PayloadEmpty sentinel on either side resolves to
	// *<gstModelPkg>.Empty. gstModelPkg is the file-level qualifier decided
	// once per router file by RouterGstModelUse: plain "model" by default,
	// the gstmodel alias when a routed business model package is itself
	// named "model". Any other action type is emitted in its declared
	// form.
	var reqExpr ast.Expr
	if isEmptyPayload(reqName) {
		reqExpr = emptyReqExpr(gstModelPkg)
	} else {
		reqExpr = actionTypeExpr(modelPkgName, reqName)
	}
	var rspExpr ast.Expr
	if isEmptyPayload(rspName) {
		rspExpr = emptyReqExpr(gstModelPkg)
	} else {
		rspExpr = actionTypeExpr(modelPkgName, rspName)
	}

	var paramExpr ast.Expr
	// expr like: &gst.ControllerConfig[*sample.Record]{}
	paramExpr = &ast.UnaryExpr{
		Op: token.AND,
		X: &ast.CompositeLit{
			Type: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("gst"),
					Sel: ast.NewIdent("ControllerConfig"),
				},
				Index: &ast.StarExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent(modelPkgName),
						Sel: ast.NewIdent(modelName),
					},
				},
			},
			Elts: []ast.Expr{},
		},
	}
	// expr like: &gst.ControllerConfig[*sample.Record]{ParamName: "id"}
	if len(paramName) > 0 {
		paramExpr = &ast.UnaryExpr{
			Op: token.AND,
			X: &ast.CompositeLit{
				Type: &ast.IndexExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent("gst"),
						Sel: ast.NewIdent("ControllerConfig"),
					},
					Index: &ast.StarExpr{
						X: &ast.SelectorExpr{
							X:   ast.NewIdent(modelPkgName),
							Sel: ast.NewIdent(modelName),
						},
					},
				},
				Elts: []ast.Expr{
					&ast.KeyValueExpr{
						Key: ast.NewIdent("ParamName"),
						Value: &ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf("%q", paramName),
						},
					},
				},
			},
		}
	}

	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexListExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("router"),
					Sel: ast.NewIdent("Register"),
				},
				Indices: []ast.Expr{
					&ast.StarExpr{
						X: &ast.SelectorExpr{
							X:   ast.NewIdent(modelPkgName),
							Sel: ast.NewIdent(modelName),
						},
					},
					reqExpr,
					rspExpr,
				},
			},
			Args: []ast.Expr{
				&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("router"),
						Sel: ast.NewIdent(routerGroup),
					},
				},
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", route),
				},
				paramExpr,
				&ast.SelectorExpr{
					X:   ast.NewIdent("consts"),
					Sel: ast.NewIdent(verb),
				},
			},
		},
	}
}
