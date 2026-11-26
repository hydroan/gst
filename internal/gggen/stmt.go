package gggen

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/ggconst"
)

// stmtLogInfo builds the statement that logs str, a quoted Go string
// literal: for str `"item create"` it builds
//
//	log.Info("item create")
func stmtLogInfo(str string) *ast.ExprStmt {
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

// emptyLine builds a statement that prints as an empty line, which the
// generated service methods place before their return statement: a body
// built from stmtLogInfo(`"item create"`), emptyLine() and
// returns(ast.NewIdent("nil")) prints as
//
//	log.Info("item create")
//
//	return nil
//
// It is an expression statement of an empty identifier: go/printer drops an
// *ast.EmptyStmt from a statement list without leaving a line for it.
func emptyLine() ast.Stmt {
	return &ast.ExprStmt{X: ast.NewIdent("")}
}

// returns builds the return statement of exprs: for ast.NewIdent("rsp") and
// ast.NewIdent("nil") it builds
//
//	return rsp, nil
func returns(exprs ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{
		Results: exprs,
	}
}

// stmtLogWithContext builds the statement that opens a generated service
// method, taking the logger of the phase from the receiver named
// modelVarName: for u it builds
//
//	log := u.WithContext(ctx, ctx.Phase())
func stmtLogWithContext(modelVarName string) *ast.AssignStmt {
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

// stmtModelRegister builds the registration of a model in model.gen.go.
// modelName is the model type as the file refers to it, qualified when the
// model is declared outside the root model package: for User and
// sample.Record it builds
//
//	model.Register[*User]()
//	model.Register[*sample.Record]()
func stmtModelRegister(modelName string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgModel),
					Sel: ast.NewIdent(ggconst.FuncRegister),
				},
				Index: &ast.StarExpr{
					X: ast.NewIdent(modelName),
				},
			},
		},
	}
}

// stmtServiceRegister builds the registration of a service in service.gen.go.
// serviceImport is the service type as the file refers to it: for user.Creator,
// apiaction.Create and /api/users it builds
//
//	service.Register[*user.Creator](service.Create, "/api/users")
//
// The route argument is the path the route is served at, the same string
// the matching stmtRouterRegister statement carries, because the service
// registry keys services by path and phase.
func stmtServiceRegister(serviceImport string, phase apiaction.Phase, route string) *ast.ExprStmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgService),
					Sel: ast.NewIdent(ggconst.FuncRegister),
				},
				Index: &ast.StarExpr{
					X: ast.NewIdent(serviceImport),
				},
			},
			Args: []ast.Expr{
				&ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgService),
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

// controllerConfigService is the field of gst.ControllerConfig a generated
// route sets for an action declaring a service of its own.
const controllerConfigService = "Service"

// stmtRouterRegister builds the registration of an action in router.gen.go:
// the model type, then the action's request and result types, reqName and
// rspName qualified by modelPkgName, the qualifier the file refers to the
// model package by, and the controller config of the route, which names the
// route parameter of a route ending in one and marks an action declaring a
// service of its own. For example, for an action of the model's own types,
// one answering a result type of its own, which declares its service, one
// reading a record by the route parameter, and one doing both, it builds
//
//	router.Register[*model.Record, *model.Record, *model.Record](router.Auth(), "/api/records", &gst.ControllerConfig[*model.Record]{}, service.Create)
//	router.Register[*model.Record, *gstmodel.Empty, *model.BoxListRsp](router.Auth(), "/api/boxes", &gst.ControllerConfig[*model.Record]{Service: true}, service.List)
//	router.Register[*box.Record, *box.Record, *box.Record](router.Auth(), "/api/boxes/:id", &gst.ControllerConfig[*box.Record]{ParamName: "id"}, service.Get)
//	router.Register[*box.Record, *box.SealReq, *box.SealRsp](router.Auth(), "/api/boxes/:id/seal", &gst.ControllerConfig[*box.Record]{ParamName: "id", Service: true}, service.Create)
//
// A dsl.PayloadEmpty side becomes *model.Empty under gstModelPkg, the
// qualifier the router file uses for model.Empty, resolved once per file by
// routerGstModelUse. routerGroup names the router group accessor ("Auth" or
// "Public"), route is the path the route is served at, shared verbatim with the
// matching stmtServiceRegister statement, paramName is the route parameter the
// controller reads the resource id from, "" for none, declaresService says
// the action declares a service of its own, which the route is then held to
// as it registers (see controllerConfigService), and phase names the phase
// of the action as the service package spells it, such as Create.
func stmtRouterRegister(modelPkgName, modelName, reqName, rspName, gstModelPkg string, routerGroup string, route string, paramName string, declaresService bool, phase string) *ast.ExprStmt {
	// The dsl.PayloadEmpty sentinel on either side resolves to
	// *<gstModelPkg>.Empty. gstModelPkg is the file-level qualifier decided
	// once per router file by routerGstModelUse: plain "model" by default,
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

	// expr like: &gst.ControllerConfig[*sample.Record]{ParamName: "id", Service: true}
	var fields []ast.Expr
	if len(paramName) > 0 {
		fields = append(fields, &ast.KeyValueExpr{
			Key:   ast.NewIdent(ggconst.GstControllerConfigParamName),
			Value: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", paramName)},
		})
	}
	if declaresService {
		fields = append(fields, &ast.KeyValueExpr{Key: ast.NewIdent(controllerConfigService), Value: ast.NewIdent("true")})
	}
	paramExpr := &ast.UnaryExpr{
		Op: token.AND,
		X: &ast.CompositeLit{
			Type: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgGst),
					Sel: ast.NewIdent(ggconst.GstControllerConfig),
				},
				Index: &ast.StarExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent(modelPkgName),
						Sel: ast.NewIdent(modelName),
					},
				},
			},
			Elts: fields,
		},
	}

	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: &ast.IndexListExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgRouter),
					Sel: ast.NewIdent(ggconst.FuncRegister),
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
						X:   ast.NewIdent(ggconst.PkgRouter),
						Sel: ast.NewIdent(routerGroup),
					},
				},
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", route),
				},
				paramExpr,
				&ast.SelectorExpr{
					X:   ast.NewIdent(ggconst.PkgService),
					Sel: ast.NewIdent(phase),
				},
			},
		},
	}
}
