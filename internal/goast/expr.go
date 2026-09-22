package goast

import "go/ast"

// IsQualified reports whether expr is a type qualified by a package name:
// true for model.User, and false for *model.User, the unqualified User and a
// selector on a selector such as a.b.User.
func IsQualified(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	_, ok = sel.X.(*ast.Ident)
	return ok && sel.Sel != nil
}

// IsPointerToQualified reports whether expr is a pointer to a type qualified
// by a package name: true for *model.User, and false for model.User,
// **model.User and *User.
func IsPointerToQualified(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	return ok && IsQualified(star.X)
}

// TypeQualifier returns the package name qualifying a type, looking through
// pointers: sample for sample.User, *sample.User and **sample.User, and ""
// for any other expression, such as User or []sample.User.
func TypeQualifier(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return TypeQualifier(t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

// IsPointerReceiver reports whether recv is a single receiver of a pointer to
// a named type: true for (u *Creator), and false for the value receiver
// (u Creator) and for (u *Creator[T]), whose type is generic.
func IsPointerReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	_, ok = star.X.(*ast.Ident)
	return ok
}

// IsBuiltinError reports whether expr is the predeclared error type, the
// bare identifier error: true for error, and false for pkg.Error or any
// other identifier.
func IsBuiltinError(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}
