package goast

import (
	"go/ast"
	"go/token"
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

// RenameIdent renames every identifier named oldName in the tree under node
// to newName. It goes by the name alone, so a field or method sharing it is
// renamed too: RenameIdent(fn, "u", "c") on
//
//	func (u *Creator) Create() error { return u.check(req.u) }
//
// turns it into
//
//	func (c *Creator) Create() error { return c.check(req.c) }
func RenameIdent(node ast.Node, oldName, newName string) {
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == oldName {
			ident.Name = newName
		}
		return true
	})
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
