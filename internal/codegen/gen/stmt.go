package gen

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/hydroan/gst/types/consts"
)

// StmtLogInfo create *ast.ExprStmt represents `log.Info(str)`
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

func EmptyLine() *ast.EmptyStmt {
	return &ast.EmptyStmt{}
}

func Returns(exprs ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{
		Results: exprs,
	}
}

// StmtLogWithContext create *ast.AssignStmt represents `log := u.WithContext(ctx, ctx.Phase())`
// modelVarName is model variable name.
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

// StmtModelRegister creates a *ast.ExprStmt represents golang code like below:
//
//	model.Register[*User]()
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

// StmtServiceRegister creates a *ast.ExprStmt represents golang code like below:
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

// StmtRouterRegister creates a *ast.ExprStmt represents golang code like below:
//
//	router.Register[*model.Group, *model.Group, *model.Group](router.Auth(), "group", &types.ControllerConfig[*model.Group]{}, consts.Create)
//	router.Register[*model.Group, *model.Group, *model.Group](router.Pub(), "login", &types.ControllerConfig[*auth.LoginReq]{}, consts.Create)
//
// routerGroup names the router group accessor ("Auth" or "Pub") and route is
// the raw route string, shared verbatim with the matching StmtServiceRegister
// statement.
func StmtRouterRegister(modelPkgName, modelName, reqName, rspName string, routerGroup string, route string, paramName string, verb string) *ast.ExprStmt {
	// The dsl.PayloadEmpty sentinel resolves to *gstmodel.Empty. The router
	// file aggregates imports from many model packages, so the gst model
	// package is always referenced under the gstmodel alias to avoid clashing
	// with a business root model package named "model". Otherwise, if reqName
	// is equal to modelName or reqName starts with *, then the reqExpr use
	// StarExpr, or use SelectorExpr.
	var reqExpr ast.Expr
	switch {
	case isEmptyPayload(reqName):
		reqExpr = emptyReqExpr(gstModelPkgAlias)
	case strings.HasPrefix(reqName, "*") || modelName == reqName:
		reqExpr = &ast.StarExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent(modelPkgName),
				Sel: ast.NewIdent(strings.TrimPrefix(reqName, "*")),
			},
		}
	default:
		reqExpr = &ast.SelectorExpr{
			X:   ast.NewIdent(modelPkgName),
			Sel: ast.NewIdent(reqName),
		}
	}

	// If rspName is equal to modelName or rspName starts with *, then the rspExpr use StarExpr,
	// otherwise use SelectorExpr
	var rspExpr ast.Expr
	if strings.HasPrefix(rspName, "*") || modelName == rspName {
		rspExpr = &ast.StarExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent(modelPkgName),
				Sel: ast.NewIdent(strings.TrimPrefix(rspName, "*")),
			},
		}
	} else {
		rspExpr = &ast.SelectorExpr{
			X:   ast.NewIdent(modelPkgName),
			Sel: ast.NewIdent(rspName),
		}
	}

	var paramExpr ast.Expr
	// expr like: &types.ControllerConfig[*config.Namespace]{}
	paramExpr = &ast.UnaryExpr{
		Op: token.AND,
		X: &ast.CompositeLit{
			Type: &ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent("types"),
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
	// expr like: &types.ControllerConfig[*config.Namespace]{ParamName: "ns"}
	if len(paramName) > 0 {
		paramExpr = &ast.UnaryExpr{
			Op: token.AND,
			X: &ast.CompositeLit{
				Type: &ast.IndexExpr{
					X: &ast.SelectorExpr{
						X:   ast.NewIdent("types"),
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
