package goast

import (
	"go/ast"
	"go/token"

	"github.com/cockroachdb/errors"
)

// FindStructTypeSpec returns the struct type spec file declares with the
// given name, or nil when the file declares no such struct. For a file
// declaring
//
//	type Creator struct{ Name string }
//
//	type Reader interface{ Read() }
//
// FindStructTypeSpec(file, "Creator") returns the spec of Creator, and
// FindStructTypeSpec(file, "Reader") returns nil, Reader being no struct.
func FindStructTypeSpec(file *ast.File, name string) *ast.TypeSpec {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name == nil || typeSpec.Name.Name != name {
				continue
			}
			if _, ok := typeSpec.Type.(*ast.StructType); ok {
				return typeSpec
			}
		}
	}
	return nil
}

// RenameIdent renames the object decl declares — a receiver, a parameter, a
// result or a local — to newName throughout node: every identifier that
// resolves to that object, decl itself when node holds it, and no other.
// go/parser resolves the identifiers of a file to the objects they name as
// it parses, so a selector's field name and an identifier of another object
// that shares the name stay as they are: with decl the receiver u,
//
//	func (u *Creator) Create() error { return u.check(req.u) }
//
// turns into
//
//	func (c *Creator) Create() error { return c.check(req.u) }
//
// It returns an error when decl was parsed without object resolution, or
// when node already names something else newName — a local, a parameter, a
// package-level name — because the rename would turn one of the two into a
// reference to the other; the caller leaves the name or fails.
func RenameIdent(node ast.Node, decl *ast.Ident, newName string) error {
	if decl == nil || decl.Obj == nil {
		return errors.Newf("cannot rename %s: the identifier was parsed without object resolution", declName(decl))
	}
	var clash bool
	walkScopedIdents(node, func(ident *ast.Ident) {
		clash = clash || ident.Name == newName && ident.Obj != decl.Obj
	})
	if clash {
		return errors.Newf("cannot rename %s to %s: %s already names something else there; rename one of the two", decl.Name, newName, newName)
	}
	walkScopedIdents(node, func(ident *ast.Ident) {
		if ident.Obj == decl.Obj {
			ident.Name = newName
		}
	})
	return nil
}

// declName returns the name of decl, or the name of a nil declaration.
func declName(decl *ast.Ident) string {
	if decl == nil {
		return "<nil>"
	}
	return decl.Name
}

// walkScopedIdents calls visit on every identifier under node that the scope
// resolves: the field name of a selector and the field key of a composite
// literal, named by the type they belong to and not by the scope, are left
// out.
func walkScopedIdents(node ast.Node, visit func(*ast.Ident)) {
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			ast.Inspect(x.X, walk)
			return false
		case *ast.KeyValueExpr:
			if key, ok := x.Key.(*ast.Ident); ok && key.Obj == nil {
				ast.Inspect(x.Value, walk)
				return false
			}
		case *ast.Ident:
			visit(x)
		}
		return true
	}
	ast.Inspect(node, walk)
}

// RemoveStructInteriorComments drops every comment group of file positioned
// inside the braces of structType: a rewritten struct body takes its comments
// with it, and a stale comment would otherwise interleave with position-less
// replacement fields when the file is printed. For
//
//	// Creator creates records.
//	type Creator struct {
//		// Name is shown to users.
//		Name string // required
//	}
//
// it drops the two comments inside the braces and keeps the doc comment of
// Creator.
func RemoveStructInteriorComments(file *ast.File, structType *ast.StructType) {
	if structType.Fields == nil || !structType.Fields.Opening.IsValid() || !structType.Fields.Closing.IsValid() {
		return
	}
	kept := file.Comments[:0]
	for _, group := range file.Comments {
		if group.Pos() > structType.Fields.Opening && group.End() < structType.Fields.Closing {
			continue
		}
		kept = append(kept, group)
	}
	file.Comments = kept
}
