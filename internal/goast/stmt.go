package goast

import (
	"go/ast"
	"go/token"
)

// NilChecked returns the identifier v when the condition of ifStmt is a
// plain comparison of it with nil, v != nil, and nil for any other
// condition:
//
//	if err != nil { ... }                // err
//	if err := load(); err != nil { ... } // err
//	if err != nil && retry { ... }       // nil
//	if errors.Is(err, io.EOF) { ... }    // nil
func NilChecked(ifStmt *ast.IfStmt) *ast.Ident {
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.NEQ {
		return nil
	}
	ident, ok := cond.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if right, ok := cond.Y.(*ast.Ident); !ok || right.Name != "nil" {
		return nil
	}
	return ident
}

// AlwaysLeaves reports whether a statement list ends by leaving the
// surrounding flow: with a return, a branch statement (break, continue,
// goto, fallthrough) or a call of panic. An empty list does not.
func AlwaysLeaves(stmts []ast.Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch last := stmts[len(stmts)-1].(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		call, ok := last.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		ident, ok := call.Fun.(*ast.Ident)
		return ok && ident.Name == "panic"
	}
	return false
}
