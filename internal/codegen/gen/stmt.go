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
//	service.Register[*user.Creator](consts.PHASE_CREATE)
func StmtServiceRegister(serviceImport string, phase consts.Phase) *ast.ExprStmt {
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
			},
		},
	}
}

// StmtRouterRegister creates a *ast.ExprStmt represents golang code like below:
//
//	router.Register[*model.Group, *model.Group, *model.Group](router.Auth(), "group", &types.ControllerConfig[*model.Group]{}, consts.Create)
//	router.Register[*model.Group, *model.Group, *model.Group](router.Pub(), "login", &types.ControllerConfig[*auth.LoginReq]{}, consts.Create)
func StmtRouterRegister(modelPkgName, modelName, reqName, rspName string, router string, endpoint string, paramName string, verb string) *ast.ExprStmt {
	// If reqName is equal to modelName or reqName starts with *, then the reqExpr use StarExpr,
	// otherwise use SelectorExpr
	var reqExpr ast.Expr
	if strings.HasPrefix(reqName, "*") || modelName == reqName {
		reqExpr = &ast.StarExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent(modelPkgName),
				Sel: ast.NewIdent(strings.TrimPrefix(reqName, "*")),
			},
		}
	} else {
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
						Sel: ast.NewIdent(router),
					},
				},
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", endpoint),
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
