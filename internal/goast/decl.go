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
// it parses, so a selector's field name, a struct literal's field key and an
// identifier of another object that shares the name stay as they are: with
// decl the receiver u,
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
	if decl == nil {
		return errors.New("cannot rename a nil identifier")
	}
	if decl.Obj == nil {
		return errors.Newf("cannot rename %s: the identifier was parsed without object resolution", decl.Name)
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

// walkScopedIdents calls visit on every identifier under node that the scope
// resolves, leaving out the ones a type names: the field name of a selector,
// and the keys of a composite literal unless the literal's type is written
// as a map, an array or a slice, whose keys are expressions. The parser
// links a struct's field key to a same-named object in scope all the same —
// the T{K: 0} case go/ast/scope.go documents — so the literal's own type
// decides, and only a key that is a single identifier can be a field name; a
// literal of an elided type is taken for a struct, which leaves a map key
// under its old name to fail the build rather than renaming a field.
func walkScopedIdents(node ast.Node, visit func(*ast.Ident)) {
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			ast.Inspect(x.X, walk)
			return false
		case *ast.CompositeLit:
			if x.Type != nil {
				ast.Inspect(x.Type, walk)
			}
			keysAreExpressions := false
			switch x.Type.(type) {
			case *ast.MapType, *ast.ArrayType:
				keysAreExpressions = true
			}
			for _, elt := range x.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					ast.Inspect(elt, walk)
					continue
				}
				if _, isIdent := kv.Key.(*ast.Ident); keysAreExpressions || !isIdent {
					ast.Inspect(kv.Key, walk)
				}
				ast.Inspect(kv.Value, walk)
			}
			return false
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
